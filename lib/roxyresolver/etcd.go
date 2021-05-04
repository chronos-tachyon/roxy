package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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

	etcdPrefix, etcdPort, query, err := ParseEtcdTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	var balancer BalancerType
	if str := query.Get("balancer"); str != "" {
		if err = balancer.Parse(str); err != nil {
			return nil, fmt.Errorf("failed to parse balancer=%q query string: %w", str, err)
		}
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeEtcdResolveFunc(opts.Etcd, etcdPrefix, etcdPort),
	})
}

func MakeEtcdResolveFunc(etcdClient *v3.Client, etcdPrefix string, etcdPort string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []*Event, error) {
		ch := make(chan []*Event)

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
				events := make([]*Event, 0, len(resp.Kvs))
				for _, kv := range resp.Kvs {
					if ev := etcdMapEvent(etcdPort, v3.EventTypePut, kv); ev != nil {
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
					events := make([]*Event, 0, n)
					for _, wev := range wresp.Events {
						if ev := etcdMapEvent(etcdPort, wev.Type, wev.Kv); ev != nil {
							events = append(events, ev)
						}
					}
					if err != nil {
						ev := &Event{
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
	return "etcd"
}

func (b etcdBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	etcdPrefix, etcdPort, query, err := ParseEtcdTarget(target)
	if err != nil {
		return nil, err
	}

	_ = query // for future use

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

func etcdMapEvent(portName string, t mvccpb.Event_EventType, kv *mvccpb.KeyValue) *Event {
	key := string(kv.Key)
	switch t {
	case v3.EventTypePut:
		return ParseServerSetData(portName, key, kv.Value)

	case v3.EventTypeDelete:
		return &Event{
			Type: DeleteEvent,
			Key:  key,
		}

	default:
		return nil
	}
}
