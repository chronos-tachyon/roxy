package main

import (
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

type EnumerateFunc func(key Key, svc *ServiceData)

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

		svc := &ServiceData{
			AllowedClientNames:         config.AllowedClientNames,
			AllowedServerNames:         config.AllowedServerNames,
			ExpectedNumClientsPerShard: config.ExpectedNumClientsPerShard,
			ExpectedNumServersPerShard: config.ExpectedNumServersPerShard,
			IsSharded:                  config.IsSharded,
			NumShards:                  config.NumShards,
			AvgSuppliedCPSPerServer:    config.AvgSuppliedCPSPerServer,
			AvgDemandedCPQ:             config.AvgDemandedCPQ,
		}

		if !svc.IsSharded {
			svc.NumShards = 0
		}

		serviceName := ServiceName(name)
		sm.list = append(sm.list, serviceName)
		sm.byName[serviceName] = svc
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
			Lo: Key{"", 0, false},
			Hi: Key{"\U0010ffff", 0, false},
		}
	}

	firstName := sm.list[0]
	firstData := sm.byName[firstName]
	lo := Key{firstName, 0, firstData.IsSharded}

	lastName := sm.list[length-1]
	lastData := sm.byName[lastName]
	lastLimit := ShardNumber(lastData.EffectiveNumShards())
	hi := Key{lastName, lastLimit - 1, lastData.IsSharded}.Next()

	return Range{Lo: lo, Hi: hi}
}

func (sm *ServiceMap) Enumerate(fn EnumerateFunc) {
	sm.EnumerateRange(sm.Range(), fn)
}

func (sm *ServiceMap) EnumerateRange(r Range, fn EnumerateFunc) {
	roxyutil.AssertNotNil(&fn)

	length := uint(len(sm.list))
	if length == 0 {
		return
	}

	firstIndex, found := sm.list.Search(r.Lo.ServiceName)
	for index := firstIndex; index < length; index++ {
		serviceName := sm.list[index]
		svc := sm.byName[serviceName]

		key := Key{serviceName, 0, svc.IsSharded}
		if found && index == firstIndex && key.HasShardNumber && r.Lo.HasShardNumber {
			key.ShardNumber = r.Lo.ShardNumber
		}

		if !key.Less(r.Hi) {
			break
		}

		if key.HasShardNumber {
			shardLimit := ShardNumber(svc.NumShards)
			limit := Key{serviceName, shardLimit, true}
			if !limit.Less(r.Hi) {
				limit = r.Hi
			}

			for key.Less(limit) {
				fn(key, svc)
				key.ShardNumber++
			}
		} else {
			fn(key, svc)
		}
	}
}

func (sm *ServiceMap) ExpectedStatsTotal() Stats {
	var sum Stats
	for _, svc := range sm.byName {
		sum = sum.Add(svc.ExpectedStats())
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
		svc := sm.byName[serviceName]

		key := Key{serviceName, 0, svc.IsSharded}
		if found && index == firstIndex && key.HasShardNumber && r.Lo.HasShardNumber {
			key.ShardNumber = r.Lo.ShardNumber
		}

		if !key.Less(r.Hi) {
			break
		}

		var actualNumShards uint = 1
		if svc.IsSharded {
			shardLimit := ShardNumber(svc.NumShards)
			limit := Key{serviceName, shardLimit, true}
			if !limit.Less(r.Hi) {
				limit = r.Hi
			}

			actualNumShards = 0
			if key.ShardNumber <= limit.ShardNumber {
				actualNumShards = uint(limit.ShardNumber - key.ShardNumber)
			}
		}

		sum = sum.Add(svc.ExpectedStatsPerShard().Scale(actualNumShards))
	}
	return sum
}

func (sm *ServiceMap) CheckInput(reqServiceName string, reqShardNumber uint32, reqHasShardNumber bool, allowWildcard bool) (Key, *ServiceData, error) {
	serviceName := ServiceName(reqServiceName)
	shardNumber := ShardNumber(0)
	if reqHasShardNumber {
		shardNumber = ShardNumber(reqShardNumber)
	}
	key := Key{serviceName, shardNumber, reqHasShardNumber}

	svc := sm.Get(serviceName)
	if svc == nil {
		return key, nil, status.Errorf(codes.NotFound, "unknown service name %q", reqServiceName)
	}

	shardLimit := svc.EffectiveNumShards()
	if !svc.IsSharded && reqHasShardNumber {
		return key, svc, status.Error(codes.InvalidArgument, "service is not sharded, but a shard_id was provided")
	}
	if svc.IsSharded && !reqHasShardNumber && !allowWildcard {
		return key, svc, status.Error(codes.InvalidArgument, "service is sharded, but no shard_id was provided")
	}
	if svc.IsSharded && reqHasShardNumber && reqShardNumber >= shardLimit {
		return key, svc, status.Errorf(codes.NotFound, "shard_id %d was not found (range 0..%d)", reqShardNumber, shardLimit-1)
	}

	return key, svc, nil
}
