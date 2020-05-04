package testutil

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ikedam/cloudbuild/log"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
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
