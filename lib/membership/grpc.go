package membership

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
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
	panic(roxyutil.PortError{Type: roxyutil.NamedPort, Port: namedPort, Err: roxyutil.ErrNotExist})
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

var grpcOperationData = []roxyutil.EnumData{
	{
		GoName: "GRPCOpAdd",
		Name:   "ADD",
		JSON:   []byte(`0`),
	},
	{
		GoName: "GRPCOpDelete",
		Name:   "DELETE",
		JSON:   []byte(`1`),
	},
}

// GoString fulfills fmt.GoStringer.
func (op GRPCOperation) GoString() string {
	return roxyutil.DereferenceEnumData("GRPCOperation", grpcOperationData, uint(op)).GoName
}

// String fulfills fmt.Stringer.
func (op GRPCOperation) String() string {
	return roxyutil.DereferenceEnumData("GRPCOperation", grpcOperationData, uint(op)).Name
}

// MarshalJSON fulfills json.Marshaler.
func (op GRPCOperation) MarshalJSON() ([]byte, error) {
	return roxyutil.MarshalEnumToJSON("GRPCOperation", grpcOperationData, uint(op))
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (op *GRPCOperation) UnmarshalJSON(raw []byte) error {
	value, err := roxyutil.UnmarshalEnumFromJSON("GRPCOperation", grpcOperationData, raw)
	if err == nil {
		*op = GRPCOperation(value)
		return nil
	}
	if err == roxyutil.ErrIsNull {
		return nil
	}
	*op = 0
	return err
}

var _ fmt.Stringer = GRPCOperation(0)
var _ fmt.GoStringer = GRPCOperation(0)
var _ json.Marshaler = GRPCOperation(0)
var _ json.Unmarshaler = (*GRPCOperation)(nil)

// }}}
