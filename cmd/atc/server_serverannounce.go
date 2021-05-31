package main

import (
	"context"
	"errors"
	"io"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

func (s *ATCServer) ServerAnnounce(
	sas roxy_v0.AirTrafficControl_ServerAnnounceServer,
) error {
	if s.admin {
		return status.Error(codes.PermissionDenied, "method ServerAnnounce not permitted over Admin interface")
	}

	log.Logger.Trace().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "ServerAnnounce").
		Str("rpcInterface", "primary").
		Str("func", "ServerAnnounce.Recv").
		Msg("Call")

	req, err := sas.Recv()
	if err != nil {
		log.Logger.Error().
			Str("rpcService", "roxy.v0.AirTrafficControl").
			Str("rpcMethod", "ServerAnnounce").
			Str("rpcInterface", "primary").
			Str("func", "ServerAnnounce.Recv").
			Err(err).
			Msg("Error")

		return err
	}

	log.Logger.Trace().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "ServerAnnounce").
		Str("rpcInterface", "primary").
		Interface("req", req).
		Msg("RPC request")

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

	if len(first.Ip) != 4 && len(first.Ip) != 16 {
		return status.Error(codes.InvalidArgument, "invalid IP address")
	}
	if first.Port >= 65536 {
		return status.Error(codes.InvalidArgument, "invalid TCP port")
	}

	impl := s.ref.AcquireSharedImpl()
	needImplRelease := true
	defer func() {
		if needImplRelease {
			s.ref.ReleaseSharedImpl()
		}
	}()

	key, svc, err := impl.ServiceMap.CheckInput(first.ServiceName, first.ShardId, first.HasShardId, false)
	if err != nil {
		return err
	}

	err = checkAuthInfo(sas.Context(), svc.AllowedServerNames)
	if err != nil {
		return err
	}

	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "ServerAnnounce").
		Str("rpcInterface", "primary").
		Str("serviceName", first.ServiceName).
		Uint32("shardID", first.ShardId).
		Bool("hasShardID", first.HasShardId).
		Msg("RPC")

	shardData := s.ref.GetOrInsertShard(key, svc)

	shardData.Mutex.Lock()
	serverData := shardData.LockedGetOrInsertServer(first)
	serverData.CostHistory.UpdateAbsolute(req.CostCounter)
	serverData.LockedUpdate(true, req.IsServing)
	goAwayCh := (<-chan *roxy_v0.GoAway)(serverData.GoAwayCh)
	shardData.Mutex.Unlock()

	needImplRelease = false
	s.ref.ReleaseSharedImpl()

	errCh := make(chan error)

	go s.serverAnnounceRecvThread(
		sas,
		impl,
		shardData,
		serverData,
		req.IsServing,
		errCh,
	)

	select {
	case err := <-errCh:
		return err

	case goAway, ok := <-goAwayCh:
		if ok {
			if goAway == nil {
				log.Logger.Trace().
					Str("rpcService", "roxy.v0.AirTrafficControl").
					Str("rpcMethod", "ServerAnnounce").
					Str("rpcInterface", "primary").
					Stringer("statusCode", codes.NotFound).
					Msg("Return")

				return status.Errorf(codes.NotFound, "Key %v no longer exists", shardData.Key())
			}

			log.Logger.Trace().
				Str("rpcService", "roxy.v0.AirTrafficControl").
				Str("rpcMethod", "ServerAnnounce").
				Str("rpcInterface", "primary").
				Str("func", "ServerAnnounce.Send").
				Interface("goAway", goAway).
				Msg("Call")

			err := sas.Send(&roxy_v0.ServerAnnounceResponse{GoAway: goAway})
			if err != nil {
				log.Logger.Error().
					Str("rpcService", "roxy.v0.AirTrafficControl").
					Str("rpcMethod", "ServerAnnounce").
					Str("rpcInterface", "primary").
					Str("func", "ServerAnnounce.Send").
					Err(err).
					Msg("Error")
			}
			return err
		}
		err := <-errCh
		return err
	}
}

func (s *ATCServer) serverAnnounceRecvThread(
	sas roxy_v0.AirTrafficControl_ServerAnnounceServer,
	impl *Impl,
	shardData *ShardData,
	serverData *ServerData,
	lastIsServing bool,
	errCh chan<- error,
) {
	var err error
	var sendErr bool
Loop:
	for {
		log.Logger.Trace().
			Str("rpcService", "roxy.v0.AirTrafficControl").
			Str("rpcMethod", "ServerAnnounce").
			Str("rpcInterface", "primary").
			Str("func", "ServerAnnounce.Recv").
			Msg("Call")

		var req *roxy_v0.ServerAnnounceRequest
		req, err = sas.Recv()

		s, ok := status.FromError(err)

		switch {
		case err == nil:
			log.Logger.Trace().
				Str("rpcService", "roxy.v0.AirTrafficControl").
				Str("rpcMethod", "ServerAnnounce").
				Str("rpcInterface", "primary").
				Interface("req", req).
				Msg("RPC request")

		case err == io.EOF:
			break Loop

		case errors.Is(err, context.Canceled):
			break Loop

		case errors.Is(err, context.DeadlineExceeded):
			break Loop

		case ok && s.Code() == codes.Canceled:
			break Loop

		default:
			log.Logger.Error().
				Str("rpcService", "roxy.v0.AirTrafficControl").
				Str("rpcMethod", "ServerAnnounce").
				Str("rpcInterface", "primary").
				Str("call", "ServerAnnounce.Recv").
				Err(err).
				Msg("Error")
			sendErr = true
			break Loop
		}

		shardData.Mutex.Lock()
		serverData.CostHistory.UpdateAbsolute(req.CostCounter)
		serverData.LockedUpdate(true, req.IsServing)
		shardData.Mutex.Unlock()

		lastIsServing = req.IsServing
	}

	shardData.Mutex.Lock()
	serverData.CostHistory.Update()
	serverData.LockedUpdate(false, lastIsServing)
	shardData.Mutex.Unlock()

	if sendErr {
		select {
		case errCh <- err:
		default:
		}
	}

	close(errCh)
}
