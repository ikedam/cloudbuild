package testutil

import (
	"encoding/json"
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
