package enums

import (
	"encoding/json"
	"fmt"
	"strings"
)

type BalancerType uint8

const (
	RandomBalancer BalancerType = iota
	RoundRobinBalancer
	LeastLoadedBalancer
)

var balancerTypeData = []enumData{
	{"RandomBalancer", "random"},
	{"RoundRobinBalancer", "roundRobin"},
	{"LeastLoadedBalancer", "leastLoaded"},
}

var balancerTypeMap = map[string]BalancerType{
	"":            RandomBalancer,
	"random":      RandomBalancer,
	"roundRobin":  RoundRobinBalancer,
	"rr":          RoundRobinBalancer,
	"leastLoaded": LeastLoadedBalancer,
	"ll":          LeastLoadedBalancer,
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
	*ptr = 0

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}

	if num, ok := balancerTypeMap[strings.ToLower(str)]; ok {
		*ptr = num
		return nil
	}

	return fmt.Errorf("illegal balancer type %q; expected one of %q", str, makeAllowedNames(balancerTypeData))
}

var _ fmt.Stringer = BalancerType(0)
var _ fmt.GoStringer = BalancerType(0)
var _ json.Marshaler = BalancerType(0)
var _ json.Unmarshaler = (*BalancerType)(nil)
