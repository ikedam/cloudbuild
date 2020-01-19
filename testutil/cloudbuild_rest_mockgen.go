// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/ikedam/cloudbuild/testutil (interfaces: CloudBuildRESTServer)

// Package testutil is a generated GoMock package.
package testutil

import (
	gomock "github.com/golang/mock/gomock"
	echo "github.com/labstack/echo"
	v1 "google.golang.org/api/cloudbuild/v1"
	reflect "reflect"
)

// MockCloudBuildRESTServer is a mock of CloudBuildRESTServer interface
type MockCloudBuildRESTServer struct {
	ctrl     *gomock.Controller
	recorder *MockCloudBuildRESTServerMockRecorder
}

// MockCloudBuildRESTServerMockRecorder is the mock recorder for MockCloudBuildRESTServer
type MockCloudBuildRESTServerMockRecorder struct {
	mock *MockCloudBuildRESTServer
}

// NewMockCloudBuildRESTServer creates a new mock instance
func NewMockCloudBuildRESTServer(ctrl *gomock.Controller) *MockCloudBuildRESTServer {
	mock := &MockCloudBuildRESTServer{ctrl: ctrl}
	mock.recorder = &MockCloudBuildRESTServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockCloudBuildRESTServer) EXPECT() *MockCloudBuildRESTServerMockRecorder {
	return m.recorder
}

// CreateBuild mocks base method
func (m *MockCloudBuildRESTServer) CreateBuild(arg0 echo.Context, arg1 string, arg2 *v1.Build) (*v1.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateBuild", arg0, arg1, arg2)
	ret0, _ := ret[0].(*v1.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateBuild indicates an expected call of CreateBuild
func (mr *MockCloudBuildRESTServerMockRecorder) CreateBuild(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateBuild", reflect.TypeOf((*MockCloudBuildRESTServer)(nil).CreateBuild), arg0, arg1, arg2)
}
