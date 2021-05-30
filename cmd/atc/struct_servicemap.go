package main

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

const MaxNumShards = uint32(1) << 31

type ServiceMap struct {
	list   ServiceNameList
	byName map[ServiceName]*ServiceData
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

func (sm *ServiceMap) Range() Range {
	length := uint(len(sm.list))

	if length == 0 {
		return Range{
			Lo: Key{ServiceName: "", ShardID: 0},
			Hi: Key{ServiceName: "\x7f", ShardID: 0},
		}
	}

	firstName := sm.list[0]
	lastName := sm.list[length-1]
	lastData := sm.byName[lastName]
	limitShardID := ShardID(lastData.EffectiveNumShards())
	return Range{
		Lo: Key{ServiceName: firstName, ShardID: 0},
		Hi: Key{ServiceName: lastName, ShardID: limitShardID},
	}
}

func (sm *ServiceMap) Enumerate(fn func(key Key, data *ServiceData)) {
	r := Range{
		Lo: Key{ServiceName: "", ShardID: 0},
		Hi: Key{ServiceName: "\x7f", ShardID: 0},
	}
	sm.EnumerateRange(r, fn)
}

func (sm *ServiceMap) EnumerateRange(r Range, fn func(key Key, data *ServiceData)) {
	if fn == nil {
		panic(errors.New("fn is nil"))
	}

	length := uint(len(sm.list))
	if length == 0 {
		return
	}

	firstIndex, found := sm.list.Search(r.Lo.ServiceName)
	for index := firstIndex; index < length; index++ {
		serviceName := sm.list[index]
		data := sm.byName[serviceName]

		numShards := data.EffectiveNumShards()
		limit := Key{ServiceName: serviceName, ShardID: ShardID(numShards)}
		key := Key{ServiceName: serviceName, ShardID: 0}
		if found && index == firstIndex {
			key.ShardID = r.Lo.ShardID
		}

		for {
			if !key.Less(r.Hi) {
				return
			}
			if !key.Less(limit) {
				break
			}
			fn(key, data)
			key.ShardID++
		}
	}
}

func (sm *ServiceMap) ExpectedStatsTotal() Stats {
	var sum Stats
	for _, data := range sm.byName {
		sum = sum.Add(data.ExpectedStats())
	}
	return sum
}

func (sm *ServiceMap) ExpectedStatsByRange(r Range) Stats {
	length := uint(len(sm.list))
	if length == 0 {
		return Stats{}
	}

	var sum Stats
	firstIndex, found := sm.list.Search(r.Lo.ServiceName)
	for index := firstIndex; index < length; index++ {
		serviceName := sm.list[index]
		data := sm.byName[serviceName]

		numShards := ShardID(data.EffectiveNumShards())

		minKey := Key{ServiceName: serviceName, ShardID: 0}
		if found && index == firstIndex {
			minKey = r.Lo
		}

		limitKey := Key{ServiceName: serviceName, ShardID: numShards}
		if serviceName > r.Hi.ServiceName {
			limitKey = minKey
		} else if serviceName == r.Hi.ServiceName && limitKey.ShardID > r.Hi.ShardID {
			limitKey = r.Hi
		}

		if minKey.ShardID >= limitKey.ShardID {
			continue
		}

		actualNumShards := uint(limitKey.ShardID - minKey.ShardID)
		sum = sum.Add(
			data.
				ExpectedStatsPerShard().
				Scale(actualNumShards))
	}
	return sum
}

func (sm *ServiceMap) CheckInput(reqServiceName string, reqShardID uint32, reqHasShardID bool, allowWildcard bool) (Key, *ServiceData, error) {
	serviceName := ServiceName(reqServiceName)
	shardID := ShardID(0)
	if reqHasShardID {
		shardID = ShardID(reqShardID)
	}
	key := Key{serviceName, shardID}

	svc := sm.Get(serviceName)
	if svc == nil {
		return key, nil, status.Errorf(codes.NotFound, "unknown service name %q", reqServiceName)
	}

	shardLimit := svc.EffectiveNumShards()
	if !svc.IsSharded && reqHasShardID {
		return key, svc, status.Error(codes.InvalidArgument, "service is not sharded, but a shard_id was provided")
	}
	if svc.IsSharded && !reqHasShardID && !allowWildcard {
		return key, svc, status.Error(codes.InvalidArgument, "service is sharded, but no shard_id was provided")
	}
	if svc.IsSharded && reqHasShardID && reqShardID >= shardLimit {
		return key, svc, status.Errorf(codes.NotFound, "shard_id %d was not found (range 0..%d)", reqShardID, shardLimit-1)
	}

	return key, svc, nil
}
