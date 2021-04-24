package grpcutil

import (
	"context"

	"google.golang.org/grpc/metadata"
)

type contextKey string

const (
	shardFuncKey = contextKey("roxy.ShardFunc")
	shardIDKey   = contextKey("roxy.ShardID")
)

type ShardFunc func(md metadata.MD) (int32, bool)

func DefaultShardFunc(md metadata.MD) (int32, bool) {
	return -1, false
}

func WithShardFunc(ctx context.Context, fn ShardFunc) context.Context {
	return context.WithValue(ctx, shardFuncKey, fn)
}

func GetShardFunc(ctx context.Context) ShardFunc {
	fn, _ := ctx.Value(shardFuncKey).(ShardFunc)
	if fn == nil {
		fn = DefaultShardFunc
	}
	return fn
}

func WithShardID(ctx context.Context, shardID int32) context.Context {
	return context.WithValue(ctx, shardIDKey, shardID)
}

func GetShardID(ctx context.Context) (shardID int32, ok bool) {
	shardID, ok = ctx.Value(shardIDKey).(int32)
	if ok {
		return
	}

	md, _ := metadata.FromIncomingContext(ctx)
	shardID, ok = GetShardFunc(ctx)(md)
	if ok {
		return
	}

	shardID = -1
	return
}
