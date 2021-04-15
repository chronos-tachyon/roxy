package main

import (
	"context"

	xid "github.com/rs/xid"
)

type reqIdKey struct{}

func idFromCtx(ctx context.Context) xid.ID {
	return ctx.Value(reqIdKey{}).(xid.ID)
}

type implKey struct{}

func implFromCtx(ctx context.Context) *Impl {
	return ctx.Value(implKey{}).(*Impl)
}
