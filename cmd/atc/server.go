package main

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/certnames"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type ATCServer struct {
	roxy_v0.UnimplementedAirTrafficControlServer
	ref   *Ref
	admin bool
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
