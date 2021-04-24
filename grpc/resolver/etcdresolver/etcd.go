package etcdresolver

import (
	"context"
	"errors"
	"math/rand"

	v3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
)

func NewBuilder(ctx context.Context, rng *rand.Rand, etcd *v3.Client, serviceConfigJSON string) resolver.Builder {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if etcd == nil {
		panic(errors.New("etcd is nil"))
	}
	return myBuilder{ctx, rng, etcd, serviceConfigJSON}
}

type myBuilder struct {
	ctx               context.Context
	rng               *rand.Rand
	etcd              *v3.Client
	serviceConfigJSON string
}

func (b myBuilder) Scheme() string {
	return "etcd"
}

func (b myBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	etcdPrefix, etcdPort, query, err := baseresolver.ParseEtcdTarget(target)
	if err != nil {
		return nil, err
	}

	_ = query // for future use

	return baseresolver.NewWatchingResolver(baseresolver.WatchingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		ResolveFunc:       baseresolver.MakeEtcdResolveFunc(b.etcd, etcdPrefix, etcdPort),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}
