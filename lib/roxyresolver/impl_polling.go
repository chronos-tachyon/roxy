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

	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

const (
	MaxPollInterval         = 24 * time.Hour
	DefaultPollInterval     = 5 * time.Minute
	DefaultCooldownInterval = 30 * time.Second
)

type PollingResolverOptions struct {
	Context           context.Context
	Random            *rand.Rand
	PollInterval      time.Duration
	CooldownInterval  time.Duration
	Balancer          BalancerType
	ResolveFunc       PollingResolveFunc
	ClientConn        grpcresolver.ClientConn
	ServiceConfigJSON string
}

type PollingResolveFunc func() ([]Resolved, error)

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

	pollInterval := opts.PollInterval
	if pollInterval == 0 {
		pollInterval = DefaultPollInterval
	}
	if pollInterval < 0 || pollInterval > MaxPollInterval {
		pollInterval = MaxPollInterval
	}

	cdInterval := opts.CooldownInterval
	if cdInterval == 0 {
		cdInterval = DefaultCooldownInterval
	}
	if cdInterval < 0 || cdInterval > pollInterval {
		cdInterval = pollInterval
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
		pollInterval: pollInterval,
		cdInterval:   cdInterval,
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
	pollInterval time.Duration
	cdInterval   time.Duration
	balancer     BalancerType
	resolveFn    PollingResolveFunc
	resolveNowCh chan struct{}
	cc           grpcresolver.ClientConn
	sc           *serviceconfig.ParseResult
	nextRR       uint32

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

func (res *PollingResolver) Err() error {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.err.ErrorOrNil()
}

func (res *PollingResolver) ResolveAll() ([]Resolved, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.resolved, res.err.ErrorOrNil()
}

func (res *PollingResolver) Resolve() (Resolved, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return balanceImpl(res.balancer, res.err, res.resolved, res.rng, res.perm, &res.nextRR)
}

func (res *PollingResolver) Update(opts UpdateOptions) {
	addr := opts.Addr.String()

	res.mu.Lock()
	dynamic := res.byAddr[addr]
	res.mu.Unlock()

	if dynamic != nil {
		dynamic.Update(opts)
	}
}

func (res *PollingResolver) Watch(fn WatchFunc) WatchID {
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

func (res *PollingResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

func (res *PollingResolver) ResolveNow(opts ResolveNowOptions) {
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
		res.byAddr = nil
		res.byUnique = nil
		res.resolved = nil
		res.perm = nil
		res.ready = true
		res.closed = true
		res.cv.Broadcast()
		res.mu.Unlock()

		close(res.resolveNowCh)
	}()

	for {
		resolved, err := res.resolveFn()
		res.onUpdate(resolved, err)

		t := time.NewTimer(res.cdInterval)
		select {
		case <-res.ctx.Done():
			t.Stop()
			return
		case <-t.C:
			// pass
		}

		if res.pollInterval > res.cdInterval {
			t = time.NewTimer(res.pollInterval - res.cdInterval)
			select {
			case <-res.ctx.Done():
				t.Stop()
				return
			case <-res.resolveNowCh:
				t.Stop()
			case <-t.C:
				// pass
			}
		}
	}
}

func (res *PollingResolver) onUpdate(newList []Resolved, newErr error) {
	for _, data := range newList {
		data.Check()
	}

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

	if sameList && newErr == nil {
		return
	}

	n := 0
	if newErr != nil {
		n++
	}
	if !sameList {
		n += len(oldList) + len(newList)
	}

	events := make([]Event, 0, n)

	if newErr != nil {
		ev := Event{
			Type: ErrorEvent,
			Err:  newErr,
		}
		ev.Check()
		events = append(events, ev)
		res.err.Errors = append(res.err.Errors, newErr)
	}

	if !sameList {
		ResolvedList(newList).Sort()

		newByAddr := make(map[string]*Dynamic, len(newList))
		newByUnique := make(map[string]int, len(newList))
		for index, newData := range newList {
			if newData.Addr != nil {
				addr := newData.Addr.String()
				dynamic, found := newByAddr[addr]
				if !found {
					dynamic, found = res.byAddr[addr]
					if !found {
						dynamic = new(Dynamic)
					}
					newByAddr[addr] = dynamic
				}
				newData.Dynamic = dynamic
				newList[index] = newData
			}
			newByUnique[newData.Unique] = index
		}

		for _, oldData := range oldList {
			index, found := newByUnique[oldData.Unique]
			if !found {
				ev := Event{
					Type: DeleteEvent,
					Key:  oldData.Unique,
				}
				ev.Check()
				events = append(events, ev)
			} else {
				newData := newList[index]
				if !oldData.Equal(newData) {
					ev := Event{
						Type: StatusChangeEvent,
						Key:  newData.Unique,
						Data: newData,
					}
					ev.Check()
					events = append(events, ev)
				}
			}
		}

		for _, newData := range newList {
			_, found := res.byUnique[newData.Unique]
			if !found {
				ev := Event{
					Type: UpdateEvent,
					Key:  newData.Unique,
					Data: newData,
				}
				ev.Check()
				events = append(events, ev)
			}
		}

		res.byAddr = newByAddr
		res.byUnique = newByUnique
		res.resolved = newList
		res.perm = computePermImpl(res.balancer, res.resolved, res.rng)

		didChange = true
	}

	if res.cc != nil {
		if newErr != nil {
			res.cc.ReportError(newErr)
		} else if len(newList) == 0 {
			res.cc.ReportError(ErrNoHealthyBackends)
		} else {
			var state grpcresolver.State
			state.Addresses = makeAddressList(newList)
			state.ServiceConfig = res.sc
			res.cc.UpdateState(state)
		}
	}

	for _, fn := range res.watches {
		fn(events)
	}
}

var _ Resolver = (*PollingResolver)(nil)
