package main

import (
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

	ctx, logger := s.rpcBegin(cas.Context(), "ClientAssign")

	logger.Trace().
		Str("func", "ClientAssign.Recv").
		Msg("Call")

	req, err := cas.Recv()
	if err != nil {
		logger.Error().
			Str("func", "ClientAssign.Recv").
			Err(err).
			Msg("Error")
		return err
	}

	logger.Trace().
		Interface("req", req).
		Msg("Request")

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

	key, svc, err := impl.ServiceMap.CheckInput(first.ServiceName, first.ShardId, first.HasShardId, false)

	logger.Debug().
		Stringer("key", key).
		Msg("RPC")

	if err != nil {
		return err
	}

	err = checkAuthInfo(ctx, svc.AllowedClientNames)
	if err != nil {
		return err
	}

	return status.Errorf(codes.Unimplemented, "method ClientAssign not implemented")
}
