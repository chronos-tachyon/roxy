package baseresolver

import (
	"encoding/json"
	"fmt"
	"strings"
)

// type EventType {{{

type EventType uint8

const (
	NoOpEvent EventType = iota
	ErrorEvent
	UpdateEvent
	DeleteEvent
	BadDataEvent
	StatusChangeEvent
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

func (t EventType) String() string {
	if uint(t) >= uint(len(eventTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return eventTypeData[t].Name
}

func (t EventType) GoString() string {
	if uint(t) >= uint(len(eventTypeData)) {
		return fmt.Sprintf("EventType(%d)", uint(t))
	}
	return eventTypeData[t].GoName
}

func (t EventType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (ptr *EventType) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*ptr = 0
		return err
	}

	return ptr.Parse(str)
}

func (ptr *EventType) Parse(str string) error {
	if num, ok := eventTypeMap[str]; ok {
		*ptr = num
		return nil
	}

	for index, row := range eventTypeData {
		if strings.EqualFold(str, row.Name) || strings.EqualFold(str, row.GoName) {
			*ptr = EventType(index)
			return nil
		}
	}

	*ptr = 0
	return fmt.Errorf("illegal event type %q; expected one of %q", str, makeAllowedNames(eventTypeData))
}

var _ fmt.Stringer = EventType(0)
var _ fmt.GoStringer = EventType(0)
var _ json.Marshaler = EventType(0)
var _ json.Unmarshaler = (*EventType)(nil)

// }}}

// type BalancerType {{{

type BalancerType uint8

const (
	RandomBalancer BalancerType = iota
	RoundRobinBalancer
	LeastLoadedBalancer
	SRVBalancer
)

var balancerTypeData = []enumData{
	{"RandomBalancer", "random"},
	{"RoundRobinBalancer", "roundRobin"},
	{"LeastLoadedBalancer", "leastLoaded"},
	{"SRVBalancer", "srv"},
}

var balancerTypeMap = map[string]BalancerType{
	"":            RandomBalancer,
	"random":      RandomBalancer,
	"roundRobin":  RoundRobinBalancer,
	"rr":          RoundRobinBalancer,
	"leastLoaded": LeastLoadedBalancer,
	"ll":          LeastLoadedBalancer,
	"srv":         SRVBalancer,
}

func (t BalancerType) String() string {
	if uint(t) >= uint(len(balancerTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return balancerTypeData[t].Name
}

func (t BalancerType) GoString() string {
	if uint(t) >= uint(len(balancerTypeData)) {
		return fmt.Sprintf("BalancerType(%d)", uint(t))
	}
	return balancerTypeData[t].GoName
}

func (t BalancerType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (ptr *BalancerType) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*ptr = 0
		return err
	}

	return ptr.Parse(str)
}

func (ptr *BalancerType) Parse(str string) error {
	if num, ok := balancerTypeMap[str]; ok {
		*ptr = num
		return nil
	}

	for index, row := range balancerTypeData {
		if strings.EqualFold(str, row.Name) || strings.EqualFold(str, row.GoName) {
			*ptr = BalancerType(index)
			return nil
		}
	}

	*ptr = 0
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
