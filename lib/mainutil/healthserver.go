package mainutil

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type healthServer struct {
	grpc_health_v1.UnimplementedHealthServer

	m *MultiServer
}

func (s healthServer) Check(
	ctx context.Context,
	req *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	log.Logger.Debug().
		Str("rpcService", "grpc.health.v1.Health").
		Str("rpcMethod", "Check").
		Msg("RPC")

	isHealthy, found := s.m.GetHealth(req.Service)
	if !found {
		return nil, status.Errorf(codes.NotFound, "unknown subsystem %q", req.Service)
	}
	return makeResponse(isHealthy), nil
}

func (s healthServer) Watch(
	req *grpc_health_v1.HealthCheckRequest,
	ws grpc_health_v1.Health_WatchServer,
) error {
	log.Logger.Debug().
		Str("rpcService", "grpc.health.v1.Health").
		Str("rpcMethod", "Watch").
		Msg("RPC")

	_, found := s.m.GetHealth(req.Service)
	if !found {
		return status.Errorf(codes.NotFound, "unknown subsystem %q", req.Service)
	}

	ch := make(chan bool, 1)

	id := s.m.WatchHealth(func(subsystemName string, isHealthy bool, isStopped bool) {
		if subsystemName == req.Service {
			ch <- isHealthy
			if isStopped {
				close(ch)
			}
		}
	})

	for isHealthy := range ch {
		if err := ws.Send(makeResponse(isHealthy)); err != nil {
			s.m.CancelWatchHealth(id)
			close(ch)
			return err
		}
	}

	return nil
}

func makeResponse(isHealthy bool) *grpc_health_v1.HealthCheckResponse {
	status := grpc_health_v1.HealthCheckResponse_NOT_SERVING
	if isHealthy {
		status = grpc_health_v1.HealthCheckResponse_SERVING
	}
	return &grpc_health_v1.HealthCheckResponse{Status: status}
}
