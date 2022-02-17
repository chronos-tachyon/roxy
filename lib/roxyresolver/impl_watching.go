package roxyresolver

import (
	"context"
	"errors"
	"io/fs"
	"math/rand"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

// WatchingResolverOptions holds options related to constructing a new WatchingResolver.
type WatchingResolverOptions struct {
	// Context is the context within which the resolver runs.  If this
	// context is cancelled or reaches its deadline, the resolver will
	// stop.
	//
	// This field is mandatory.
	Context context.Context

	// Random is the source of randomness for balancer algorithms that need
	// one.
	//
	// If provided, it MUST be thread-safe; see the syncrand package for
	// more information.  If nil, the syncrand.Global() instance will be
	// used.
	Random *rand.Rand

	// Balancer selects which load balancer algorithm to use.
	Balancer BalancerType

	// ResolveFunc will be called as needed to (re)start the event stream.
	//
	// This field is mandatory.
	ResolveFunc WatchingResolveFunc

	// ClientConn is a gRPC ClientConn that will receive state updates.
	ClientConn resolver.ClientConn

	// ServiceConfigJSON is the gRPC Service Config which will be provided
	// to ClientConn on each state update.
	ServiceConfigJSON string
}

// WatchingResolveFunc represents a closure that will be called as needed to
// start a resolver subscription.  If it spawns any goroutines, they should
// register themselves with the provided WaitGroup.  If it needs to retry any
// external I/O, it should use the provided ExpBackoff.
type WatchingResolveFunc func(ctx context.Context, wg *sync.WaitGroup, backoff expbackoff.ExpBackoff) (<-chan []Event, error)

// NewWatchingResolver constructs a new WatchingResolver.
func NewWatchingResolver(opts WatchingResolverOptions) (*WatchingResolver, error) {
	if opts.Context == nil {
		panic(errors.New("Context is nil"))
	}

	if opts.ResolveFunc == nil {
		panic(errors.New("ResolveFunc is nil"))
	}

	rng := opts.Random
	if rng == nil {
		rng = syncrand.Global()
	}

	var parsedServiceConfig *serviceconfig.ParseResult
	if opts.ClientConn != nil && opts.ServiceConfigJSON != "" {
		parsedServiceConfig = opts.ClientConn.ParseServiceConfig(opts.ServiceConfigJSON)
	}

	ctx, cancelFn := context.WithCancel(opts.Context)
	res := &WatchingResolver{
		ctx:        ctx,
		cancelFn:   cancelFn,
		rng:        rng,
		balancer:   opts.Balancer,
		backoff:    expbackoff.BuildDefault(),
		watchFn:    opts.ResolveFunc,
		cc:         opts.ClientConn,
		sc:         parsedServiceConfig,
		watches:    make(map[WatchID]WatchFunc, 1),
		byAddr:     make(map[string]*Dynamic, 16),
		byUniqueID: make(map[string]int, 16),
	}
	res.cv = sync.NewCond(&res.mu)
	go res.resolverThread()
	return res, nil
}

// WatchingResolver is an implementation of the Resolver interface that
// subscribes to an event-oriented resolution source.
type WatchingResolver struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	rng      *rand.Rand
	balancer BalancerType
	backoff  expbackoff.ExpBackoff
	watchFn  WatchingResolveFunc
	cc       resolver.ClientConn
	sc       *serviceconfig.ParseResult
	nextRR   uint32

	mu         sync.Mutex
	cv         *sync.Cond
	watches    map[WatchID]WatchFunc
	byAddr     map[string]*Dynamic
	byUniqueID map[string]int
	resolved   []Resolved
	perm       []int
	err        multierror.Error
	ready      bool
	closed     bool
}

// Err returns any errors encountered since the last call to Err or ResolveAll.
func (res *WatchingResolver) Err() error {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	err := misc.ErrorOrNil(res.err)
	res.err.Errors = nil
	return err
}

// ResolveAll returns all resolved addresses, plus any errors encountered since
// the last call to Err or ResolveAll.
func (res *WatchingResolver) ResolveAll() ([]Resolved, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	err := misc.ErrorOrNil(res.err)
	res.err.Errors = nil
	return res.resolved, err
}

// Resolve returns the resolved address of a healthy backend, if one is
// available, or else returns the error that prevented it from doing so.
func (res *WatchingResolver) Resolve() (Resolved, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	err := misc.ErrorOrNil(res.err)
	return balanceImpl(res.balancer, err, res.resolved, res.rng, res.perm, &res.nextRR)
}

// Update changes the status of a server.
func (res *WatchingResolver) Update(opts UpdateOptions) {
	addr := opts.Addr.String()

	res.mu.Lock()
	dynamic := res.byAddr[addr]
	res.mu.Unlock()

	if dynamic != nil {
		dynamic.Update(opts)
	}
}

// Watch registers a WatchFunc.  The WatchFunc will be called immediately with
// synthetic events for each resolved address currently known, plus it will be
// called whenever the Resolver's state changes.
func (res *WatchingResolver) Watch(fn WatchFunc) WatchID {
	if fn == nil {
		panic(errors.New("WatchFunc is nil"))
	}

	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		panic(fs.ErrClosed)
	}

	if res.ready {
		events := make([]Event, 0, len(res.resolved))
		for _, data := range res.resolved {
			ev := Event{
				Type: UpdateEvent,
				Key:  data.UniqueID,
				Data: data,
			}
			ev.Check()
			events = append(events, ev)
		}
		fn(events)
	}

	id := generateWatchID()
	res.watches[id] = fn
	return id
}

