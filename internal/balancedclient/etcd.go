package balancedclient

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"

	mvccpb "go.etcd.io/etcd/api/v3/mvccpb"
	etcdclient "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

func NewEtcdResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(fmt.Errorf("Context is nil"))
	}

	if opts.Etcd == nil {
		panic(fmt.Errorf("Etcd is nil"))
	}

	if opts.Target == "" {
		return nil, fmt.Errorf("Target is empty")
	}

	if strings.Contains(opts.Target, "//") {
		return nil, fmt.Errorf("Target %q must not contain two or more consecutive slashes", opts.Target)
	}
	if strings.HasSuffix(opts.Target, "/") {
		return nil, fmt.Errorf("Target %q must not end with slash", opts.Target)
	}

	switch opts.Balancer {
	case enums.RandomBalancer:
		// pass
	case enums.RoundRobinBalancer:
		// pass
	default:
		return nil, fmt.Errorf("EtcdResolver does not support %#v", opts.Balancer)
	}

	rng := opts.Random
	if rng == nil {
		rng = NewThreadSafeRandom(rand.Int63())
	}

	ctx, cancelFn := context.WithCancel(opts.Context)
	res := &etcdResolver{
		ctx:      ctx,
		rng:      rng,
		etcd:     opts.Etcd,
		target:   opts.Target,
		cancelFn: cancelFn,
		balancer: opts.Balancer,
		watches:  make(map[WatchID]WatchFunc, 1),
		known:    make(map[string]struct{}, 16),
		down:     make(map[string]struct{}, 4),
		resolved: make(map[string]etcdAddrData, 16),
	}
	res.cv = sync.NewCond(&res.mu)
	go res.watchThread()
	return res, nil
}

// type etcdResolver {{{

type etcdResolver struct {
	ctx      context.Context
	rng      *rand.Rand
	etcd     *etcdclient.Client
	target   string
	cancelFn context.CancelFunc
	balancer enums.BalancerType

	mu       sync.Mutex
	cv       *sync.Cond
	nextRR   uint
	lastID   WatchID
	watches  map[WatchID]WatchFunc
	known    map[string]struct{}
	down     map[string]struct{}
	resolved map[string]etcdAddrData
	perm     []string
	err      error
	ready    bool
	closed   bool
}

func (res *etcdResolver) ServerHostname() string {
	return "localhost"
}

func (res *etcdResolver) ResolveAll() ([]net.Addr, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv.Wait()
	}

	all := make([]net.Addr, 0, len(res.resolved))
	for _, data := range res.resolved {
		all = append(all, data.Addr)
	}
	tcpAddrList(all).Sort()
	return all, res.err
}

