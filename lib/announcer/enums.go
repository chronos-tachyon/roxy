package announcer

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/chronos-tachyon/roxy/internal/constants"
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

var stateData = []enumData{
	{"StateReady", "READY"},
	{"StateRunning", "RUNNING"},
	{"StateDead", "DEAD"},
	{"StateClosed", "CLOSED"},
}

// GoString fulfills fmt.GoStringer.
func (state State) GoString() string {
	if uint(state) >= uint(len(stateData)) {
		panic(fmt.Errorf("invalid State value %d", uint(state)))
	}
	return stateData[state].GoName
}

// String fulfills fmt.Stringer.
func (state State) String() string {
	if uint(state) >= uint(len(stateData)) {
		panic(fmt.Errorf("invalid State value %d", uint(state)))
	}
	return stateData[state].Name
}

// IsReady returns true if it is safe to call Announce.
func (state State) IsReady() bool {
	return state == StateReady
}

// IsRunning returns true if Withdraw needs to be called.
func (state State) IsRunning() bool {
	switch state {
	case StateRunning:
		fallthrough
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

var formatData = []enumData{
	{"RoxyFormat", "roxy"},
	{"FinagleFormat", "finagle"},
	{"GRPCFormat", "grpc"},
}

var formatJSON = [][]byte{
	[]byte(`"roxy"`),
	[]byte(`"finagle"`),
	[]byte(`"grpc"`),
}

// GoString fulfills fmt.GoStringer.
func (format Format) GoString() string {
	if uint(format) >= uint(len(formatData)) {
		panic(fmt.Errorf("invalid Format value %d", uint(format)))
	}
	return formatData[format].GoName
}

// String fulfills fmt.Stringer.
func (format Format) String() string {
	if uint(format) >= uint(len(formatData)) {
		panic(fmt.Errorf("invalid Format value %d", uint(format)))
	}
	return formatData[format].Name
}

// MarshalJSON fulfills json.Marshaler.
func (format Format) MarshalJSON() ([]byte, error) {
	if uint(format) >= uint(len(formatJSON)) {
		return nil, fmt.Errorf("invalid Format value %d", uint(format))
	}
	return formatJSON[format], nil
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (format *Format) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}
	for index, value := range formatJSON {
		if bytes.Equal(raw, value) {
			*format = Format(index)
			return nil
		}
	}
	return fmt.Errorf("expected \"roxy\", \"finagle\", or \"grpc\"; got %s", string(raw))
}

// Parse parses from a string value.
func (format *Format) Parse(str string) error {
	if str == "" {
		return nil
	}
	for index, row := range formatData {
		if str == row.Name {
			*format = Format(index)
			return nil
		}
	}
	return fmt.Errorf("expected \"roxy\", \"finagle\", or \"grpc\"; got %q", str)
}

var _ fmt.GoStringer = Format(0)
var _ fmt.Stringer = Format(0)
var _ json.Marshaler = Format(0)
var _ json.Unmarshaler = (*Format)(nil)

// }}}

type enumData struct {
	GoName string
	Name   string
}
