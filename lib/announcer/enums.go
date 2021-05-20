package announcer

import (
	"encoding/json"
	"fmt"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// type State {{{

// State is used to keep track of which Announcer methods have been called.
type State uint8

const (
	// StateReady marks that Announce() or Close() may be called.
	StateReady State = iota

	// StateRunning marks that Withdraw() may be called, and that the
	// announcement may be active.
	StateRunning

	// StateDead marks that Withdraw() may be called, and that the
	// announcement has ended due to a context cancellation or the like.
	StateDead

	// StateClosed marks that no more methods may be called.
	StateClosed
)

var stateData = []roxyutil.EnumData{
	{
		GoName: "StateReady",
		Name:   "READY",
	},
	{
		GoName: "StateRunning",
		Name:   "RUNNING",
	},
	{
		GoName: "StateDead",
		Name:   "DEAD",
	},
	{
		GoName: "StateClosed",
		Name:   "CLOSED",
	},
}

// GoString fulfills fmt.GoStringer.
func (state State) GoString() string {
	return roxyutil.DereferenceEnumData("State", stateData, uint(state)).GoName
}

// String fulfills fmt.Stringer.
func (state State) String() string {
	return roxyutil.DereferenceEnumData("State", stateData, uint(state)).Name
}

// IsReady returns true if it is safe to call Announce.
func (state State) IsReady() bool {
	return state == StateReady
}

// IsRunning returns true if Withdraw needs to be called.
func (state State) IsRunning() bool {
	switch state {
	case StateRunning:
		return true
	case StateDead:
		return true

	default:
		return false
	}
}

var _ fmt.GoStringer = State(0)
var _ fmt.Stringer = State(0)

// }}}

// type Format {{{

// Format selects which server advertisement serialization format to use.
type Format uint8

const (
	// RoxyFormat selects Roxy's native JSON format.
	RoxyFormat Format = iota

	// FinagleFormat selects the Finagle ServerSet JSON format.
	FinagleFormat

	// GRPCFormat selects the etcd.io gRPC JSON format.
	GRPCFormat
)

var formatData = []roxyutil.EnumData{
	{
		GoName: "RoxyFormat",
		Name:   "roxy",
		JSON:   []byte(`"roxy"`),
	},
	{
		GoName: "FinagleFormat",
		Name:   "finagle",
		JSON:   []byte(`"finagle"`),
	},
	{
		GoName: "GRPCFormat",
		Name:   "grpc",
		JSON:   []byte(`"grpc"`),
	},
}

// GoString fulfills fmt.GoStringer.
func (format Format) GoString() string {
	return roxyutil.DereferenceEnumData("Format", formatData, uint(format)).GoName
}

// String fulfills fmt.Stringer.
func (format Format) String() string {
	return roxyutil.DereferenceEnumData("Format", formatData, uint(format)).Name
}

// MarshalJSON fulfills json.Marshaler.
func (format Format) MarshalJSON() ([]byte, error) {
	return roxyutil.MarshalEnumToJSON("Format", formatData, uint(format))
}

// Parse parses from a string value.
func (format *Format) Parse(str string) error {
	value, err := roxyutil.ParseEnum("Format", formatData, str)
	if err == nil {
		*format = Format(value)
		return nil
	}
	*format = 0
	return err
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (format *Format) UnmarshalJSON(raw []byte) error {
	value, err := roxyutil.UnmarshalEnumFromJSON("Format", formatData, raw)
	if err == nil {
		*format = Format(value)
		return nil
	}
	if err == roxyutil.ErrIsNull {
		return nil
	}
	*format = 0
	return err
}

var _ fmt.GoStringer = Format(0)
var _ fmt.Stringer = Format(0)
var _ json.Marshaler = Format(0)
var _ json.Unmarshaler = (*Format)(nil)

// }}}
