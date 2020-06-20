package testutil

import (
	context "context"
	"fmt"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

// MockCloudStorageJSONServerSetup is launched mocked cloud build REST api server
type MockCloudStorageJSONServerSetup struct {
	Mock   *MockCloudStorageJSONServer
	Ctrl   *gomock.Controller
	server *CloudStorageJSONServerRun
}

// Close stops the mocked server
func (m *MockCloudStorageJSONServerSetup) Close() {
	m.server.Close()
	m.Ctrl.Finish()
}

// SetLogLevel sets log level for the server
func (m *MockCloudStorageJSONServerSetup) SetLogLevel(level logrus.Level) {
	m.server.SetLogLevel(level)
}

// NewClient creates a new cloud storage client connecting to this mock.
func (m *MockCloudStorageJSONServerSetup) NewClient(t *testing.T) (*storage.Client, error) {
	var gcsClient *storage.Client
	var err error
	MockEnvironment(
		t,
		"STORAGE_EMULATOR_HOST",
		m.server.objectAddr.String(),
		func() {
			gcsClient, err = storage.NewClient(
				context.Background(),
				option.WithEndpoint(fmt.Sprintf("http://%v/", m.server.apiAddr.String())),
				option.WithoutAuthentication(),
			)
		},
	)
	return gcsClient, err
}

// SetupMockCloudStorageJSONServer starts a JSON api server for mocked cloud storage.
// The return value holds the information for the launched server.
// Ensure to call `Close()` for the returned value when test finishes,
// recommended to call with `defer`.
// You can get the address of the server with m.Addr.String(), where m is the
// return value.
// Skip the test if the return value is `nil`, as it means
// the mocked cloud storageserver is not supported.
func SetupMockCloudStorageJSONServer(t *testing.T) *MockCloudStorageJSONServerSetup {
	ctrl := gomock.NewController(t)
	mock := NewMockCloudStorageJSONServer(ctrl)

	s, err := NewCloudStorageJSONServer(mock)
	if err != nil {
		ctrl.Finish()
		t.Fatal(err)
	}

	return &MockCloudStorageJSONServerSetup{
		Mock:   mock,
		Ctrl:   ctrl,
		server: s,
	}
}
