package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/xid"
	"github.com/rs/zerolog"
)

type ctxKey string

const (
	ccKey = ctxKey("roxy.ConnContext")
	rcKey = ctxKey("roxy.RequestContext")
)

type ConnContext struct {
	Context    context.Context
	Logger     zerolog.Logger
	Proto      string
	LocalAddr  net.Addr
	RemoteAddr net.Addr
}

func GetConnContext(ctx context.Context) *ConnContext {
	return ctx.Value(ccKey).(*ConnContext)
}

func WithConnContext(ctx context.Context, cc *ConnContext) context.Context {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if cc == nil {
		panic(errors.New("cc is nil"))
	}
	return context.WithValue(ctx, ccKey, cc)
}

type RequestContext struct {
	Context      context.Context
	Logger       zerolog.Logger
	Proto        string
	LocalAddr    net.Addr
	RemoteAddr   net.Addr
	XID          xid.ID
	Impl         *Impl
	Metrics      *Metrics
	Request      *http.Request
	Body         WrappedReader
	Writer       WrappedWriter
	StartTime    time.Time
	EndTime      time.Time
	TargetKey    string
	TargetConfig *TargetConfig
}

func (rc *RequestContext) RoxyTarget() string {
	return fmt.Sprintf("%s; %s", rc.TargetKey, rc.TargetConfig.Target)
}

func GetRequestContext(ctx context.Context) *RequestContext {
	return ctx.Value(rcKey).(*RequestContext)
}

func WithRequestContext(ctx context.Context, rc *RequestContext) context.Context {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if rc == nil {
		panic(errors.New("rc is nil"))
	}
	return context.WithValue(ctx, rcKey, rc)
}
