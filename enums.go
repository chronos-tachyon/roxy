package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// type TargetType {{{

type TargetType uint8

const (
	UndefinedTargetType TargetType = iota
	FileSystemTargetType
	BackendTargetType
)

var targetTypeData = []enumData{
	{"UndefinedTargetType", ""},
	{"FileSystemTargetType", "filesystem"},
	{"BackendTargetType", "backend"},
}

var targetTypeMap = map[string]TargetType{
	"":           UndefinedTargetType,
	"fs":         FileSystemTargetType,
	"filesystem": FileSystemTargetType,
	"backend":    BackendTargetType,
}

func (t TargetType) String() string {
	if uint(t) >= uint(len(targetTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return targetTypeData[t].Name
}

func (t TargetType) GoString() string {
	if uint(t) >= uint(len(targetTypeData)) {
		return fmt.Sprintf("TargetType(%d)", uint(t))
	}
	return targetTypeData[t].GoName
}

func (t TargetType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (ptr *TargetType) UnmarshalJSON(raw []byte) error {
	*ptr = 0

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}

	if num, ok := targetTypeMap[strings.ToLower(str)]; ok {
		*ptr = num
		return nil
	}

	allowedNames := make([]string, len(targetTypeData))
	for i, data := range targetTypeData {
		allowedNames[i] = data.Name
	}
	return fmt.Errorf("illegal target type %q; expected one of %q", str, allowedNames)
}

var _ fmt.Stringer = TargetType(0)
var _ json.Marshaler = TargetType(0)
var _ json.Unmarshaler = (*TargetType)(nil)

// }}}

// type MutationType {{{

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

func (ptr *MutationType) UnmarshalJSON(raw []byte) error {
	*ptr = 0

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}

	if num, ok := mutationTypeMap[strings.ToLower(str)]; ok {
		*ptr = num
		return nil
	}

	allowedNames := make([]string, len(mutationTypeData))
	for i, data := range mutationTypeData {
		allowedNames[i] = data.Name
	}
	return fmt.Errorf("illegal mutation type %q; expected one of %q", str, allowedNames)
}

var _ fmt.Stringer = MutationType(0)
var _ json.Marshaler = MutationType(0)
var _ json.Unmarshaler = (*MutationType)(nil)

// }}}

type enumData struct {
	GoName string
	Name   string
}
