package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ikedam/cloudbuild/log"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
)

var (
	// DefaultLogLevel is the initial log level for mocks.
	DefaultLogLevel = logrus.WarnLevel
)

// MarshalWithTypeURL marshals an object with "@type" field.
func MarshalWithTypeURL(typeURL string, v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	work := map[string]interface{}{}
	if err := json.Unmarshal(data, &work); err != nil {
		return nil, err
	}

	work["@type"] = typeURL

	return json.Marshal(&work)
}

type matcherApplyer struct {
	field string
	m     gomock.Matcher
}

// Matches returns whether x is a match.
func (m *matcherApplyer) Matches(x interface{}) bool {
	fields := strings.Split(m.field, ".")
	for _, field := range fields {
		for {
			typeX := reflect.TypeOf(x)
			refX := reflect.ValueOf(x)
			if typeX.Kind() == reflect.Ptr {
				if refX.IsZero() {
					log.WithField("value", fmt.Sprintf("%+v", x)).
						WithField("type", typeX).
						WithField("field", field).
						Warning("Nil value is passed to ApplyMatcherTo")
				}
				x = refX.Elem().Interface()
				continue
			}
			if typeX.Kind() != reflect.Struct {
				log.WithField("value", fmt.Sprintf("%+v", x)).
					WithField("type", typeX).
					WithField("field", field).
					Warning("Non struct type is passed to ApplyMatcherTo")
				return false
			}
			if _, ok := typeX.FieldByName(field); !ok {
				log.WithField("value", fmt.Sprintf("%+v", x)).
					WithField("type", typeX).
					WithField("field", field).
					Warning("Invalid field is specified")
				return false
			}
			value := refX.FieldByName(field)
			x = value.Interface()
			break
		}
	}
	return m.m.Matches(x)
}

// String describes what the matcher matches.
func (m *matcherApplyer) String() string {
	return fmt.Sprintf(".%v %v", m.field, m.m.String())
}

// ApplyMatcherTo applies matcher to specified field
func ApplyMatcherTo(field string, m gomock.Matcher) gomock.Matcher {
	return &matcherApplyer{
		field: field,
		m:     m,
	}
}

// AssertErrorIs assert that an error is a specified type.
func AssertErrorIs(t *testing.T, expected, actual error) bool {
	if expected == nil {
		return assert.Nil(t, actual)
	}
	if !assert.NotNil(t, actual) {
		return false
	}
	if !xerrors.Is(actual, expected) {
		return assert.Fail(
			t,
			fmt.Sprintf(
				"Error is not a: \n"+
					"expected: %+v\n"+
					"actual  : %+v",
				expected,
				actual,
			),
		)
	}
	return true
}

// ResponseSniffer wraps http.ResponseWriter
type ResponseSniffer struct {
	writer   http.ResponseWriter
	code     int
	bodySize int
}

// NewResponseSniffer creates a new ResponseSniffer
func NewResponseSniffer(writer http.ResponseWriter) *ResponseSniffer {
	return &ResponseSniffer{
		writer: writer,
	}
}

// Code returns status code
func (s *ResponseSniffer) Code() int {
	return s.code
}

// BodySize returns response body size
func (s *ResponseSniffer) BodySize() int {
	return s.bodySize
}

// Header returns Header object to write headers to.
func (s *ResponseSniffer) Header() http.Header {
	return s.writer.Header()
}

// Write writes response body.
func (s *ResponseSniffer) Write(body []byte) (int, error) {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	s.bodySize += len(body)
	return s.writer.Write(body)
}

// WriteHeader writes status code.
func (s *ResponseSniffer) WriteHeader(statusCode int) {
	s.code = statusCode
	s.writer.WriteHeader(statusCode)
}

// MockEnvironment mocks an environment variable.
func MockEnvironment(t *testing.T, name, value string, f func()) {
	origValue, exists := os.LookupEnv(name)
	defer func() {
		if exists {
			assert.NoError(t, os.Setenv(name, origValue))
		} else {
			os.Unsetenv(name)
		}
	}()
	assert.NoError(t, os.Setenv(name, value))
	f()
}

// NewLogMux wraps http.Handler and record logs
func NewLogMux(m http.Handler, log *logrus.Logger) http.Handler {
	logMux := http.NewServeMux()
	logMux.HandleFunc("/", func(rsp http.ResponseWriter, req *http.Request) {
		log.Debugf("%+v %+v", req.Method, req.URL)
		rspWrapper := NewResponseSniffer(rsp)
		m.ServeHTTP(rspWrapper, req)
		log.Infof("%+v %+v %+v size=%v", rspWrapper.Code(), req.Method, req.URL, rspWrapper.BodySize())
	})
	return logMux
}
