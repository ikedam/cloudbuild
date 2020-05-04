package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"

	"gopkg.in/yaml.v3"

	"golang.org/x/xerrors"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/googleapi"

	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/archive"
	"github.com/ikedam/cloudbuild/log"
	"github.com/rs/xid"
)

// CloudBuildSubmit holds running state of build submission
type CloudBuildSubmit struct {
	Config
	sourcePath     *GcsPath
	buildID        string
	completeStatus string
}

// Execute performs the sequence to submit a build to CloudBuild
func (s *CloudBuildSubmit) Execute() error {
	sourcePath := fmt.Sprintf(
		"%v/%v.tgz",
		s.Config.GcsSourceStagingDir,
		xid.New().String(),
	)

	var err error
	if s.sourcePath, err = ParseGcsURL(sourcePath); err != nil {
		return NewConfigError(
			fmt.Sprintf("Invalid gcs URL '%v'", s.Config.GcsSourceStagingDir),
			err,
		)
	}

	build, err := s.readCloudBuild()
	if err != nil {
		return NewConfigError(
			fmt.Sprintf("Failed to read %v", s.Config.Config),
			err,
		)
	}

	for backoff := NewBackoff(); true; {
		if err := func() error {
			tar, err := s.createSourceArchive()
			if err != nil {
				return NewConfigError(
					fmt.Sprintf("Failed to create source arvhive %v", s.Config.SourceDir),
					err,
				)
			}
			defer tar.Close()

			if err := s.uploadCloudStorage(tar); err != nil {
				return NewServiceError(
					fmt.Sprintf("Failed to upload source arvhive to %v", s.sourcePath),
					err,
				)
			}
			return nil
		}(); err != nil {
			if (s.Config.MaxUploadTryCount <= 0 || backoff.Attempt() < s.Config.MaxUploadTryCount) &&
				isRetryableError(err) {
				log.WithError(err).WithField("attempt", backoff.Attempt()).
					Warning("Failed to upload. Retrying...")
				backoff.Sleep()
				continue
			}
			return err
		}
		break
	}

	for backoff := NewBackoff(); true; {
		err = s.runCloudBuild(build)
		if err != nil {
			if (s.Config.MaxStartBuildTryCount <= 0 || backoff.Attempt() < s.Config.MaxStartBuildTryCount) &&
				isRetryableError(err) {
				log.WithError(err).WithField("attempt", backoff.Attempt()).
					Warning("Failed to start build. Retrying...")
				backoff.Sleep()
				continue
			}
			return NewServiceError(
				fmt.Sprintf("Failed to create a new build for source arvhive %v", s.sourcePath),
				err,
			)
		}
		break
	}

	status, err := s.watchCloudBuild(s.buildID)
	if err != nil {
		return err
	}
	if status != "SUCCESS" {
		return NewBuildResultError(s.buildID, status)
	}

	return nil
}

func (s *CloudBuildSubmit) readCloudBuild() (*cloudbuild.Build, error) {
	log.WithField("file", s.Config.Config).Debug("reading cloudbuild.yaml")
	yamlBody, err := func() ([]byte, error) {
		fd, err := os.Open(s.Config.Config)
		if err != nil {
			return nil, err
		}
		defer fd.Close()

		yamlBody, err := ioutil.ReadAll(fd)
		if err != nil {
			return nil, err
		}
		return yamlBody, nil
	}()
	if err != nil {
		return nil, xerrors.Errorf("Failed to read %v: %w", s.Config.Config, err)
	}
	m := make(map[string]interface{})
	if err := yaml.Unmarshal(yamlBody, &m); err != nil {
		return nil, xerrors.Errorf("Failed to read %v: %w", s.Config.Config, err)
	}
	jsonData, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return nil, xerrors.Errorf("Failed to serialize %v: %w", s.Config.Config, err)
	}
	log.WithField("json", jsonData).Trace("Marshal cloudbuild.yaml to json format")
	build := &cloudbuild.Build{}
	if err := json.Unmarshal(jsonData, build); err != nil {
		return nil, xerrors.Errorf("Failed to serialize %v: %w", s.Config.Config, err)
	}
	log.WithField("file", s.Config.Config).WithField("build", build).Trace("finished to read cloudbuild.yaml")

	if len(s.Config.Substitutions) > 0 {
		if build.Substitutions == nil {
			build.Substitutions = make(map[string]string)
		}
		for _, substitution := range s.Config.Substitutions {
			keyValue := strings.SplitN(substitution, "=", 2)
			build.Substitutions[keyValue[0]] = keyValue[1]
		}
	}

	return build, nil
}

