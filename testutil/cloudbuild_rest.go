package testutil

import (
	"net"
	"net/http"

	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
)

// CloudBuildRESTServer implements cloudbuild REST apis.
type CloudBuildRESTServer interface {
	// https://cloud.google.com/cloud-build/docs/api/reference/rest/v1/projects.builds/create
	CreateBuild(echo.Context, string, *cloudbuild.Build) (*cloudbuild.Operation, error)
	// https://cloud.google.com/cloud-build/docs/api/reference/rest/v1/projects.builds/get
	GetBuild(echo.Context, string, string) (*cloudbuild.Build, error)
}

// CloudBuildRESTServerRun holds information for running CloudBuildRESTServer
type CloudBuildRESTServerRun struct {
	server *http.Server
	addr   net.Addr
	log    *logrus.Logger
}

// Addr returns bound address
func (r *CloudBuildRESTServerRun) Addr() net.Addr {
	return r.addr
}

// Close shuts down the server
func (r *CloudBuildRESTServerRun) Close() error {
	return r.server.Close()
}

// SetLogLevel sets the log level
func (r *CloudBuildRESTServerRun) SetLogLevel(level logrus.Level) {
	r.log.SetLevel(level)
}

// NewCloudBuildRESTServer starts a server for cloud build REST api.
func NewCloudBuildRESTServer(s CloudBuildRESTServer) (*CloudBuildRESTServerRun, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	log := logrus.New()
	log.Level = DefaultLogLevel

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
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
	e.GET("/v1/projects/:projectID/builds/:buildID", func(c echo.Context) error {
		build, err := s.GetBuild(c, c.Param("projectID"), c.Param("buildID"))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, build)
	})

	logMux := http.NewServeMux()
	logMux.HandleFunc("/", func(rsp http.ResponseWriter, req *http.Request) {
		log.Debugf("%+v %+v", req.Method, req.URL)
		rspWrapper := NewResponseSniffer(rsp)
		e.ServeHTTP(rspWrapper, req)
		log.Infof("%+v %+v %+v size=%v", rspWrapper.Code(), req.Method, req.URL, rspWrapper.BodySize())
	})
	server := &http.Server{
		Handler: logMux,
	}
	go server.Serve(l)
	return &CloudBuildRESTServerRun{
		server: server,
		addr:   l.Addr(),
		log:    log,
	}, nil
}
