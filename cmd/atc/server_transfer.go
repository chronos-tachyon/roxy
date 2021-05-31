package main

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/costhistory"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

func (s *ATCServer) Transfer(
	ctx context.Context,
	req *roxy_v0.TransferRequest,
) (
	*roxy_v0.TransferResponse,
	error,
) {
	if s.admin {
		return nil, status.Error(codes.PermissionDenied, "method Transfer not permitted over Admin interface")
	}

	s.ref.mu.Lock()
	next := s.ref.next[req.ConfigId]
	s.ref.mu.Unlock()

	if next == nil {
		return nil, status.Errorf(codes.NotFound, "config_id %d is not loaded", req.ConfigId)
	}

	key, svc, err := next.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, false)
	if err != nil {
		return nil, err
	}

	err = checkAuthInfo(ctx, s.ref.cfg.AllowedPeers)
	if err != nil {
		return nil, err
	}

	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "Transfer").
		Str("rpcInterface", "primary").
		Str("serviceName", req.ServiceName).
		Uint32("shardID", req.ShardId).
		Bool("hasShardID", req.HasShardId).
		Msg("RPC")

	shardData := s.ref.GetOrInsertShard(key, svc)

	shardData.Mutex.Lock()
	defer shardData.Mutex.Unlock()

	for _, client := range req.Clients {
		clientData := shardData.LockedGetOrInsertClient(client)
		if clientData.IsServing {
			shardData.MeasuredDemandCPS -= clientData.MeasuredCPS
		}
		clientData.CostHistory.Reset()
		clientData.CostHistory.BulkAppend(costhistory.SamplesFromProto(client.History)...)
		clientData.MeasuredCPS = clientData.CostHistory.Data().PerSecond
		clientData.IsServing = client.IsServing
		if clientData.IsServing {
			shardData.MeasuredDemandCPS += clientData.MeasuredCPS
		}
	}

	for _, server := range req.Servers {
		serverData := shardData.LockedGetOrInsertServer(server)
		if serverData.IsServing {
			shardData.MeasuredSupplyCPS -= serverData.MeasuredCPS
		}
		serverData.CostHistory.Reset()
		serverData.CostHistory.BulkAppend(costhistory.SamplesFromProto(server.History)...)
		serverData.MeasuredCPS = serverData.CostHistory.Data().PerSecond
		serverData.IsServing = server.IsServing
		if serverData.IsServing {
			shardData.MeasuredSupplyCPS += serverData.MeasuredCPS
		}
	}

	return &roxy_v0.TransferResponse{}, nil
}
