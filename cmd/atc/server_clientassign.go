package main

import (
	"context"
	"errors"
	"io"

	"github.com/rs/zerolog"
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

	err = roxyutil.ValidateATCServiceName(first.ServiceName)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	err = roxyutil.ValidateATCUniqueID(first.UniqueId)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	err = roxyutil.ValidateATCLocation(first.Location)
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

	key, svc, err := impl.ServiceMap.CheckInput(first.ServiceName, first.ShardNumber, first.HasShardNumber, false)

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

	shardData := s.ref.GetOrInsertShard(key, svc, impl.CostMap)

	shardData.Mutex.Lock()
	clientData := shardData.GetOrInsertClientLocked(first)
	clientData.CostHistory.UpdateAbsolute(req.CostCounter)
	clientData.UpdateLocked(true, req.IsServing)
	clientData.OnConnectLocked()
	goAwayCh := (<-chan *roxy_v0.GoAway)(clientData.GoAwayCh)
	shardData.Mutex.Unlock()

	needImplRelease = false
	s.ref.ReleaseSharedImpl()

	active := &activeClientAssign{
		logger:     logger,
		s:          s,
		cas:        cas,
		shardData:  shardData,
		clientData: clientData,
		flushCh:    clientData.FlushCh,
		errCh:      make(chan error),
	}

	go active.recvThread(req.IsServing)

	for {
		select {
		case <-gMultiServer.Done():
			err := status.Error(codes.Unavailable, "ATC server is shutting down")
			logger.Trace().
				Err(err).
				Msg("Return")
			return err

		case err := <-active.errCh:
			return err

		case goAway, ok := <-goAwayCh:
			if !ok {
				return <-active.errCh
			}

			if goAway == nil {
				err := status.Errorf(codes.NotFound, "Key %v no longer exists", shardData.Key)
				logger.Trace().
					Err(err).
					Msg("Return")
				return err
			}

			err := active.doSend(&roxy_v0.ClientAssignResponse{GoAway: goAway})
			if err == nil {
				err = <-active.errCh
			}
			return err

		case <-active.flushCh:
			shardData.Mutex.Lock()
			events := clientData.EventQueue
			clientData.EventQueue = nil
			shardData.Mutex.Unlock()

			if len(events) != 0 {
				err := active.doSend(&roxy_v0.ClientAssignResponse{Events: events})
				if err != nil {
					return err
				}
			}
		}
	}
}

type activeClientAssign struct {
	logger     zerolog.Logger
	s          *ATCServer
	cas        roxy_v0.AirTrafficControl_ClientAssignServer
	shardData  *ShardData
	clientData *ClientData
	flushCh    chan struct{}
	errCh      chan error
}

func (active *activeClientAssign) doSend(resp *roxy_v0.ClientAssignResponse) error {
	active.logger.Trace().
		Str("func", "ClientAssign.Send").
		Interface("resp", resp).
		Msg("Call")

	err := active.cas.Send(resp)
	if err != nil {
		active.logger.Error().
			Str("func", "ClientAssign.Send").
			Err(err).
			Msg("Error")
	}
	return err
}

func (active *activeClientAssign) recvThread(lastIsServing bool) {
	var err error
	var sendErr bool
	defer func() {
		active.shardData.Mutex.Lock()
		active.clientData.CostHistory.Update()
		active.clientData.UpdateLocked(false, lastIsServing)
		active.shardData.Mutex.Unlock()

		if sendErr {
			select {
			case active.errCh <- err:
			default:
			}
		}

		close(active.errCh)
	}()

	for {
		active.logger.Trace().
			Str("func", "ClientAssign.Recv").
			Msg("Call")

		var req *roxy_v0.ClientAssignRequest
		req, err = active.cas.Recv()
		s, ok := status.FromError(err)

		switch {
		case err == nil:
			active.logger.Trace().
				Interface("req", req).
				Msg("Request")

		case err == io.EOF:
			fallthrough
		case errors.Is(err, context.Canceled):
			fallthrough
		case errors.Is(err, context.DeadlineExceeded):
			fallthrough
		case ok && s.Code() == codes.Canceled:
			active.logger.Trace().
				Str("func", "ClientAssign.Recv").
				Err(err).
				Msg("Hangup")
			return

		default:
			active.logger.Error().
				Str("func", "ClientAssign.Recv").
				Err(err).
				Msg("Error")
			sendErr = true
			return
		}

		active.shardData.Mutex.Lock()
		active.clientData.CostHistory.UpdateAbsolute(req.CostCounter)
		active.clientData.UpdateLocked(true, req.IsServing)
		active.shardData.Mutex.Unlock()

		lastIsServing = req.IsServing
	}
}
