package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/chronos-tachyon/roxy/lib/certnames"
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
	AllowedClientNames         certnames.CertNames
	AllowedServerNames         certnames.CertNames
	ExpectedNumClientsPerShard uint32
	ExpectedNumServersPerShard uint32
	IsSharded                  bool
	NumShards                  uint32
	AvgSuppliedCPSPerServer    float64
	AvgDemandedCPQ             float64
}

func NewServiceMap(file ServicesFile) (*ServiceMap, error) {
	sm := &ServiceMap{
		list:   make(ServiceNameList, 0, len(file)),
		byName: make(map[ServiceName]*ServiceData, len(file)),
	}

	for name, config := range file {
		err := roxyutil.ValidateATCServiceName(name)
		if err != nil {
			return nil, err
		}

		if config.IsSharded && config.NumShards > MaxNumShards {
			return nil, fmt.Errorf("Service %q: NumShards %d > %d", name, config.NumShards, MaxNumShards)
		}

		data := &ServiceData{
			AllowedClientNames:         config.AllowedClientNames,
			AllowedServerNames:         config.AllowedServerNames,
			ExpectedNumClientsPerShard: config.ExpectedNumClientsPerShard,
			ExpectedNumServersPerShard: config.ExpectedNumServersPerShard,
			IsSharded:                  config.IsSharded,
			NumShards:                  config.NumShards,
			AvgSuppliedCPSPerServer:    config.AvgSuppliedCPSPerServer,
			AvgDemandedCPQ:             config.AvgDemandedCPQ,
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