// CancelWatch cancels a previous call to Watch.
func (res *WatchingResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

// ResolveNow is a no-op.
func (res *WatchingResolver) ResolveNow(opts resolver.ResolveNowOptions) {
	// pass
}

// Close stops the resolver and frees all resources.
func (res *WatchingResolver) Close() {
	res.mu.Lock()
	defer res.mu.Unlock()

	res.cancelFn()
	for !res.closed {
		res.cv.Wait()
	}
}

func (res *WatchingResolver) resolverThread() {
	var wg sync.WaitGroup
	var ch <-chan []Event

	defer func() {
		res.cancelFn()

		for range ch {
			// pass
		}

		wg.Wait()

		res.mu.Lock()
		res.watches = nil
		res.byAddr = nil
		res.byUniqueID = nil
		res.resolved = nil
		res.perm = nil
		res.ready = true
		res.closed = true
		res.cv.Broadcast()
		res.mu.Unlock()
	}()

	retries := 0

	var err error
	for {
		for {
			ch, err = res.watchFn(res.ctx, &wg, res.backoff)
			if err == nil {
				break
			}

			if errors.Is(err, fs.ErrClosed) {
				return
			}

			events := make([]Event, 1)
			events[0] = Event{
				Type: ErrorEvent,
				Err:  err,
			}
			res.sendEvents(events)

			if !res.sleep(&retries) {
				return
			}
		}

		retries = 0
		readingChannel := true
		for readingChannel {
			select {
			case <-res.ctx.Done():
				return

			case events, ok := <-ch:
				if ok {
					res.sendEvents(events)
				} else {
					if !res.sleep(&retries) {
						return
					}
					readingChannel = false
				}
			}
		}
	}
}

func (res *WatchingResolver) sendEvents(events []Event) {
	for _, ev := range events {
		ev.Check()
	}

	var newErrors multierror.Error
	indicesToDelete := make(map[int]struct{}, len(events))
	rebuildByAddr := false
	didChange := false

	res.mu.Lock()
	defer func() {
		if didChange {
			res.ready = true
			res.cv.Broadcast()
		}
		res.mu.Unlock()
	}()

	gotDelete := func(uniqueID string) (oldData Resolved, ok bool) {
		if index, found := res.byUniqueID[uniqueID]; found {
			oldData = res.resolved[index]
			ok = true
			delete(res.byUniqueID, uniqueID)
			indicesToDelete[index] = struct{}{}
			rebuildByAddr = true
			didChange = true
		}
		return
	}

	gotInsert := func(newData Resolved) {
		if newData.Addr != nil {
			addr := newData.Addr.String()
			dynamic, found := res.byAddr[addr]
			if !found {
				dynamic = new(Dynamic)
				res.byAddr[addr] = dynamic
			}
			newData.Dynamic = dynamic
		}
		res.byUniqueID[newData.UniqueID] = len(res.resolved)
		res.resolved = append(res.resolved, newData)
		didChange = true
	}

	for _, ev := range events {
		switch ev.Type {
		case ErrorEvent:
			res.err.Errors = append(res.err.Errors, ev.Err)
			newErrors.Errors = append(newErrors.Errors, ev.Err)
		case UpdateEvent:
			gotDelete(ev.Key)
			gotInsert(ev.Data)
		case DeleteEvent:
			gotDelete(ev.Key)
		case BadDataEvent:
			gotDelete(ev.Key)
			gotInsert(ev.Data)
			res.err.Errors = append(res.err.Errors, ev.Data.Err)
			newErrors.Errors = append(newErrors.Errors, ev.Data.Err)
		}
	}

	if len(indicesToDelete) != 0 {
		n := len(res.resolved) - len(indicesToDelete)
		newList := make([]Resolved, 0, n)
		newByUniqueID := make(map[string]int, n)
		for index, data := range res.resolved {
			if _, found := indicesToDelete[index]; !found {
				newByUniqueID[data.UniqueID] = len(newList)
				newList = append(newList, data)
			}
		}
		res.resolved = newList
		res.byUniqueID = newByUniqueID
	}

	if rebuildByAddr {
		known := make(map[*Dynamic]struct{}, len(res.byAddr))
		for _, data := range res.resolved {
			if data.Dynamic != nil {
				known[data.Dynamic] = struct{}{}
			}
		}
		for addr, dynamic := range res.byAddr {
			if _, found := known[dynamic]; !found {
				delete(res.byAddr, addr)
			}
		}
	}

	if didChange {
		ResolvedList(res.resolved).Sort()
		res.perm = computePermImpl(res.balancer, res.resolved, res.rng)
	}

	if res.cc != nil {
		if len(res.resolved) != 0 {
			var state resolver.State
			state.Addresses = makeAddressList(res.resolved)
			state.ServiceConfig = res.sc
			_ = res.cc.UpdateState(state)
		} else if err := misc.ErrorOrNil(newErrors); err != nil {
			res.cc.ReportError(err)
		} else {
			res.cc.ReportError(roxyutil.ErrNoHealthyBackends)
		}
	}

	for _, fn := range res.watches {
		fn(events)
	}
}

func (res *WatchingResolver) sleep(retries *int) bool {
	t := time.NewTimer(res.backoff.Backoff(*retries))
	*retries++

	select {
	case <-t.C:
		return true

	case <-res.ctx.Done():
		t.Stop()
		return false
	}
}

var _ Resolver = (*WatchingResolver)(nil)
