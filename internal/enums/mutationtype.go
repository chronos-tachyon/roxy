package enums

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// MutationType represents a request/response mutation.
type MutationType uint8

// UndefinedMutationType et al are the legal values for MutationType.
const (
	UndefinedMutationType MutationType = iota
	RequestHostMutationType
	RequestPathMutationType
	RequestQueryMutationType
	RequestHeaderMutationType
	ResponseHeaderPreMutationType
	ResponseHeaderPostMutationType
)

var mutationTypeData = []enumData{
	{"UndefinedMutationType", ""},
	{"RequestHostMutationType", "request-host"},
	{"RequestPathMutationType", "request-path"},
	{"RequestQueryMutationType", "request-query"},
	{"RequestHeaderMutationType", "request-header"},
	{"ResponseHeaderPreMutationType", "response-header-pre"},
	{"ResponseHeaderPostMutationType", "response-header-post"},
}

var mutationTypeJSON = [][][]byte{
	{[]byte(`""`)},
	{[]byte(`"request-host"`), []byte(`"req-host"`)},
	{[]byte(`"request-path"`), []byte(`"req-path"`)},
	{[]byte(`"request-query"`), []byte(`"req-query"`)},
	{[]byte(`"request-header"`), []byte(`"req-header"`)},
	{[]byte(`"response-header-pre"`), []byte(`"resp-header-pre"`)},
	{[]byte(`"response-header-post"`), []byte(`"resp-header-post"`)},
}

// GoString returns the Go constant name.
func (t MutationType) GoString() string {
	if uint(t) >= uint(len(mutationTypeData)) {
		return fmt.Sprintf("MutationType(%d)", uint(t))
	}
	return mutationTypeData[t].GoName
}

// String returns the string representation.
func (t MutationType) String() string {
	if uint(t) >= uint(len(mutationTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return mutationTypeData[t].Name
}

// MarshalJSON fulfills json.Marshaler.
func (t MutationType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (t *MutationType) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	*t = 0

	for index, list := range mutationTypeJSON {
		for _, item := range list {
			if bytes.Equal(raw, item) {
				*t = MutationType(index)
				return nil
			}
		}
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}

	for index, data := range mutationTypeData {
		if strings.EqualFold(str, data.Name) || strings.EqualFold(str, data.GoName) {
			*t = MutationType(index)
			return nil
		}
	}

	return fmt.Errorf("illegal mutation type %q; expected one of %q", str, makeAllowedNames(mutationTypeData))
}

var _ fmt.GoStringer = MutationType(0)
var _ fmt.Stringer = MutationType(0)
var _ json.Marshaler = MutationType(0)
var _ json.Unmarshaler = (*MutationType)(nil)
