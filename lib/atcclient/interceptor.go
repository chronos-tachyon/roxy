package atcclient

import (
	"context"
	"fmt"
	"net/http"
	"path"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type InterceptorFactory struct {
	DefaultCostPerQuery    uint32
	DefaultCostPerRequest  uint32
	DefaultCostPerResponse uint32
	ByFullMethod           map[string]InterceptorPerMethod
}

type InterceptorPerMethod struct {
	CostPerQuery    uint32
	CostPerRequest  uint32
	CostPerResponse uint32
	AdjustFunc      InterceptorAdjustFunc
}

type InterceptorAdjustFunc func(oldCost uint32, request interface{}) uint32

func (factory InterceptorFactory) Cost(method string) (costPerQuery, costPerReq, costPerResp uint32, adjustFn InterceptorAdjustFunc) {
	costPerQuery = factory.DefaultCostPerQuery
	costPerReq = factory.DefaultCostPerRequest
	costPerResp = factory.DefaultCostPerResponse
	if perMethod, ok := factory.ByFullMethod[method]; ok {
		costPerQuery = perMethod.CostPerQuery
		costPerReq = perMethod.CostPerRequest
		costPerResp = perMethod.CostPerResponse
		adjustFn = perMethod.AdjustFunc
	}
	return
}

func (factory InterceptorFactory) DialOptions(more ...grpc.DialOption) []grpc.DialOption {
	out := make([]grpc.DialOption, 2+len(more))
	out[0] = grpc.WithUnaryInterceptor(factory.UnaryClientInterceptor())
	out[1] = grpc.WithStreamInterceptor(factory.StreamClientInterceptor())
	copy(out[2:], more)
	return out
}

func (factory InterceptorFactory) ServerOptions(more ...grpc.ServerOption) []grpc.ServerOption {
	out := make([]grpc.ServerOption, 2+len(more))
	out[0] = grpc.UnaryInterceptor(factory.UnaryServerInterceptor())
	out[1] = grpc.StreamInterceptor(factory.StreamServerInterceptor())
	copy(out[2:], more)
	return out
}

func (factory InterceptorFactory) RoundTripper(inner http.RoundTripper) http.RoundTripper {
	return interceptorRoundTripper{factory, inner}
}

func (factory InterceptorFactory) Handler(inner http.Handler) http.Handler {
	return interceptorHandler{factory, inner}
}

func (factory InterceptorFactory) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req interface{},
		resp interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		costPerQuery, _, _, adjustFn := factory.Cost(method)
		if adjustFn != nil {
			costPerQuery = adjustFn(costPerQuery, req)
		}
		Spend(uint(costPerQuery))
		log.Logger.Trace().
			Str("method", method).
			Uint32("cost", costPerQuery).
			Msg("UnaryClientInterceptor")
		return invoker(ctx, method, req, resp, cc, opts...)
	}
}

func (factory InterceptorFactory) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		costPerQuery, costPerReq, costPerResp, adjustFn := factory.Cost(method)
		Spend(uint(costPerQuery))
		log.Logger.Trace().
			Str("method", method).
			Str("event", "Call").
			Uint32("cost", costPerQuery).
			Msg("StreamClientInterceptor")
		inner, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}
		wrapped := interceptorClientStream{factory, inner, method, costPerReq, costPerResp, adjustFn}
		return wrapped, nil
	}
}

func (factory InterceptorFactory) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		costPerQuery, _, _, adjustFn := factory.Cost(info.FullMethod)
		if adjustFn != nil {
			costPerQuery = adjustFn(costPerQuery, req)
		}
		Spend(uint(costPerQuery))
		log.Logger.Trace().
			Str("method", info.FullMethod).
			Uint32("cost", costPerQuery).
			Msg("UnaryServerInterceptor")
		return handler(ctx, req)
	}
}

func (factory InterceptorFactory) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		costPerQuery, costPerReq, costPerResp, adjustFn := factory.Cost(info.FullMethod)
		Spend(uint(costPerQuery))
		log.Logger.Trace().
			Str("method", info.FullMethod).
			Str("event", "Call").
			Uint32("cost", costPerQuery).
			Msg("StreamServerInterceptor")
		wrapped := interceptorServerStream{factory, ss, info.FullMethod, costPerReq, costPerResp, adjustFn}
		return handler(srv, wrapped)
	}
}

// type interceptorClientStream {{{

