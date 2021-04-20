package balancedclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"net"
	"path"
	"strings"
	"sync"
	"time"

	zkclient "github.com/go-zookeeper/zk"
	multierror "github.com/hashicorp/go-multierror"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

func NewZKResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(fmt.Errorf("Context is nil"))
	}

	if opts.ZK == nil {
		panic(fmt.Errorf("ZK is nil"))
	}

	var (
		ssPath    string
		ssPort    string
		isDefault bool
	)
	if i := strings.IndexByte(opts.Target, ':'); i >= 0 {
		ssPath, ssPort, isDefault = opts.Target[:i], opts.Target[i+1:], false
	} else {
		ssPath, ssPort, isDefault = opts.Target, "", true
	}

	if ssPath == "" {
		return nil, fmt.Errorf("path must not be empty")
	}
	if !strings.HasPrefix(ssPath, "/") {
		return nil, fmt.Errorf("path %q must start with slash", ssPath)
	}
	if strings.HasSuffix(ssPath, "/") {
		return nil, fmt.Errorf("path %q must not end with slash", ssPath)
	}
	if strings.Contains(ssPath, "//") {
		return nil, fmt.Errorf("path %q must not contain two or more consecutive slashes", ssPath)
	}
	if strings.Contains(ssPath+"/", "/./") {
		return nil, fmt.Errorf("path %q must not contain \"/./\"", ssPath)
	}
	if strings.Contains(ssPath+"/", "/../") {
		return nil, fmt.Errorf("path %q must not contain \"/../\"", ssPath)
	}
	if ssPort == "" && !isDefault {
		return nil, fmt.Errorf("port must not be empty (if it is specified at all)", ssPort)
	}

	switch opts.Balancer {
	case enums.RandomBalancer:
		// pass
	case enums.RoundRobinBalancer:
		// pass
	default:
		return nil, fmt.Errorf("ZKResolver does not support %#v", opts.Balancer)
	}

	rng := opts.Random
	if rng == nil {
		rng = NewThreadSafeRandom(rand.Int63())
	}

	res := &zkResolver{
		ctx:          opts.Context,
		rng:          rng,
		zk:           opts.ZK,
		path:         ssPath,
		port:         ssPort,
		balancer:     opts.Balancer,
		childEventCh: make(chan zkEvent),
		childDoneCh:  make(chan struct{}),
		doneCh:       make(chan struct{}),
		watches:      make(map[WatchID]WatchFunc, 1),
		alive:        make(map[string]struct{}, 16),
		known:        make(map[string]struct{}, 16),
		down:         make(map[string]struct{}, 4),
		resolved:     make(map[string]zkAddrData, 16),
	}
	res.cv1 = sync.NewCond(&res.mu)
	res.cv2 = sync.NewCond(&res.mu)
	go res.watchThread()
	return res, nil
}

// type zkResolver {{{

type zkResolver struct {
	ctx          context.Context
	rng          *rand.Rand
	zk           *zkclient.Conn
	path         string
	port         string
	balancer     enums.BalancerType
	childEventCh chan zkEvent
	childDoneCh  chan struct{}
	doneCh       chan struct{}

	once     sync.Once
	wg       sync.WaitGroup
	mu       sync.Mutex
	cv1      *sync.Cond
	cv2      *sync.Cond
	nextRR   uint
	lastID   WatchID
	watches  map[WatchID]WatchFunc
	alive    map[string]struct{} // cv2
	known    map[string]struct{}
	down     map[string]struct{}
	resolved map[string]zkAddrData
	perm     []string
	err      multierror.Error
	ready    bool // cv1
	closed   bool // cv1
}

func (res *zkResolver) ServerHostname() string {
	return "localhost"
}

func (res *zkResolver) ResolveAll() ([]net.Addr, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv1.Wait()
	}

	all := make([]net.Addr, 0, len(res.resolved))
	for _, data := range res.resolved {
		if data.Addr != nil {
			all = append(all, data.Addr)
		}
	}
	tcpAddrList(all).Sort()
	return all, res.err.ErrorOrNil()
}

