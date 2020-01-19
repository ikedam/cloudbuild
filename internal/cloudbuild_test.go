package internal

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ikedam/cloudbuild/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestRunCloudBuild(t *testing.T) {
	mockServer := testutil.SetupMockCloudBuildRESTServer(t)
	if mockServer == nil {
		t.Skip()
	}
	defer mockServer.Close()

	cbService, err := cloudbuild.NewService(
		context.Background(),
		option.WithEndpoint(fmt.Sprintf("http://%v/", mockServer.Addr().String())),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)

	s := &CloudBuildSubmit{
		Config: Config{
			Project: "testProject",
		},
		sourcePath: &GcsPath{
			Bucket: "test",
			Object: "path/to/source.tgz",
		},
		cbService: cbService,
	}
	build := &cloudbuild.Build{}

	metadata := &cloudbuild.BuildOperationMetadata{
		Build: &cloudbuild.Build{
			Id: "test-id",
		},
	}
	metadataJSON, err := testutil.MarshalWithTypeURL(
		"type.googleapis.com/google.devtools.cloudbuild.v1.BuildOperationMetadata",
		&metadata,
	)
	require.NoError(t, err)

	mockServer.Mock.EXPECT().
		CreateBuild(
			// context
			gomock.Any(),
			// projectID
			gomock.Eq("testProject"),
			// Build
			gomock.Any(),
		).
		Return(
			&cloudbuild.Operation{
				Metadata: googleapi.RawMessage(metadataJSON),
			},
			nil,
		).
		Times(1)
	err = s.runCloudBuild(build)

	assert.NoError(t, err)
	assert.Equal(t, "test-id", s.buildID)
}
