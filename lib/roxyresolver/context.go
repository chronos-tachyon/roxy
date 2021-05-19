package roxyresolver

import (
	"context"

	"github.com/go-zookeeper/zk"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/lib/atcclient"
)

type contextKey string

const (
	// ShardIDContextKey is the context.Context key for attaching a shard ID.
	ShardIDContextKey = contextKey("roxy.ShardID")

	// ZKConnContextKey is the context.Context key for attaching a *zk.Conn.
	ZKConnContextKey = contextKey("roxy.zk.Conn")

	// V3ClientContextKey is the context.Context key for attaching an etcd.io *v3.Client.
	V3ClientContextKey = contextKey("roxy.etcd.V3Client")

	// ATCClientContextKey is the context.Context key for attaching an *atcclient.ATCClient.
	ATCClientContextKey = contextKey("roxy.atc.Client")
)

// WithShardID attaches a shard ID to the given context.
func WithShardID(ctx context.Context, shardID int32) context.Context {
	return context.WithValue(ctx, ShardIDContextKey, shardID)
}

// WithZKConn attaches a ZooKeeper client connection to the given context.
func WithZKConn(ctx context.Context, zkconn *zk.Conn) context.Context {
	return context.WithValue(ctx, ZKConnContextKey, zkconn)
}

// WithEtcdV3Client attaches an etcd.io client to the given context.
func WithEtcdV3Client(ctx context.Context, etcd *v3.Client) context.Context {
	return context.WithValue(ctx, V3ClientContextKey, etcd)
}

// WithATCClient attaches an ATC client to the given context.
func WithATCClient(ctx context.Context, client *atcclient.ATCClient) context.Context {
	return context.WithValue(ctx, ATCClientContextKey, client)
}

// GetShardID retrieves the attached shard ID.
func GetShardID(ctx context.Context) (shardID int32, ok bool) {
	shardID, ok = ctx.Value(ShardIDContextKey).(int32)
	return
}

// GetZKConn retrieves the attached ZooKeeper client connection.
func GetZKConn(ctx context.Context) *zk.Conn {
	zkconn, _ := ctx.Value(ZKConnContextKey).(*zk.Conn)
	return zkconn
}

// GetEtcdV3Client retrieves the attached etcd.io client.
func GetEtcdV3Client(ctx context.Context) *v3.Client {
	etcd, _ := ctx.Value(V3ClientContextKey).(*v3.Client)
	return etcd
}

// GetATCClient retrieves the attached ATC client.
func GetATCClient(ctx context.Context) *atcclient.ATCClient {
	cc, _ := ctx.Value(ATCClientContextKey).(*atcclient.ATCClient)
	return cc
}
