package main

import (
	"context"
	"errors"
	"io"
	"net"

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

	tcpAddr, err := makeTCPAddr(first.Ip, first.Port, first.Zone)
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

	err = checkAuthInfo(ctx, svc.AllowedServerNames)
	if err != nil {
		return err
	}

	shardData := s.ref.GetOrInsertShard(key, svc, impl.CostMap)

	shardData.Mutex.Lock()
	serverData := shardData.LockedGetOrInsertServer(first, tcpAddr)
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

	// Send an empty response as an acknowledgement that the first request
	// has been received and accepted.  This has the benefit of resetting
	// the retry counter in lib/atcclient.
	err = active.doSend(&roxy_v0.ServerAnnounceResponse{})
	if err != nil {
		return err
	}

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

			err := active.doSend(&roxy_v0.ServerAnnounceResponse{GoAway: goAway})
			if err == nil {
				err = <-active.errCh
			}
			return err
		}
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

func (active *activeServerAnnounce) doSend(resp *roxy_v0.ServerAnnounceResponse) error {
	active.logger.Trace().
		Str("func", "ServerAnnounce.Send").
		Interface("resp", resp).
		Msg("Call")

	err := active.sas.Send(resp)
	if err != nil {
		active.logger.Error().
			Str("func", "ServerAnnounce.Send").
			Err(err).
			Msg("Error")
	}
	return err
}

func (active *activeServerAnnounce) recvThread(lastIsServing bool) {
	var err error
	var sendErr bool

	defer func() {
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
	}()

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
				Str("func", "ServerAnnounce.Recv").
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
				Str("func", "ServerAnnounce.Recv").
				Err(err).
				Msg("Hangup")
			return

		default:
			active.logger.Error().
				Str("func", "ServerAnnounce.Recv").
				Err(err).
				Msg("Error")
			sendErr = true
			return
		}

		active.shardData.Mutex.Lock()
		active.serverData.CostHistory.UpdateAbsolute(req.CostCounter)
		active.serverData.LockedUpdate(true, req.IsServing)
		active.shardData.Mutex.Unlock()

		lastIsServing = req.IsServing
	}
}

func makeTCPAddr(ip []byte, port uint32, zone string) (*net.TCPAddr, error) {
	if port >= 65536 {
		return nil, errors.New("invalid TCP port")
	}

	portNum := int(port)

	if len(ip) != net.IPv4len && len(ip) != net.IPv6len {
		return nil, errors.New("invalid IP address")
	}

	ipAddr := net.IP(ip)
	if ipv4 := ipAddr.To4(); ipv4 != nil {
		ipAddr = ipv4
	}

	isIPv6 := (len(ip) == net.IPv6len)
	isLocal := ipAddr.IsLinkLocalUnicast() || ipAddr.IsLinkLocalMulticast() || ipAddr.IsInterfaceLocalMulticast()
	if isIPv6 && isLocal && zone == "" {
		return nil, errors.New("link-local IPv6 address requires zone")
	}
	if zone != "" && !isLocal && !isIPv6 {
		return nil, errors.New("zone requires link-local IPv6 address")
	}

	tcpAddr := &net.TCPAddr{
		IP:   ipAddr,
		Port: portNum,
		Zone: zone,
	}
	return tcpAddr, nil
}
