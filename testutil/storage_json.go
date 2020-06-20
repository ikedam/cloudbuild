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
	apiServer    *http.Server
	apiAddr      net.Addr
	objectServer *http.Server
	objectAddr   net.Addr
	log          *logrus.Logger
}

// Close shuts down the server
func (r *CloudStorageJSONServerRun) Close() error {
	if err := r.apiServer.Close(); err != nil {
		return err
	}
	return r.objectServer.Close()
}

// SetLogLevel sets the log level
func (r *CloudStorageJSONServerRun) SetLogLevel(level logrus.Level) {
	r.log.SetLevel(level)
}

// NewCloudStorageJSONServer starts a server for cloud storage JSON api.
func NewCloudStorageJSONServer(s CloudStorageJSONServer) (*CloudStorageJSONServerRun, error) {
	apiListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	objectListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		apiListener.Close()
		return nil, err
	}

	log := logrus.New()
	log.Level = DefaultLogLevel

	// Some handlers require stream handlings,
	// so handle with raw net/http and partially pass to echo.
	apiRouter := mux.NewRouter()

	// Handle  with raw handler,
	// as it requires stream handling
	apiRouter.HandleFunc("/upload/storage/v1/b/{bucket}/o", func(w http.ResponseWriter, r *http.Request) {
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

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	apiRouter.PathPrefix("/").Handler(e.Server.Handler)

	objectRouter := mux.NewRouter()
	objectRouter.HandleFunc("/{bucket}/{object:.*}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		if err := s.GetObject(vars["bucket"], vars["object"], w, r); err != nil {
			if httpError, ok := err.(*echo.HTTPError); ok {
				w.WriteHeader(httpError.Code)
				w.Write([]byte(fmt.Sprintf("%+v", httpError.Message)))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			log.WithError(err).Error("Error in the handler")
			return
		}
	})

	apiServer := &http.Server{
		Handler: NewLogMux(apiRouter, log),
	}
	go apiServer.Serve(apiListener)

	objectServer := &http.Server{
		Handler: NewLogMux(objectRouter, log),
	}
	go objectServer.Serve(objectListener)

	return &CloudStorageJSONServerRun{
		apiServer:    apiServer,
		apiAddr:      apiListener.Addr(),
		objectServer: objectServer,
		objectAddr:   objectListener.Addr(),
		log:          log,
	}, nil
}