func (s *CloudBuildSubmit) createSourceArchive() (io.ReadCloser, error) {
	log.WithField("source", s.Config.SourceDir).Info("Archiving the source directory")
	path, err := filepath.Abs(s.Config.SourceDir)
	if err != nil {
		return nil, xerrors.Errorf("Failed to stat %v: %w", s.Config.SourceDir, err)
	}
	ignoreFile := filepath.Join(path, s.Config.IgnoreFile)
	excludes := []string{}
	if _, err := os.Stat(ignoreFile); err == nil {
		func() {
			log.WithField("file", ignoreFile).Debug("reading .gcloudignore")
			fd, err := os.Open(ignoreFile)
			if err != nil {
				log.WithError(err).WithField("file", ignoreFile).Warning("Failed to open .glcoudignore")
				return
			}
			defer fd.Close()
			if readExcludes, err := dockerignore.ReadAll(fd); err == nil {
				excludes = readExcludes
			} else {
				log.WithError(err).WithField("file", ignoreFile).Warning("Failed to read .glcoudignore")
				return
			}
			log.WithField("file", ignoreFile).WithField("ignores", excludes).Trace("finished to read .gcloudignore")
		}()
	}
	tar, err := archive.TarWithOptions(path, &archive.TarOptions{
		Compression:     archive.Gzip,
		ExcludePatterns: excludes,
	})
	if err != nil {
		return nil, xerrors.Errorf("Failed to create source archive: %w", err)
	}
	log.WithField("source", s.Config.SourceDir).Info("Finished to archiving the source directory")
	return tar, nil
}

func (s *CloudBuildSubmit) uploadCloudStorage(stream io.Reader) error {
	log.WithField("gcsPath", s.sourcePath).Info("Uploading the source archive")
	bucketName := s.sourcePath.Bucket
	objectPath := s.sourcePath.Object

	ctx := context.Background()
	if s.Config.UploadTimeoutMsec > 0 {
		timeoutCtx, cancel := context.WithTimeout(
			ctx,
			time.Duration(s.Config.UploadTimeoutMsec)*time.Millisecond,
		)
		ctx = timeoutCtx
		defer cancel()
	}
	client, err := storage.NewClient(ctx)
	if err != nil {
		return xerrors.Errorf("Failed to initialize gcs client: %w", err)
	}
	object := client.Bucket(bucketName).Object(objectPath)
	writer := object.NewWriter(ctx)
	defer writer.Close()
	transferred, err := io.Copy(writer, stream)
	if err != nil {
		return xerrors.Errorf("Failed to upload source archive to %v: %w", s.sourcePath, err)
	}
	log.WithField("gcsPath", s.sourcePath).WithField("size", transferred).Info("Finished to upload the source archive")
	return nil
}

func (s *CloudBuildSubmit) runCloudBuild(build *cloudbuild.Build) error {
	log.WithField("source", s.sourcePath).Info("Queueing build")
	bucketName := s.sourcePath.Bucket
	objectPath := s.sourcePath.Object

	build.Source = &cloudbuild.Source{
		StorageSource: &cloudbuild.StorageSource{
			Bucket: bucketName,
			Object: objectPath,
		},
	}

	ctx := context.Background()
	service, err := cloudbuild.NewService(ctx)
	if err != nil {
		return xerrors.Errorf("Failed to create coudbuild service: %w", err)
	}
	buildService := cloudbuild.NewProjectsBuildsService(service)
	call := buildService.Create(s.Config.Project, build)
	createCtx := ctx
	if s.Config.CloudBuildTimeoutMsec > 0 {
		timeoutCtx, cancel := context.WithTimeout(
			createCtx,
			time.Duration(s.Config.CloudBuildTimeoutMsec)*time.Millisecond,
		)
		createCtx = timeoutCtx
		defer cancel()
	}
	operation, err := call.Context(createCtx).Do()
	if err != nil {
		return xerrors.Errorf("Failed to queue build: %w", err)
	}

	metadata := &cloudbuild.BuildOperationMetadata{}
	if err := json.Unmarshal(operation.Metadata, &metadata); err != nil {
		return xerrors.Errorf("Failed to parse result(%s): %w", string(operation.Metadata), err)
	}
	s.buildID = metadata.Build.Id
	log.WithField("build", metadata).Trace("Build metadata")
	log.WithField("buildID", s.buildID).Info("Build queued")
	return nil
}

