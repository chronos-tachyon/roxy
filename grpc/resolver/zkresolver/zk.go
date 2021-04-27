package zkresolver

import (
	"context"
	"errors"
	"math/rand"

	"github.com/go-zookeeper/zk"
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
)

func NewBuilder(ctx context.Context, rng *rand.Rand, zkconn *zk.Conn, serviceConfigJSON string) resolver.Builder {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if zkconn == nil {
		panic(errors.New("zkconn is nil"))
	}
	return myBuilder{ctx, rng, zkconn, serviceConfigJSON}
}

type myBuilder struct {
	ctx               context.Context
	rng               *rand.Rand
	zkconn            *zk.Conn
	serviceConfigJSON string
}

func (b myBuilder) Scheme() string {
	return "zk"
}

func (b myBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	zkPath, zkPort, query, err := baseresolver.ParseZKTarget(target)
	if err != nil {
		return nil, err
	}

	_ = query // for future use

	return baseresolver.NewWatchingResolver(baseresolver.WatchingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		ResolveFunc:       baseresolver.MakeZKResolveFunc(b.zkconn, zkPath, zkPort),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}
