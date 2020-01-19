package testutil

import (
	"net"
	"net/http"

	"github.com/labstack/echo"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
)

// CloudBuildRESTServer implements cloudbuild REST apis.
type CloudBuildRESTServer interface {
	CreateBuild(echo.Context, string, *cloudbuild.Build) (*cloudbuild.Operation, error)
}

// CloudBuildRESTServerRun holds information for running CloudBuildRESTServer
type CloudBuildRESTServerRun struct {
	e    *echo.Echo
	addr net.Addr
}

// Addr returns bound address
func (r *CloudBuildRESTServerRun) Addr() net.Addr {
	return r.addr
}

// Close shuts down the server
func (r *CloudBuildRESTServerRun) Close() error {
	return r.e.Close()
}

// NewCloudBuildRESTServer starts a server for cloud build REST api.
func NewCloudBuildRESTServer(s CloudBuildRESTServer) (*CloudBuildRESTServerRun, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Listener = l
	e.POST("/v1/projects/:projectID/builds", func(c echo.Context) error {
		build := &cloudbuild.Build{}
		if err := c.Bind(&build); err != nil {
			return c.NoContent(http.StatusBadRequest)
		}
		operation, err := s.CreateBuild(c, c.Param("projectID"), build)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, operation)
	})

	go e.Start("")
	return &CloudBuildRESTServerRun{
		e:    e,
		addr: l.Addr(),
	}, nil
}
