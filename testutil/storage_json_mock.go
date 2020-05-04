package testutil

import (
	"net"
	"testing"

	"github.com/golang/mock/gomock"
)

// MockCloudStorageJSONServerSetup is launched mocked cloud build REST api server
type MockCloudStorageJSONServerSetup struct {
	Mock   *MockCloudStorageJSONServer
	Ctrl   *gomock.Controller
	server *CloudStorageJSONServerRun
}

// Addr returns bound address
func (m *MockCloudStorageJSONServerSetup) Addr() net.Addr {
	return m.server.Addr()
}

// Close stops the mocked server
func (m *MockCloudStorageJSONServerSetup) Close() {
	m.server.Close()
	m.Ctrl.Finish()
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
