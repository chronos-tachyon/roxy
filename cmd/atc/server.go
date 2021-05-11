package main

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/roxypb"
)

type ATCServer struct {
	roxypb.UnimplementedAirTrafficControlServer
	ref   *Ref
	admin bool
}

func (s *ATCServer) Lookup(ctx context.Context, req *roxypb.LookupRequest) (*roxypb.LookupResponse, error) {
	log.Logger.Debug().
		Str("rpcService", "roxy.AirTrafficControl").
		Str("rpcMethod", "Lookup").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Msg("RPC")

	impl := s.ref.Get()
	sm := impl.ServiceMap()
	svc := sm.Get(ServiceName(req.ServiceName))
	if svc == nil {
		return nil, status.Errorf(codes.NotFound, "service name %q not found", req.ServiceName)
	}
	resp := &roxypb.LookupResponse{
		IsSharded:                  svc.IsSharded,
		NumShards:                  svc.NumShards,
		MaxLoadPerServer:           svc.MaxLoadPerServer,
		ExpectedNumClientsPerShard: svc.ExpectedNumClientsPerShard,
		ExpectedNumServersPerShard: svc.ExpectedNumServersPerShard,
		AllowedClientCommonNames:   makeCommonNameList(svc.AllowedClientCommonNames()),
		AllowedServerCommonNames:   makeCommonNameList(svc.AllowedServerCommonNames()),
	}
	return resp, nil
}

func (s *ATCServer) Find(ctx context.Context, req *roxypb.FindRequest) (*roxypb.FindResponse, error) {
	log.Logger.Debug().
		Str("rpcService", "roxy.AirTrafficControl").
		Str("rpcMethod", "Find").
		Str("rpcInterface", rpcInterfaceName(s.admin)).
		Msg("RPC")

	key := SplitKey{
		ServiceName: ServiceName(req.ServiceName),
		ShardID:     ShardID(req.ShardId),
	}
	impl := s.ref.Get()
	peer, ok := impl.PeerBySplitKey(key)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "key %#v not found", key)
	}
	resp := &roxypb.FindResponse{
		GoAway: peer.GoAway(),
	}
	return resp, nil
}

func (s *ATCServer) ServerAnnounce(rs roxypb.AirTrafficControl_ServerAnnounceServer) error {
	if s.admin {
		return status.Errorf(codes.PermissionDenied, "method ServerAnnounce not permitted over Admin interface")
	}

	log.Logger.Debug().
		Str("rpcService", "roxy.AirTrafficControl").
		Str("rpcMethod", "ServerAnnounce").
		Str("rpcInterface", "primary").
		Msg("RPC")

	return status.Errorf(codes.Unimplemented, "method ServerAnnounce not implemented")
}

func (s *ATCServer) ClientAssign(bs roxypb.AirTrafficControl_ClientAssignServer) error {
	if s.admin {
		return status.Errorf(codes.PermissionDenied, "method ClientAssign not permitted over Admin interface")
	}

	log.Logger.Debug().
		Str("rpcService", "roxy.AirTrafficControl").
		Str("rpcMethod", "ClientAssign").
		Str("rpcInterface", "primary").
		Msg("RPC")

	return status.Errorf(codes.Unimplemented, "method ClientAssign not implemented")
}

func makeCommonNameList(list []string) *roxypb.CommonNameList {
	if list == nil {
		return nil
	}
	return &roxypb.CommonNameList{CommonName: list}
}

func rpcInterfaceName(isAdmin bool) string {
	if isAdmin {
		return "admin"
	}
	return "primary"
}
