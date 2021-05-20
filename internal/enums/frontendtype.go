package enums

import (
	"encoding/json"
	"fmt"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
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

var frontendTypeData = []roxyutil.EnumData{
	{
		GoName: "UndefinedFrontendType",
		Name:   "",
		JSON:   []byte(`""`),
	},
	{
		GoName: "FileSystemFrontendType",
		Name:   "fs",
		JSON:   []byte(`"fs"`),
	},
	{
		GoName: "HTTPBackendFrontendType",
		Name:   "http",
		JSON:   []byte(`"http"`),
	},
	{
		GoName: "GRPCBackendFrontendType",
		Name:   "grpc",
		JSON:   []byte(`"grpc"`),
	},
}

// GoString returns the Go constant name.
func (t FrontendType) GoString() string {
	return roxyutil.DereferenceEnumData("FrontendType", frontendTypeData, uint(t)).GoName
}

// String returns the string representation.
func (t FrontendType) String() string {
	return roxyutil.DereferenceEnumData("FrontendType", frontendTypeData, uint(t)).Name
}

// MarshalJSON fulfills json.Marshaler.
func (t FrontendType) MarshalJSON() ([]byte, error) {
	return roxyutil.MarshalEnumToJSON("FrontendType", frontendTypeData, uint(t))
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (t *FrontendType) UnmarshalJSON(raw []byte) error {
	value, err := roxyutil.UnmarshalEnumFromJSON("FrontendType", frontendTypeData, raw)
	if err == nil {
		*t = FrontendType(value)
		return nil
	}
	if err == roxyutil.ErrIsNull {
		return nil
	}
	*t = 0
	return err
}

var _ fmt.GoStringer = FrontendType(0)
var _ fmt.Stringer = FrontendType(0)
var _ json.Marshaler = FrontendType(0)
var _ json.Unmarshaler = (*FrontendType)(nil)
