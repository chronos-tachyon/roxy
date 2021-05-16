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

	"github.com/chronos-tachyon/roxy/lib/mainutil"
)

type RequestContext struct {
	Context        context.Context
	Logger         zerolog.Logger
	Subsystem      string
	LocalAddr      net.Addr
	RemoteAddr     net.Addr
	XID            xid.ID
	Impl           *Impl
	Metrics        *Metrics
	Request        *http.Request
	Body           WrappedReader
	Writer         WrappedWriter
	StartTime      time.Time
	EndTime        time.Time
	FrontendKey    string
	FrontendConfig *FrontendConfig
}

func (rc *RequestContext) RoxyFrontend() string {
	return fmt.Sprintf("%s; %v", rc.FrontendKey, rc.FrontendConfig.Client.Target)
}

func GetRequestContext(ctx context.Context) *RequestContext {
	return ctx.Value(mainutil.RequestContextKey).(*RequestContext)
}

func WithRequestContext(ctx context.Context, rc *RequestContext) context.Context {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if rc == nil {
		panic(errors.New("rc is nil"))
	}
	return context.WithValue(ctx, mainutil.RequestContextKey, rc)
}
