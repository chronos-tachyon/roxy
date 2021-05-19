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
	// ConnContextKey is a context.Context key for *ConnContext.
	ConnContextKey = contextKey("roxy.ConnContext")

	// RequestContextKey is a context.Context key for *RequestContext (app-specific).
	RequestContextKey = contextKey("roxy.RequestContext")
)

var (
	gRootContext context.Context
	gRootCancel  context.CancelFunc
)

// InitContext initializes the root context.
//
// The caller must ensure that CancelRootContext gets called at least once by
// the end of the program's lifecycle.
func InitContext() {
	gRootContext = context.Background()
	gRootContext, gRootCancel = context.WithCancel(gRootContext)
}

// RootContext returns the root context.
func RootContext() context.Context {
	return gRootContext
}

// CancelRootContext cancels the root context.
func CancelRootContext() {
	gRootCancel()
}

// ConnContext is data associated with each incoming net.Conn of an
// http.Server.
type ConnContext struct {
	Context    context.Context
	Logger     zerolog.Logger
	Subsystem  string
	LocalAddr  net.Addr
	RemoteAddr net.Addr
}

// WithConnContext attaches the given *ConnContext to the context.
func WithConnContext(ctx context.Context, cc *ConnContext) context.Context {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if cc == nil {
		panic(errors.New("cc is nil"))
	}
	return context.WithValue(ctx, ConnContextKey, cc)
}

// GetConnContext retrieves the associated *ConnContext.
func GetConnContext(ctx context.Context) *ConnContext {
	return ctx.Value(ConnContextKey).(*ConnContext)
}

// MakeBaseContextFunc returns a "net/http".(*Server).BaseContextFunc that
// returns the root context.
func MakeBaseContextFunc() func(net.Listener) context.Context {
	return func(l net.Listener) context.Context {
		return RootContext()
	}
}

// MakeConnContextFunc returns a "net/http".(*Server).ConnContextFunc that
// prepares a *ConnContext and calls WithConnContext.
func MakeConnContextFunc(name string) func(context.Context, net.Conn) context.Context {
	return func(ctx context.Context, c net.Conn) context.Context {
		cc := &ConnContext{
			Logger:     log.Logger.With().Str("subsystem", name).Logger(),
			Subsystem:  name,
			LocalAddr:  c.LocalAddr(),
			RemoteAddr: c.RemoteAddr(),
		}
		ctx = WithConnContext(ctx, cc)
		ctx = cc.Logger.WithContext(ctx)
		cc.Context = ctx
		return ctx
	}
}
