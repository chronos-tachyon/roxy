package roxyresolver

import (
	"context"
	"errors"
	"io/fs"
	"math/rand"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	grpcresolver "google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"

	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

type WatchingResolverOptions struct {
	Context           context.Context
	Random            *rand.Rand
	Balancer          BalancerType
	ResolveFunc       WatchingResolveFunc
	ClientConn        grpcresolver.ClientConn
	ServiceConfigJSON string
}

type WatchingResolveFunc func(ctx context.Context, wg *sync.WaitGroup, backoff expbackoff.ExpBackoff) (<-chan []Event, error)

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
		ctx:      ctx,
		cancelFn: cancelFn,
		rng:      rng,
		balancer: opts.Balancer,
		backoff:  expbackoff.BuildDefault(),
		watchFn:  opts.ResolveFunc,
		cc:       opts.ClientConn,
		sc:       parsedServiceConfig,
		watches:  make(map[WatchID]WatchFunc, 1),
		byAddr:   make(map[string]*Dynamic, 16),
		byUnique: make(map[string]int, 16),
	}
	res.cv = sync.NewCond(&res.mu)
	go res.resolverThread()
	return res, nil
}

type WatchingResolver struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	rng      *rand.Rand
	balancer BalancerType
	backoff  expbackoff.ExpBackoff
	watchFn  WatchingResolveFunc
	cc       grpcresolver.ClientConn
	sc       *serviceconfig.ParseResult
	nextRR   uint32

	mu       sync.Mutex
	cv       *sync.Cond
	lastID   WatchID
	watches  map[WatchID]WatchFunc
	byAddr   map[string]*Dynamic
	byUnique map[string]int
	resolved []Resolved
	perm     []int
	err      multierror.Error
	ready    bool
	closed   bool
}

func (res *WatchingResolver) Err() error {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.err.ErrorOrNil()
}

func (res *WatchingResolver) ResolveAll() ([]Resolved, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.resolved, res.err.ErrorOrNil()
}

func (res *WatchingResolver) Resolve() (Resolved, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return balanceImpl(res.balancer, res.err, res.resolved, res.rng, res.perm, &res.nextRR)
}

func (res *WatchingResolver) Update(opts UpdateOptions) {
	addr := opts.Addr.String()

	res.mu.Lock()
	dynamic := res.byAddr[addr]
	res.mu.Unlock()

	if dynamic != nil {
		dynamic.Update(opts)
	}
}

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
				Key:  data.Unique,
				Data: data,
			}
			ev.Check()
			events = append(events, ev)
		}
		fn(events)
	}

	res.lastID++
	id := res.lastID
	res.watches[id] = fn
	return id
}

func (res *WatchingResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

func (res *WatchingResolver) ResolveNow(opts ResolveNowOptions) {
	// pass
}

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

	defer func() {
		res.cancelFn()

		wg.Wait()

		res.mu.Lock()
		res.watches = nil
		res.byAddr = nil
		res.byUnique = nil
		res.resolved = nil
		res.perm = nil
		res.ready = true
		res.closed = true
		res.cv.Broadcast()
		res.mu.Unlock()
	}()

	retries := 0

	var ch <-chan []Event
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

	gotDelete := func(unique string) (oldData Resolved, ok bool) {
		if index, found := res.byUnique[unique]; found {
			oldData = res.resolved[index]
			ok = true
			delete(res.byUnique, unique)
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
		res.byUnique[newData.Unique] = len(res.resolved)
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
		newByUnique := make(map[string]int, n)
		for index, data := range res.resolved {
			if _, found := indicesToDelete[index]; !found {
				newByUnique[data.Unique] = len(newList)
				newList = append(newList, data)
			}
		}
		res.resolved = newList
		res.byUnique = newByUnique
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
		if err := newErrors.ErrorOrNil(); err != nil {
			res.cc.ReportError(err)
		} else if len(res.resolved) == 0 {
			res.cc.ReportError(ErrNoHealthyBackends)
		} else {
			var state grpcresolver.State
			state.Addresses = makeAddressList(res.resolved)
			state.ServiceConfig = res.sc
			res.cc.UpdateState(state)
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
