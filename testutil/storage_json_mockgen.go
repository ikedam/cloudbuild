// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/ikedam/cloudbuild/testutil (interfaces: CloudStorageJSONServer)

// Package testutil is a generated GoMock package.
package testutil

import (
	gomock "github.com/golang/mock/gomock"
	v1 "google.golang.org/api/storage/v1"
	io "io"
	http "net/http"
	reflect "reflect"
)

// MockCloudStorageJSONServer is a mock of CloudStorageJSONServer interface
type MockCloudStorageJSONServer struct {
	ctrl     *gomock.Controller
	recorder *MockCloudStorageJSONServerMockRecorder
}

// MockCloudStorageJSONServerMockRecorder is the mock recorder for MockCloudStorageJSONServer
type MockCloudStorageJSONServerMockRecorder struct {
	mock *MockCloudStorageJSONServer
}

// NewMockCloudStorageJSONServer creates a new mock instance
func NewMockCloudStorageJSONServer(ctrl *gomock.Controller) *MockCloudStorageJSONServer {
	mock := &MockCloudStorageJSONServer{ctrl: ctrl}
	mock.recorder = &MockCloudStorageJSONServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockCloudStorageJSONServer) EXPECT() *MockCloudStorageJSONServerMockRecorder {
	return m.recorder
}

// GetObject mocks base method
func (m *MockCloudStorageJSONServer) GetObject(arg0, arg1 string, arg2 http.ResponseWriter, arg3 *http.Request) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetObject", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetObject indicates an expected call of GetObject
func (mr *MockCloudStorageJSONServerMockRecorder) GetObject(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetObject", reflect.TypeOf((*MockCloudStorageJSONServer)(nil).GetObject), arg0, arg1, arg2, arg3)
}

// InsertWithMetadata mocks base method
func (m *MockCloudStorageJSONServer) InsertWithMetadata(arg0 string, arg1 map[string]interface{}, arg2 io.ReadCloser, arg3 *http.Request) (*v1.Object, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertWithMetadata", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(*v1.Object)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InsertWithMetadata indicates an expected call of InsertWithMetadata
func (mr *MockCloudStorageJSONServerMockRecorder) InsertWithMetadata(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertWithMetadata", reflect.TypeOf((*MockCloudStorageJSONServer)(nil).InsertWithMetadata), arg0, arg1, arg2, arg3)
}
