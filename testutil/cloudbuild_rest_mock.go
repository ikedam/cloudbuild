package testutil

import (
	context "context"
	"fmt"
	"net"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/option"
)

// MockCloudBuildRESTServerSetup is launched mocked cloud build REST api server
type MockCloudBuildRESTServerSetup struct {
	Mock   *MockCloudBuildRESTServer
	ctrl   *gomock.Controller
	server *CloudBuildRESTServerRun
}

// Addr returns bound address
func (m *MockCloudBuildRESTServerSetup) Addr() net.Addr {
	return m.server.Addr()
}

// Close stops the mocked server
func (m *MockCloudBuildRESTServerSetup) Close() {
	m.server.Close()
	m.ctrl.Finish()
}

// SetLogLevel sets log level for the server
func (m *MockCloudBuildRESTServerSetup) SetLogLevel(level logrus.Level) {
	m.server.SetLogLevel(level)
}

// NewService creates a new cloudbuild service connecting to this mock.
func (m *MockCloudBuildRESTServerSetup) NewService(t *testing.T) (*cloudbuild.Service, error) {
	return cloudbuild.NewService(
		context.Background(),
		option.WithEndpoint(fmt.Sprintf("http://%v/", m.Addr().String())),
		option.WithoutAuthentication(),
	)
}

// SetupMockCloudBuildRESTServer starts a REST api server for mocked cloud build.
// The return value holds the information for the launched server.
// Ensure to call `Close()` for the returned value when test finishes,
// recommended to call with `defer`.
// You can get the address of the server with m.Addr.String(), where m is the
// return value.
// Skip the test if the return value is `nil`, as it means
// the mocked cloud build server is not supported.
func SetupMockCloudBuildRESTServer(t *testing.T) *MockCloudBuildRESTServerSetup {
	ctrl := gomock.NewController(t)
	mock := NewMockCloudBuildRESTServer(ctrl)

	// var s *CloudBuildRESTServerRun
	s, err := NewCloudBuildRESTServer(mock)
	if err != nil {
		ctrl.Finish()
		t.Fatal(err)
	}

	return &MockCloudBuildRESTServerSetup{
		Mock:   mock,
		ctrl:   ctrl,
		server: s,
	}
}
