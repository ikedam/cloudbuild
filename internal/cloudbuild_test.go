package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/ikedam/cloudbuild/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/longrunning"
)

func TestRunCloudBuild(t *testing.T) {
	mockServer := testutil.SetupMockGrpcCloudBuildServer(t)
	if mockServer == nil {
		t.Skip()
	}
	defer mockServer.Close()

	cbService, err := cloudbuild.NewService(
		context.Background(),
		option.WithEndpoint(fmt.Sprintf("http://%v/", mockServer.Addr.String())),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)

	s := &CloudBuildSubmit{
		Config: Config{},
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
	metadataJSON, err := json.Marshal(&metadata)
	require.NoError(t, err)

	mockServer.Mock.EXPECT().
		CreateBuild(
			// context
			gomock.Any(),
			// pb.CreateBuildRequest
			gomock.Any(),
		).
		Return(
			&longrunning.Operation{
				Metadata: &any.Any{
					TypeUrl: "type.googleapis.com/google.devtools.cloudbuild.v1.BuildOperationMetadata",
					Value:   metadataJSON,
				},
			},
			nil,
		).
		Times(1)
	err = s.runCloudBuild(build)

	assert.NoError(t, err)
	assert.Equal(t, "test-id", s.buildID)
}
