package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"

	"gopkg.in/yaml.v3"

	"golang.org/x/xerrors"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/googleapi"

	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/archive"
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
	build := &cloudbuild.Build{}
	if err := json.Unmarshal(jsonData, build); err != nil {
		return nil, xerrors.Errorf("Failed to serialize %v: %w", s.Config.Config, err)
	}
	return build, nil
}

func (s *CloudBuildSubmit) createSourceArchive() (io.ReadCloser, error) {
	excludes := []string{}
	_, err := os.Stat(s.Config.IgnoreFile)
	if err == nil {
		func() {
			fd, err := os.Open(s.Config.IgnoreFile)
			if err != nil {
				log.Printf("Warning: ignored %v: %+v", s.Config.IgnoreFile, err)
				return
			}
			defer fd.Close()
			if readExcludes, err := dockerignore.ReadAll(fd); err == nil {
				excludes = readExcludes
			} else {
				log.Printf("Warning: ignored %v: %+v", s.Config.IgnoreFile, err)
			}
		}()
	}
	path, err := filepath.Abs(s.Config.SourceDir)
	if err != nil {
		return nil, xerrors.Errorf("Failed to stat %v: %w", s.Config.SourceDir, err)
	}
	tar, err := archive.TarWithOptions(path, &archive.TarOptions{
		Compression:     archive.Gzip,
		ExcludePatterns: excludes,
	})
	if err != nil {
		return nil, xerrors.Errorf("Failed to create source archive: %w", err)
	}
	return tar, nil
}

func (s *CloudBuildSubmit) uploadCloudStorage(stream io.Reader) error {
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
	if _, err := io.Copy(writer, stream); err != nil {
		return xerrors.Errorf("Failed to upload source archive to %v: %w", s.sourcePath, err)
	}
	return nil
}

func (s *CloudBuildSubmit) runCloudBuild(build *cloudbuild.Build) (string, error) {
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
	return metadata.Build.Id, nil
}

func (s *CloudBuildSubmit) watchCloudBuild(buildID string) (string, error) {
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

	logURLStr := fmt.Sprintf("%v/log-%v.txt", build.LogsBucket, build.Id)
	logURL, err := ParseGcsURL(logURLStr)
	if err != nil {
		return "", xerrors.Errorf("Invalid url '%s': %w", build.LogUrl, err)
	}
	bucketName := logURL.Bucket
	objectPath := logURL.Object

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
			log.Printf("Failed to stat build %v: %+v", buildID, err)
		} else {
			cbErrCount = 0
			if isBuildCompleted(build.Status) {
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
				log.Printf("Failed to read log (%v): %+v", gcsErrCount, err)
			} else {
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
						log.Printf("Failed to read log (%v): %+v", gcsErrCount, err)
						offset += count
					} else {
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
