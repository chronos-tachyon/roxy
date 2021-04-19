package enums

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TargetType uint8

const (
	UndefinedTargetType TargetType = iota
	FileSystemTargetType
	HTTPBackendTargetType
	GRPCBackendTargetType
)

var targetTypeData = []enumData{
	{"UndefinedTargetType", ""},
	{"FileSystemTargetType", "filesystem"},
	{"HTTPBackendTargetType", "http"},
	{"GRPCBackendTargetType", "grpc"},
}

var targetTypeMap = map[string]TargetType{
	"":           UndefinedTargetType,
	"fs":         FileSystemTargetType,
	"filesystem": FileSystemTargetType,
	"http":       HTTPBackendTargetType,
	"https":      HTTPBackendTargetType,
	"http+tls":   HTTPBackendTargetType,
	"grpc":       GRPCBackendTargetType,
	"grpcs":      GRPCBackendTargetType,
	"grpc+tls":   GRPCBackendTargetType,
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

	return fmt.Errorf("illegal target type %q; expected one of %q", str, makeAllowedNames(targetTypeData))
}

var _ fmt.Stringer = TargetType(0)
var _ fmt.GoStringer = TargetType(0)
var _ json.Marshaler = TargetType(0)
var _ json.Unmarshaler = (*TargetType)(nil)