type interceptorClientStream struct {
	factory     InterceptorFactory
	inner       grpc.ClientStream
	method      string
	costPerReq  uint32
	costPerResp uint32
	adjustFn    InterceptorAdjustFunc
}

func (ics interceptorClientStream) Context() context.Context {
	return ics.inner.Context()
}

func (ics interceptorClientStream) Header() (metadata.MD, error) {
	return ics.inner.Header()
}

func (ics interceptorClientStream) Trailer() metadata.MD {
	return ics.inner.Trailer()
}

func (ics interceptorClientStream) SendMsg(m interface{}) error {
	costPerReq := ics.costPerReq
	if ics.adjustFn != nil {
		costPerReq = ics.adjustFn(costPerReq, m)
	}
	Spend(uint(costPerReq))
	log.Logger.Trace().
		Str("method", ics.method).
		Str("event", "Send").
		Uint32("cost", costPerReq).
		Msg("StreamClientInterceptor")
	return ics.inner.SendMsg(m)
}

func (ics interceptorClientStream) RecvMsg(m interface{}) error {
	costPerResp := ics.costPerResp
	Spend(uint(costPerResp))
	log.Logger.Trace().
		Str("method", ics.method).
		Str("event", "Recv").
		Uint32("cost", costPerResp).
		Msg("StreamClientInterceptor")
	return ics.inner.RecvMsg(m)
}

func (ics interceptorClientStream) CloseSend() error {
	return ics.inner.CloseSend()
}

var _ grpc.ClientStream = interceptorClientStream{}

// }}}

// type interceptorServerStream {{{

type interceptorServerStream struct {
	factory     InterceptorFactory
	inner       grpc.ServerStream
	method      string
	costPerReq  uint32
	costPerResp uint32
	adjustFn    InterceptorAdjustFunc
}

func (iss interceptorServerStream) Context() context.Context {
	return iss.inner.Context()
}

func (iss interceptorServerStream) SetHeader(md metadata.MD) error {
	return iss.inner.SetHeader(md)
}

func (iss interceptorServerStream) SendHeader(md metadata.MD) error {
	return iss.inner.SendHeader(md)
}

func (iss interceptorServerStream) SetTrailer(md metadata.MD) {
	iss.inner.SetTrailer(md)
}

func (iss interceptorServerStream) RecvMsg(m interface{}) error {
	costPerReq := iss.costPerReq
	if iss.adjustFn != nil {
		costPerReq = iss.adjustFn(costPerReq, m)
	}
	Spend(uint(costPerReq))
	log.Logger.Trace().
		Str("method", iss.method).
		Str("event", "Recv").
		Uint32("cost", costPerReq).
		Msg("StreamServerInterceptor")
	return iss.inner.RecvMsg(m)
}

func (iss interceptorServerStream) SendMsg(m interface{}) error {
	costPerResp := iss.costPerResp
	Spend(uint(costPerResp))
	log.Logger.Trace().
		Str("method", iss.method).
		Str("event", "Send").
		Uint32("cost", costPerResp).
		Msg("StreamServerInterceptor")
	return iss.inner.SendMsg(m)
}

var _ grpc.ServerStream = interceptorServerStream{}

// }}}

// type interceptorRoundTripper {{{

type interceptorRoundTripper struct {
	factory InterceptorFactory
	inner   http.RoundTripper
}

func (t interceptorRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	method := fmt.Sprint(path.Clean(r.URL.Path), "@", r.Method)
	costPerQuery, _, _, _ := t.factory.Cost(method)
	Spend(uint(costPerQuery))
	log.Logger.Trace().
		Str("method", method).
		Uint32("cost", costPerQuery).
		Msg("http.RoundTripper Interceptor")
	return t.inner.RoundTrip(r)
}

var _ http.RoundTripper = interceptorRoundTripper{}

// }}}

// type interceptorHandler {{{

type interceptorHandler struct {
	factory InterceptorFactory
	inner   http.Handler
}

func (h interceptorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := fmt.Sprint(path.Clean(r.URL.Path), "@", r.Method)
	costPerQuery, _, _, _ := h.factory.Cost(method)
	Spend(uint(costPerQuery))
	log.Logger.Trace().
		Str("method", method).
		Uint32("cost", costPerQuery).
		Msg("http.Handler Interceptor")
	h.inner.ServeHTTP(w, r)
}

var _ http.Handler = interceptorHandler{}

// }}}
