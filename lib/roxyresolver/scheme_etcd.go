package roxyresolver

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"sync"

	"go.etcd.io/etcd/api/v3/mvccpb"
	v3 "go.etcd.io/etcd/client/v3"
	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/expbackoff"
)

func NewEtcdBuilder(ctx context.Context, rng *rand.Rand, etcd *v3.Client, serviceConfigJSON string) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}
	return etcdBuilder{ctx, rng, etcd, serviceConfigJSON}
}

func NewEtcdResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	etcd := GetEtcdV3Client(opts.Context)
	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}

	etcdPath, etcdPort, balancer, serverName, err := ParseEtcdTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeEtcdResolveFunc(etcd, etcdPath, etcdPort, serverName),
	})
}

func ParseEtcdTarget(rt RoxyTarget) (etcdPath string, etcdPort string, balancer BalancerType, serverName string, err error) {
	if rt.Authority != "" {
		err = BadAuthorityError{Authority: rt.Authority, Err: ErrExpectEmpty}
		return
	}

	pathAndPort := rt.Endpoint
	if pathAndPort == "" {
		err = BadEndpointError{Endpoint: rt.Endpoint, Err: ErrExpectNonEmpty}
		return
	}

	var hasPort bool
	if i := strings.IndexByte(pathAndPort, ':'); i >= 0 {
		etcdPath, etcdPort, hasPort = pathAndPort[:i], pathAndPort[i+1:], true
	} else {
		etcdPath, etcdPort, hasPort = pathAndPort, "", false
	}

	if !strings.HasSuffix(etcdPath, "/") {
		etcdPath += "/"
	}
	err = ValidateEtcdPath(etcdPath)
	if err != nil {
		err = BadEndpointError{Endpoint: rt.Endpoint, Err: err}
		return
	}
	if hasPort {
		err = ValidateServerSetPort(etcdPort)
		if err != nil {
			err = BadEndpointError{Endpoint: rt.Endpoint, Err: err}
			return
		}
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = BadQueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	serverName = rt.Query.Get("serverName")

	return
}

func ValidateEtcdPath(etcdPath string) error {
	if !strings.HasSuffix(etcdPath, "/") {
		return BadPathError{Path: etcdPath, Err: ErrExpectTrailingSlash}
	}
	if strings.Contains(etcdPath, "//") {
		return BadPathError{Path: etcdPath, Err: ErrExpectNoDoubleSlash}
	}
	if strings.Contains("/"+etcdPath, "/./") {
		return BadPathError{Path: etcdPath, Err: ErrExpectNoDot}
	}
	if strings.Contains("/"+etcdPath, "/../") {
		return BadPathError{Path: etcdPath, Err: ErrExpectNoDotDot}
	}
	return nil
}

func MakeEtcdResolveFunc(etcdClient *v3.Client, etcdPath string, etcdPort string, serverName string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []Event, error) {
		ch := make(chan []Event)

		resp, err := etcdClient.KV.Get(
			ctx,
			etcdPath,
			v3.WithPrefix(),
			v3.WithSerializable())
		err = MapEtcdError(err)
		if err != nil {
			close(ch)
			return nil, err
		}

		wch := etcdClient.Watcher.Watch(
			ctx,
			etcdPath,
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
					if ev := etcdMapEvent(etcdPort, serverName, v3.EventTypePut, kv); ev.Type != NoOpEvent {
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
					err := MapEtcdError(wresp.Err())
					if err != nil {
						n++
					}
					events := make([]Event, 0, n)
					for _, wev := range wresp.Events {
						if ev := etcdMapEvent(etcdPort, serverName, wev.Type, wev.Kv); ev.Type != NoOpEvent {
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

func MapEtcdError(err error) error {
	return err
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
	rt, err := RoxyTargetFromTarget(target)
	if err != nil {
		return nil, err
	}

	etcdPath, etcdPort, _, serverName, err := ParseEtcdTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		ResolveFunc:       MakeEtcdResolveFunc(b.etcd, etcdPath, etcdPort, serverName),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

// }}}

func etcdMapEvent(portName string, serverName string, t mvccpb.Event_EventType, kv *mvccpb.KeyValue) Event {
	key := string(kv.Key)
	switch t {
	case v3.EventTypePut:
		return ParseServerSetData(portName, serverName, key, kv.Value)

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
