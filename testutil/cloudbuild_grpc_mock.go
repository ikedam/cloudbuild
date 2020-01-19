package testutil

import (
	"net"
	"testing"

	"github.com/golang/mock/gomock"

	pb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
	"google.golang.org/grpc"
)

// MockCloudBuildGrpcServerSetup is launched mocked cloud build server
type MockCloudBuildGrpcServerSetup struct {
	Mock   *MockCloudBuildServer
	ctrl   *gomock.Controller
	server *grpc.Server
	addr   net.Addr
}

// Addr returns bound address
func (m *MockCloudBuildGrpcServerSetup) Addr() net.Addr {
	return m.addr
}

// Close stops the mocked server
func (m *MockCloudBuildGrpcServerSetup) Close() {
	m.server.Stop()
	m.ctrl.Finish()
}

// SetupMockCloudBuildGrpcServer starts a server for mocked cloud build.
// The return value holds the information for the launched server.
// Ensure to call `Close()` for the returned value when test finishes,
// recommended to call with `defer`.
// You can get the address of the server with m.Addr.String(), where m is the
// return value.
// Skip the test if the return value is `nil`, as it means
// the mocked cloud build server is not supported.
func SetupMockCloudBuildGrpcServer(t *testing.T) *MockCloudBuildGrpcServerSetup {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind address: %+v", err)
	}

	ctrl := gomock.NewController(t)
	mock := NewMockCloudBuildServer(ctrl)

	s := grpc.NewServer()
	pb.RegisterCloudBuildServer(s, mock)
	go s.Serve(l)

	return &MockCloudBuildGrpcServerSetup{
		Mock:   mock,
		ctrl:   ctrl,
		server: s,
		addr:   l.Addr(),
	}
}
