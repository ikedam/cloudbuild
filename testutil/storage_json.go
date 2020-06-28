package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	storage_v1 "google.golang.org/api/storage/v1"
)

// CloudStorageJSONServer implements cloud storage apis.
// https://cloud.google.com/storage/docs/json_api
type CloudStorageJSONServer interface {
	// https://cloud.google.com/storage/docs/json_api/v1/objects/insert
	InsertWithMetadata(string, map[string]interface{}, io.ReadCloser, *http.Request) (*storage_v1.Object, error)
	GetObject(string, string, http.ResponseWriter, *http.Request) error
}

// CloudStorageJSONServerRun holds information for running CloudStorageJSONServer
type CloudStorageJSONServerRun struct {
	serverInterface CloudStorageJSONServer
	server          *http.Server
	addr            net.Addr
	log             *logrus.Logger
	router          *mux.Router
}

// Close shuts down the server
func (run *CloudStorageJSONServerRun) Close() error {
	return run.server.Close()
}

// SetLogLevel sets the log level
func (run *CloudStorageJSONServerRun) SetLogLevel(level logrus.Level) {
	run.log.SetLevel(level)
}

// PrepareBucket prepares bucket
// This is required as generic bucket routing easily masks api interfaces.
func (run *CloudStorageJSONServerRun) PrepareBucket(bucket string) {
	run.router.HandleFunc(fmt.Sprintf("/%s/{object:.*}", bucket), func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		if err := run.serverInterface.GetObject(bucket, vars["object"], w, r); err != nil {
			if httpError, ok := err.(*echo.HTTPError); ok {
				w.WriteHeader(httpError.Code)
				w.Write([]byte(fmt.Sprintf("%+v", httpError.Message)))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			run.log.WithError(err).Error("Error in the handler")
			return
		}
	})
}

// NewCloudStorageJSONServer starts a server for cloud storage JSON api.
func NewCloudStorageJSONServer(s CloudStorageJSONServer) (*CloudStorageJSONServerRun, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	log := logrus.New()
	log.Level = DefaultLogLevel

	// cloud.google.com/go/storage library behaves really strange way:
	// * cloud storage consists of an api server and a storage server.
	// * The library accepts an alternate api server address as a client option.
	// * The library accepts analternate storage server address with
	//     the environment variable "STORAGE_EMULATOR_HOST"
	// * Strange behavior: the library uses "STORAGE_EMULATOR_HOST" as
	//     the address for the api server for some operations like uploads.
	//
	// This behavior prevents us to provide seperate api server and storage server
	// for mocking.
	router := mux.NewRouter()

	// Handle  with raw handler,
	// as it requires stream handling
	router.HandleFunc("/upload/storage/v1/b/{bucket}/o", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)

		if "multipart" != r.URL.Query().Get("uploadType") {
			// only supports multipart.
			// especially, resumable is not supported
			// https://cloud.google.com/storage/docs/resumable-uploads
			w.WriteHeader(http.StatusInternalServerError)
			log.Errorf(
				"Unexpected uploadType '%v': only multipart is supported."+
					" Don't send too large contents in tests",
				r.URL.Query().Get("uploadType"),
			)
			return
		}

		// https://cloud.google.com/storage/docs/json_api/v1/how-tos/multipart-upload
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.WithError(err).Errorf("Unexpected error for pasring Content-Type: %v", r.Header.Get("Content-Type"))
			return
		}
		if "multipart/related" != mediaType {
			w.WriteHeader(http.StatusInternalServerError)
			log.Errorf("Unexpected Content-Type: %v", mediaType)
			return
		}
		defer r.Body.Close()
		mr := multipart.NewReader(r.Body, params["boundary"])
		metadataReader, err := mr.NextPart()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.WithError(err).Error("Unexpected error for pasring first part")
			return
		}
		defer metadataReader.Close()
		metadataJSON, err := ioutil.ReadAll(metadataReader)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.WithError(err).Error("Failed to read metadata")
			return
		}
		var metadata map[string]interface{}
		if err = json.Unmarshal(metadataJSON, &metadata); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.WithError(err).WithField("metadata", metadataJSON).Error("Failed to parse metadata")
			return
		}

		contentReader, err := mr.NextPart()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.WithError(err).Error("Unexpected error for reading second part")
			return
		}

		bucket, err := s.InsertWithMetadata(vars["bucket"], metadata, contentReader, r)
		if err != nil {
			if httpError, ok := err.(*echo.HTTPError); ok {
				w.WriteHeader(httpError.Code)
				w.Write([]byte(fmt.Sprintf("%+v", httpError.Message)))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			log.WithError(err).Error("Error in the handler")
			return
		}
		body, err := json.Marshal(bucket)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.WithError(err).Errorf("Error marshaling response")
			return
		}
		w.Write(body)
		return
	})

	// Many APIs work as REST, and then pass to echo.
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var match mux.RouteMatch
		if router.Match(req, &match) {
			router.ServeHTTP(w, req)
			return
		}
		e.ServeHTTP(w, req)
	})

	server := &http.Server{
		Handler: NewLogMux(handler, log),
	}
	go server.Serve(listener)

	return &CloudStorageJSONServerRun{
		serverInterface: s,
		server:          server,
		addr:            listener.Addr(),
		router:          router,
		log:             log,
	}, nil
}
