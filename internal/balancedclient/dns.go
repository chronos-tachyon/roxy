package balancedclient

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

func NewDNSResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(fmt.Errorf("Context is nil"))
	}

	if opts.Target == "" {
		return nil, fmt.Errorf("Target is empty")
	}

	host, port, err := net.SplitHostPort(opts.Target)
	if err != nil {
		return nil, fmt.Errorf("Target %q cannot be parsed as <host>:<port>: %w", opts.Target, err)
	}
	if host == "" {
		return nil, fmt.Errorf("Target %q contains empty <host>", opts.Target)
	}
	if port == "" {
		return nil, fmt.Errorf("Target %q contains empty <port>", opts.Target)
	}

	switch opts.Balancer {
	case enums.RandomBalancer:
		// pass
	case enums.RoundRobinBalancer:
		// pass
	default:
		return nil, fmt.Errorf("DNSResolver does not support %#v", opts.Balancer)
	}

	rng := opts.Random
	if rng == nil {
		rng = NewThreadSafeRandom(rand.Int63())
	}

	interval := opts.PollInterval
	if interval == 0 {
		interval = 1 * time.Minute
	}

	res := &dnsResolver{
		ctx:      opts.Context,
		rng:      rng,
		doneCh:   make(chan struct{}),
		host:     host,
		port:     port,
		interval: interval,
		balancer: opts.Balancer,
		watches:  make(map[WatchID]WatchFunc, 1),
		known:    make(map[string]struct{}, 16),
		down:     make(map[string]struct{}, 4),
	}
	res.cv = sync.NewCond(&res.mu)
	go res.resolverThread()
	return res, nil
}

// type dnsResolver {{{

type dnsResolver struct {
	ctx      context.Context
	rng      *rand.Rand
	doneCh   chan struct{}
	host     string
	port     string
	interval time.Duration
	balancer enums.BalancerType

	mu       sync.Mutex
	cv       *sync.Cond
	nextRR   uint
	lastID   WatchID
	watches  map[WatchID]WatchFunc
	known    map[string]struct{}
	down     map[string]struct{}
	resolved []net.Addr
	perm     []int
	err      error
	ready    bool
	closed   bool
}

func (res *dnsResolver) ServerHostname() string {
	return res.host
}

func (res *dnsResolver) ResolveAll() ([]net.Addr, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	return res.resolved, res.err
}

func (res *dnsResolver) Resolve() (net.Addr, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	if res.err != nil && len(res.resolved) == 0 {
		return nil, res.err
	}

	switch res.balancer {
	case enums.RandomBalancer:
		candidates := make([]net.Addr, 0, len(res.resolved))
		for _, addr := range res.resolved {
			addrKey := addr.String()
			if _, found := res.down[addrKey]; !found {
				candidates = append(candidates, addr)
			}
		}

		if len(candidates) == 0 && res.err != nil {
			return nil, res.err
		}

		if len(candidates) == 0 {
			return nil, ErrNoHealthyBackends
		}

		index := res.rng.Intn(len(candidates))
		return candidates[index], nil

	case enums.RoundRobinBalancer:
		for tries := uint(len(res.perm)); tries > 0; tries-- {
			k := res.nextRR
			res.nextRR = (res.nextRR + 1) % uint(len(res.perm))
			addr := res.resolved[res.perm[k]]
			addrKey := addr.String()
			if _, found := res.down[addrKey]; !found {
				return addr, nil
			}
		}
		if res.err != nil {
			return nil, res.err
		}
		return nil, ErrNoHealthyBackends

	default:
		panic(fmt.Errorf("%#v not implemented", res.balancer))
	}
}

func (res *dnsResolver) MarkHealthy(addr net.Addr, healthy bool) {
	addrKey := addr.String()

	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	if res.closed {
		return
	}

	if _, found := res.known[addrKey]; found {
		if healthy {
			delete(res.down, addrKey)
		} else {
			res.down[addrKey] = struct{}{}
		}
	}
}

