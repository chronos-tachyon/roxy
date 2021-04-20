package main

import (
	"context"
	"net"
)

type implKey struct{}

func implFromCtx(ctx context.Context) *Impl {
	return ctx.Value(implKey{}).(*Impl)
}

type laddrKey struct{}

func laddrFromCtx(ctx context.Context) net.Addr {
	return ctx.Value(laddrKey{}).(net.Addr)
}

type raddrKey struct{}

func raddrFromCtx(ctx context.Context) net.Addr {
	return ctx.Value(raddrKey{}).(net.Addr)
}
