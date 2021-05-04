package roxyresolver

import (
	"context"
	"errors"
	"math/rand"
	"net/url"
	"strings"
	"sync"

	"go.etcd.io/etcd/api/v3/mvccpb"
	v3 "go.etcd.io/etcd/client/v3"
	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/expbackoff"
)

func NewEtcdBuilder(ctx context.Context, rng *rand.Rand, etcd *v3.Client, serviceConfigJSON string) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	if etcd == nil {
		panic(errors.New("etcd is nil"))
	}
	return etcdBuilder{ctx, rng, etcd, serviceConfigJSON}
}

func NewEtcdResolver(opts Options) (Resolver, error) {
	if opts.Etcd == nil {
		panic(errors.New("Etcd is nil"))
	}

	etcdPrefix, etcdPort, balancer, err := ParseEtcdTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeEtcdResolveFunc(opts.Etcd, etcdPrefix, etcdPort),
	})
}

func ParseEtcdTarget(target Target) (etcdPrefix string, etcdPort string, balancer BalancerType, err error) {
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
		etcdPrefix, etcdPort, hasPort = unescaped[:i], unescaped[i+1:], true
	} else {
		etcdPrefix, etcdPort, hasPort = unescaped, "", false
	}

	if !strings.HasSuffix(etcdPrefix, "/") {
		etcdPrefix += "/"
	}
	if strings.Contains(etcdPrefix, "//") {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: etcdPrefix,
				Err:  ErrExpectNoDoubleSlash,
			},
		}
		return
	}

	if hasPort && etcdPort == "" {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPortError{
				Port: etcdPort,
				Err:  ErrExpectNonEmpty,
			},
		}
		return
	}
	if hasPort && !reNamedPort.MatchString(etcdPort) {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPortError{
				Port: etcdPort,
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

func MakeEtcdResolveFunc(etcdClient *v3.Client, etcdPrefix string, etcdPort string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []Event, error) {
		ch := make(chan []Event)

		resp, err := etcdClient.KV.Get(
			ctx,
			etcdPrefix,
			v3.WithPrefix(),
			v3.WithSerializable())
		err = etcdMapError(err)
		if err != nil {
			close(ch)
			return nil, err
		}

		wch := etcdClient.Watcher.Watch(
			ctx,
			etcdPrefix,
			v3.WithPrefix(),
			v3.WithRev(resp.Header.Revision+1))

		wg.Add(1)
		go func() {
			defer func() {
				close(ch)
				wg.Done()
			}()

			if len(resp.Kvs) != 0 {
				events := make([]Event, 0, len(resp.Kvs))
				for _, kv := range resp.Kvs {
					if ev := etcdMapEvent(etcdPort, v3.EventTypePut, kv); ev.Type != NoOpEvent {
						events = append(events, ev)
					}
				}
				ch <- events
			}

			for {
				select {
				case <-ctx.Done():
					return

				case wresp, ok := <-wch:
					if !ok {
						return
					}
					n := len(wresp.Events)
					err := etcdMapError(wresp.Err())
					if err != nil {
						n++
					}
					events := make([]Event, 0, n)
					for _, wev := range wresp.Events {
						if ev := etcdMapEvent(etcdPort, wev.Type, wev.Kv); ev.Type != NoOpEvent {
							events = append(events, ev)
						}
					}
					if err != nil {
						ev := Event{
							Type: ErrorEvent,
							Err:  err,
						}
						events = append(events, ev)
					}
					ch <- events
				}
			}
		}()

		return ch, nil
	}
}

// type etcdBuilder {{{

type etcdBuilder struct {
	ctx               context.Context
	rng               *rand.Rand
	etcd              *v3.Client
	serviceConfigJSON string
}

func (b etcdBuilder) Scheme() string {
	return etcdScheme
}

func (b etcdBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	etcdPrefix, etcdPort, _, err := ParseEtcdTarget(target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		ResolveFunc:       MakeEtcdResolveFunc(b.etcd, etcdPrefix, etcdPort),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

// }}}

func etcdMapError(err error) error {
	return err
}

func etcdMapEvent(portName string, t mvccpb.Event_EventType, kv *mvccpb.KeyValue) Event {
	key := string(kv.Key)
	switch t {
	case v3.EventTypePut:
		return ParseServerSetData(portName, key, kv.Value)

	case v3.EventTypeDelete:
		return Event{
			Type: DeleteEvent,
			Key:  key,
		}

	default:
		return Event{
			Type: NoOpEvent,
		}
	}
}
