package main

import (
	"context"

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
	ctx, logger := s.rpcBegin(ctx, "Find")
	_ = ctx

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, _, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, false)

	logger.Debug().
		Stringer("key", key).
		Msg("RPC")

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
