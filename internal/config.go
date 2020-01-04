package internal

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2/google"
)

// Config holds the configuration for cloudbuild
type Config struct {
	// SourceDir is the source directory to archive.
	SourceDir string

	// Project is the ID of Google Cloud Project
	Project string

	// GcsSourceStagingDir is the directory on the Google Cloud Storage
	// to upload source archives.
	GcsSourceStagingDir string

	// IgnoreFile is the ignore file to use instead of .gcloudignore
	IgnoreFile string

	// Config is the file to use instead of cloudbuild.yaml
	Config string

	// PollingIntervalMsec is the interval for polling build statuses and logs.
	PollingIntervalMsec int

	// UploadTry is the number to try uploading. 0 is infinite
	UploadTry int

	// UploadTimeoutMsec is the milliseconds to consider the upload is timed out.
	UploadTimeoutMsec int

	// MaxGetBuildErrorCount is the maximum count to give up to get build informations.
	MaxGetBuildErrorCount int

	// MaxReadLogErrorCount is the maximum count to give up to read logs.
	MaxReadLogErrorCount int
}

// ResolveDefaults fills default values for configurations.
func (c *Config) ResolveDefaults() error {
	if err := c.resolveProject(); err != nil {
		return err
	}
	if err := c.resolveGcsSourceStagingDir(); err != nil {
		return err
	}
	return nil
}

func (c *Config) resolveProject() error {
	if c.Project != "" {
		return nil
	}

	if projectID := os.Getenv("GOOGLE_PROJECT_ID"); projectID != "" {
		c.Project = projectID
		return nil
	}

	ctx := context.Background()
	cred, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return NewConfigError("Failed to get default credentials", err)
	}
	if cred.ProjectID == "" {
		return NewConfigError("No projectId is configured. Please set GOOGLE_PROJECT_ID.", nil)
	}

	c.Project = cred.ProjectID

	return nil
}

func (c *Config) resolveGcsSourceStagingDir() error {
	if c.GcsSourceStagingDir != "" {
		return nil
	}
	c.GcsSourceStagingDir = fmt.Sprintf(
		"gs://%v_cloudbuild/source",
		c.Project,
	)
	return nil
}