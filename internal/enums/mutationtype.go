package enums

import (
	"encoding/json"
	"fmt"
	"strings"
)

type MutationType uint8

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

var mutationTypeMap = map[string]MutationType{
	"":                     UndefinedMutationType,
	"request-host":         RequestHostMutationType,
	"request-path":         RequestPathMutationType,
	"request-query":        RequestQueryMutationType,
	"request-header":       RequestHeaderMutationType,
	"response-header-pre":  ResponseHeaderPreMutationType,
	"response-header-post": ResponseHeaderPostMutationType,
	"req-host":             RequestHostMutationType,
	"req-path":             RequestPathMutationType,
	"req-query":            RequestQueryMutationType,
	"req-header":           RequestHeaderMutationType,
	"resp-header-pre":      ResponseHeaderPreMutationType,
	"resp-header-post":     ResponseHeaderPostMutationType,
}

func (t MutationType) String() string {
	if uint(t) >= uint(len(mutationTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return mutationTypeData[t].Name
}

func (t MutationType) GoString() string {
	if uint(t) >= uint(len(mutationTypeData)) {
		return fmt.Sprintf("MutationType(%d)", uint(t))
	}
	return mutationTypeData[t].GoName
}

func (t MutationType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *MutationType) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*t = 0
		return err
	}

	if num, ok := mutationTypeMap[str]; ok {
		*t = num
		return nil
	}

	for key, num := range mutationTypeMap {
		if strings.EqualFold(key, str) {
			*t = num
			return nil
		}
	}

	*t = 0
	return fmt.Errorf("illegal mutation type %q; expected one of %q", str, makeAllowedNames(mutationTypeData))
}

var _ fmt.Stringer = MutationType(0)
var _ fmt.GoStringer = MutationType(0)
var _ json.Marshaler = MutationType(0)
var _ json.Unmarshaler = (*MutationType)(nil)
