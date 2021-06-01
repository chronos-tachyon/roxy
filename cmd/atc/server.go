package main

import (
	"context"
	"crypto/x509"

	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/certnames"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type contextKey string

const certificateDataKey contextKey = "roxy.atc.CertificateData"

type certificateData struct {
	Cert *x509.Certificate
	Err  error
}

type ATCServer struct {
	roxy_v0.UnimplementedAirTrafficControlServer
	ref   *Ref
	admin bool
}

func (s *ATCServer) rpcBegin(ctx context.Context, methodName string) (context.Context, zerolog.Logger) {
	id, idOK := hlog.IDFromCtx(ctx)
	if !idOK {
		id = xid.New()
		ctx = hlog.CtxWithID(ctx, id)
	}

	certData := extractCertificate(ctx)
	ctx = context.WithValue(ctx, certificateDataKey, certData)

	logctx := log.Logger.With()
	logctx = logctx.Str("rpcInterface", s.rpcInterfaceName())
	if certData.Cert != nil {
		logctx = logctx.Stringer("rpcPeer", certData.Cert.Subject)
	}
	logctx = logctx.Str("rpcService", "roxy.v0.AirTrafficControl")
	logctx = logctx.Str("rpcMethod", methodName)
	logctx = logctx.Stringer("rpcID", id)

	return ctx, logctx.Logger()
}

func (s *ATCServer) rpcInterfaceName() string {
	if s.admin {
		return "admin"
	}
	return "primary"
}

func extractCertificate(ctx context.Context) certificateData {
	var authInfo credentials.AuthInfo
	if p, ok := peer.FromContext(ctx); ok && p != nil && p.AuthInfo != nil {
		authInfo = p.AuthInfo
	} else if ri, ok := credentials.RequestInfoFromContext(ctx); ok && ri.AuthInfo != nil {
		authInfo = ri.AuthInfo
	} else {
		return certificateData{
			Err: status.Error(codes.Unauthenticated, "client is not authenticated"),
		}
	}

	tlsInfo, ok := authInfo.(credentials.TLSInfo)
	if !ok {
		return certificateData{
			Err: status.Errorf(codes.Unauthenticated, "expected credentials.TLSInfo, got %T", authInfo),
		}
	}
	if len(tlsInfo.State.VerifiedChains) == 0 {
		return certificateData{
			Err: status.Error(codes.Unauthenticated, "TLSInfo.State.VerifiedChains[] has length 0"),
		}
	}
	if len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return certificateData{
			Err: status.Error(codes.Unauthenticated, "TLSInfo.State.VerifiedChains[0][] has length 0"),
		}
	}

	cert := tlsInfo.State.VerifiedChains[0][0]
	return certificateData{Cert: cert}
}

func checkAuthInfo(ctx context.Context, cns certnames.CertNames) error {
	if cns.IsPermitAll() {
		return nil
	}

	certData, ok := ctx.Value(certificateDataKey).(certificateData)
	if !ok {
		return status.Error(codes.Unauthenticated, "client is not authenticated")
	}

	if certData.Err != nil {
		return certData.Err
	}

	cert := certData.Cert
	if !cns.Check(cert) {
		return status.Errorf(codes.PermissionDenied, "client certificate rejected; requires %v", cns)
	}
	return nil
}
