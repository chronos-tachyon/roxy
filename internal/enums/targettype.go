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
	{"FileSystemTargetType", "fs"},
	{"HTTPBackendTargetType", "http"},
	{"GRPCBackendTargetType", "grpc"},
}

var targetTypeMap = map[string]TargetType{
	"":     UndefinedTargetType,
	"fs":   FileSystemTargetType,
	"http": HTTPBackendTargetType,
	"grpc": GRPCBackendTargetType,
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

func (t *TargetType) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*t = 0
		return err
	}

	if num, ok := targetTypeMap[str]; ok {
		*t = num
		return nil
	}

	for key, num := range targetTypeMap {
		if strings.EqualFold(key, str) {
			*t = num
			return nil
		}
	}

	*t = 0
	return fmt.Errorf("illegal target type %q; expected one of %q", str, makeAllowedNames(targetTypeData))
}

var _ fmt.Stringer = TargetType(0)
var _ fmt.GoStringer = TargetType(0)
var _ json.Marshaler = TargetType(0)
var _ json.Unmarshaler = (*TargetType)(nil)
