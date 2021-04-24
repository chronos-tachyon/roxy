package baseresolver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"

	"github.com/chronos-tachyon/roxy/common/expbackoff"
	"github.com/chronos-tachyon/roxy/common/syncrand"
)

type WatchingResolverOptions struct {
	Context           context.Context
	Random            *rand.Rand
	Balancer          BalancerType
	ResolveFunc       WatchingResolveFunc
	ClientConn        resolver.ClientConn
	ServiceConfigJSON string
}

type WatchingResolveFunc func(ctx context.Context, wg *sync.WaitGroup, retries *int, backoff expbackoff.ExpBackoff) (<-chan []*Event, error)

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
		ctx:       ctx,
		cancelFn:  cancelFn,
		rng:       rng,
		balancer:  opts.Balancer,
		backoff:   expbackoff.BuildDefault(),
		watchFn:   opts.ResolveFunc,
		cc:        opts.ClientConn,
		sc:        parsedServiceConfig,
		watches:   make(map[WatchID]WatchFunc, 1),
		byAddrKey: make(map[string][]*AddrData, 16),
		byDataKey: make(map[string]*AddrData, 16),
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
	cc       resolver.ClientConn
	sc       *serviceconfig.ParseResult

	mu        sync.Mutex
	cv        *sync.Cond
	nextRR    uint
	lastID    WatchID
	watches   map[WatchID]WatchFunc
	byAddrKey map[string][]*AddrData
	byDataKey map[string]*AddrData
	resolved  []*AddrData
	perm      []int
	err       multierror.Error
	ready     bool
	closed    bool
}

func (res *WatchingResolver) Err() error {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.err.ErrorOrNil()
}

func (res *WatchingResolver) ResolveAll() ([]*AddrData, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.resolved, res.err.ErrorOrNil()
}

func (res *WatchingResolver) Resolve() (*AddrData, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return balanceImpl(res.balancer, res.err, res.resolved, res.rng, res.perm, &res.mu, &res.nextRR)
}

func (res *WatchingResolver) Update(opts UpdateOptions) {
	addrKey := opts.Addr.String()

	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	if res.closed {
		return
	}

	for _, data := range res.byAddrKey[addrKey] {
		data.Update(opts)
	}
}

func (res *WatchingResolver) Watch(fn WatchFunc) WatchID {
	if fn == nil {
		panic(fmt.Errorf("WatchFunc is nil"))
	}

	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		panic(fs.ErrClosed)
	}

	if res.ready {
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

func (res *WatchingResolver) ResolveNow(opts resolver.ResolveNowOptions) {
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
		res.byAddrKey = nil
		res.byDataKey = nil
		res.resolved = nil
		res.perm = nil
		res.ready = true
		res.closed = true
		res.cv.Broadcast()
		res.mu.Unlock()
	}()

	retries := 0

	var ch <-chan []*Event
	var err error
	for {
		for {
			ch, err = res.watchFn(res.ctx, &wg, &retries, res.backoff)
			if err == nil {
				break
			}

			if errors.Is(err, fs.ErrClosed) {
				return
			}

			res.sendEvents([]*Event{{Type: ErrorEvent, Err: err}})

			if !res.sleep(&retries) {
				return
			}
		}

		retries = 0
		looping := true
		for looping {
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
					looping = false
				}
			}
		}
	}
}

func (res *WatchingResolver) sendEvents(events []*Event) {
	var newErrors multierror.Error
	pointersToDelete := make(map[*AddrData]struct{}, len(events))
	rebuildByAddrKey := false
	didChange := false

	res.mu.Lock()
	defer func() {
		if didChange {
			res.ready = true
			res.cv.Broadcast()
		}
		res.mu.Unlock()
	}()

	gotDelete := func(dataKey string) (oldData *AddrData) {
		data := res.byDataKey[dataKey]
		if data == nil {
			return nil
		}

		delete(res.byDataKey, dataKey)
		pointersToDelete[data] = struct{}{}
		rebuildByAddrKey = true
		didChange = true
		return data
	}

	gotInsert := func(dataKey string, data *AddrData) {
		if computedDataKey := data.Key(); dataKey != computedDataKey {
			panic(fmt.Errorf("AddrData key mismatch: %q vs %q", dataKey, computedDataKey))
		}
		if data.Addr != nil {
			addrKey := data.Addr.String()
			res.byAddrKey[addrKey] = append(res.byAddrKey[addrKey], data)
		}
		res.byDataKey[dataKey] = data
		res.resolved = append(res.resolved, data)
		didChange = true
	}

	for _, ev := range events {
		switch ev.Type {
		case ErrorEvent:
			res.err.Errors = append(res.err.Errors, ev.Err)
			newErrors.Errors = append(newErrors.Errors, ev.Err)
		case UpdateEvent:
			oldData := gotDelete(ev.Key)
			if oldData != nil {
				ev.Data.UpdateFrom(oldData)
			}
			gotInsert(ev.Key, ev.Data)
		case DeleteEvent:
			gotDelete(ev.Key)
		case BadDataEvent:
			oldData := gotDelete(ev.Key)
			if oldData != nil {
				ev.Data.UpdateFrom(oldData)
			}
			gotInsert(ev.Key, ev.Data)
			res.err.Errors = append(res.err.Errors, ev.Data.Err)
			newErrors.Errors = append(newErrors.Errors, ev.Data.Err)
		}
	}

	if len(pointersToDelete) != 0 {
		list := make([]*AddrData, 0, len(res.resolved)-len(pointersToDelete))
		for _, data := range res.resolved {
			if _, found := pointersToDelete[data]; !found {
				list = append(list, data)
			}
		}
		res.resolved = list
	}

	if rebuildByAddrKey {
		for addrKey := range res.byAddrKey {
			delete(res.byAddrKey, addrKey)
		}
		for _, data := range res.resolved {
			if data.Addr != nil {
				addrKey := data.Addr.String()
				res.byAddrKey[addrKey] = append(res.byAddrKey[addrKey], data)
			}
		}
	}

	if didChange {
		addrDataList(res.resolved).Sort()
		if res.balancer == RoundRobinBalancer {
			res.nextRR = 0
			res.perm = res.rng.Perm(len(res.resolved))
		}
	}

	if res.cc != nil {
		err := newErrors.ErrorOrNil()
		if err != nil {
			res.cc.ReportError(err)
		}

		var state resolver.State
		state.Addresses = make([]resolver.Address, 0, len(res.resolved))
		for _, data := range res.resolved {
			if data.Addr != nil {
				state.Addresses = append(state.Addresses, data.Address)
			}
		}
		state.ServiceConfig = res.sc
		if len(state.Addresses) == 0 && err != nil {
			// pass
		} else if len(state.Addresses) == 0 {
			res.cc.ReportError(ErrNoHealthyBackends)
		} else {
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
