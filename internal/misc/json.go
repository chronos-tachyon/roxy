package misc

import (
	"bytes"
	"encoding/json"
)

func StrictUnmarshalJSON(raw []byte, v interface{}) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	d.UseNumber()
	return d.Decode(v)
}
