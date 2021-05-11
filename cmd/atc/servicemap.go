package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

const MaxNumShards = uint32(1) << 31

type ServiceName string

type ServiceNameList []ServiceName

type ShardID uint32

type ServiceMap struct {
	list   ServiceNameList
	byName map[ServiceName]*ServiceData
}

type ServiceData struct {
	IsSharded                  bool
	NumShards                  uint32
	ExpectedNumClientsPerShard uint32
	ExpectedNumServersPerShard uint32
	MaxLoadPerServer           float32
	clientCNSet                map[string]struct{}
	serverCNSet                map[string]struct{}
}

func NewServiceMap(file MainFile) (*ServiceMap, error) {
	sm := &ServiceMap{
		list:   make(ServiceNameList, 0, len(file.Services)),
		byName: make(map[ServiceName]*ServiceData, len(file.Services)),
	}

	for name, config := range file.Services {
		err := roxyutil.ValidateATCServiceName(name)
		if err != nil {
			return nil, err
		}

		if config.IsSharded && config.NumShards > MaxNumShards {
			return nil, fmt.Errorf("Service %q: NumShards %d > %d", name, config.NumShards, MaxNumShards)
		}
		if config.ExpectedNumClientsPerShard < 1 {
			return nil, fmt.Errorf("Service %q: ExpectedNumClientsPerShard %d < 1", name, config.ExpectedNumClientsPerShard)
		}
		if config.ExpectedNumServersPerShard < 1 {
			return nil, fmt.Errorf("Service %q: ExpectedNumServersPerShard %d < 1", name, config.ExpectedNumServersPerShard)
		}
		if config.MaxLoadPerServer <= 0.0 {
			return nil, fmt.Errorf("Service %q: MaxLoadPerServer %f <= 0.0", name, config.MaxLoadPerServer)
		}

		data := &ServiceData{
			IsSharded:                  config.IsSharded,
			NumShards:                  config.NumShards,
			ExpectedNumClientsPerShard: config.ExpectedNumClientsPerShard,
			ExpectedNumServersPerShard: config.ExpectedNumServersPerShard,
			MaxLoadPerServer:           config.MaxLoadPerServer,
			clientCNSet:                makeCNSet(config.AllowedClientCommonNames),
			serverCNSet:                makeCNSet(config.AllowedServerCommonNames),
		}

		if !data.IsSharded {
			data.NumShards = 0
		}

		serviceName := ServiceName(name)
		sm.list = append(sm.list, serviceName)
		sm.byName[serviceName] = data
	}

	sm.list.Sort()
	return sm, nil
}

func (sm *ServiceMap) ServiceNames() ServiceNameList {
	return sm.list
}

func (sm *ServiceMap) Get(serviceName ServiceName) *ServiceData {
	return sm.byName[serviceName]
}

func (sm *ServiceMap) Enumerate(fn func(serviceName ServiceName, shardID ShardID, data *ServiceData)) {
	if fn == nil {
		panic(errors.New("fn is nil"))
	}
	for _, serviceName := range sm.list {
		data := sm.byName[serviceName]
		if data.IsSharded {
			for shardID := ShardID(0); shardID < ShardID(data.NumShards); shardID++ {
				fn(serviceName, shardID, data)
			}
		} else {
			fn(serviceName, 0, data)
		}
	}
}

func (sm *ServiceMap) TotalExpectedBalancerCost() float64 {
	var total float64
	for _, data := range sm.byName {
		total += data.ExpectedBalancerCost()
	}
	return total
}

func (data *ServiceData) ExpectedBalancerCostPerShard() float64 {
	const BaseCost = 8
	const ClientCostFactor = 1
	const ServerCostFactor = 1
	var sum float64
	sum += BaseCost
	sum += ClientCostFactor * float64(data.ExpectedNumClientsPerShard)
	sum += ServerCostFactor * float64(data.ExpectedNumServersPerShard)
	return sum
}

func (data *ServiceData) ExpectedBalancerCost() float64 {
	total := data.ExpectedBalancerCostPerShard()
	if data.IsSharded {
		total *= float64(data.NumShards)
	}
	return total
}

func (data *ServiceData) AllowedClientCommonNames() []string {
	if data.clientCNSet == nil {
		return nil
	}
	out := make([]string, 0, len(data.clientCNSet))
	for key := range data.clientCNSet {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (data *ServiceData) AllowedServerCommonNames() []string {
	if data.serverCNSet == nil {
		return nil
	}
	out := make([]string, 0, len(data.serverCNSet))
	for key := range data.serverCNSet {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (data *ServiceData) IsAllowedClientCommonName(cn string) bool {
	if data.clientCNSet == nil {
		return true
	}
	_, found := data.clientCNSet[cn]
	return found
}

func (data *ServiceData) IsAllowedServerCommonName(cn string) bool {
	if data.serverCNSet == nil {
		return true
	}
	_, found := data.serverCNSet[cn]
	return found
}

func (list ServiceNameList) Len() int {
	return len(list)
}

func (list ServiceNameList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list ServiceNameList) Less(i, j int) bool {
	a, b := list[i], list[j]
	return a < b
}

func (list ServiceNameList) Sort() {
	sort.Sort(list)
}

var _ sort.Interface = ServiceNameList(nil)

func makeCNSet(list []string) map[string]struct{} {
	if list == nil {
		return nil
	}
	set := make(map[string]struct{}, len(list))
	for _, item := range list {
		set[item] = struct{}{}
	}
	return set
}
