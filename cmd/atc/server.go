package main

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/certnames"
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
		Msg("RPC")

	impl := s.ref.Get()
	sm := impl.ServiceMap()
	svc := sm.Get(ServiceName(req.ServiceName))
	if svc == nil {
		return nil, status.Errorf(codes.NotFound, "service name %q not found", req.ServiceName)
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
	return resp, nil
}

func (s *ATCServer) Find(ctx context.Context, req *roxy_v0.FindRequest) (*roxy_v0.FindResponse, error) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
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
	resp := &roxy_v0.FindResponse{
		GoAway: peer.GoAway(),
	}
	return resp, nil
}

func (s *ATCServer) ServerAnnounce(sas roxy_v0.AirTrafficControl_ServerAnnounceServer) error {
	if s.admin {
		return status.Error(codes.PermissionDenied, "method ServerAnnounce not permitted over Admin interface")
	}

	impl := s.ref.Get()

	req, err := sas.Recv()
	if err != nil {
		return err
	}

	if req.First == nil {
		return status.Error(codes.InvalidArgument, "first is absent")
	}

	first := req.First
	serviceName := ServiceName(first.ServiceName)

	svc := impl.ServiceMap().Get(serviceName)
	if svc == nil {
		return status.Errorf(codes.NotFound, "unknown service name %q", first.ServiceName)
	}
	if svc.IsSharded && !first.HasShardId {
		return status.Error(codes.InvalidArgument, "service is sharded, but no shard_id was provided")
	}
	if !svc.IsSharded && first.HasShardId {
		return status.Error(codes.InvalidArgument, "service is not sharded, but a shard_id was provided")
	}
	if svc.IsSharded && first.HasShardId && first.ShardId >= svc.NumShards {
		return status.Errorf(codes.NotFound, "shard_id %d was not found (range 0..%d)", first.ShardId, svc.NumShards-1)
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

	err = checkPeer(sas.Context(), svc.AllowedServerNames)
	if err != nil {
		return err
	}

	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "ServerAnnounce").
		Str("rpcInterface", "primary").
		Msg("RPC")

	return status.Errorf(codes.Unimplemented, "method ServerAnnounce not implemented")
}

func (s *ATCServer) ClientAssign(cas roxy_v0.AirTrafficControl_ClientAssignServer) error {
	if s.admin {
		return status.Error(codes.PermissionDenied, "method ClientAssign not permitted over Admin interface")
	}

	impl := s.ref.Get()

	req, err := cas.Recv()
	if err != nil {
		return err
	}

	if req.First == nil {
		return status.Error(codes.InvalidArgument, "first is absent")
	}

	first := req.First
	serviceName := ServiceName(first.ServiceName)

	svc := impl.ServiceMap().Get(serviceName)
	if svc == nil {
		return status.Errorf(codes.NotFound, "unknown service name %q", first.ServiceName)
	}
	if svc.IsSharded && !first.HasShardId {
		return status.Error(codes.InvalidArgument, "service is sharded, but no shard_id was provided")
	}
	if !svc.IsSharded && first.HasShardId {
		return status.Error(codes.InvalidArgument, "service is not sharded, but a shard_id was provided")
	}
	if svc.IsSharded && first.HasShardId && first.ShardId >= svc.NumShards {
		return status.Errorf(codes.NotFound, "shard_id %d was not found (range 0..%d)", first.ShardId, svc.NumShards-1)
	}

	err = roxyutil.ValidateATCLocation(first.Location)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	err = roxyutil.ValidateATCUnique(first.Unique)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	err = checkPeer(cas.Context(), svc.AllowedClientNames)
	if err != nil {
		return err
	}

	log.Logger.Debug().
		Str("rpcService", "roxy.v0.AirTrafficControl").
		Str("rpcMethod", "ClientAssign").
		Str("rpcInterface", "primary").
		Msg("RPC")

	return status.Errorf(codes.Unimplemented, "method ClientAssign not implemented")
}

func rpcInterfaceName(isAdmin bool) string {
	if isAdmin {
		return "admin"
	}
	return "primary"
}

func checkPeer(ctx context.Context, cns certnames.CertNames) error {
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
