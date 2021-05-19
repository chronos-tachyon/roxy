package roxyresolver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
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

var eventTypeData = []enumData{
	{"NoOpEvent", "noOp"},
	{"ErrorEvent", "error"},
	{"UpdateEvent", "update"},
	{"DeleteEvent", "delete"},
	{"BadDataEvent", "badData"},
	{"StatusChangeEvent", "statusChange"},
	{"NewServiceConfigEvent", "newServiceConfig"},
}

var eventTypeMap = map[string]EventType{
	"":                 NoOpEvent,
	"noOp":             NoOpEvent,
	"error":            ErrorEvent,
	"update":           UpdateEvent,
	"delete":           DeleteEvent,
	"badData":          BadDataEvent,
	"statusChange":     StatusChangeEvent,
	"newServiceConfig": NewServiceConfigEvent,
}

// GoString returns the Go constant name.
func (t EventType) GoString() string {
	if uint(t) >= uint(len(eventTypeData)) {
		panic(fmt.Errorf("EventType %d is out of range", uint(t)))
	}
	return eventTypeData[t].GoName
}

// String returns the string representation.
func (t EventType) String() string {
	if uint(t) >= uint(len(eventTypeData)) {
		panic(fmt.Errorf("EventType %d is out of range", uint(t)))
	}
	return eventTypeData[t].Name
}

// MarshalJSON fulfills json.Marshaler.
func (t EventType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (t *EventType) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*t = 0
		return err
	}

	return t.Parse(str)
}

// Parse parses the string representation.
func (t *EventType) Parse(str string) error {
	if num, ok := eventTypeMap[str]; ok {
		*t = num
		return nil
	}

	for index, row := range eventTypeData {
		if strings.EqualFold(str, row.Name) || strings.EqualFold(str, row.GoName) {
			*t = EventType(index)
			return nil
		}
	}

	for key, num := range eventTypeMap {
		if strings.EqualFold(str, key) {
			*t = num
			return nil
		}
	}

	*t = 0
	return fmt.Errorf("illegal event type %q; expected one of %q", str, makeAllowedNames(eventTypeData))
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

var balancerTypeData = []enumData{
	{"RandomBalancer", "random"},
	{"RoundRobinBalancer", "roundRobin"},
	{"WeightedRandomBalancer", "weightedRandom"},
	{"WeightedRoundRobinBalancer", "weightedRoundRobin"},
	{"SRVBalancer", "srv"},
}

var balancerTypeMap = map[string]BalancerType{
	"":                   RandomBalancer,
	"random":             RandomBalancer,
	"rand":               RandomBalancer,
	"r":                  RandomBalancer,
	"roundRobin":         RoundRobinBalancer,
	"rr":                 RoundRobinBalancer,
	"weightedRandom":     WeightedRandomBalancer,
	"wr":                 WeightedRandomBalancer,
	"weightedRoundRobin": WeightedRoundRobinBalancer,
	"wrr":                WeightedRoundRobinBalancer,
	"srv":                SRVBalancer,
}

// GoString returns the Go constant name.
func (t BalancerType) GoString() string {
	if uint(t) >= uint(len(balancerTypeData)) {
		panic(fmt.Errorf("BalancerType %d is out of range", uint(t)))
	}
	return balancerTypeData[t].GoName
}

// String returns the string representation.
func (t BalancerType) String() string {
	if uint(t) >= uint(len(balancerTypeData)) {
		panic(fmt.Errorf("BalancerType %d is out of range", uint(t)))
	}
	return balancerTypeData[t].Name
}

// MarshalJSON fulfills json.Marshaler.
func (t BalancerType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (t *BalancerType) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*t = 0
		return err
	}

	return t.Parse(str)
}

// Parse parses the string representation.
func (t *BalancerType) Parse(str string) error {
	if num, ok := balancerTypeMap[str]; ok {
		*t = num
		return nil
	}

	for index, row := range balancerTypeData {
		if strings.EqualFold(str, row.Name) || strings.EqualFold(str, row.GoName) {
			*t = BalancerType(index)
			return nil
		}
	}

	for key, num := range balancerTypeMap {
		if strings.EqualFold(str, key) {
			*t = num
			return nil
		}
	}

	*t = 0
	return fmt.Errorf("illegal balancer type %q; expected one of %q", str, makeAllowedNames(balancerTypeData))
}

var _ fmt.Stringer = BalancerType(0)
var _ fmt.GoStringer = BalancerType(0)
var _ json.Marshaler = BalancerType(0)
var _ json.Unmarshaler = (*BalancerType)(nil)

// }}}

func makeAllowedNames(data []enumData) []string {
	out := make([]string, len(data))
	for i, row := range data {
		out[i] = row.Name
	}
	return out
}

type enumData struct {
	GoName string
	Name   string
}
