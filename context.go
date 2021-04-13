package main

import (
	"context"
)

type implKey struct{}

func implFromCtx(ctx context.Context) *Impl {
	return ctx.Value(implKey{}).(*Impl)
}
