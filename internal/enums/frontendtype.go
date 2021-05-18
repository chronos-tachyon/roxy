package enums

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

// FrontendType represents a front-end implementation.
type FrontendType uint8

// UndefinedFrontendType et al are the legal values for FrontendType.
const (
	UndefinedFrontendType FrontendType = iota
	FileSystemFrontendType
	HTTPBackendFrontendType
	GRPCBackendFrontendType
)

var frontendTypeData = []enumData{
	{"UndefinedFrontendType", ""},
	{"FileSystemFrontendType", "fs"},
	{"HTTPBackendFrontendType", "http"},
	{"GRPCBackendFrontendType", "grpc"},
}

var frontendTypeJSON = [][][]byte{
	{[]byte(`""`)},
	{[]byte(`"fs"`)},
	{[]byte(`"http"`)},
	{[]byte(`"grpc"`)},
}

// GoString returns the Go constant name.
func (t FrontendType) GoString() string {
	if uint(t) >= uint(len(frontendTypeData)) {
		return fmt.Sprintf("FrontendType(%d)", uint(t))
	}
	return frontendTypeData[t].GoName
}

// String returns the string representation.
func (t FrontendType) String() string {
	if uint(t) >= uint(len(frontendTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return frontendTypeData[t].Name
}

// MarshalJSON fulfills json.Marshaler.
func (t FrontendType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (t *FrontendType) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	*t = 0

	for index, list := range frontendTypeJSON {
		for _, item := range list {
			if bytes.Equal(raw, item) {
				*t = FrontendType(index)
				return nil
			}
		}
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}

	for index, data := range frontendTypeData {
		if strings.EqualFold(str, data.Name) || strings.EqualFold(str, data.GoName) {
			*t = FrontendType(index)
			return nil
		}
	}

	return fmt.Errorf("illegal frontend type %q; expected one of %q", str, makeAllowedNames(frontendTypeData))
}

var _ fmt.GoStringer = FrontendType(0)
var _ fmt.Stringer = FrontendType(0)
var _ json.Marshaler = FrontendType(0)
var _ json.Unmarshaler = (*FrontendType)(nil)
