package mainutil

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type (
	healthCheckRequest  = grpc_health_v1.HealthCheckRequest
	healthCheckResponse = grpc_health_v1.HealthCheckResponse
)

const (
	hcrNotServing = grpc_health_v1.HealthCheckResponse_NOT_SERVING
	hcrServing    = grpc_health_v1.HealthCheckResponse_SERVING
)

type HealthServer struct {
	grpc_health_v1.UnimplementedHealthServer

	mu        sync.Mutex
	byService map[string]*serviceHealth
	stopped   bool
}

type serviceHealth struct {
	cv *sync.Cond
	ok bool
}

func (s *HealthServer) Set(subsystemName string, healthy bool) {
	s.mu.Lock()
	if s.byService == nil {
		s.byService = make(map[string]*serviceHealth, 16)
	}
	h := s.byService[subsystemName]
	if h == nil {
		h = &serviceHealth{
			cv: sync.NewCond(&s.mu),
			ok: healthy,
		}
		s.byService[subsystemName] = h
	} else {
		h.ok = healthy
		h.cv.Broadcast()
	}
	s.mu.Unlock()
}

func (s *HealthServer) Stop() {
	s.mu.Lock()
	s.stopped = true
	for _, h := range s.byService {
		h.ok = false
		h.cv.Broadcast()
	}
	s.mu.Unlock()
}

func (s *HealthServer) Check(ctx context.Context, req *healthCheckRequest) (*healthCheckResponse, error) {
	log.Logger.Debug().
		Str("rpcService", "grpc.health.v1.Health").
		Str("rpcMethod", "Check").
		Msg("RPC")

	s.mu.Lock()
	defer s.mu.Unlock()

	h := s.byService[req.Service]
	if h == nil {
		return nil, status.Errorf(codes.NotFound, "unknown subsystem %q", req.Service)
	}
	status := hcrNotServing
	if h.ok {
		status = hcrServing
	}
	return &healthCheckResponse{Status: status}, nil
}

func (s *HealthServer) Watch(req *healthCheckRequest, ws grpc_health_v1.Health_WatchServer) error {
	log.Logger.Debug().
		Str("rpcService", "grpc.health.v1.Health").
		Str("rpcMethod", "Watch").
		Msg("RPC")

	s.mu.Lock()
	h := s.byService[req.Service]
	if h == nil {
		s.mu.Unlock()
		return status.Errorf(codes.NotFound, "unknown subsystem %q", req.Service)
	}
	ok := h.ok
	stopped := s.stopped
	s.mu.Unlock()

	status := hcrNotServing
	if ok {
		status = hcrServing
	}
	if err := ws.Send(&healthCheckResponse{Status: status}); err != nil {
		return err
	}

	for !stopped {
		s.mu.Lock()
		for !stopped && ok == h.ok {
			h.cv.Wait()
		}
		didChange := (ok != h.ok)
		ok = h.ok
		stopped = s.stopped
		s.mu.Unlock()

		if didChange {
			status := hcrNotServing
			if ok {
				status = hcrServing
			}
			if err := ws.Send(&healthCheckResponse{Status: status}); err != nil {
				return err
			}
		}
	}
	return nil
}