func (res *etcdResolver) Resolve() (net.Addr, error) {
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
		for _, data := range res.resolved {
			addrKey := data.Addr.String()
			if _, found := res.down[addrKey]; !found {
				candidates = append(candidates, data.Addr)
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
			addr := res.resolved[res.perm[k]].Addr
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

func (res *etcdResolver) MarkHealthy(addr net.Addr, healthy bool) {
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

func (res *etcdResolver) Watch(fn WatchFunc) WatchID {
	if fn == nil {
		panic(fmt.Errorf("WatchFunc is nil"))
	}

	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		panic(ErrClosed)
	}

	if res.ready {
		events := make([]*Event, 0, len(res.resolved))
		for pathKey, data := range res.resolved {
			ev := &Event{
				Type:     enums.UpdateEvent,
				Key:      pathKey,
				Addr:     data.Addr,
				Metadata: data.Metadata,
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

func (res *etcdResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

func (res *etcdResolver) Close() error {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return ErrClosed
	}

	res.cancelFn()

	for !res.closed {
		res.cv.Wait()
	}

	return nil
}

func (res *etcdResolver) watchThread() {
	defer func() {
		res.mu.Lock()
		res.watches = nil
		res.known = nil
		res.down = nil
		res.resolved = nil
		res.perm = nil
		res.err = ErrClosed
		res.ready = true
		res.closed = true
		res.cv.Broadcast()
		res.mu.Unlock()
	}()

	prefix := res.target + "/"
	resp, err := res.etcd.KV.Get(
		res.ctx,
		prefix,
		etcdclient.WithPrefix(),
		etcdclient.WithSerializable())

	n := 0
	if resp != nil {
		n += len(resp.Kvs)
	}
	if err != nil {
		n++
	}

	events := make([]*Event, 0, n)
	if resp != nil {
		for _, kv := range resp.Kvs {
			ev := parseKv(kv)
			if ev != nil {
				events = append(events, ev)
			}
		}
	}
	if err != nil {
		ev := &Event{
			Type: enums.ErrorEvent,
			Err:  err,
		}
		events = append(events, ev)
	}
	res.doUpdate(events)

	if err != nil && resp == nil {
		return
	}

	wch := res.etcd.Watcher.Watch(
		res.ctx,
		prefix,
		etcdclient.WithPrefix(),
		etcdclient.WithRev(resp.Header.Revision+1))

Loop:
	for {
		select {
		case <-res.ctx.Done():
			break Loop

		case wresp, ok := <-wch:
			if !ok {
				break Loop
			}

			err := wresp.Err()

			n = len(wresp.Events)
			if err != nil {
				n++
			}
			events = make([]*Event, 0, n)

			if err != nil {
				ev := &Event{
					Type: enums.ErrorEvent,
					Err:  err,
				}
				events = append(events, ev)
			}

			for _, wev := range wresp.Events {
				var ev *Event

				switch wev.Type {
				case etcdclient.EventTypePut:
					ev = parseKv(wev.Kv)

				case etcdclient.EventTypeDelete:
					pathKey := string(wev.Kv.Key)
					ev = &Event{
						Type: enums.DeleteEvent,
						Key:  pathKey,
					}
				}

				if ev != nil {
					events = append(events, ev)
				}
			}

			res.doUpdate(events)
		}
	}
}

func (res *etcdResolver) doUpdate(events []*Event) {
	if len(events) == 0 {
		return
	}

	res.mu.Lock()
	defer func() {
		res.ready = true
		res.cv.Broadcast()
		res.mu.Unlock()
	}()

	if res.closed {
		return
	}

	for addrKey := range res.known {
		delete(res.known, addrKey)
	}

	didChange := false
	for _, ev := range events {
		switch ev.Type {
		case enums.ErrorEvent:
			res.err = ev.Err
		case enums.UpdateEvent:
			res.resolved[ev.Key] = etcdAddrData{ev.Addr, ev.Metadata}
			didChange = true
		case enums.DeleteEvent:
			delete(res.resolved, ev.Key)
			didChange = true
		case enums.CorruptEvent:
			delete(res.resolved, ev.Key)
			didChange = true
		}
	}

	for _, data := range res.resolved {
		res.known[data.Addr.String()] = struct{}{}
	}

	for addrKey := range res.down {
		if _, found := res.known[addrKey]; !found {
			delete(res.down, addrKey)
		}
	}

	if didChange && res.balancer == enums.RoundRobinBalancer {
		keys := make([]string, 0, len(res.resolved))
		for pathKey := range res.resolved {
			keys = append(keys, pathKey)
		}
		res.rng.Shuffle(len(keys), func(i, j int) {
			keys[i], keys[j] = keys[j], keys[i]
		})
		res.nextRR = 0
		res.perm = keys
	}

	for _, fn := range res.watches {
		fn(events)
	}
}

var _ Resolver = (*etcdResolver)(nil)

// }}}

type grpcOperation uint8

const (
	grpcOpAdd grpcOperation = iota
	grpcOpDelete
)

type grpcUpdate struct {
	Op       grpcOperation `json:"Op"`
	Addr     string        `json:"Addr"`
	Metadata interface{}   `json:"Metadata"`
}

func parseKv(kv *mvccpb.KeyValue) *Event {
	pathKey := string(kv.Key)

	var update grpcUpdate
	if err := json.Unmarshal(kv.Value, &update); err != nil {
		err = fmt.Errorf("failed to parse JSON %q: %w", string(kv.Value), err)
		return &Event{
			Type: enums.CorruptEvent,
			Key:  pathKey,
			Err:  err,
		}
	}

	switch update.Op {
	case grpcOpAdd:
		host, port, err := net.SplitHostPort(update.Addr)
		if err != nil {
			err = fmt.Errorf("failed to parse IP:port %q: %w", update.Addr, err)
			return &Event{
				Type: enums.CorruptEvent,
				Key:  pathKey,
				Err:  err,
			}
		}

		var (
			ipStr string
			zone  string
		)
		if i := strings.IndexByte(host, '%'); i >= 0 {
			ipStr, zone = host[:i], host[i+1:]
		} else {
			ipStr = host
		}

		ip := net.ParseIP(ipStr)
		if ip == nil {
			err = fmt.Errorf("failed to parse IP address %q", ipStr)
			return &Event{
				Type: enums.CorruptEvent,
				Key:  pathKey,
				Err:  err,
			}
		}

		portNum, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			err = fmt.Errorf("failed to parse port %q: %w", port, err)
			return &Event{
				Type: enums.CorruptEvent,
				Key:  pathKey,
				Err:  err,
			}
		}

		return &Event{
			Type:     enums.UpdateEvent,
			Key:      pathKey,
			Addr:     &net.TCPAddr{IP: ip, Port: int(portNum), Zone: zone},
			Metadata: update.Metadata,
		}

	case grpcOpDelete:
		return &Event{
			Type: enums.DeleteEvent,
			Key:  pathKey,
		}

	default:
		return nil
	}
}

type etcdAddrData struct {
	Addr     net.Addr
	Metadata interface{}
}
