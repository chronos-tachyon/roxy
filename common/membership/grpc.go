package membership

import (
	"encoding/json"
	"fmt"
	"strings"
)

type GRPC struct {
	Op       GRPCOperation `json:"Op"`
	Addr     string        `json:"Addr"`
	Metadata interface{}   `json:"Metadata,omitempty"`
}

func (grpc *GRPC) AsJSON() []byte {
	raw, err := json.Marshal(grpc)
	if err != nil {
		panic(fmt.Errorf("failed to json.Marshal your GRPC: %w", err))
	}
	return raw
}

type GRPCOperation uint8

const (
	GRPCOpAdd GRPCOperation = iota
	GRPCOpDelete

	GRPCOpInvalid = ^GRPCOperation(0)
)

var grpcOperationData = []enumData{
	{"GRPCOpAdd", "ADD"},
	{"GRPCOpDelete", "DELETE"},
	{"GRPCOpInvalid", "<invalid>"},
}

var grpcOperationMap = map[string]GRPCOperation{
	"0":      GRPCOpAdd,
	"1":      GRPCOpDelete,
	"ADD":    GRPCOpAdd,
	"DELETE": GRPCOpDelete,
}

func (op GRPCOperation) String() string {
	if uint(op) >= uint(len(grpcOperationData)) {
		return "<invalid>"
	}
	return grpcOperationData[op].Name
}

func (op GRPCOperation) GoString() string {
	if uint(op) >= uint(len(grpcOperationData)) {
		return fmt.Sprintf("GRPCOperation(%d)", uint(op))
	}
	return grpcOperationData[op].GoName
}

func (op GRPCOperation) MarshalJSON() ([]byte, error) {
	return json.Marshal(uint(op))
}

func (ptr *GRPCOperation) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	if op, found := grpcOperationMap[string(raw)]; found {
		*ptr = op
		return nil
	}

	var num uint8
	err0 := json.Unmarshal(raw, &num)
	if err0 == nil {
		*ptr = GRPCOperation(num)
		return nil
	}

	var str string
	err1 := json.Unmarshal(raw, &str)
	if err1 == nil {
		if op, found := grpcOperationMap[str]; found {
			*ptr = op
			return nil
		}
		for index, data := range grpcOperationData {
			if strings.EqualFold(str, data.Name) {
				*ptr = GRPCOperation(index)
				return nil
			}
		}
		*ptr = GRPCOpInvalid
		return fmt.Errorf("unknown GRPCOperation %q", str)
	}

	*ptr = GRPCOpInvalid
	return err0
}

var _ fmt.Stringer = GRPCOperation(0)
var _ fmt.GoStringer = GRPCOperation(0)
var _ json.Marshaler = GRPCOperation(0)
var _ json.Unmarshaler = (*GRPCOperation)(nil)