func (res *zkResolver) Resolve() (net.Addr, error) {
	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv1.Wait()
	}

	if len(res.resolved) == 0 {
		if err := res.err.ErrorOrNil(); err != nil {
			return nil, err
		}
	}

	switch res.balancer {
	case enums.RandomBalancer:
		candidates := make([]net.Addr, 0, len(res.resolved))
		for _, data := range res.resolved {
			if data.Addr != nil {
				addrKey := data.Addr.String()
				if _, found := res.down[addrKey]; !found {
					candidates = append(candidates, data.Addr)
				}
			}
		}
		if len(candidates) != 0 {
			res.err.Errors = nil
			index := res.rng.Intn(len(candidates))
			return candidates[index], nil
		}
		if err := res.err.ErrorOrNil(); err != nil {
			return nil, err
		}
		return nil, ErrNoHealthyBackends

	case enums.RoundRobinBalancer:
		for tries := uint(len(res.perm)); tries > 0; tries-- {
			k := res.nextRR
			res.nextRR = (res.nextRR + 1) % uint(len(res.perm))
			addr := res.resolved[res.perm[k]].Addr
			addrKey := addr.String()
			if _, found := res.down[addrKey]; !found {
				res.err.Errors = nil
				return addr, nil
			}
		}
		if err := res.err.ErrorOrNil(); err != nil {
			return nil, err
		}
		return nil, ErrNoHealthyBackends

	default:
		panic(fmt.Errorf("%#v not implemented", res.balancer))
	}
}

