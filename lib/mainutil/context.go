package mainutil

import (
	"context"
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
