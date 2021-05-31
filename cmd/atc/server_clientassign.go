package main

import (
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

func (s *ATCServer) ClientAssign(
	cas roxy_v0.AirTrafficControl_ClientAssignServer,
) error {
	if s.admin {
		return status.Error(codes.PermissionDenied, "method ClientAssign not permitted over Admin interface")
	}

	req, err := cas.Recv()
	if err != nil {
		return err
	}

	first := req.First
	if first == nil {
		return status.Error(codes.InvalidArgument, "first is absent")
	}

	err = roxyutil.ValidateATCLocation(first.Location)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	err = roxyutil.ValidateATCUnique(first.Unique)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	impl := s.ref.AcquireSharedImpl()
	needImplRelease := true
	defer func() {
		if needImplRelease {
			s.ref.ReleaseSharedImpl()
		}
	}()

	_, svc, err := impl.ServiceMap.CheckInput(first.ServiceName, first.ShardId, first.HasShardId, false)
	if err != nil {
		return err
	}

	err = checkAuthInfo(cas.Context(), svc.AllowedClientNames)
	if err != nil {
		return err
	}

	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "ClientAssign").
		Str("rpcInterface", "primary").
		Str("serviceName", first.ServiceName).
		Uint32("shardID", first.ShardId).
		Bool("hasShardID", first.HasShardId).
		Msg("RPC")

	return status.Errorf(codes.Unimplemented, "method ClientAssign not implemented")
}
