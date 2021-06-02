package roxyresolver

import (
	"context"
	"errors"
	"io/fs"
	"math/rand"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// ZKTarget represents a parsed target spec for the "zk" scheme.
type ZKTarget struct {
	Path       string
	Port       string
	ServerName string
	Balancer   BalancerType
}

// FromTarget breaks apart a Target into component data.
func (t *ZKTarget) FromTarget(rt Target) error {
	*t = ZKTarget{}

	wantZero := true
	defer func() {
		if wantZero {
			*t = ZKTarget{}
		}
	}()

	if rt.Authority != "" {
		err := roxyutil.AuthorityError{Authority: rt.Authority, Err: roxyutil.ErrExpectEmpty}
		return err
	}

	pathAndPort := rt.Endpoint
	if pathAndPort == "" {
		err := roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return err
	}

	var hasPort bool
	if i := strings.IndexByte(pathAndPort, ':'); i >= 0 {
		t.Path, t.Port, hasPort = pathAndPort[:i], pathAndPort[i+1:], true
	} else {
		t.Path, t.Port, hasPort = pathAndPort, "", false
	}

	err := roxyutil.ValidateZKPath(t.Path)
	if err != nil {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return err
	}

	if hasPort {
		err = roxyutil.ValidateNamedPort(t.Port)
		if err != nil {
			err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
			return err
		}
	}

	t.ServerName = rt.Query.Get("serverName")

	if str := rt.Query.Get("balancer"); str != "" {
		err = t.Balancer.Parse(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "balancer", Value: str, Err: err}
			return err
		}
	}

	wantZero = false
	return nil
}

// AsTarget recombines the component data into a Target.
func (t ZKTarget) AsTarget() Target {
	query := make(url.Values, 2)
	query.Set("balancer", t.Balancer.String())
	if t.ServerName != "" {
		query.Set("serverName", t.ServerName)
	}

	var endpoint string
	endpoint = t.Path
	if t.Port != "" {
		endpoint = t.Path + ":" + t.Port
	}

	return Target{
		Scheme:     constants.SchemeZK,
		Endpoint:   endpoint,
		Query:      query,
		ServerName: t.ServerName,
		HasSlash:   true,
	}
}

// NewZKBuilder constructs a new gRPC resolver.Builder for the "zk" scheme.
func NewZKBuilder(ctx context.Context, rng *rand.Rand, zkConn *zk.Conn, serviceConfigJSON string) resolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if zkConn == nil {
		panic(errors.New("*zk.Conn is nil"))
	}
	return zkBuilder{ctx, rng, zkConn, serviceConfigJSON}
}

// NewZKResolver constructs a new Resolver for the "zk" scheme.
func NewZKResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	zkConn := GetZKConn(opts.Context)
	if zkConn == nil {
		panic(errors.New("*zk.Conn is nil"))
	}

	var t ZKTarget
	err := t.FromTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    t.Balancer,
		ResolveFunc: MakeZKResolveFunc(zkConn, t),
	})
}

// MakeZKResolveFunc constructs a WatchingResolveFunc for building your own
// custom WatchingResolver with the "zk" scheme.
func MakeZKResolveFunc(zkConn *zk.Conn, t ZKTarget) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, backoff expbackoff.ExpBackoff) (<-chan []Event, error) {
		children, _, zch, err := zkConn.ChildrenW(t.Path)
		err = MapZKError(err)
		if err != nil {
			return nil, err
		}

		childEventCh := make(chan Event)
		childDoneCh := make(chan struct{})
		ch := make(chan []Event)
		alive := make(map[string]struct{}, 16)

		for _, child := range children {
			childPath := path.Join(t.Path, child)
			alive[childPath] = struct{}{}
			wg.Add(1)
			go zkChildThread(ctx, wg, backoff, zkConn, t.Port, t.ServerName, childEventCh, childDoneCh, childPath)
		}

		wg.Add(1)
		go zkParentThread(ctx, wg, backoff, zkConn, t.Path, t.Port, t.ServerName, zch, childEventCh, childDoneCh, ch, alive)

		return ch, nil
	}
}

// MapZKError converts a ZooKeeper-specific error to a generic error.
func MapZKError(err error) error {
	switch err {
	case zk.ErrConnectionClosed:
		return fs.ErrClosed
	case zk.ErrClosing:
		return fs.ErrClosed
	case zk.ErrNodeExists:
		return fs.ErrExist
	case zk.ErrNoNode:
		return fs.ErrNotExist
	default:
		return err
	}
}

// type zkBuilder {{{

type zkBuilder struct {
	ctx               context.Context
	rng               *rand.Rand
	zkConn            *zk.Conn
	serviceConfigJSON string
}

func (b zkBuilder) Scheme() string {
	return constants.SchemeZK
}