func (res *zkResolver) MarkHealthy(addr net.Addr, healthy bool) {
	addrKey := addr.String()

	res.mu.Lock()
	defer res.mu.Unlock()

	for !res.ready {
		res.cv1.Wait()
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

func (res *zkResolver) Watch(fn WatchFunc) WatchID {
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
		for pathKey, data := range res.resolved {
			ev := &Event{
				Type:     enums.UpdateEvent,
				Key:      pathKey,
				Addr:     data.Addr,
				Metadata: data.Raw,
			}
			if data.Addr == nil {
				ev.Type = enums.CorruptEvent
				ev.Err = data.Err
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

func (res *zkResolver) CancelWatch(id WatchID) {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	delete(res.watches, id)
}

func (res *zkResolver) Close() error {
	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return fs.ErrClosed
	}

	close(res.doneCh)

	for !res.closed {
		res.cv1.Wait()
	}

	return nil
}

func (res *zkResolver) watchThread() {
	defer res.doClose()

RetryFirst:
	children, _, zch, err := res.zk.ChildrenW(res.path)
	err = zkMapError(err)
	if err != nil {
		if res.sendError(err) {
			if res.sleep() {
				goto RetryFirst
			}
			res.makeLastErrorPermanent()
		}
		<-res.doneCh
		return
	}

	res.mu.Lock()
	for _, child := range children {
		childPath := path.Join(res.path, child)
		res.alive[childPath] = struct{}{}
		res.wg.Add(1)
		go res.watchChildThread(childPath)
	}
	res.mu.Unlock()

	for {
		select {
		case <-res.doneCh:
			return

		case <-res.ctx.Done():
			res.sendError(res.ctx.Err())
			<-res.doneCh
			return

		case cev := <-res.childEventCh:
			ev := &Event{Key: cev.Path}

			switch {
			case errors.Is(cev.Data.Err, fs.ErrNotExist):
				ev.Type = enums.DeleteEvent

				res.mu.Lock()
				oldData := res.resolved[cev.Path]
				delete(res.resolved, cev.Path)
				if oldData.Addr != nil {
					res.onChange(false)
				}
				res.mu.Unlock()

			case cev.Data.Err != nil:
				ev.Type = enums.CorruptEvent
				ev.Metadata = cev.Data.Raw
				ev.Err = cev.Data.Err

				res.mu.Lock()
				if oldData := res.resolved[cev.Path]; oldData.Addr == nil {
					res.resolved[cev.Path] = cev.Data
				}
				res.err.Errors = append(res.err.Errors, cev.Data.Err)
				res.ready = true
				res.cv1.Broadcast()
				res.mu.Unlock()

			default:
				ev.Type = enums.UpdateEvent
				ev.Addr = cev.Data.Addr
				ev.Metadata = cev.Data.Raw

				res.mu.Lock()
				if oldData, exists := res.resolved[cev.Path]; exists {
					oldAddr := oldData.Addr.(*net.TCPAddr)
					newAddr := cev.Data.Addr.(*net.TCPAddr)
					if tcpAddrCompare(*oldAddr, *newAddr) == 0 {
						ev.Type = enums.StatusChangeEvent
					}
				}
				res.resolved[cev.Path] = cev.Data
				if ev.Type == enums.UpdateEvent {
					res.onChange(true)
				}
				res.mu.Unlock()
			}

			events := []*Event{ev}
			res.doUpdate(events)

		case zev := <-zch:
			switch zev.Type {
			case zkclient.EventNodeChildrenChanged:
				// pass

			case zkclient.EventNodeDeleted:
				err = fmt.Errorf("node %q was deleted: %w", res.path, fs.ErrNotExist)
				res.sendError(err)
				<-res.doneCh
				return

			default:
				if zev.Err != nil {
					err = zkMapError(zev.Err)
					if !res.sendError(err) {
						<-res.doneCh
						return
					}
				}
			}

		RetrySubsequent:
			children, _, zch, err = res.zk.ChildrenW(res.path)
			err = zkMapError(err)
			if err != nil {
				if res.sendError(err) {
					if res.sleep() {
						goto RetrySubsequent
					}
					res.makeLastErrorPermanent()
				}
				<-res.doneCh
				return
			}

			res.mu.Lock()
			for _, child := range children {
				childPath := path.Join(res.path, child)
				if _, exists := res.alive[childPath]; !exists {
					res.alive[childPath] = struct{}{}
					res.wg.Add(1)
					go res.watchChildThread(childPath)
				}
			}
			res.mu.Unlock()
		}
	}
}

func (res *zkResolver) watchChildThread(myPath string) {
	defer func() {
		res.mu.Lock()
		delete(res.alive, myPath)
		if len(res.alive) <= 0 {
			res.cv2.Broadcast()
		}
		res.mu.Unlock()
		res.wg.Done()
	}()

	var myData zkAddrData

RetryFirst:
	raw, _, zch, err := res.zk.GetW(myPath)
	err = zkMapError(err)
	if err != nil {
		myData = zkAddrData{Err: err}
		res.childEventCh <- zkEvent{Path: myPath, Data: myData}
		if zkErrorIsTemporary(err) {
			if res.sleep() {
				goto RetryFirst
			}
		}
		return
	}

	res.parseServerSet(raw, &myData)
	res.childEventCh <- zkEvent{Path: myPath, Data: myData}

	for {
		select {
		case <-res.childDoneCh:
			return

		case zev := <-zch:
			switch zev.Type {
			case zkclient.EventNodeDataChanged:
				// pass

			case zkclient.EventNodeDeleted:
				myData = zkAddrData{Err: fs.ErrNotExist}
				res.childEventCh <- zkEvent{Path: myPath, Data: myData}
				return

			default:
				if zev.Err != nil {
					err = zkMapError(zev.Err)
					myData = zkAddrData{Err: err}
					res.childEventCh <- zkEvent{Path: myPath, Data: myData}
					if !zkErrorIsTemporary(err) {
						return
					}
				}
			}

		RetrySubsequent:
			raw, _, zch, err = res.zk.GetW(myPath)
			err = zkMapError(err)
			if err != nil {
				myData = zkAddrData{Err: err}
				res.childEventCh <- zkEvent{Path: myPath, Data: myData}
				if zkErrorIsTemporary(err) {
					if res.sleep() {
						goto RetrySubsequent
					}
				}
				return
			}

			res.parseServerSet(raw, &myData)
			res.childEventCh <- zkEvent{Path: myPath, Data: myData}
		}
	}
}

func (res *zkResolver) doClose() {
	res.once.Do(res.doShutdown)

	res.mu.Lock()
	res.watches = nil
	res.known = nil
	res.down = nil
	res.resolved = nil
	res.perm = nil
	res.err.Errors = []error{fs.ErrClosed}
	res.ready = true
	res.closed = true
	res.cv1.Broadcast()
	res.mu.Unlock()

	res.wg.Wait()
}

func (res *zkResolver) sendError(err error) bool {
	if err == nil {
		panic(fmt.Errorf("err is nil"))
	}

	events := []*Event{
		{
			Type: enums.ErrorEvent,
			Err:  err,
		},
	}

	if zkErrorIsTemporary(err) {
		res.mu.Lock()
		if !res.closed {
			res.err.Errors = append(res.err.Errors, err)
		}
		res.mu.Unlock()
		res.doUpdate(events)
		return true
	}

	res.once.Do(res.doShutdown)
	res.mu.Lock()
	if !res.closed {
		res.err.Errors = append(res.err.Errors, err)
		res.ready = true
		res.cv1.Broadcast()
	}
	res.mu.Unlock()
	res.doUpdate(events)
	return false
}

func (res *zkResolver) makeLastErrorPermanent() {
	res.once.Do(res.doShutdown)
	res.mu.Lock()
	res.ready = true
	res.cv1.Broadcast()
	res.mu.Unlock()
}

func (res *zkResolver) doShutdown() {
	close(res.childDoneCh)

	res.wg.Add(1)
	go func() {
		for range res.childEventCh {
		}
		res.wg.Done()
	}()

	res.wg.Add(1)
	go func() {
		res.mu.Lock()
		for len(res.alive) > 0 {
			res.cv2.Wait()
		}
		res.mu.Unlock()
		close(res.childEventCh)
		res.wg.Done()
	}()
}

func (res *zkResolver) doUpdate(events []*Event) {
	if len(events) == 0 {
		return
	}

	res.mu.Lock()
	defer res.mu.Unlock()

	if res.closed {
		return
	}

	for _, fn := range res.watches {
		fn(events)
	}
}

func (res *zkResolver) onChange(makeReady bool) {
	// Precondition: res.mu is held

	for addrKey := range res.known {
		delete(res.known, addrKey)
	}

	for _, data := range res.resolved {
		if data.Addr != nil {
			addrKey := data.Addr.String()
			res.known[addrKey] = struct{}{}
		}
	}

	for addrKey := range res.down {
		if _, found := res.known[addrKey]; !found {
			delete(res.down, addrKey)
		}
	}

	if res.balancer == enums.RoundRobinBalancer {
		keys := make([]string, 0, len(res.resolved))
		for pathKey, data := range res.resolved {
			if data.Addr != nil {
				keys = append(keys, pathKey)
			}
		}
		res.rng.Shuffle(len(keys), func(i, j int) {
			keys[i], keys[j] = keys[j], keys[i]
		})
		res.nextRR = 0
		res.perm = keys
	}

	if makeReady {
		res.ready = true
		res.cv1.Broadcast()
	}
}

func (res *zkResolver) parseServerSet(raw []byte, out *zkAddrData) {
	*out = zkAddrData{}

	if err := json.Unmarshal(raw, &out.Raw); err != nil {
		out.Err = err
		return
	}

	if out.Raw.Status != StatusAlive {
		out.Err = fmt.Errorf("status is %v", out.Raw.Status)
		return
	}

	var (
		endpoint ServerSetEndpoint
		found    bool
	)
	if res.port == "" {
		endpoint, found = out.Raw.ServiceEndpoint, true
	} else {
		endpoint, found = out.Raw.AdditionalEndpoints[res.port]
	}
	if !found {
		out.Err = fmt.Errorf("no such named port %q", res.port)
		return
	}

	var (
		ipStr string
		zone  string
	)
	if i := strings.IndexByte(endpoint.Host, '%'); i >= 0 {
		ipStr, zone = endpoint.Host[:i], endpoint.Host[i+1:]
	} else {
		ipStr = endpoint.Host
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		out.Err = fmt.Errorf("invalid IP %q", ipStr)
		return
	}

	if endpoint.Port == 0 {
		out.Err = fmt.Errorf("invalid port 0")
		return
	}

	out.Addr = &net.TCPAddr{
		IP:   ip,
		Port: int(endpoint.Port),
		Zone: zone,
	}
}

func (res *zkResolver) sleep() bool {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case <-res.doneCh:
		return false
	case <-res.childDoneCh:
		return false
	case <-res.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

var _ Resolver = (*zkResolver)(nil)

// }}}

func zkMapError(err error) error {
	switch err {
	case zkclient.ErrConnectionClosed:
		return fs.ErrClosed
	case zkclient.ErrNodeExists:
		return fs.ErrExist
	case zkclient.ErrNoNode:
		return fs.ErrNotExist
	default:
		return err
	}
}

func zkErrorIsTemporary(err error) bool {
	switch err {
	case zkclient.ErrSessionExpired:
		return true
	case zkclient.ErrSessionMoved:
		return true
	default:
		return false
	}
}

type zkEvent struct {
	Path string
	Data zkAddrData
}

type zkAddrData struct {
	Raw  ServerSetMember
	Addr net.Addr
	Err  error
}
