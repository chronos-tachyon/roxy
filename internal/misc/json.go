package misc

import (
	"bytes"
	"encoding/json"
)

// StrictUnmarshalJSON is a variant of json.Unmarshal that always sets
// DisallowUnknownFields() and UseNumber().
func StrictUnmarshalJSON(raw []byte, v interface{}) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	d.UseNumber()
	return d.Decode(v)
}
