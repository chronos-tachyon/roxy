package main

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

func (s *ATCServer) Find(
	ctx context.Context,
	req *roxy_v0.FindRequest,
) (
	*roxy_v0.FindResponse,
	error,
) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "Find").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Str("serviceName", req.ServiceName).
		Uint32("shardID", req.ShardId).
		Bool("hasShardID", req.HasShardId).
		Msg("RPC")

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, _, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, false)
	if err != nil {
		return nil, err
	}

	peerData := impl.PeerByKey(key)
	if peerData == nil {
		return nil, status.Errorf(codes.NotFound, "Key %v not found", key)
	}

	resp := &roxy_v0.FindResponse{
		GoAway: peerData.GoAway(),
	}
	return resp, nil
}
