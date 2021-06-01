package main

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
	"github.com/rs/zerolog"
)

func (s *ATCServer) ServerAnnounce(
	sas roxy_v0.AirTrafficControl_ServerAnnounceServer,
) error {
	if s.admin {
		return status.Error(codes.PermissionDenied, "method ServerAnnounce not permitted over Admin interface")
	}

	ctx, logger := s.rpcBegin(sas.Context(), "ServerAnnounce")

	logger.Trace().
		Str("func", "ServerAnnounce.Recv").
		Msg("Call")

	req, err := sas.Recv()
	if err != nil {
		logger.Error().
			Str("func", "ServerAnnounce.Recv").
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

	logger.Debug().
		Stringer("key", key).
		Msg("RPC")

	if err != nil {
		return err
	}

	err = checkAuthInfo(ctx, svc.AllowedServerNames)
	if err != nil {
		return err
	}

	shardData := s.ref.GetOrInsertShard(key, svc)

	shardData.Mutex.Lock()
	serverData := shardData.LockedGetOrInsertServer(first)
	serverData.CostHistory.UpdateAbsolute(req.CostCounter)
	serverData.LockedUpdate(true, req.IsServing)
	goAwayCh := (<-chan *roxy_v0.GoAway)(serverData.GoAwayCh)
	shardData.Mutex.Unlock()

	needImplRelease = false
	s.ref.ReleaseSharedImpl()

	active := &activeServerAnnounce{
		logger:     logger,
		s:          s,
		sas:        sas,
		shardData:  shardData,
		serverData: serverData,
		errCh:      make(chan error),
	}

	go active.recvThread(req.IsServing)

	select {
	case err := <-active.errCh:
		return err

	case goAway, ok := <-goAwayCh:
		if ok {
			if goAway == nil {
				logger.Trace().
					Stringer("statusCode", codes.NotFound).
					Msg("Return")

				return status.Errorf(codes.NotFound, "Key %v no longer exists", shardData.Key())
			}

			logger.Trace().
				Str("func", "ServerAnnounce.Send").
				Interface("goAway", goAway).
				Msg("Call")

			err := sas.Send(&roxy_v0.ServerAnnounceResponse{GoAway: goAway})
			if err != nil {
				logger.Error().
					Str("func", "ServerAnnounce.Send").
					Err(err).
					Msg("Error")
			}
			return err
		}
		err := <-active.errCh
		return err
	}
}

type activeServerAnnounce struct {
	logger     zerolog.Logger
	s          *ATCServer
	sas        roxy_v0.AirTrafficControl_ServerAnnounceServer
	shardData  *ShardData
	serverData *ServerData
	errCh      chan error
}

func (active *activeServerAnnounce) recvThread(lastIsServing bool) {
	var err error
	var sendErr bool
Loop:
	for {
		active.logger.Trace().
			Str("func", "ServerAnnounce.Recv").
			Msg("Call")

		var req *roxy_v0.ServerAnnounceRequest
		req, err = active.sas.Recv()

		s, ok := status.FromError(err)

		switch {
		case err == nil:
			active.logger.Trace().
				Interface("req", req).
				Msg("Request")

		case err == io.EOF:
			break Loop

		case errors.Is(err, context.Canceled):
			break Loop

		case errors.Is(err, context.DeadlineExceeded):
			break Loop

		case ok && s.Code() == codes.Canceled:
			break Loop

		default:
			active.logger.Error().
				Str("func", "ServerAnnounce.Recv").
				Err(err).
				Msg("Error")
			sendErr = true
			break Loop
		}

		active.shardData.Mutex.Lock()
		active.serverData.CostHistory.UpdateAbsolute(req.CostCounter)
		active.serverData.LockedUpdate(true, req.IsServing)
		active.shardData.Mutex.Unlock()

		lastIsServing = req.IsServing
	}

	active.shardData.Mutex.Lock()
	active.serverData.CostHistory.Update()
	active.serverData.LockedUpdate(false, lastIsServing)
	active.shardData.Mutex.Unlock()

	if sendErr {
		select {
		case active.errCh <- err:
		default:
		}
	}

	close(active.errCh)
}
