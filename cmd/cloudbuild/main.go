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

	"cloud.google.com/go/storage"
	"gopkg.in/yaml.v3"

	"golang.org/x/oauth2/google"
	"golang.org/x/xerrors"

	cloudbuild "google.golang.org/api/cloudbuild/v1"

	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/archive"
	"github.com/rs/xid"
)

func main() {
	projectId, err := getProjectId()
	if err != nil {
		log.Printf("Failed to upload source: %+v", err)
		os.Exit(1)
		return
	}

	gsFile := fmt.Sprintf(
		"gs://%v_cloudbuild/source/%v.tgz",
		projectId,
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

	if err := runCloudBuild(projectId, build, gsFile); err != nil {
		log.Printf("Failed to run cloud build: %+v", err)
		os.Exit(1)
		return
	}
	os.Exit(0)
	return
}

func getProjectId() (string, error) {
	if projectId := os.Getenv("GOOGLE_PROJECT_ID"); projectId != "" {
		return projectId, nil
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
			if read_excludes, err := dockerignore.ReadAll(fd); err == nil {
				excludes = read_excludes
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
	gsUrl, err := url.Parse(gsFile)
	if err != nil {
		return xerrors.Errorf("Invalid url '%s': %w", gsFile, err)
	}
	bucketName := gsUrl.Host
	objectPath := gsUrl.Path[1:]

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

func runCloudBuild(projectId string, build *cloudbuild.Build, source string) error {
	gsUrl, err := url.Parse(source)
	if err != nil {
		return xerrors.Errorf("Invalid url '%s': %w", source, err)
	}
	bucketName := gsUrl.Host
	objectPath := gsUrl.Path[1:]

	build.Source = &cloudbuild.Source{
		StorageSource: &cloudbuild.StorageSource{
			Bucket: bucketName,
			Object: objectPath,
		},
	}

	ctx := context.Background()
	service, err := cloudbuild.NewService(ctx)
	if err != nil {
		return xerrors.Errorf("Failed to create cloudbuild service: %w", err)
	}
	buildService := cloudbuild.NewProjectsBuildsService(service)
	call := buildService.Create(projectId, build)
	operation, err := call.Do()
	if err != nil {
		return xerrors.Errorf("Failed to start build: %w", err)
	}

	log.Printf("Started as: %v", operation.Name)
	return nil
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
