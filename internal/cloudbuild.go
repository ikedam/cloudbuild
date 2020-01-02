package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"

	"gopkg.in/yaml.v3"

	"golang.org/x/oauth2/google"
	"golang.org/x/xerrors"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/googleapi"

	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/archive"
	"github.com/rs/xid"
)

func main() {
	projectID, err := getProjectID()
	if err != nil {
		log.Printf("Failed to upload source: %+v", err)
		os.Exit(1)
		return
	}

	gsFile := fmt.Sprintf(
		"gs://%v_cloudbuild/source/%v.tgz",
		projectID,
		xid.New().String(),
	)

	build, err := readCloudBuild()
	if err != nil {
		log.Printf("Failed to read cloudbuild.yaml: %+v", err)
		os.Exit(1)
		return
	}

	if err := func() error {
		tar, err := createSourceArchive()
		if err != nil {
			log.Printf("Failed to create source: %+v", err)
			os.Exit(1)
			return err
		}
		defer tar.Close()

		if err := uploadCloudStorage(gsFile, tar); err != nil {
			log.Printf("Failed to upload source: %+v", err)
			return err
		}
		return nil
	}(); err != nil {
		os.Exit(1)
		return
	}

	buildID, err := runCloudBuild(projectID, build, gsFile)
	if err != nil {
		log.Printf("Failed to run cloud build: %+v", err)
		os.Exit(1)
		return
	}

	if err := watchCloudBuild(projectID, buildID); err != nil {
		log.Printf("Failed to watch cloud build %s: %+v", buildID, err)
		os.Exit(1)
		return
	}
	os.Exit(0)
	return
}

func getProjectID() (string, error) {
	if projectID := os.Getenv("GOOGLE_PROJECT_ID"); projectID != "" {
		return projectID, nil
	}
	ctx := context.Background()
	cred, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return "", xerrors.Errorf("Failed to get default credentials: %w", err)
	}
	if cred.ProjectID == "" {
		return "", xerrors.New("No projectId is configured. Please set GOOGLE_PROJECT_ID.")
	}

	return cred.ProjectID, nil
}

func createSourceArchive() (io.ReadCloser, error) {
	excludes := []string{}
	_, err := os.Stat(".gcloudignore")
	if err == nil {
		func() {
			fd, err := os.Open(".gcloudignore")
			if err != nil {
				log.Printf("Warn: ignored .gcloudignore: %+v", err)
				return
			}
			defer fd.Close()
			if readExcludes, err := dockerignore.ReadAll(fd); err == nil {
				excludes = readExcludes
			} else {
				log.Printf("Warn: ignored .gcloudignore: %+v", err)
			}
		}()
	}
	path, err := filepath.Abs(".")
	if err != nil {
		return nil, xerrors.Errorf("Failed to stat .: %w", err)
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

func uploadCloudStorage(gsFile string, stream io.Reader) error {
	gsURL, err := url.Parse(gsFile)
	if err != nil {
		return xerrors.Errorf("Invalid url '%s': %w", gsFile, err)
	}
	bucketName := gsURL.Host
	objectPath := gsURL.Path[1:]

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return xerrors.Errorf("Failed to initialize gcs client: %w", err)
	}
	object := client.Bucket(bucketName).Object(objectPath)
	writer := object.NewWriter(ctx)
	defer writer.Close()
	if _, err := io.Copy(writer, stream); err != nil {
		return xerrors.Errorf("Failed to upload source archive: %w", err)
	}
	return nil
}

func runCloudBuild(projectID string, build *cloudbuild.Build, source string) (string, error) {
	gsURL, err := url.Parse(source)
	if err != nil {
		return "", xerrors.Errorf("Invalid url '%s': %w", source, err)
	}
	bucketName := gsURL.Host
	objectPath := gsURL.Path[1:]

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
	call := buildService.Create(projectID, build)
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

func readCloudBuild() (*cloudbuild.Build, error) {
	yamlBody, err := func() ([]byte, error) {
		fd, err := os.Open("cloudbuild.yaml")
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
		return nil, xerrors.Errorf("Failed to read cloudbuild.yaml: %w", err)
	}
	m := make(map[string]interface{})
	if err := yaml.Unmarshal(yamlBody, &m); err != nil {
		return nil, xerrors.Errorf("Failed to read cloudbuild.yaml: %w", err)
	}
	jsonData, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return nil, xerrors.Errorf("Failed to serialize cloudbuild.yaml: %w", err)
	}
	build := &cloudbuild.Build{}
	if err := json.Unmarshal(jsonData, build); err != nil {
		return nil, xerrors.Errorf("Failed to serialize cloudbuild.yaml: %w", err)
	}
	return build, nil
}

func watchCloudBuild(projectID, buildID string) error {
	ctx := context.Background()
	service, err := cloudbuild.NewService(ctx)
	if err != nil {
		xerrors.Errorf("Failed to create cloudbuild service: %w", err)
	}
	buildService := cloudbuild.NewProjectsBuildsService(service)

	call := buildService.Get(projectID, buildID)
	build, err := call.Do()
	if err != nil {
		return xerrors.Errorf("Failed to stat build %s: %w", buildID, err)
	}

	logURLStr := fmt.Sprintf("%v/log-%v.txt", build.LogsBucket, build.Id)
	logURL, err := url.Parse(logURLStr)
	if err != nil {
		return xerrors.Errorf("Invalid url '%s': %w", build.LogUrl, err)
	}
	bucketName := logURL.Host
	objectPath := logURL.Path[1:]

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return xerrors.Errorf("Failed to initialize gcs client: %w", err)
	}
	logObject := gcsClient.Bucket(bucketName).Object(objectPath)

	complete := false
	cbErrCount := 0
	gcsErrCount := 0
	offset := int64(0)

	for !complete {
		if build, err = call.Do(); err != nil {
			cbErrCount++
			log.Printf("Failed to stat build (%v): %v", buildID, err)
		} else {
			cbErrCount = 0
			if isBuildCompleted(build.Status) {
				complete = true
			}
		}
		if reader, err := logObject.NewRangeReader(ctx, offset, -1); err != nil {
			if !isIgnorableGcsError(err) {
				gcsErrCount++
				log.Printf("Failed to read log (%v): %+v", gcsErrCount, err)
			} else {
				gcsErrCount = 0
			}
		} else {
			func() {
				defer reader.Close()
				if count, err := io.Copy(os.Stdout, reader); err != nil {
					if !isIgnorableGcsError(err) {
						gcsErrCount++
						log.Printf("Failed to read log (%v): %+v", gcsErrCount, err)
					} else {
						gcsErrCount = 0
					}
				} else {
					gcsErrCount = 0
					offset += count
				}
			}()
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("Build is complete with %v", build.Status)
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
