package enums

import (
	"encoding/json"
	"fmt"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
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

var mutationTypeData = []roxyutil.EnumData{
	{
		GoName: "UndefinedMutationType",
		Name:   "",
	},
	{
		GoName:  "RequestHostMutationType",
		Name:    "request-host",
		Aliases: []string{"req-host"},
	},
	{
		GoName:  "RequestPathMutationType",
		Name:    "request-path",
		Aliases: []string{"req-path"},
	},
	{
		GoName:  "RequestQueryMutationType",
		Name:    "request-query",
		Aliases: []string{"req-query"},
	},
	{
		GoName:  "RequestHeaderMutationType",
		Name:    "request-header",
		Aliases: []string{"req-header"},
	},
	{
		GoName:  "ResponseHeaderPreMutationType",
		Name:    "response-header-pre",
		Aliases: []string{"resp-header-pre"},
	},
	{
		GoName:  "ResponseHeaderPostMutationType",
		Name:    "response-header-post",
		Aliases: []string{"resp-header-post"},
	},
}

// GoString returns the Go constant name.
func (t MutationType) GoString() string {
	return roxyutil.DereferenceEnumData("MutationType", mutationTypeData, uint(t)).GoName
}

// String returns the string representation.
func (t MutationType) String() string {
	return roxyutil.DereferenceEnumData("MutationType", mutationTypeData, uint(t)).Name
}

// MarshalJSON fulfills json.Marshaler.
func (t MutationType) MarshalJSON() ([]byte, error) {
	return roxyutil.MarshalEnumToJSON("MutationType", mutationTypeData, uint(t))
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (t *MutationType) UnmarshalJSON(raw []byte) error {
	value, err := roxyutil.UnmarshalEnumFromJSON("MutationType", mutationTypeData, raw)
	if err == nil {
		*t = MutationType(value)
		return nil
	}
	if err == roxyutil.ErrIsNull {
		return nil
	}
	*t = 0
	return err
}

var _ fmt.GoStringer = MutationType(0)
var _ fmt.Stringer = MutationType(0)
var _ json.Marshaler = MutationType(0)
var _ json.Unmarshaler = (*MutationType)(nil)
