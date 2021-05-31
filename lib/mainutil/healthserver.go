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
	resp := makeResponse(isHealthy, true)
	return resp, nil
}

func (s healthServer) Watch(
	req *grpc_health_v1.HealthCheckRequest,
	ws grpc_health_v1.Health_WatchServer,
) error {
	log.Logger.Debug().
		Str("rpcService", "grpc.health.v1.Health").
		Str("rpcMethod", "Watch").
		Msg("RPC")

	ch := make(chan *grpc_health_v1.HealthCheckResponse, 1)

	_, found := s.m.GetHealth(req.Service)
	if !found {
		ch <- makeResponse(false, false)
	}

	id := s.m.WatchHealth(func(subsystemName string, isHealthy bool, isStopped bool) {
		if subsystemName == req.Service {
			ch <- makeResponse(isHealthy, true)
			if isStopped {
				close(ch)
			}
		}
	})

	for resp := range ch {
		if err := ws.Send(resp); err != nil {
			s.m.CancelWatchHealth(id)
			close(ch)
			return err
		}
	}

	return nil
}

func makeResponse(isHealthy bool, doesExist bool) *grpc_health_v1.HealthCheckResponse {
	status := makeStatus(isHealthy, doesExist)
	return &grpc_health_v1.HealthCheckResponse{Status: status}
}

func makeStatus(isHealthy bool, doesExist bool) grpc_health_v1.HealthCheckResponse_ServingStatus {
	switch {
	case doesExist && isHealthy:
		return grpc_health_v1.HealthCheckResponse_SERVING
	case doesExist:
		return grpc_health_v1.HealthCheckResponse_NOT_SERVING
	default:
		return grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN
	}
}
