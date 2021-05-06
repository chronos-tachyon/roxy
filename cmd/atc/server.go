package main

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/roxypb"
)

type ATCServer struct {
	roxypb.UnimplementedAirTrafficControlServer
}

func (s *ATCServer) Report(rs roxypb.AirTrafficControl_ReportServer) error {
	return status.Errorf(codes.Unimplemented, "method Report not implemented")
}

func (s *ATCServer) Balance(bs roxypb.AirTrafficControl_BalanceServer) error {
	return status.Errorf(codes.Unimplemented, "method Balance not implemented")
}
