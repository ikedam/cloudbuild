package main

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/archive"
)

func main() {
	if err := uploadSource(); err != nil {
		log.Printf("Failed to upload source: %+v", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func uploadSource() error {
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
		return err
	}
	tar, err := archive.TarWithOptions(path, &archive.TarOptions{
		Compression:     archive.Gzip,
		ExcludePatterns: excludes,
	})
	if err != nil {
		return err
	}
	defer tar.Close()
	fd, err := os.OpenFile("source.tar.gz", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer fd.Close()
	if _, err := io.Copy(fd, tar); err != nil {
		return err
	}
	return nil
}
