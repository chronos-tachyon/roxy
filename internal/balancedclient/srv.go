package balancedclient

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

func NewSRVResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(fmt.Errorf("Context is nil"))
	}

	if opts.Target == "" {
		return nil, fmt.Errorf("Target is empty")
	}

	i := strings.IndexByte(opts.Target, '/')
	if i < 0 {
		return nil, fmt.Errorf("Target %q cannot be parsed as <name>/<service>: missing '/'", opts.Target)
	}
	name, service := opts.Target[:i], opts.Target[i+1:]
	if name == "" {
		return nil, fmt.Errorf("Target %q contains empty <name>", opts.Target)
	}
	if service == "" {
		return nil, fmt.Errorf("Target %q contains empty <service>", opts.Target)
	}
	if j := strings.IndexByte(service, '/'); j >= 0 {
		return nil, fmt.Errorf("Target %q contains multiple slashes", opts.Target)
	}

	if opts.Balancer != enums.RandomBalancer {
		return nil, fmt.Errorf("SRVResolver does not support %#v", opts.Balancer)
	}

	rng := opts.Random
	if rng == nil {
		rng = NewThreadSafeRandom(rand.Int63())
	}

	interval := opts.PollInterval
	if interval == 0 {
		interval = 1 * time.Minute
	}

	res := &srvResolver{
		ctx:      opts.Context,
		rng:      rng,
		doneCh:   make(chan struct{}),
		name:     name,
		service:  service,
		interval: interval,
		watches:  make(map[WatchID]WatchFunc, 1),
		resolved: make(ResolvedSRVList, 0),
		known:    make(map[string]struct{}, 16),
		down:     make(map[string]struct{}, 4),
	}
	res.cv = sync.NewCond(&res.mu)
	go res.resolveThread()
	return res, nil
}

// type srvResolver {{{

type srvResolver struct {
	ctx      context.Context
	rng      *rand.Rand
	doneCh   chan struct{}
	name     string
	service  string
	interval time.Duration

	mu       sync.Mutex
	cv       *sync.Cond
	lastID   WatchID
	watches  map[WatchID]WatchFunc
	resolved ResolvedSRVList
	known    map[string]struct{}
	down     map[string]struct{}
	err      error
	ready    bool
	closed   bool
}

func (res *srvResolver) ServerHostname() string {
	return res.name
}

func (res *srvResolver) ResolveAll() ([]net.Addr, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	var all []net.Addr
	if res.resolved != nil {
		all = make([]net.Addr, len(res.resolved))
		for index, srv := range res.resolved {
			all[index] = srv.TCPAddr()
		}
	}
	return all, res.err
}

func (res *srvResolver) Resolve() (net.Addr, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	if res.err != nil && len(res.resolved) == 0 {
		return nil, res.err
	}

	down := make(map[int]struct{}, len(res.down))
	for index, srv := range res.resolved {
		if _, found := res.down[srv.Addr.String()]; found {
			down[index] = struct{}{}
		}
	}

	index, ok := res.resolved.Pick(res.rng, down)
	if !ok {
		return nil, ErrNoHealthyBackends
	}
	return res.resolved[index].TCPAddr(), nil
}

