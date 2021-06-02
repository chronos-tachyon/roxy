package main

import (
	"context"
	"net"

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

	ctx, logger := s.rpcBegin(ctx, "Transfer")

	s.ref.mu.Lock()
	impl := s.ref.next[req.ConfigId]
	s.ref.mu.Unlock()

	if impl == nil {
		return nil, status.Errorf(codes.NotFound, "config_id %d is not loaded", req.ConfigId)
	}

	key, svc, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardNumber, req.HasShardNumber, false)

	logger.Debug().
		Uint64("configID", req.ConfigId).
		Stringer("key", key).
		Msg("RPC")

	if err != nil {
		return nil, err
	}

	err = checkAuthInfo(ctx, s.ref.cfg.AllowedPeers)
	if err != nil {
		return nil, err
	}

	shardData := s.ref.GetOrInsertShard(key, svc, impl.CostMap)

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
		tcpAddr := &net.TCPAddr{
			IP:   net.IP(server.Ip),
			Port: int(server.Port),
			Zone: server.Zone,
		}
		serverData := shardData.LockedGetOrInsertServer(server, tcpAddr)
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
