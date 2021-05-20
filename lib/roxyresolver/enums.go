package roxyresolver

import (
	"encoding/json"
	"fmt"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// type EventType {{{

// EventType indicates the type of an Event.
type EventType uint8

const (
	// NoOpEvent is an Event with no data.
	NoOpEvent EventType = iota

	// ErrorEvent is an Event with a global error.
	ErrorEvent

	// UpdateEvent is an Event with a new or changed address.
	UpdateEvent

	// DeleteEvent is an Event with a deleted address.
	DeleteEvent

	// BadDataEvent is an Event with an erroneous address.
	BadDataEvent

	// StatusChangeEvent is an Event with an address whose metadata has
	// changed.
	StatusChangeEvent

	// NewServiceConfigEvent is an Event with a new gRPC service config.
	NewServiceConfigEvent
)

var eventTypeData = []roxyutil.EnumData{
	{
		GoName: "NoOpEvent",
		Name:   "noOp",
	},
	{
		GoName: "ErrorEvent",
		Name:   "error",
	},
	{
		GoName: "UpdateEvent",
		Name:   "update",
	},
	{
		GoName: "DeleteEvent",
		Name:   "delete",
	},
	{
		GoName: "BadDataEvent",
		Name:   "badData",
	},
	{
		GoName: "StatusChangeEvent",
		Name:   "statusChange",
	},
	{
		GoName: "NewServiceConfigEvent",
		Name:   "newServiceConfig",
	},
}

// GoString returns the Go constant name.
func (t EventType) GoString() string {
	return roxyutil.DereferenceEnumData("EventType", eventTypeData, uint(t)).GoName
}

// String returns the string representation.
func (t EventType) String() string {
	return roxyutil.DereferenceEnumData("EventType", eventTypeData, uint(t)).Name
}

// MarshalJSON fulfills json.Marshaler.
func (t EventType) MarshalJSON() ([]byte, error) {
	return roxyutil.MarshalEnumToJSON("EventType", eventTypeData, uint(t))
}

// Parse parses the string representation.
func (t *EventType) Parse(str string) error {
	value, err := roxyutil.ParseEnum("EventType", eventTypeData, str)
	if err == nil {
		*t = EventType(value)
		return nil
	}
	*t = 0
	return err
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (t *EventType) UnmarshalJSON(raw []byte) error {
	value, err := roxyutil.UnmarshalEnumFromJSON("EventType", eventTypeData, raw)
	if err == nil {
		*t = EventType(value)
		return nil
	}
	if err == roxyutil.ErrIsNull {
		return nil
	}
	*t = 0
	return err
}

var _ fmt.GoStringer = EventType(0)
var _ fmt.Stringer = EventType(0)
var _ json.Marshaler = EventType(0)
var _ json.Unmarshaler = (*EventType)(nil)

// }}}

// type BalancerType {{{

// BalancerType indicates which load balancer algorithm is in use.
type BalancerType uint8

const (
	// RandomBalancer chooses an address with uniform probability.
	RandomBalancer BalancerType = iota

	// RoundRobinBalancer chooses an address from a random permutation.
	// The permutation changes each time an address is added, changed, or
	// deleted.
	RoundRobinBalancer

	// WeightedRandomBalancer chooses an address with weighted probability.
	WeightedRandomBalancer

	// WeightedRoundRobinBalancer chooses an address from a random shuffle
	// that has zero or more copies of each address, each in proportion to
	// that address's weight.  The shuffle changes each time an address is
	// added, changed, or deleted.
	WeightedRoundRobinBalancer

	// SRVBalancer chooses an address using the rules for DNS SRV records.
	SRVBalancer
)

var balancerTypeData = []roxyutil.EnumData{
	{
		GoName:  "RandomBalancer",
		Name:    "random",
		Aliases: []string{"rand", "r"},
	},
	{
		GoName:  "RoundRobinBalancer",
		Name:    "roundRobin",
		Aliases: []string{"rr"},
	},
	{
		GoName:  "WeightedRandomBalancer",
		Name:    "weightedRandom",
		Aliases: []string{"wr"},
	},
	{
		GoName:  "WeightedRoundRobinBalancer",
		Name:    "weightedRoundRobin",
		Aliases: []string{"wrr"},
	},
	{
		GoName: "SRVBalancer",
		Name:   "srv",
	},
}

// GoString returns the Go constant name.
func (t BalancerType) GoString() string {
	return roxyutil.DereferenceEnumData("BalancerType", balancerTypeData, uint(t)).GoName
}

// String returns the string representation.
func (t BalancerType) String() string {
	return roxyutil.DereferenceEnumData("BalancerType", balancerTypeData, uint(t)).Name
}

// MarshalJSON fulfills json.Marshaler.
func (t BalancerType) MarshalJSON() ([]byte, error) {
	return roxyutil.MarshalEnumToJSON("BalancerType", balancerTypeData, uint(t))
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (t *BalancerType) UnmarshalJSON(raw []byte) error {
	value, err := roxyutil.UnmarshalEnumFromJSON("BalancerType", balancerTypeData, raw)
	if err == nil {
		*t = BalancerType(value)
		return nil
	}
	if err == roxyutil.ErrIsNull {
		return nil
	}
	*t = 0
	return err
}

// Parse parses the string representation.
func (t *BalancerType) Parse(str string) error {
	value, err := roxyutil.ParseEnum("BalancerType", balancerTypeData, str)
	if err == nil {
		*t = BalancerType(value)
		return nil
	}
	*t = 0
	return err
}

var _ fmt.GoStringer = BalancerType(0)
var _ fmt.Stringer = BalancerType(0)
var _ json.Marshaler = BalancerType(0)
var _ json.Unmarshaler = (*BalancerType)(nil)

// }}}
