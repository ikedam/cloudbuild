package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/ikedam/cloudbuild/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestRunCloudBuildParameters(t *testing.T) {
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
			gomock.All(
				testutil.ApplyMatcherTo("Source.StorageSource.Bucket", gomock.Eq("test")),
				testutil.ApplyMatcherTo("Source.StorageSource.Object", gomock.Eq("path/to/source.tgz")),
			),
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

func TestRunCloudBuildTimeout(t *testing.T) {
	testcases := []struct {
		sleepMsec int
		expectErr error
	}{
		{
			sleepMsec: 0,
			expectErr: nil,
		},
		{
			sleepMsec: 500,
			expectErr: context.DeadlineExceeded,
		},
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

	for _, testcase := range testcases {
		t.Run(fmt.Sprintf("sleepMsec=%v", testcase.sleepMsec), func(t *testing.T) {
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
					Project:               "testProject",
					CloudBuildTimeoutMsec: 100,
				},
				sourcePath: &GcsPath{
					Bucket: "test",
					Object: "path/to/source.tgz",
				},
				cbService: cbService,
			}

			mockServer.Mock.EXPECT().
				CreateBuild(
					// context
					gomock.Any(),
					// projectID
					gomock.Any(),
					// Build
					gomock.Any(),
				).
				DoAndReturn(func(_ interface{}, _ interface{}, _ interface{}) (*cloudbuild.Operation, error) {
					time.Sleep(time.Duration(testcase.sleepMsec) * time.Millisecond)
					return &cloudbuild.Operation{
						Metadata: googleapi.RawMessage(metadataJSON),
					}, nil
				}).
				Times(1)
			err = s.runCloudBuild(build)

			testutil.AssertErrorIs(t, testcase.expectErr, err)
		})
	}
}
