package roxyresolver

import (
	"errors"
	"math/rand"

	multierror "github.com/hashicorp/go-multierror"
	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

type StaticResolverOptions struct {
	Random            *rand.Rand
	Balancer          BalancerType
	Records           []*Resolved
	ClientConn        grpcresolver.ClientConn
	ServiceConfigJSON string
}

func NewStaticResolver(opts StaticResolverOptions) (*StaticResolver, error) {
	rng := opts.Random
	if rng == nil {
		rng = syncrand.Global()
	}

	resolved := opts.Records
	byAddrKey := make(map[string][]*Resolved, len(resolved))
	for _, data := range resolved {
		data.Check()
		if data.Addr != nil {
			addrKey := data.Addr.String()
			byAddrKey[addrKey] = append(byAddrKey[addrKey], data)
		}
	}
	perm := computePermImpl(opts.Balancer, resolved, rng)

	if opts.ClientConn != nil {
		if len(resolved) == 0 {
			opts.ClientConn.ReportError(ErrNoHealthyBackends)
		} else {
			var state grpcresolver.State
			state.Addresses = makeAddressList(resolved)
			if opts.ServiceConfigJSON != "" {
				state.ServiceConfig = opts.ClientConn.ParseServiceConfig(opts.ServiceConfigJSON)
			}
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
	byAddrKey map[string][]*Resolved
	resolved  []*Resolved
	perm      []int
	nextRR    uint32
}

func (res *StaticResolver) Err() error {
	return nil
}

func (res *StaticResolver) ResolveAll() ([]*Resolved, error) {
	return res.resolved, nil
}

func (res *StaticResolver) Resolve() (*Resolved, error) {
	if len(res.resolved) == 0 {
		return nil, ErrNoHealthyBackends
	}

	return balanceImpl(res.balancer, multierror.Error{}, res.resolved, res.rng, res.perm, &res.nextRR)
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
			Key:  data.Unique,
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

func (res *StaticResolver) ResolveNow(opts ResolveNowOptions) {
	// pass
}

func (res *StaticResolver) Close() {
	// pass
}

var _ Resolver = (*StaticResolver)(nil)
