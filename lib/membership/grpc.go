package membership

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
)

// type GRPC {{{

// GRPC represents a service advertisement in etcd.io's gRPC Naming format.
//
// See: https://etcd.io/docs/v3.4/dev-guide/grpc_naming/
type GRPC struct {
	Op       GRPCOperation `json:"Op"`
	Addr     string        `json:"Addr"`
	Metadata interface{}   `json:"Metadata,omitempty"`
}

// IsAlive returns true if this represents the advertisement of a live server.
func (grpc *GRPC) IsAlive() bool {
	if grpc == nil {
		return false
	}
	return grpc.Op == GRPCOpAdd
}

// NamedPorts returns the list of named ports advertized by the server.
//
// (For GRPC format, this is always the nil list.)
func (grpc *GRPC) NamedPorts() []string {
	return nil
}

// PrimaryAddr returns the server's primary endpoint as a TCPAddr.
func (grpc *GRPC) PrimaryAddr() *net.TCPAddr {
	if !grpc.IsAlive() {
		return nil
	}
	tcpAddr, err := misc.ParseTCPAddr(grpc.Addr, "")
	if err != nil {
		panic(err)
	}
	return tcpAddr
}

// NamedAddr returns the server's named endpoint as a TCPAddr.
//
// (For GRPC format, this always panics for non-"" namedPort.)
func (grpc *GRPC) NamedAddr(namedPort string) *net.TCPAddr {
	if namedPort == "" {
		return grpc.PrimaryAddr()
	}
	if !grpc.IsAlive() {
		return nil
	}
	panic(fmt.Errorf("unknown named port %q", namedPort))
}

var _ Interface = (*GRPC)(nil)

// }}}

// type GRPCOperation {{{

// GRPCOperation represents the status of the server being advertized.
type GRPCOperation uint8

// GRPCOperation constants:
//
//	GRPCOpAdd     healthy
//	GRPCOpDelete  not healthy
const (
	GRPCOpAdd GRPCOperation = iota
	GRPCOpDelete
)

var grpcOperationData = []enumData{
	{"GRPCOpAdd", "ADD"},
	{"GRPCOpDelete", "DELETE"},
}

var grpcOperationJSON = []enumJSON{
	{uint(GRPCOpAdd), []byte("0")},
	{uint(GRPCOpAdd), []byte(`"add"`)},
	{uint(GRPCOpAdd), []byte(`"ADD"`)},
	{uint(GRPCOpDelete), []byte("1")},
	{uint(GRPCOpDelete), []byte(`"delete"`)},
	{uint(GRPCOpDelete), []byte(`"DELETE"`)},
}

// GoString fulfills fmt.GoStringer.
func (op GRPCOperation) GoString() string {
	if uint(op) >= uint(len(grpcOperationData)) {
		panic(fmt.Errorf("invalid GRPCOperation value %d", uint(op)))
	}
	return grpcOperationData[op].GoName
}

// String fulfills fmt.Stringer.
func (op GRPCOperation) String() string {
	if uint(op) >= uint(len(grpcOperationData)) {
		panic(fmt.Errorf("invalid GRPCOperation value %d", uint(op)))
	}
	return grpcOperationData[op].Name
}

// MarshalJSON fulfills json.Marshaler.
func (op GRPCOperation) MarshalJSON() ([]byte, error) {
	return json.Marshal(uint(op))
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (op *GRPCOperation) UnmarshalJSON(raw []byte) error {
	if raw == nil {
		panic(errors.New("raw is nil"))
	}

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	*op = ^GRPCOperation(0)

	for _, row := range grpcOperationJSON {
		if bytes.Equal(raw, row.bytes) {
			*op = GRPCOperation(row.index)
			return nil
		}
	}

	var num uint8
	err0 := json.Unmarshal(raw, &num)
	if err0 == nil {
		if uint(num) < uint(len(grpcOperationData)) {
			*op = GRPCOperation(num)
			return nil
		}
		return fmt.Errorf("invalid GRPCOperation value %d", num)
	}

	var str string
	err1 := json.Unmarshal(raw, &str)
	if err1 == nil {
		for index, data := range grpcOperationData {
			if strings.EqualFold(str, data.Name) || strings.EqualFold(str, data.GoName) {
				*op = GRPCOperation(index)
				return nil
			}
		}
		return fmt.Errorf("invalid GRPCOperation value %q", str)
	}

	return err0
}

var _ fmt.Stringer = GRPCOperation(0)
var _ fmt.GoStringer = GRPCOperation(0)
var _ json.Marshaler = GRPCOperation(0)
var _ json.Unmarshaler = (*GRPCOperation)(nil)

// }}}
