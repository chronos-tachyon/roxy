package main

import (
	"context"

	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

func (s *ATCServer) Lookup(
	ctx context.Context,
	req *roxy_v0.LookupRequest,
) (
	*roxy_v0.LookupResponse,
	error,
) {
	ctx, logger := s.rpcBegin(ctx, "Lookup")
	_ = ctx

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, svc, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardNumber, req.HasShardNumber, true)

	logger.Debug().
		Stringer("key", key).
		Msg("RPC")

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

	if svc.IsSharded && !req.HasShardNumber {
		shardLimit := ShardNumber(svc.NumShards)
		s.ref.mu.Lock()
		for id := ShardNumber(0); id < shardLimit; id++ {
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
	ctx, logger := s.rpcBegin(ctx, "LookupClients")
	_ = ctx

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, _, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardNumber, req.HasShardNumber, false)

	logger.Debug().
		Stringer("key", key).
		Str("uniqueID", req.UniqueId).
		Msg("RPC")

	if err != nil {
		return nil, err
	}

	resp := &roxy_v0.LookupClientsResponse{}

	shardData := s.ref.Shard(key)
	if shardData != nil {
		shardData.Mutex.Lock()
		if req.UniqueId == "" {
			resp.Clients = make([]*roxy_v0.ClientData, 0, len(shardData.Clients))
			for _, clientData := range shardData.Clients {
				resp.Clients = append(resp.Clients, clientData.LockedToProto())
			}
		} else {
			resp.Clients = make([]*roxy_v0.ClientData, 0, 1)
			clientData := shardData.Clients[req.UniqueId]
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
	ctx, logger := s.rpcBegin(ctx, "LookupServers")
	_ = ctx

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, _, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardNumber, req.HasShardNumber, false)

	logger.Debug().
		Stringer("key", key).
		Str("uniqueID", req.UniqueId).
		Msg("RPC")

	if err != nil {
		return nil, err
	}

	resp := &roxy_v0.LookupServersResponse{}

	shardData := s.ref.Shard(key)
	if shardData != nil {
		shardData.Mutex.Lock()
		if req.UniqueId == "" {
			resp.Servers = make([]*roxy_v0.ServerData, 0, len(shardData.Servers))
			for _, serverData := range shardData.Servers {
				resp.Servers = append(resp.Servers, serverData.LockedToProto())
			}
		} else {
			resp.Servers = make([]*roxy_v0.ServerData, 0, 1)
			serverData := shardData.Servers[req.UniqueId]
			if serverData != nil {
				resp.Servers = append(resp.Servers, serverData.LockedToProto())
			}
		}
		shardData.Mutex.Unlock()
	}

	return resp, nil
}
