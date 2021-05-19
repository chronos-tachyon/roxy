package roxyresolver

import (
	"errors"
	"math/rand"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

// StaticResolverOptions holds options related to constructing a new StaticResolver.
type StaticResolverOptions struct {
	// Records lists the Resolved addresses for this resolver to return.
	Records []Resolved

	// Random is the source of randomness for balancer algorithms that need
	// one.
	//
	// If provided, it MUST be thread-safe; see the syncrand package for
	// more information.  If nil, the syncrand.Global() instance will be
	// used.
	Random *rand.Rand

	// Balancer selects which load balancer algorithm to use.
	Balancer BalancerType

	// ClientConn is a gRPC ClientConn that will receive state updates.
	ClientConn resolver.ClientConn

	// ServiceConfigJSON is the gRPC Service Config which will be provided
	// to ClientConn on each state update.
	ServiceConfigJSON string
}

// NewStaticResolver constructs a new StaticResolver.
func NewStaticResolver(opts StaticResolverOptions) (*StaticResolver, error) {
	rng := opts.Random
	if rng == nil {
		rng = syncrand.Global()
	}

	resolved := make([]Resolved, len(opts.Records))
	copy(resolved, opts.Records)

	byAddr := make(map[string]*Dynamic, len(resolved))
	for index, data := range resolved {
		if data.Addr != nil {
			addr := data.Addr.String()
			dynamic, found := byAddr[addr]
			if !found {
				dynamic = new(Dynamic)
				byAddr[addr] = dynamic
			}
			data.Dynamic = dynamic
			resolved[index] = data
		}
		data.Check()
	}
	perm := computePermImpl(opts.Balancer, resolved, rng)

	if opts.ClientConn != nil {
		if len(resolved) == 0 {
			opts.ClientConn.ReportError(roxyutil.ErrNoHealthyBackends)
		} else {
			var state resolver.State
			state.Addresses = makeAddressList(resolved)
			if opts.ServiceConfigJSON != "" {
				state.ServiceConfig = opts.ClientConn.ParseServiceConfig(opts.ServiceConfigJSON)
			}
			opts.ClientConn.UpdateState(state)
		}
	}

	res := &StaticResolver{
		rng:      rng,
		balancer: opts.Balancer,
		byAddr:   byAddr,
		resolved: resolved,
		perm:     perm,
	}
	return res, nil
}

// StaticResolver is an implementation of the Resolver interface that returns
// the same static records every time.
type StaticResolver struct {
	rng      *rand.Rand
	balancer BalancerType
	byAddr   map[string]*Dynamic
	resolved []Resolved
	perm     []int
	nextRR   uint32
}

// Err returns any errors encountered since the last call to Err or ResolveAll.
func (res *StaticResolver) Err() error {
	return nil
}

// ResolveAll returns all resolved addresses, plus any errors encountered since
// the last call to Err or ResolveAll.
func (res *StaticResolver) ResolveAll() ([]Resolved, error) {
	return res.resolved, nil
}

// Resolve returns the resolved address of a healthy backend, if one is
// available, or else returns the error that prevented it from doing so.
func (res *StaticResolver) Resolve() (Resolved, error) {
	if len(res.resolved) == 0 {
		return Resolved{}, roxyutil.ErrNoHealthyBackends
	}

	return balanceImpl(res.balancer, nil, res.resolved, res.rng, res.perm, &res.nextRR)
}

// Update changes the status of a server.
func (res *StaticResolver) Update(opts UpdateOptions) {
	addr := opts.Addr.String()
	dynamic := res.byAddr[addr]
	if dynamic != nil {
		dynamic.Update(opts)
	}
}

// Watch registers a WatchFunc.  The WatchFunc will be called immediately with
// synthetic events for each resolved address currently known, plus it will be
// called whenever the Resolver's state changes.
func (res *StaticResolver) Watch(fn WatchFunc) WatchID {
	if fn == nil {
		panic(errors.New("WatchFunc is nil"))
	}

	if len(res.resolved) == 0 {
		return 0
	}

	events := make([]Event, 0, len(res.resolved))
	for _, data := range res.resolved {
		ev := Event{
			Type: UpdateEvent,
			Key:  data.Unique,
			Data: data,
		}
		ev.Check()
		events = append(events, ev)
	}
	fn(events)
	return 0
}

// CancelWatch cancels a previous call to Watch.
func (res *StaticResolver) CancelWatch(id WatchID) {
	// pass
}

// ResolveNow is a no-op.
func (res *StaticResolver) ResolveNow(opts resolver.ResolveNowOptions) {
	// pass
}

// Close stops the resolver and frees all resources.
func (res *StaticResolver) Close() {
	// pass
}

var _ Resolver = (*StaticResolver)(nil)
