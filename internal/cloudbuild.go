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
	sourcePath *GcsPath
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
		return err
	}

	buildID, err := s.runCloudBuild(build)
	if err != nil {
		return NewServiceError(
			fmt.Sprintf("Failed to create a new build for source arvhive %v", s.sourcePath),
			err,
		)
	}

	status, err := s.watchCloudBuild(buildID)
	if err != nil {
		return err
	}
	if status != "SUCCESS" {
		return NewBuildResultError(buildID, status)
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

func (s *CloudBuildSubmit) runCloudBuild(build *cloudbuild.Build) (string, error) {
	log.WithField("source", s.sourcePath).Info("Starting build")
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
		return "", xerrors.Errorf("Failed to create cloudbuild service: %w", err)
	}
	buildService := cloudbuild.NewProjectsBuildsService(service)
	call := buildService.Create(s.Config.Project, build)
	operation, err := call.Do()
	if err != nil {
		return "", xerrors.Errorf("Failed to start build: %w", err)
	}

	metadata := &cloudbuild.BuildOperationMetadata{}
	if err := json.Unmarshal(operation.Metadata, &metadata); err != nil {
		return "", xerrors.Errorf("Failed to parse result(%s): %w", string(operation.Metadata), err)
	}
	log.WithField("build", metadata).Trace("Build metadata")
	log.WithField("build", metadata.Build.Id).Info("Build started")
	return metadata.Build.Id, nil
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
	build, err := call.Do()
	if err != nil {
		return "", NewServiceError(
			fmt.Sprintf("Failed to stat build %s", buildID),
			err,
		)
	}
	log.WithField("build", build).Trace("Stat build")

	logURLStr := fmt.Sprintf("%v/log-%v.txt", build.LogsBucket, build.Id)
	logURL, err := ParseGcsURL(logURLStr)
	if err != nil {
		return "", xerrors.Errorf("Invalid url '%s': %w", build.LogUrl, err)
	}
	bucketName := logURL.Bucket
	objectPath := logURL.Object
	log.WithField("gcsUrl", logURL).Trace("Stat log")

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return "", NewServiceError("Failed to initialize gcs client", err)
	}
	logObject := gcsClient.Bucket(bucketName).Object(objectPath)

	complete := false
	cbErrCount := 0
	gcsErrCount := 0
	offset := int64(0)

	for !complete {
		if build, err = call.Do(); err != nil {
			cbErrCount++
			if s.Config.MaxGetBuildErrorCount > 0 &&
				cbErrCount >= s.Config.MaxGetBuildErrorCount {
				return "", NewServiceError(
					fmt.Sprintf("Failed to stat build %v", buildID),
					err,
				)
			}
			log.WithError(err).WithField("buildID", buildID).WithField("errCount", cbErrCount).Warn("Failed to stat build")
		} else {
			cbErrCount = 0
			if isBuildCompleted(build.Status) {
				log.WithField("build", build).Trace("Build completed")
				complete = true
			}
		}
		if reader, err := logObject.NewRangeReader(ctx, offset, -1); err != nil {
			if !isIgnorableGcsError(err) {
				gcsErrCount++
				if s.Config.MaxReadLogErrorCount > 0 &&
					gcsErrCount >= s.Config.MaxReadLogErrorCount {
					return "", NewServiceError(
						"Failed to read log",
						err,
					)
				}
				log.WithError(err).
					WithField("gcsUrl", logURL).
					WithField("errCount", gcsErrCount).
					WithField("offset", offset).
					Warn("Failed to stat log stream")
			} else {
				log.WithError(err).
					WithField("gcsUrl", logURL).
					WithField("offset", offset).
					Trace("Ignorable error for stating log stream")
				gcsErrCount = 0
			}
		} else {
			if err := func() error {
				defer reader.Close()
				if count, err := io.Copy(os.Stdout, reader); err != nil {
					if !isIgnorableGcsError(err) {
						gcsErrCount++
						if s.Config.MaxReadLogErrorCount > 0 &&
							gcsErrCount >= s.Config.MaxReadLogErrorCount {
							return NewServiceError(
								"Failed to read log",
								err,
							)
						}
						log.WithError(err).
							WithField("gcsUrl", logURL).
							WithField("errCount", gcsErrCount).
							WithField("offset", offset).
							WithField("count", count).
							Warn("Failed to read log stream")
						offset += count
					} else {
						log.WithError(err).
							WithField("gcsUrl", logURL).
							WithField("offset", offset).
							WithField("count", count).
							Trace("Ignorable error for reading log stream")
						gcsErrCount = 0
						offset += count
					}
				} else {
					gcsErrCount = 0
					offset += count
				}
				return nil
			}(); err != nil {
				return "", err
			}
		}
		time.Sleep(time.Duration(s.Config.PollingIntervalMsec) * time.Millisecond)
	}
	log.WithField("build", build).
		WithField("gcsUrl", logURL).
		WithField("logSize", offset).
		Debug("Finished to watch build")
	log.WithField("buildID", build.Id).
		WithField("status", build.Status).
		Info("Finished to watch build")
	return build.Status, nil
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

func isIgnorableGcsError(err error) bool {
	if err == nil {
		return true
	}

	var apiError *googleapi.Error
	if !xerrors.As(err, &apiError) {
		return false
	}
	// We can ignore 404 (the log file isn't ready yet) and 416 (no new contents)
	if apiError.Code == 404 || apiError.Code == 416 {
		return true
	}
	return false
}
