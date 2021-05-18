package main

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type AdminServer struct {
	roxy_v0.UnimplementedAdminServer
}

func (AdminServer) Ping(ctx context.Context, req *roxy_v0.PingRequest) (*roxy_v0.PingResponse, error) {
	log.Logger.Info().
		Str("rpcService", "roxy.v0.Admin").
		Str("rpcMethod", "Ping").
		Str("rpcInterface", "admin").
		Msg("RPC")

	return &roxy_v0.PingResponse{}, nil
}

func (AdminServer) Reload(ctx context.Context, req *roxy_v0.ReloadRequest) (*roxy_v0.ReloadResponse, error) {
	log.Logger.Info().
		Str("rpcService", "roxy.v0.Admin").
		Str("rpcMethod", "Reload").
		Str("rpcInterface", "admin").
		Msg("RPC")

	if err := gMultiServer.Reload(); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &roxy_v0.ReloadResponse{}, nil
}

func (AdminServer) Shutdown(ctx context.Context, req *roxy_v0.ShutdownRequest) (*roxy_v0.ShutdownResponse, error) {
	log.Logger.Info().
		Str("rpcService", "roxy.v0.Admin").
		Str("rpcMethod", "Shutdown").
		Str("rpcInterface", "admin").
		Msg("RPC")

	if err := gMultiServer.Shutdown(true); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &roxy_v0.ShutdownResponse{}, nil
}

func (AdminServer) SetHealth(ctx context.Context, req *roxy_v0.SetHealthRequest) (*roxy_v0.SetHealthResponse, error) {
	log.Logger.Info().
		Str("rpcService", "roxy.v0.Admin").
		Str("rpcMethod", "SetHealth").
		Str("rpcInterface", "admin").
		Str("subsystem", req.SubsystemName).
		Bool("healthy", req.IsHealthy).
		Msg("RPC")

	gHealthServer.Set(req.SubsystemName, req.IsHealthy)
	return &roxy_v0.SetHealthResponse{}, nil
}
