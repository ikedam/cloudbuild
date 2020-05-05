package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/labstack/echo"
	storage_v1 "google.golang.org/api/storage/v1"
)

// CloudStorageJSONServer implements cloud storage apis.
// https://cloud.google.com/storage/docs/json_api
type CloudStorageJSONServer interface {
	// https://cloud.google.com/storage/docs/json_api/v1/objects/insert
	InsertWithMetadata(string, map[string]interface{}, io.ReadCloser, *http.Request) (*storage_v1.Object, error)
}

// CloudStorageJSONServerRun holds information for running CloudStorageJSONServer
type CloudStorageJSONServerRun struct {
	server *http.Server
	addr   net.Addr
}

// Addr returns bound address
func (r *CloudStorageJSONServerRun) Addr() net.Addr {
	return r.addr
}

// Close shuts down the server
func (r *CloudStorageJSONServerRun) Close() error {
	return r.server.Close()
}

// NewCloudStorageJSONServer starts a server for cloud storage JSON api.
func NewCloudStorageJSONServer(s CloudStorageJSONServer) (*CloudStorageJSONServerRun, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	// Some handlers require stream handlings,
	// so handle with raw net/http and partially pass to echo.
	r := mux.NewRouter()

	// Handle  with raw handler,
	// as it requires stream handling
	r.HandleFunc("/upload/storage/v1/b/{bucket}/o", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)

		if "multipart" != r.URL.Query().Get("uploadType") {
			// only supports multipart.
			// especially, resumable is not supported
			// https://cloud.google.com/storage/docs/resumable-uploads
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf(
				"ERROR: Unexpected uploadType '%v': only multipart is supported."+
					" Don't send too large contents in tests",
				r.URL.Query().Get("uploadType"),
			)
			return
		}

		// https://cloud.google.com/storage/docs/json_api/v1/how-tos/multipart-upload
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("ERROR: Unexpected error for pasring Content-Type: %+v", err)
			return
		}
		if "multipart/related" != mediaType {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("ERROR: Unexpected Content-Type: %v", mediaType)
			return
		}
		defer r.Body.Close()
		mr := multipart.NewReader(r.Body, params["boundary"])
		metadataReader, err := mr.NextPart()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("ERROR: Unexpected error for pasring first part: %+v", err)
			return
		}
		defer metadataReader.Close()
		metadataJSON, err := ioutil.ReadAll(metadataReader)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("ERROR: Failed to read metadata: %+v", err)
			return
		}
		var metadata map[string]interface{}
		if err = json.Unmarshal(metadataJSON, &metadata); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("ERROR: Failed to parse metadata: %+v", err)
			log.Printf("%s", metadataJSON)
			return
		}

		contentReader, err := mr.NextPart()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("ERROR: Unexpected error for reading second part: %+v", err)
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
			log.Printf("ERROR: Error in the handler: %+v", err)
			return
		}
		body, err := json.Marshal(bucket)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("ERROR: Error marshaling response: %+v", err)
			return
		}
		w.Write(body)
		return
	})

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	r.PathPrefix("/").Handler(e.Server.Handler)

	logMux := http.NewServeMux()
	logMux.HandleFunc("/", func(rsp http.ResponseWriter, req *http.Request) {
		log.Printf("%+v %+v", req.Method, req.URL)
		rspWrapper := NewResponseSniffer(rsp)
		r.ServeHTTP(rspWrapper, req)
		log.Printf("%+v %+v %+v size=%v", rspWrapper.Code(), req.Method, req.URL, rspWrapper.BodySize())
	})
	server := &http.Server{
		Handler: logMux,
	}
	go server.Serve(l)
	return &CloudStorageJSONServerRun{
		server: server,
		addr:   l.Addr(),
	}, nil
}