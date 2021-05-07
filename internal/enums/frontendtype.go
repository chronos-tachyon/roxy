package enums

import (
	"encoding/json"
	"fmt"
	"strings"
)

type FrontendType uint8

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

var frontendTypeMap = map[string]FrontendType{
	"":     UndefinedFrontendType,
	"fs":   FileSystemFrontendType,
	"http": HTTPBackendFrontendType,
	"grpc": GRPCBackendFrontendType,
}

func (t FrontendType) String() string {
	if uint(t) >= uint(len(frontendTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return frontendTypeData[t].Name
}

func (t FrontendType) GoString() string {
	if uint(t) >= uint(len(frontendTypeData)) {
		return fmt.Sprintf("FrontendType(%d)", uint(t))
	}
	return frontendTypeData[t].GoName
}

func (t FrontendType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *FrontendType) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*t = 0
		return err
	}

	if num, ok := frontendTypeMap[str]; ok {
		*t = num
		return nil
	}

	for key, num := range frontendTypeMap {
		if strings.EqualFold(key, str) {
			*t = num
			return nil
		}
	}

	*t = 0
	return fmt.Errorf("illegal frontend type %q; expected one of %q", str, makeAllowedNames(frontendTypeData))
}

var _ fmt.Stringer = FrontendType(0)
var _ fmt.GoStringer = FrontendType(0)
var _ json.Marshaler = FrontendType(0)
var _ json.Unmarshaler = (*FrontendType)(nil)