func (res *srvResolver) MarkHealthy(addr net.Addr, healthy bool) {
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

func (res *srvResolver) Watch(fn WatchFunc) WatchID {
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
		for _, srv := range res.resolved {
			srvKey := srv.Key()
			srvAddr := srv.TCPAddr()
			ev := &Event{
				Type:     enums.UpdateEvent,
				Key:      srvKey,
				Addr:     srvAddr,
				Metadata: srv,
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

func (res *srvResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

func (res *srvResolver) Close() error {
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

func (res *srvResolver) resolveThread() {
	defer func() {
		res.mu.Lock()
		res.watches = nil
		res.resolved = nil
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

func (res *srvResolver) doResolve() (ResolvedSRVList, error) {
	// Resolve the SRV records.
	_, records, err := net.LookupSRV(res.service, "tcp", res.name)
	if err != nil {
		return nil, err
	}

	// Generate the ResolvedSRV records.
	out := make(ResolvedSRVList, 0, len(records))
	for _, record := range records {
		// Resolve the A/AAAA records.
		addrs, err := net.LookupHost(record.Target)
		if err != nil {
			return nil, err
		}

		// Divide the weight evenly across all IP addresses.
		weight := float32(record.Weight) / float32(len(addrs))

		// Synthesize a ResolvedSRV record for each IP address.
		for _, addr := range addrs {
			ip := net.ParseIP(addr)
			if ip == nil {
				return nil, fmt.Errorf("net.LookupHost returned unparseable IP address %q", addr)
			}
			port := int(record.Port)
			out = append(out, ResolvedSRV{
				Priority:  record.Priority,
				RawWeight: record.Weight,
				Weight:    weight,
				Addr:      net.TCPAddr{IP: ip, Port: port},
			})
		}
	}

	// Sort the ResolvedSRV records and return.
	out.Sort()
	return out, nil
}

func (res *srvResolver) doUpdate(newList ResolvedSRVList, newErr error) {
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
				oldSRV := oldList[index]
				newSRV := newList[index]
				cmp := srvCompare(oldSRV, newSRV)
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
		for index, srv := range oldList {
			oldSet[srv.Key()] = index
		}

		newSet := make(map[string]int, len(newList))
		for index, srv := range newList {
			newSet[srv.Key()] = index
		}

		for srvKey, index := range oldSet {
			srv := oldList[index]

			if _, found := newSet[srvKey]; !found {
				ev := &Event{
					Type:     enums.DeleteEvent,
					Key:      srvKey,
					Metadata: srv,
				}
				events = append(events, ev)
			}
		}

		for srvKey, index := range newSet {
			srv := newList[index]
			srvAddr := srv.TCPAddr()
			addrKey := srvAddr.String()

			res.known[addrKey] = struct{}{}

			if _, found := oldSet[srvKey]; !found {
				ev := &Event{
					Type:     enums.UpdateEvent,
					Key:      srvKey,
					Addr:     srvAddr,
					Metadata: srv,
				}
				events = append(events, ev)
			}
		}

		for addrKey := range res.down {
			if _, found := res.known[addrKey]; !found {
				delete(res.down, addrKey)
			}
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

var _ Resolver = (*srvResolver)(nil)

// }}}

// type ResolvedSRV {{{

type ResolvedSRV struct {
	Priority  uint16
	RawWeight uint16
	Weight    float32
	Addr      net.TCPAddr
}

func (srv ResolvedSRV) Key() string {
	return fmt.Sprintf("%d/%d/%s", srv.Priority, srv.RawWeight, srv.Addr.String())
}

func (srv ResolvedSRV) TCPAddr() *net.TCPAddr {
	addr := new(net.TCPAddr)
	*addr = srv.Addr
	return addr
}

// type ResolvedSRVList {{{

type ResolvedSRVList []ResolvedSRV

func (list ResolvedSRVList) Len() int {
	return len(list)
}

func (list ResolvedSRVList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list ResolvedSRVList) Less(i, j int) bool {
	a, b := list[i], list[j]
	return srvCompare(a, b) < 0
}

func (list ResolvedSRVList) Sort() {
	sort.Sort(list)
}

func (list ResolvedSRVList) Pick(rng *rand.Rand, unhealthy map[int]struct{}) (int, bool) {
	// Identify the first healthy backend of the highest available priority tier.
	start := 0
	length := len(list)
	for start < length {
		if _, found := unhealthy[start]; !found {
			break
		}
		start++
	}

	// No healthy backends?  Can't pick anything.
	if start == length {
		return -1, false
	}

	// Collect the indices of the candidate backends.
	//
	// A backend is a candidate if it is:
	//   (a) healthy, and
	//   (b) in the same tier as list[start]
	//
	candidates := make([]int, 0, length-start)
	candidates = append(candidates, start)
	a := list[start]
	for index := start + 1; index < length; index++ {
		b := list[index]
		if a.Priority != b.Priority {
			break
		}
		if _, found := unhealthy[index]; !found {
			candidates = append(candidates, index)
		}
	}

	// If there is only one candidate, return it.
	if len(candidates) == 1 {
		return candidates[0], true
	}

	// Sum the weights of all candidates.
	var sumOfWeights float32
	for _, index := range candidates {
		sumOfWeights += list[index].Weight
	}

	// If the weights are non-zero...
	if sumOfWeights > 0.0 {
		type dataRow struct {
			f float32
			i int
		}

		// ... normalize the weights to convert them to probabilities,
		// then compute the cumulative probability for each candidate
		// so we can compute bucket boundaries.
		//
		// cumulative[i] contains the upper bound of the bucket for candidates[i].
		//
		recip := 1.0 / sumOfWeights
		cumulative := make([]float32, len(candidates))
		var w float32
		for i, index := range candidates {
			p := recip * list[index].Weight
			w = w + p
			cumulative[i] = w
		}

		// Pick the candidate whose bucket contains the random number K.
		k := rng.Float32()
		for i, w := range cumulative {
			if k < w {
				return candidates[i], true
			}
		}

		// If the floating point math went awry, fall back by
		// pretending that sumOfWeights was zero.
	}

	// Pick a candidate with uniform probability.
	i := rng.Intn(len(candidates))
	return candidates[i], true
}

// }}}

// }}}
