package roxyresolver

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

func (t *EventType) UnmarshalJSON(raw []byte) error {
	if string(raw) == nullString {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*t = 0
		return err
	}

	return t.Parse(str)
}

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
	WeightedRandomBalancer
	WeightedRoundRobinBalancer
)

var balancerTypeData = []enumData{
	{"RandomBalancer", "random"},
	{"RoundRobinBalancer", "roundRobin"},
	{"LeastLoadedBalancer", "leastLoaded"},
	{"SRVBalancer", "srv"},
	{"WeightedRandomBalancer", "weightedRandom"},
	{"WeightedRoundRobinBalancer", "weightedRoundRobin"},
}

var balancerTypeMap = map[string]BalancerType{
	"":                   RandomBalancer,
	"random":             RandomBalancer,
	"rand":               RandomBalancer,
	"r":                  RandomBalancer,
	"roundRobin":         RoundRobinBalancer,
	"rr":                 RoundRobinBalancer,
	"leastLoaded":        LeastLoadedBalancer,
	"ll":                 LeastLoadedBalancer,
	"srv":                SRVBalancer,
	"weightedRandom":     WeightedRandomBalancer,
	"wr":                 WeightedRandomBalancer,
	"weightedRoundRobin": WeightedRoundRobinBalancer,
	"wrr":                WeightedRoundRobinBalancer,
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

func (t *BalancerType) UnmarshalJSON(raw []byte) error {
	if string(raw) == nullString {
		return nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		*t = 0
		return err
	}

	return t.Parse(str)
}

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
