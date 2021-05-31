package main

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

func (s *ATCServer) Lookup(
	ctx context.Context,
	req *roxy_v0.LookupRequest,
) (
	*roxy_v0.LookupResponse,
	error,
) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "Lookup").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Str("serviceName", req.ServiceName).
		Uint32("shardID", req.ShardId).
		Bool("hasShardID", req.HasShardId).
		Msg("RPC")

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, svc, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, true)
	if err != nil {
		return nil, err
	}

	resp := &roxy_v0.LookupResponse{
		AllowedClientNames:                svc.AllowedClientNames.List(),
		AllowedServerNames:                svc.AllowedServerNames.List(),
		ExpectedNumClientsPerShard:        svc.ExpectedNumClientsPerShard,
		ExpectedNumServersPerShard:        svc.ExpectedNumServersPerShard,
		IsSharded:                         svc.IsSharded,
		NumShards:                         svc.NumShards,
		AvgSuppliedCostPerSecondPerServer: svc.AvgSuppliedCPSPerServer,
		AvgDemandedCostPerQuery:           svc.AvgDemandedCPQ,
	}

	if svc.IsSharded && !req.HasShardId {
		shardLimit := ShardID(svc.NumShards)
		s.ref.mu.Lock()
		for id := ShardID(0); id < shardLimit; id++ {
			key2 := Key{key.ServiceName, id, true}
			shardData := s.ref.shardsByKey[key2]
			if shardData != nil {
				resp.Shards = append(resp.Shards, shardData.ToProto())
			}
		}
		s.ref.mu.Unlock()
	} else {
		shardData := s.ref.Shard(key)
		if shardData != nil {
			resp.Shards = append(resp.Shards, shardData.ToProto())
		}
	}

	return resp, nil
}

func (s *ATCServer) LookupClients(
	ctx context.Context,
	req *roxy_v0.LookupClientsRequest,
) (
	*roxy_v0.LookupClientsResponse,
	error,
) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "LookupClients").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Str("serviceName", req.ServiceName).
		Uint32("shardID", req.ShardId).
		Bool("hasShardID", req.HasShardId).
		Str("unique", req.Unique).
		Msg("RPC")

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, _, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, false)
	if err != nil {
		return nil, err
	}

	resp := &roxy_v0.LookupClientsResponse{}

	shardData := s.ref.Shard(key)
	if shardData != nil {
		shardData.Mutex.Lock()
		if req.Unique == "" {
			resp.Clients = make([]*roxy_v0.ClientData, 0, len(shardData.ClientsByUnique))
			for _, clientData := range shardData.ClientsByUnique {
				resp.Clients = append(resp.Clients, clientData.LockedToProto())
			}
		} else {
			resp.Clients = make([]*roxy_v0.ClientData, 0, 1)
			clientData := shardData.ClientsByUnique[req.Unique]
			if clientData != nil {
				resp.Clients = append(resp.Clients, clientData.LockedToProto())
			}
		}
		shardData.Mutex.Unlock()
	}

	return resp, nil
}

func (s *ATCServer) LookupServers(
	ctx context.Context,
	req *roxy_v0.LookupServersRequest,
) (
	*roxy_v0.LookupServersResponse,
	error,
) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "LookupServers").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Str("serviceName", req.ServiceName).
		Uint32("shardID", req.ShardId).
		Bool("hasShardID", req.HasShardId).
		Str("unique", req.Unique).
		Msg("RPC")

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, _, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, false)
	if err != nil {
		return nil, err
	}

	resp := &roxy_v0.LookupServersResponse{}

	shardData := s.ref.Shard(key)
	if shardData != nil {
		shardData.Mutex.Lock()
		if req.Unique == "" {
			resp.Servers = make([]*roxy_v0.ServerData, 0, len(shardData.ServersByUnique))
			for _, serverData := range shardData.ServersByUnique {
				resp.Servers = append(resp.Servers, serverData.LockedToProto())
			}
		} else {
			resp.Servers = make([]*roxy_v0.ServerData, 0, 1)
			serverData := shardData.ServersByUnique[req.Unique]
			if serverData != nil {
				resp.Servers = append(resp.Servers, serverData.LockedToProto())
			}
		}
		shardData.Mutex.Unlock()
	}

	return resp, nil
}
