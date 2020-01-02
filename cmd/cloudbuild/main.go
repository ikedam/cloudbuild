package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2/google"
	"golang.org/x/xerrors"

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

	tar, err := createSourceArchive()
	if err != nil {
		log.Printf("Failed to create source: %+v", err)
		os.Exit(1)
		return
	}
	defer tar.Close()

	gsFile := fmt.Sprintf(
		"gs://%v_cloudbuild/source/%v.tgz",
		projectId,
		xid.New().String(),
	)

	if err := uploadCloudStorage(gsFile, tar); err != nil {
		log.Printf("Failed to upload source: %+v", err)
		os.Exit(1)
		return
	}

	log.Printf("Uploaded as %v", gsFile)
	/*
		fd, err := os.OpenFile("source.tar.gz", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			log.Printf("Failed to create source.tar.gz: %+v", err)
			os.Exit(1)
			return
		}
		defer fd.Close()
		if _, err := io.Copy(fd, tar); err != nil {
			log.Printf("Failed to create source.tar.gz: %+v", err)
			os.Exit(1)
			return
		}
	*/
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