func (s *CloudBuildSubmit) watchCloudBuild(buildID string) (string, error) {
	log.WithField("source", s.sourcePath).Debug("Watching build")
	ctx := context.Background()
	service, err := cloudbuild.NewService(ctx)
	if err != nil {
		return "", NewServiceError("Failed to create cloudbuild service", err)
	}
	buildService := cloudbuild.NewProjectsBuildsService(service)

	call := buildService.Get(s.Config.Project, buildID)
	var build *cloudbuild.Build
	for backoff := NewBackoff(); true; {
		if build, err = func() (*cloudbuild.Build, error) {
			getCtx := ctx
			if s.Config.CloudBuildTimeoutMsec > 0 {
				timeoutCtx, cancel := context.WithTimeout(
					getCtx,
					time.Duration(s.Config.CloudBuildTimeoutMsec)*time.Millisecond,
				)
				defer cancel()
				getCtx = timeoutCtx
			}
			return call.Context(getCtx).Do()
		}(); err != nil {
			if (s.Config.MaxGetBuildTryCount <= 0 || backoff.Attempt() < s.Config.MaxGetBuildTryCount) &&
				isRetryableError(err) {
				log.WithError(err).
					WithField("build", buildID).
					WithField("attempt", backoff.Attempt()).
					Warning("Failed to stat build. Retrying...")
				backoff.Sleep()
				continue
			}
			return "", NewServiceError(
				fmt.Sprintf("Failed to stat build %s", buildID),
				err,
			)
		}
		break
	}
	log.WithField("build", build).Trace("Stat build")

	logURLStr := fmt.Sprintf("%v/log-%v.txt", build.LogsBucket, build.Id)
	logURL, err := ParseGcsURL(logURLStr)
	if err != nil {
		return "", xerrors.Errorf("Invalid url '%s': %w", build.LogUrl, err)
	}
	bucketName := logURL.Bucket
	objectPath := logURL.Object
	log.WithField("gcsBucket", bucketName).
		WithField("gcsObject", objectPath).
		Trace("Stat log")

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return "", NewServiceError("Failed to initialize gcs client", err)
	}
	logObject := gcsClient.Bucket(bucketName).Object(objectPath)

	w := &watchLogStatus{
		config:       &s.Config,
		ctx:          ctx,
		build:        build,
		getBuildCall: call,
		cbAttempt:    0,
		logObject:    logObject,
		gcsAttempt:   0,
		offset:       0,
		started:      false,
		complete:     false,
	}

	if w.build.Status == "QUEUED" {
		log.Info("Waiting build starts...")
	}

	for !w.complete {
		if err := w.watchLog(); err != nil {
			return "", err
		}
		time.Sleep(time.Duration(s.Config.PollingIntervalMsec) * time.Millisecond)
	}
	build = w.build
	s.completeStatus = build.Status
	log.WithField("build", build).
		WithField("gcsBucket", logURL.Bucket).
		WithField("gcsObject", logURL.Object).
		WithField("logSize", w.offset).
		Debug("Finished to watch build")
	log.WithField("buildID", build.Id).
		WithField("status", build.Status).
		Info("Build completed")
	return s.completeStatus, nil
}

type watchLogStatus struct {
	config       *Config
	ctx          context.Context
	build        *cloudbuild.Build
	getBuildCall *cloudbuild.ProjectsBuildsGetCall
	cbAttempt    int
	logObject    *storage.ObjectHandle
	offset       int64
	gcsAttempt   int
	started      bool
	complete     bool
}

