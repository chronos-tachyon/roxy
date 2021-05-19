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
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

// MaxPollInterval is the maximum permitted value of PollInterval.
const MaxPollInterval = 24 * time.Hour

// DefaultPollInterval is the default value of PollInterval if not specified.
const DefaultPollInterval = 5 * time.Minute

// DefaultCooldownInterval is the default value of CooldownInterval if not specified.
const DefaultCooldownInterval = 30 * time.Second

// PollingResolverOptions holds options related to constructing a new PollingResolver.
type PollingResolverOptions struct {
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

	// PollInterval is the interval between automatic calls to ResolveFunc.
	// If zero, DefaultPollInterval is used.  If negative, MaxPollInterval
	// is used.
	PollInterval time.Duration

	// CooldownInterval is the minimum interval between calls to
	// ResolveFunc, whether automatic or triggered by ResolveNow.  If zero,
	// DefaultCooldownInterval is used.  If negative, the final value of
	// PollInterval is used.
	CooldownInterval time.Duration

	// Balancer selects which load balancer algorithm to use.
	Balancer BalancerType

	// ResolveFunc will be called periodically to resolve addresses.
	//
	// This field is mandatory.
	ResolveFunc PollingResolveFunc

	// ClientConn is a gRPC ClientConn that will receive state updates.
	ClientConn resolver.ClientConn

	// ServiceConfigJSON is the gRPC Service Config which will be provided
	// to ClientConn on each state update.
	ServiceConfigJSON string
}

// PollingResolveFunc represents a closure that will be called periodically to
// resolve addresses.  It will be called at least once every PollInterval, and
// no less than CooldownInterval will pass between calls.
type PollingResolveFunc func() ([]Resolved, error)

// NewPollingResolver constructs a new PollingResolver.
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

// PollingResolver is an implementation of the Resolver interface that
// periodically polls for record changes.
type PollingResolver struct {
	ctx          context.Context
	cancelFn     context.CancelFunc
	rng          *rand.Rand
	pollInterval time.Duration
	cdInterval   time.Duration
	balancer     BalancerType
	resolveFn    PollingResolveFunc
	resolveNowCh chan struct{}
	cc           resolver.ClientConn
	sc           *serviceconfig.ParseResult
	nextRR       uint32

	mu       sync.Mutex
	cv       *sync.Cond
	watches  map[WatchID]WatchFunc
	byAddr   map[string]*Dynamic
	byUnique map[string]int
	resolved []Resolved
	perm     []int
	err      multierror.Error
	ready    bool
	closed   bool
}

// Err returns any errors encountered since the last call to Err or ResolveAll.
func (res *PollingResolver) Err() error {
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
func (res *PollingResolver) ResolveAll() ([]Resolved, error) {
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
func (res *PollingResolver) Resolve() (Resolved, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	err := misc.ErrorOrNil(res.err)
	return balanceImpl(res.balancer, err, res.resolved, res.rng, res.perm, &res.nextRR)
}

// Update changes the status of a server.
func (res *PollingResolver) Update(opts UpdateOptions) {
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

	id := generateWatchID()
	res.watches[id] = fn
	return id
}

// CancelWatch cancels a previous call to Watch.
func (res *PollingResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

// ResolveNow forces a poll immediately, or as soon as possible if an immediate
// poll would violate the CooldownInterval.
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

// Close stops the resolver and frees all resources.
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
			res.cc.ReportError(roxyutil.ErrNoHealthyBackends)
		} else {
			var state resolver.State
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
