package baseresolver

import (
	"errors"
	"math/rand"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/syncrand"
)

type StaticResolverOptions struct {
	Random            *rand.Rand
	Balancer          BalancerType
	Records           []*AddrData
	ClientConn        resolver.ClientConn
	ServiceConfigJSON string
}

func NewStaticResolver(opts StaticResolverOptions) (*StaticResolver, error) {
	rng := opts.Random
	if rng == nil {
		rng = syncrand.Global()
	}

	resolved := opts.Records
	byAddrKey := make(map[string][]*AddrData, len(resolved))
	for _, data := range resolved {
		if data.Addr != nil {
			addrKey := data.Addr.String()
			byAddrKey[addrKey] = append(byAddrKey[addrKey], data)
		}
	}
	perm := rng.Perm(len(resolved))

	if opts.ClientConn != nil {
		var state resolver.State
		state.Addresses = make([]resolver.Address, 0, len(resolved))
		for _, data := range resolved {
			if data.Addr != nil {
				state.Addresses = append(state.Addresses, data.Address)
			}
		}

		if opts.ServiceConfigJSON != "" {
			state.ServiceConfig = opts.ClientConn.ParseServiceConfig(opts.ServiceConfigJSON)
		}

		if len(state.Addresses) == 0 {
			opts.ClientConn.ReportError(ErrNoHealthyBackends)
		} else {
			opts.ClientConn.UpdateState(state)
		}
	}

	res := &StaticResolver{
		rng:       rng,
		balancer:  opts.Balancer,
		byAddrKey: byAddrKey,
		resolved:  resolved,
		perm:      perm,
	}
	return res, nil
}

type StaticResolver struct {
	rng       *rand.Rand
	balancer  BalancerType
	byAddrKey map[string][]*AddrData
	resolved  []*AddrData
	perm      []int

	mu     sync.Mutex
	nextRR uint
}

func (res *StaticResolver) Err() error {
	return nil
}

func (res *StaticResolver) ResolveAll() ([]*AddrData, error) {
	return res.resolved, nil
}

func (res *StaticResolver) Resolve() (*AddrData, error) {
	if len(res.resolved) == 0 {
		return nil, ErrNoHealthyBackends
	}

	return balanceImpl(res.balancer, multierror.Error{}, res.resolved, res.rng, res.perm, &res.mu, &res.nextRR)
}

func (res *StaticResolver) Update(opts UpdateOptions) {
	addrKey := opts.Addr.String()
	for _, data := range res.byAddrKey[addrKey] {
		data.Update(opts)
	}
}

func (res *StaticResolver) Watch(fn WatchFunc) WatchID {
	if fn == nil {
		panic(errors.New("WatchFunc is nil"))
	}

	if len(res.resolved) == 0 {
		return 0
	}

	events := make([]*Event, 0, len(res.resolved))
	for _, data := range res.resolved {
		ev := &Event{
			Type: UpdateEvent,
			Key:  data.Key(),
			Data: data,
		}
		events = append(events, ev)
	}
	fn(events)
	return 0
}

func (res *StaticResolver) CancelWatch(id WatchID) {
	// pass
}

func (res *StaticResolver) ResolveNow(opts resolver.ResolveNowOptions) {
	// pass
}

func (res *StaticResolver) Close() {
	// pass
}

var _ Resolver = (*StaticResolver)(nil)