func (w *watchLogStatus) watchLog() error {
	w.cbAttempt++
	if newBuild, err := func() (*cloudbuild.Build, error) {
		getCtx := w.ctx
		if w.config.CloudBuildTimeoutMsec > 0 {
			timeoutCtx, cancel := context.WithTimeout(
				getCtx,
				time.Duration(w.config.CloudBuildTimeoutMsec)*time.Millisecond,
			)
			defer cancel()
			getCtx = timeoutCtx
		}
		return w.getBuildCall.Context(getCtx).Do()
	}(); err != nil {
		if (w.config.MaxGetBuildTryCount > 0 && w.cbAttempt >= w.config.MaxGetBuildTryCount) ||
			!isRetryableError(err) {
			return NewServiceError(
				fmt.Sprintf("Failed to stat build %v", w.build.Id),
				err,
			)
		}
		log.WithError(err).
			WithField("buildID", w.build.Id).
			WithField("attempt", w.cbAttempt).
			Warn("Failed to stat build")
	} else {
		w.build = newBuild
		w.cbAttempt = 0
	}

	if !w.started {
		if w.build.Status == "QUEUED" {
			return nil
		}
		log.Info("Build started")
		w.started = true
	}

	w.gcsAttempt++
	if count, err := func() (int64, error) {
		readCtx := w.ctx
		if w.config.ReadLogTimeoutMsec > 0 {
			timeoutCtx, cancel := context.WithTimeout(
				readCtx,
				time.Duration(w.config.ReadLogTimeoutMsec)*time.Millisecond,
			)
			defer cancel()
			readCtx = timeoutCtx
		}
		reader, err := w.logObject.NewRangeReader(readCtx, w.offset, -1)
		if err != nil {
			return int64(0), err
		}
		defer reader.Close()
		return io.Copy(os.Stdout, reader)
	}(); err != nil {
		if !isIgnorableGcsError(err) {
			if (w.config.MaxReadLogTryCount > 0 && w.gcsAttempt >= w.config.MaxReadLogTryCount) ||
				!isRetryableError(err) {
				return NewServiceError(
					"Failed to read log",
					err,
				)
			}
			log.WithError(err).
				WithField("gcsBucket", w.logObject.BucketName()).
				WithField("gcsObject", w.logObject.ObjectName()).
				WithField("attempt", w.gcsAttempt).
				WithField("offset", w.offset).
				WithField("size", count).
				Warn("Failed to read log")
		} else {
			log.WithError(err).
				WithField("gcsBucket", w.logObject.BucketName()).
				WithField("gcsObject", w.logObject.ObjectName()).
				WithField("offset", w.offset).
				WithField("size", count).
				Trace("Ignorable error for reading log stream")
			w.gcsAttempt = 0
		}
		w.offset += count
	} else {
		w.gcsAttempt = 0
		w.offset += count
	}

	if isBuildCompleted(w.build.Status) {
		log.WithField("build", w.build).Trace("Build completed")
		w.complete = true
	}

	return nil
}

func isBuildCompleted(status string) bool {
	//   "STATUS_UNKNOWN" - Status of the build is unknown.
	//   "QUEUED" - Build or step is queued; work has not yet begun.
	//   "WORKING" - Build or step is being executed.
	//   "SUCCESS" - Build or step finished successfully.
	//   "FAILURE" - Build or step failed to complete successfully.
	//   "INTERNAL_ERROR" - Build or step failed due to an internal cause.
	//   "TIMEOUT" - Build or step took longer than was allowed.
	//   "CANCELLED" - Build or step was canceled by a user.
	return status == "SUCCESS" ||
		status == "FAILURE" ||
		status == "INTERNAL_ERROR" ||
		status == "TIMEOUT" ||
		status == "CANCELLED"
}

// Cancel cancels running build
func (s *CloudBuildSubmit) Cancel() error {
	if s.buildID == "" {
		log.Debug("No need to cancel build as it's not started yet.")
		return nil
	}
	if s.completeStatus != "" {
		log.WithField("buildID", s.buildID).
			WithField("status", s.completeStatus).
			Debug("No need to cancel build as build has already completed.")
		return nil
	}
	log.WithField("buildID", s.buildID).
		Info("Canceling build...")

	ctx := context.Background()
	service, err := cloudbuild.NewService(ctx)
	if err != nil {
		return xerrors.Errorf("Failed to create coudbuild service: %w", err)
	}
	cancel := &cloudbuild.CancelBuildRequest{}
	buildService := cloudbuild.NewProjectsBuildsService(service)
	call := buildService.Cancel(s.Project, s.buildID, cancel)
	createCtx := ctx
	if s.Config.CloudBuildTimeoutMsec > 0 {
		timeoutCtx, cancel := context.WithTimeout(
			createCtx,
			time.Duration(s.Config.CloudBuildTimeoutMsec)*time.Millisecond,
		)
		createCtx = timeoutCtx
		defer cancel()
	}
	for backoff := NewBackoff(); true; {
		if _, err := call.Context(createCtx).Do(); err != nil {
			if googleapi.IsNotModified(err) {
				break
			}
			if (s.Config.MaxStartBuildTryCount <= 0 || backoff.Attempt() < s.Config.MaxStartBuildTryCount) &&
				isRetryableError(err) {
				log.WithError(err).WithField("attempt", backoff.Attempt()).
					Warning("Failed to cancel build. Retrying...")
				backoff.Sleep()
				continue
			}
			return xerrors.Errorf("Failed to cancel build %v: %w", s.buildID, err)
		}
		break
	}
	log.WithField("buildID", s.buildID).
		Info("Canceled")
	return nil
}
