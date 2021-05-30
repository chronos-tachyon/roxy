package main

import (
	"context"
	"errors"
	"io"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/certnames"
	"github.com/chronos-tachyon/roxy/lib/costhistory"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type ATCServer struct {
	roxy_v0.UnimplementedAirTrafficControlServer
	ref   *Ref
	admin bool
}

func (s *ATCServer) Lookup(ctx context.Context, req *roxy_v0.LookupRequest) (*roxy_v0.LookupResponse, error) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "Lookup").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Str("serviceName", req.ServiceName).
		Uint32("shardID", req.ShardId).
		Bool("hasShardID", req.HasShardId).
		Msg("RPC")

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, svc, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, true)
	if err != nil {
		return nil, err
	}

	resp := &roxy_v0.LookupResponse{
		AllowedClientNames:                svc.AllowedClientNames.List(),
		AllowedServerNames:                svc.AllowedServerNames.List(),
		ExpectedNumClientsPerShard:        svc.ExpectedNumClientsPerShard,
		ExpectedNumServersPerShard:        svc.ExpectedNumServersPerShard,
		IsSharded:                         svc.IsSharded,
		NumShards:                         svc.NumShards,
		AvgSuppliedCostPerSecondPerServer: svc.AvgSuppliedCPSPerServer,
		AvgDemandedCostPerQuery:           svc.AvgDemandedCPQ,
	}

	if req.HasShardId {
		shardData := s.ref.Shard(key)
		if shardData != nil {
			resp.Shards = append(resp.Shards, shardData.ToProto())
		}
	} else {
		shardLimit := ShardID(svc.EffectiveNumShards())
		s.ref.mu.Lock()
		for id := ShardID(0); id < shardLimit; id++ {
			key2 := Key{key.ServiceName, id}
			shardData := s.ref.shardsByKey[key2]
			if shardData != nil {
				resp.Shards = append(resp.Shards, shardData.ToProto())
			}
		}
		s.ref.mu.Unlock()
	}

	return resp, nil
}

func (s *ATCServer) LookupClients(ctx context.Context, req *roxy_v0.LookupClientsRequest) (*roxy_v0.LookupClientsResponse, error) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "LookupClients").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Str("serviceName", req.ServiceName).
		Uint32("shardID", req.ShardId).
		Bool("hasShardID", req.HasShardId).
		Str("unique", req.Unique).
		Msg("RPC")

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, _, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, false)
	if err != nil {
		return nil, err
	}

	resp := &roxy_v0.LookupClientsResponse{}

	shardData := s.ref.Shard(key)
	if shardData != nil {
		shardData.Mutex.Lock()
		if req.Unique == "" {
			resp.Clients = make([]*roxy_v0.ClientData, 0, len(shardData.ClientsByUnique))
			for _, clientData := range shardData.ClientsByUnique {
				resp.Clients = append(resp.Clients, clientData.LockedToProto())
			}
		} else {
			resp.Clients = make([]*roxy_v0.ClientData, 0, 1)
			clientData := shardData.ClientsByUnique[req.Unique]
			if clientData != nil {
				resp.Clients = append(resp.Clients, clientData.LockedToProto())
			}
		}
		shardData.Mutex.Unlock()
	}

	return resp, nil
}

func (s *ATCServer) LookupServers(ctx context.Context, req *roxy_v0.LookupServersRequest) (*roxy_v0.LookupServersResponse, error) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "LookupServers").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Str("serviceName", req.ServiceName).
		Uint32("shardID", req.ShardId).
		Bool("hasShardID", req.HasShardId).
		Str("unique", req.Unique).
		Msg("RPC")

	impl := s.ref.AcquireSharedImpl()
	defer s.ref.ReleaseSharedImpl()

	key, _, err := impl.ServiceMap.CheckInput(req.ServiceName, req.ShardId, req.HasShardId, false)
	if err != nil {
		return nil, err
	}

	resp := &roxy_v0.LookupServersResponse{}

	shardData := s.ref.Shard(key)
	if shardData != nil {
		shardData.Mutex.Lock()
		if req.Unique == "" {
			resp.Servers = make([]*roxy_v0.ServerData, 0, len(shardData.ServersByUnique))
			for _, serverData := range shardData.ServersByUnique {
				resp.Servers = append(resp.Servers, serverData.LockedToProto())
			}
		} else {
			resp.Servers = make([]*roxy_v0.ServerData, 0, 1)
			serverData := shardData.ServersByUnique[req.Unique]
			if serverData != nil {
				resp.Servers = append(resp.Servers, serverData.LockedToProto())
			}
		}
		shardData.Mutex.Unlock()
	}

	return resp, nil
}

func (s *ATCServer) Find(ctx context.Context, req *roxy_v0.FindRequest) (*roxy_v0.FindResponse, error) {
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

func (s *ATCServer) ServerAnnounce(sas roxy_v0.AirTrafficControl_ServerAnnounceServer) error {
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
					Str("statusCode", codes.NotFound.String()).
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

func (s *ATCServer) ClientAssign(cas roxy_v0.AirTrafficControl_ClientAssignServer) error {
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

func (s *ATCServer) Transfer(ctx context.Context, req *roxy_v0.TransferRequest) (*roxy_v0.TransferResponse, error) {
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

func rpcInterfaceName(isAdmin bool) string {
	if isAdmin {
		return "admin"
	}
	return "primary"
}

func checkAuthInfo(ctx context.Context, cns certnames.CertNames) error {
	if cns.IsPermitAll() {
		return nil
	}

	var authInfo credentials.AuthInfo
	if p, ok := peer.FromContext(ctx); ok && p != nil && p.AuthInfo != nil {
		authInfo = p.AuthInfo
	} else if ri, ok := credentials.RequestInfoFromContext(ctx); ok && ri.AuthInfo != nil {
		authInfo = ri.AuthInfo
	} else {
		return status.Error(codes.Unauthenticated, "client is not authenticated")
	}

	tlsInfo, ok := authInfo.(credentials.TLSInfo)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "expected credentials.TLSInfo, got %T", authInfo)
	}
	if len(tlsInfo.State.VerifiedChains) == 0 {
		return status.Error(codes.Unauthenticated, "TLSInfo.State.VerifiedChains[] has length 0")
	}
	if len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return status.Error(codes.Unauthenticated, "TLSInfo.State.VerifiedChains[0][] has length 0")
	}

	cert := tlsInfo.State.VerifiedChains[0][0]
	if !cns.Check(cert) {
		return status.Errorf(codes.PermissionDenied, "client certificate rejected; requires %v", cns)
	}
	return nil
}
