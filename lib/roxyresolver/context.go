package roxyresolver

import (
	"context"
)

type contextKey string

const (
	ShardIDContextKey = contextKey("roxy.ShardID")
)

func WithShardID(ctx context.Context, shardID int32) context.Context {
	return context.WithValue(ctx, ShardIDContextKey, shardID)
}

func GetShardID(ctx context.Context) (shardID int32, ok bool) {
	shardID, ok = ctx.Value(ShardIDContextKey).(int32)
	return
}