func (b zkBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	var rt Target
	if err := rt.FromGRPCTarget(target); err != nil {
		return nil, err
	}

	var t ZKTarget
	err := t.FromTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		ResolveFunc:       MakeZKResolveFunc(b.zkConn, t),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

// }}}

func zkIsChildExit(ev Event) (pathKey string, ok bool) {
	if ev.Type == ErrorEvent {
		if xerr, ok2 := ev.Err.(childExitError); ok2 {
			pathKey = xerr.Path
			ok = true
		}
	}
	return
}

func zkIsFatalError(err error) bool {
	return errors.Is(err, fs.ErrClosed) || errors.Is(err, fs.ErrNotExist)
}

func zkParentThread(
	ctx context.Context,
	wg *sync.WaitGroup,
	backoff expbackoff.ExpBackoff,
	zkConn *zk.Conn,
	zkPath string,
	zkPort string,
	serverName string,
	zch <-chan zk.Event,
	childEventCh chan Event,
	childDoneCh chan struct{},
	ch chan<- []Event,
	alive map[string]struct{},
) {
	defer func() {
		close(ch)
		close(childDoneCh)
		for len(alive) != 0 {
			ev := <-childEventCh
			if pathKey, ok := zkIsChildExit(ev); ok {
				delete(alive, pathKey)
			}
		}
		close(childEventCh)
		wg.Done()
	}()

	var children []string
	var err error

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-childEventCh:
			if pathKey, ok := zkIsChildExit(ev); ok {
				delete(alive, pathKey)
			} else {
				ch <- []Event{ev}
			}
		case zev := <-zch:
			switch zev.Type {
			case zk.EventNodeChildrenChanged:
				// pass

			case zk.EventNodeDeleted:
				err = NodeDeletedError{Path: zkPath}
				ch <- []Event{
					{
						Type: ErrorEvent,
						Err:  err,
					},
				}
				return

			default:
				err = MapZKError(zev.Err)
				if err != nil {
					ch <- []Event{
						{
							Type: ErrorEvent,
							Err:  err,
						},
					}
					return
				}
			}

			children, _, zch, err = zkConn.ChildrenW(zkPath)
			err = MapZKError(err)
			if err != nil {
				ch <- []Event{
					{
						Type: ErrorEvent,
						Err:  err,
					},
				}
				return
			}

			for _, child := range children {
				childPath := path.Join(zkPath, child)
				if _, exists := alive[childPath]; !exists {
					alive[childPath] = struct{}{}
					wg.Add(1)
					go zkChildThread(ctx, wg, backoff, zkConn, zkPort, serverName, childEventCh, childDoneCh, childPath)
				}
			}
		}
	}
}

func zkChildThread(
	ctx context.Context,
	wg *sync.WaitGroup,
	backoff expbackoff.ExpBackoff,
	zkConn *zk.Conn,
	zkPort string,
	serverName string,
	childEventCh chan<- Event,
	childDoneCh <-chan struct{},
	myPath string,
) {
	defer func() {
		err := childExitError{Path: myPath}
		childEventCh <- Event{
			Type: ErrorEvent,
			Err:  err,
		}
		wg.Done()
	}()

	var raw []byte
	var zch <-chan zk.Event
	var err error

	retries := 0
	childSleep := func() bool {
		t := time.NewTimer(backoff.Backoff(retries))
		retries++
		select {
		case <-ctx.Done():
			t.Stop()
			return false
		case <-childDoneCh:
			t.Stop()
			return false
		case <-t.C:
			return true
		}
	}

	for {
		for {
			raw, _, zch, err = zkConn.GetW(myPath)
			err = MapZKError(err)
			if err == nil {
				break
			}
			if zkIsFatalError(err) {
				childEventCh <- Event{
					Type: DeleteEvent,
					Key:  myPath,
				}
				return
			}
			childEventCh <- Event{
				Type: BadDataEvent,
				Key:  myPath,
				Data: Resolved{
					UniqueID: myPath,
					Err:      err,
				},
			}
			if !childSleep() {
				return
			}
		}

		retries = 0
		childEventCh <- parseMembershipData(zkPort, serverName, myPath, raw)

		select {
		case <-ctx.Done():
			return
		case <-childDoneCh:
			return
		case zev := <-zch:
			switch zev.Type {
			case zk.EventNodeDataChanged:
				// pass
			case zk.EventNodeDeleted:
				childEventCh <- Event{
					Type: DeleteEvent,
					Key:  myPath,
				}
				return
			default:
				err = MapZKError(zev.Err)
				if err != nil {
					if zkIsFatalError(err) {
						childEventCh <- Event{
							Type: DeleteEvent,
							Key:  myPath,
						}
						return
					}
					childEventCh <- Event{
						Type: BadDataEvent,
						Key:  myPath,
						Data: Resolved{
							UniqueID: myPath,
							Err:      err,
						},
					}
					if !childSleep() {
						return
					}
				}
			}
		}
	}
}
