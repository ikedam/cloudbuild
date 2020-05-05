package internal

import (
	"fmt"
	"net/url"

	"golang.org/x/xerrors"
)

// GcsPath represents the location of the object on Google Cloud Storage
type GcsPath struct {
	// Bucket is the bucket name
	Bucket string
	// Object is the path to the object in the bucket
	Object string
}

func (p *GcsPath) String() string {
	return fmt.Sprintf("gs://%v/%v", p.Bucket, p.Object)
}

// ParseGcsURL parses gs://... URL and returns the bucket name and the object name
func ParseGcsURL(gcsURL string) (*GcsPath, error) {
	parsedURL, err := url.Parse(gcsURL)
	if err != nil {
		return nil, xerrors.Errorf("Invalid url '%s': %w", gcsURL, err)
	}
	return &GcsPath{
		Bucket: parsedURL.Host,
		Object: parsedURL.Path[1:],
	}, nil
}
