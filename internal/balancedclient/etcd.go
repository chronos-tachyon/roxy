package balancedclient

import (
	"fmt"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
)

func NewEtcdResolver(opts Options) (baseresolver.Resolver, error) {
	if opts.Etcd == nil {
		panic(fmt.Errorf("Etcd is nil"))
	}

	etcdPrefix, etcdPort, query, err := baseresolver.ParseEtcdTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	var balancer baseresolver.BalancerType
	if str := query.Get("balancer"); str != "" {
		if err = balancer.Parse(str); err != nil {
			return nil, fmt.Errorf("failed to parse balancer=%q query string: %w", str, err)
		}
	}

	return baseresolver.NewWatchingResolver(baseresolver.WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: baseresolver.MakeEtcdResolveFunc(opts.Etcd, etcdPrefix, etcdPort),
	})
}
