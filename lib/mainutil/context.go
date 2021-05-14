package mainutil

import (
	"context"
	"errors"
	"net"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type contextKey string

const (
	ConnContextKey    = contextKey("roxy.ConnContext")
	RequestContextKey = contextKey("roxy.RequestContext")
)

var (
	gRootContext context.Context
	gRootCancel  context.CancelFunc
)

func InitContext() {
	gRootContext = context.Background()
	gRootContext, gRootCancel = context.WithCancel(gRootContext)
}

func RootContext() context.Context {
	return gRootContext
}

func CancelRootContext() {
	gRootCancel()
}

type ConnContext struct {
	Context    context.Context
	Logger     zerolog.Logger
	Proto      string
	LocalAddr  net.Addr
	RemoteAddr net.Addr
}

func GetConnContext(ctx context.Context) *ConnContext {
	return ctx.Value(ConnContextKey).(*ConnContext)
}

func WithConnContext(ctx context.Context, cc *ConnContext) context.Context {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if cc == nil {
		panic(errors.New("cc is nil"))
	}
	return context.WithValue(ctx, ConnContextKey, cc)
}

func MakeBaseContextFunc() func(net.Listener) context.Context {
	return func(l net.Listener) context.Context {
		return RootContext()
	}
}

func MakeConnContextFunc(name string) func(context.Context, net.Conn) context.Context {
	return func(ctx context.Context, c net.Conn) context.Context {
		cc := &ConnContext{
			Logger:     log.Logger.With().Str("server", name).Logger(),
			Proto:      name,
			LocalAddr:  c.LocalAddr(),
			RemoteAddr: c.RemoteAddr(),
		}
		ctx = WithConnContext(ctx, cc)
		ctx = cc.Logger.WithContext(ctx)
		cc.Context = ctx
		return ctx
	}
}
