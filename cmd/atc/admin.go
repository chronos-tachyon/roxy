package main

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/roxypb"
)

type AdminServer struct {
	roxypb.UnimplementedAdminServer
}

func (AdminServer) Ping(ctx context.Context, req *roxypb.PingRequest) (*roxypb.PingResponse, error) {
	log.Logger.Info().
		Str("rpcService", "roxy.Admin").
		Str("rpcMethod", "Ping").
		Str("rpcInterface", "admin").
		Msg("RPC")

	return &roxypb.PingResponse{}, nil
}

func (AdminServer) Reload(ctx context.Context, req *roxypb.ReloadRequest) (*roxypb.ReloadResponse, error) {
	log.Logger.Info().
		Str("rpcService", "roxy.Admin").
		Str("rpcMethod", "Reload").
		Str("rpcInterface", "admin").
		Msg("RPC")

	if err := gMultiServer.Reload(); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &roxypb.ReloadResponse{}, nil
}

func (AdminServer) Shutdown(ctx context.Context, req *roxypb.ShutdownRequest) (*roxypb.ShutdownResponse, error) {
	log.Logger.Info().
		Str("rpcService", "roxy.Admin").
		Str("rpcMethod", "Shutdown").
		Str("rpcInterface", "admin").
		Msg("RPC")

	if err := gMultiServer.Shutdown(true); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &roxypb.ShutdownResponse{}, nil
}
