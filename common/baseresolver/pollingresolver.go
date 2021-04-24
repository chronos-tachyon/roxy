package baseresolver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"reflect"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"

	"github.com/chronos-tachyon/roxy/common/syncrand"
)

type PollingResolverOptions struct {
	Context           context.Context
	Random            *rand.Rand
	PollInterval      time.Duration
	Balancer          BalancerType
	ResolveFunc       PollingResolveFunc
	ClientConn        resolver.ClientConn
	ServiceConfigJSON string
}

type PollingResolveFunc func() ([]*AddrData, error)

func NewPollingResolver(opts PollingResolverOptions) (*PollingResolver, error) {
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

	interval := opts.PollInterval
	if interval == 0 {
		interval = 1 * time.Minute
	}

	var parsedServiceConfig *serviceconfig.ParseResult
	if opts.ClientConn != nil && opts.ServiceConfigJSON != "" {
		parsedServiceConfig = opts.ClientConn.ParseServiceConfig(opts.ServiceConfigJSON)
	}

	ctx, cancelFn := context.WithCancel(opts.Context)
	res := &PollingResolver{
		ctx:          ctx,
		cancelFn:     cancelFn,
		rng:          rng,
		interval:     interval,
		balancer:     opts.Balancer,
		resolveFn:    opts.ResolveFunc,
		resolveNowCh: make(chan struct{}, 1),
		cc:           opts.ClientConn,
		sc:           parsedServiceConfig,
		watches:      make(map[WatchID]WatchFunc, 1),
	}
	res.cv = sync.NewCond(&res.mu)
	go res.resolverThread()
	return res, nil
}

type PollingResolver struct {
	ctx          context.Context
	cancelFn     context.CancelFunc
	rng          *rand.Rand
	interval     time.Duration
	balancer     BalancerType
	resolveFn    PollingResolveFunc
	resolveNowCh chan struct{}
	cc           resolver.ClientConn
	sc           *serviceconfig.ParseResult

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

func (res *PollingResolver) Err() error {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.err.ErrorOrNil()
}

func (res *PollingResolver) ResolveAll() ([]*AddrData, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.resolved, res.err.ErrorOrNil()
}

func (res *PollingResolver) Resolve() (*AddrData, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return balanceImpl(res.balancer, res.err, res.resolved, res.rng, res.perm, &res.mu, &res.nextRR)
}

func (res *PollingResolver) Update(opts UpdateOptions) {
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

func (res *PollingResolver) Watch(fn WatchFunc) WatchID {
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

func (res *PollingResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

func (res *PollingResolver) ResolveNow(opts resolver.ResolveNowOptions) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	select {
	case res.resolveNowCh <- struct{}{}:
	default:
	}
}

func (res *PollingResolver) Close() {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	res.cancelFn()
	for !res.closed {
		res.cv.Wait()
	}
}

func (res *PollingResolver) resolverThread() {
	defer func() {
		res.cancelFn()

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

	for {
		resolved, err := res.resolveFn()
		res.onUpdate(resolved, err)

		if res.interval <= 0 {
			return
		}

		t := time.NewTimer(res.interval)

		select {
		case <-res.ctx.Done():
			t.Stop()
			return

		case <-res.resolveNowCh:
			t.Stop()
			continue

		case <-t.C:
			continue
		}
	}
}

func (res *PollingResolver) onUpdate(newList []*AddrData, newErr error) {
	didChange := false

	res.mu.Lock()
	defer func() {
		if didChange {
			res.ready = true
			res.cv.Broadcast()
		}
		res.mu.Unlock()
	}()

	if res.closed {
		return
	}

	oldList := res.resolved
	sameList := false
	if newList == nil {
		newList = oldList
		sameList = true
	}

	n := 0
	if !sameList {
		n += len(oldList) + len(newList)
	}
	if newErr != nil {
		n++
	}
	if n == 0 {
		return
	}

	events := make([]*Event, 0, n)

	if !sameList {
		addrDataList(newList).Sort()

		newByAddrKey := make(map[string][]*AddrData, len(newList))
		newByDataKey := make(map[string]*AddrData, len(newList))
		for _, newData := range newList {
			dataKey := newData.Key()
			newByDataKey[dataKey] = newData

			if newData.Addr != nil {
				addrKey := newData.Addr.String()
				newByAddrKey[addrKey] = append(newByAddrKey[addrKey], newData)
			}

			if oldData := res.byDataKey[dataKey]; oldData != nil {
				newData.UpdateFrom(oldData)
			}
		}

		for _, oldData := range oldList {
			dataKey := oldData.Key()
			if newData := newByDataKey[dataKey]; newData == nil {
				ev := &Event{
					Type: DeleteEvent,
					Key:  dataKey,
					Data: oldData,
				}
				events = append(events, ev)
			} else if !reflect.DeepEqual(oldData, newData) {
				ev := &Event{
					Type: StatusChangeEvent,
					Key:  dataKey,
					Data: newData,
				}
				events = append(events, ev)
			}
		}

		for _, newData := range newList {
			dataKey := newData.Key()
			if oldData := res.byDataKey[dataKey]; oldData == nil {
				ev := &Event{
					Type: UpdateEvent,
					Key:  dataKey,
					Data: newData,
				}
				events = append(events, ev)
			}
		}

		res.byAddrKey = newByAddrKey
		res.byDataKey = newByDataKey
		res.resolved = newList
		if res.balancer == RoundRobinBalancer {
			res.nextRR = 0
			res.perm = res.rng.Perm(len(newList))
		}

		didChange = true
	}

	if newErr != nil {
		ev := &Event{
			Type: ErrorEvent,
			Err:  newErr,
		}
		events = append(events, ev)
		res.err.Errors = append(res.err.Errors, newErr)
	}

	if res.cc != nil {
		if newErr != nil {
			res.cc.ReportError(newErr)
		} else {
			var state resolver.State
			state.Addresses = make([]resolver.Address, 0, len(newList))
			for _, data := range newList {
				if data.Addr != nil {
					state.Addresses = append(state.Addresses, data.Address)
				}
			}
			state.ServiceConfig = res.sc
			if len(state.Addresses) == 0 {
				res.cc.ReportError(ErrNoHealthyBackends)
			} else {
				res.cc.UpdateState(state)
			}
		}
	}

	for _, fn := range res.watches {
		fn(events)
	}
}

var _ Resolver = (*PollingResolver)(nil)
