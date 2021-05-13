package roxyresolver

import (
	"context"

	"github.com/go-zookeeper/zk"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/lib/atcclient"
)

type contextKey string

const (
	ShardIDContextKey   = contextKey("roxy.ShardID")
	ZKConnContextKey    = contextKey("roxy.zk.Conn")
	V3ClientContextKey  = contextKey("roxy.etcd.V3Client")
	ATCClientContextKey = contextKey("roxy.atc.Client")
)

func WithShardID(ctx context.Context, shardID int32) context.Context {
	return context.WithValue(ctx, ShardIDContextKey, shardID)
}

func WithZKConn(ctx context.Context, zkconn *zk.Conn) context.Context {
	return context.WithValue(ctx, ZKConnContextKey, zkconn)
}

func WithEtcdV3Client(ctx context.Context, etcd *v3.Client) context.Context {
	return context.WithValue(ctx, V3ClientContextKey, etcd)
}

func WithATCClient(ctx context.Context, client *atcclient.ATCClient) context.Context {
	return context.WithValue(ctx, ATCClientContextKey, client)
}

func GetShardID(ctx context.Context) (shardID int32, ok bool) {
	shardID, ok = ctx.Value(ShardIDContextKey).(int32)
	return
}

func GetZKConn(ctx context.Context) *zk.Conn {
	zkconn, _ := ctx.Value(ZKConnContextKey).(*zk.Conn)
	return zkconn
}

func GetEtcdV3Client(ctx context.Context) *v3.Client {
	etcd, _ := ctx.Value(V3ClientContextKey).(*v3.Client)
	return etcd
}

func GetATCClient(ctx context.Context) *atcclient.ATCClient {
	cc, _ := ctx.Value(ATCClientContextKey).(*atcclient.ATCClient)
	return cc
}