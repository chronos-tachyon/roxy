package roxyresolver

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
)

// WithStandardResolvers returns a gRPC DialOption that enables as many Target
// schemes as possible on the gRPC ClientConn being created.
func WithStandardResolvers(ctx context.Context) grpc.DialOption {
	resolvers := make([]resolver.Builder, 3, 6)
	resolvers[0] = NewIPBuilder(nil, "")
	resolvers[1] = NewDNSBuilder(ctx, nil, "")
	resolvers[2] = NewSRVBuilder(ctx, nil, "")
	if zkconn := GetZKConn(ctx); zkconn != nil {
		resolvers = append(resolvers, NewZKBuilder(ctx, nil, zkconn, ""))
	}
	if etcd := GetEtcdV3Client(ctx); etcd != nil {
		resolvers = append(resolvers, NewEtcdBuilder(ctx, nil, etcd, ""))
	}
	if lbcc := GetATCClient(ctx); lbcc != nil {
		resolvers = append(resolvers, NewATCBuilder(ctx, nil, lbcc))
	}
	return grpc.WithResolvers(resolvers...)
}