func (res *dnsResolver) Watch(fn WatchFunc) WatchID {
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
		for _, addr := range res.resolved {
			addrKey := addr.String()
			ev := &Event{
				Type: enums.UpdateEvent,
				Key:  addrKey,
				Addr: addr,
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

func (res *dnsResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

func (res *dnsResolver) Close() error {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return fs.ErrClosed
	}

	close(res.doneCh)

	for !res.closed {
		res.cv.Wait()
	}

	return nil
}

func (res *dnsResolver) resolverThread() {
	defer func() {
		res.mu.Lock()
		res.watches = nil
		res.resolved = nil
		res.perm = nil
		res.known = nil
		res.down = nil
		res.err = fs.ErrClosed
		res.ready = true
		res.closed = true
		res.cv.Broadcast()
		res.mu.Unlock()
	}()

	resolved, err := res.doResolve()
	res.doUpdate(resolved, err)

	if res.interval < 0 {
		<-res.doneCh
		return
	}

	ticker := time.NewTicker(res.interval)
	defer ticker.Stop()

	for {
		select {
		case <-res.doneCh:
			return

		case <-res.ctx.Done():
			ticker.Stop()
			<-res.doneCh
			return

		case <-ticker.C:
			resolved, err = res.doResolve()
			res.doUpdate(resolved, err)
		}
	}
}

func (res *dnsResolver) doResolve() ([]net.Addr, error) {
	// Resolve the port number.
	portNum, err := net.LookupPort("tcp", res.port)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve port %q: %w", res.port, err)
	}

	// Resolve the A/AAAA records.
	ipStrList, err := net.LookupHost(res.host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve host %q: %w", res.host, err)
	}

	// Synthesize a *net.TCPAddr for each IP address.
	out := make([]net.Addr, len(ipStrList))
	for index, ipStr := range ipStrList {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, fmt.Errorf("failed to parse IP %q", ipStr)
		}
		out[index] = &net.TCPAddr{IP: ip, Port: int(portNum)}
	}

	// Sort the *net.TCPAddr records and return.
	tcpAddrList(out).Sort()
	return out, nil
}

func (res *dnsResolver) doUpdate(newList []net.Addr, newErr error) {
	res.mu.Lock()
	defer func() {
		res.resolved = newList
		res.err = newErr
		res.ready = true
		res.cv.Broadcast()
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

	differenceCount := 0
	if !sameList {
		if len(oldList) == len(newList) {
			for index := range oldList {
				oldAddr := oldList[index].(*net.TCPAddr)
				newAddr := newList[index].(*net.TCPAddr)
				cmp := tcpAddrCompare(*oldAddr, *newAddr)
				if cmp != 0 {
					differenceCount++
				}
			}
			if differenceCount == 0 {
				sameList = true
			}
		} else {
			differenceCount = len(oldList) + len(newList)
		}
	}

	n := differenceCount
	if newErr != nil {
		n++
	}

	events := make([]*Event, 0, n)

	if !sameList {
		for addrKey := range res.known {
			delete(res.known, addrKey)
		}

		oldSet := make(map[string]int, len(oldList))
		for index, addr := range oldList {
			oldSet[addr.String()] = index
		}

		newSet := make(map[string]int, len(newList))
		for index, addr := range newList {
			newSet[addr.String()] = index
		}

		for addrKey := range oldSet {
			if _, found := newSet[addrKey]; !found {
				ev := &Event{
					Type: enums.DeleteEvent,
					Key:  addrKey,
				}
				events = append(events, ev)
			}
		}

		for addrKey, index := range newSet {
			res.known[addrKey] = struct{}{}

			if _, found := oldSet[addrKey]; !found {
				ev := &Event{
					Type: enums.UpdateEvent,
					Key:  addrKey,
					Addr: newList[index],
				}
				events = append(events, ev)
			}
		}

		for addrKey := range res.down {
			if _, found := res.known[addrKey]; !found {
				delete(res.down, addrKey)
			}
		}

		if res.balancer == enums.RoundRobinBalancer {
			res.nextRR = 0
			res.perm = res.rng.Perm(len(newList))
		}
	}

	if newErr != nil {
		ev := &Event{
			Type: enums.ErrorEvent,
			Err:  newErr,
		}
		events = append(events, ev)
	}

	if len(events) == 0 {
		return
	}

	for _, fn := range res.watches {
		fn(events)
	}
}

var _ Resolver = (*dnsResolver)(nil)

// }}}
