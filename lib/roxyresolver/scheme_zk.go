package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func NewZKBuilder(ctx context.Context, rng *rand.Rand, zkconn *zk.Conn, serviceConfigJSON string) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if zkconn == nil {
		panic(errors.New("zkconn is nil"))
	}
	return zkBuilder{ctx, rng, zkconn, serviceConfigJSON}
}

func NewZKResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	zkconn := GetZKConn(opts.Context)
	if zkconn == nil {
		panic(errors.New("*zk.Conn is nil"))
	}

	zkPath, zkPort, balancer, serverName, err := ParseZKTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeZKResolveFunc(zkconn, zkPath, zkPort, serverName),
	})
}

func ParseZKTarget(rt RoxyTarget) (zkPath string, zkPort string, balancer BalancerType, serverName string, err error) {
	if rt.Authority != "" {
		err = roxyutil.BadAuthorityError{Authority: rt.Authority, Err: roxyutil.ErrExpectEmpty}
		return
	}

	pathAndPort := rt.Endpoint
	if pathAndPort == "" {
		err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}

	var hasPort bool
	if i := strings.IndexByte(pathAndPort, ':'); i >= 0 {
		zkPath, zkPort, hasPort = pathAndPort[:i], pathAndPort[i+1:], false
	} else {
		zkPath, zkPort, hasPort = pathAndPort, "", true
	}

	if !strings.HasPrefix(zkPath, "/") {
		zkPath = "/" + zkPath
	}
	err = roxyutil.ValidateZKPath(zkPath)
	if err != nil {
		err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: err}
		return
	}
	if hasPort {
		err = roxyutil.ValidateNamedPort(zkPort)
		if err != nil {
			err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: err}
			return
		}
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = roxyutil.BadQueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	serverName = rt.Query.Get("serverName")

	return
}

//nolint:gocyclo
func MakeZKResolveFunc(zkconn *zk.Conn, zkPath string, zkPort string, serverName string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, backoff expbackoff.ExpBackoff) (<-chan []Event, error) {
		var children []string
		var zch <-chan zk.Event
		var err error

		children, _, zch, err = zkconn.ChildrenW(zkPath)
		err = MapZKError(err)
		if err != nil {
			return nil, err
		}

		childEventCh := make(chan Event)
		childDoneCh := make(chan struct{})

		// begin child thread body
		// {{{

		childThread := func(myPath string) {
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
					raw, _, zch, err = zkconn.GetW(myPath)
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
							Unique: myPath,
							Err:    err,
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
									Unique: myPath,
									Err:    err,
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

		// end child thread body
		// }}}

		ch := make(chan []Event)
		alive := make(map[string]struct{}, 16)

		for _, child := range children {
			childPath := path.Join(zkPath, child)
			alive[childPath] = struct{}{}
			wg.Add(1)
			go childThread(childPath)
		}

		// begin parent thread body
		// {{{

		wg.Add(1)
		go func() {
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
						err := fmt.Errorf("node %q was deleted: %w", zkPath, fs.ErrNotExist)
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

					children, _, zch, err = zkconn.ChildrenW(zkPath)
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
							go childThread(childPath)
						}
					}
				}
			}
		}()

		// end parent thread body
		// }}}

		return ch, nil
	}
}

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
	zkconn            *zk.Conn
	serviceConfigJSON string
}

func (b zkBuilder) Scheme() string {
	return zkScheme
}

func (b zkBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	rt, err := RoxyTargetFromTarget(target)
	if err != nil {
		return nil, err
	}

	zkPath, zkPort, _, serverName, err := ParseZKTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		ResolveFunc:       MakeZKResolveFunc(b.zkconn, zkPath, zkPort, serverName),
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
