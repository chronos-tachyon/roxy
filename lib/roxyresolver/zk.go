package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/expbackoff"
)

func NewZKBuilder(ctx context.Context, rng *rand.Rand, zkconn *zk.Conn, serviceConfigJSON string) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if zkconn == nil {
		panic(errors.New("zkconn is nil"))
	}
	return zkBuilder{ctx, rng, zkconn, serviceConfigJSON}
}

func NewZKResolver(opts Options) (Resolver, error) {
	if opts.ZK == nil {
		panic(errors.New("ZK is nil"))
	}

	zkPath, zkPort, balancer, err := ParseZKTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeZKResolveFunc(opts.ZK, zkPath, zkPort),
	})
}

func ParseZKTarget(target Target) (zkPath string, zkPort string, balancer BalancerType, err error) {
	if target.Authority != "" {
		err = BadAuthorityError{Authority: target.Authority, Err: ErrExpectEmpty}
		return
	}

	ep := target.Endpoint
	if ep == "" {
		err = BadEndpointError{Endpoint: target.Endpoint, Err: ErrExpectNonEmpty}
		return
	}

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	if ep == "" {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err:      BadPathError{Path: ep, Err: ErrExpectNonEmpty},
		}
		return
	}

	unescaped, err := url.PathUnescape(ep)
	if err != nil {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err:      BadPathError{Path: ep, Err: err},
		}
		return
	}

	var hasPort bool
	if i := strings.IndexByte(unescaped, ':'); i >= 0 {
		zkPath, zkPort, hasPort = unescaped[:i], unescaped[i+1:], false
	} else {
		zkPath, zkPort, hasPort = unescaped, "", true
	}

	if !strings.HasPrefix(zkPath, "/") {
		zkPath = "/" + zkPath
	}
	if zkPath != "/" && strings.HasSuffix(zkPath, "/") {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: zkPath,
				Err:  ErrExpectNoEndSlash,
			},
		}
		return
	}
	if strings.Contains(zkPath, "//") {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: zkPath,
				Err:  ErrExpectNoDoubleSlash,
			},
		}
		return
	}
	if strings.Contains(zkPath+"/", "/./") {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: zkPath,
				Err:  ErrExpectNoDot,
			},
		}
		return
	}
	if strings.Contains(zkPath+"/", "/../") {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: zkPath,
				Err:  ErrExpectNoDotDot,
			},
		}
		return
	}

	if hasPort && zkPort == "" {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPortError{
				Port: zkPort,
				Err:  ErrExpectNonEmpty,
			},
		}
		return
	}
	if hasPort && !reNamedPort.MatchString(zkPort) {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPortError{
				Port: zkPort,
				Err:  ErrFailedToMatch,
			},
		}
		return
	}

	var query url.Values
	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err:      BadQueryStringError{QueryString: qs, Err: err},
			}
			return
		}
	}

	if str := query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err: BadQueryStringError{
					QueryString: qs,
					Err: BadQueryParamError{
						Name:  "balancer",
						Value: str,
						Err:   err,
					},
				},
			}
			return
		}
	}

	return
}

func MakeZKResolveFunc(zkconn *zk.Conn, zkPath string, zkPort string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, backoff expbackoff.ExpBackoff) (<-chan []Event, error) {
		var children []string
		var zch <-chan zk.Event
		var err error

		children, _, zch, err = zkconn.ChildrenW(zkPath)
		err = zkMapError(err)
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
					err = zkMapError(err)
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
				childEventCh <- ParseServerSetData(zkPort, myPath, raw)

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
						err = zkMapError(zev.Err)
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
						err = zkMapError(zev.Err)
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
					err = zkMapError(err)
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
	zkPath, zkPort, _, err := ParseZKTarget(target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		ResolveFunc:       MakeZKResolveFunc(b.zkconn, zkPath, zkPort),
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

func zkMapError(err error) error {
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
