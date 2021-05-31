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
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// EtcdTarget represents a parsed target spec for the "etcd" scheme.
type EtcdTarget struct {
	Path       string
	Port       string
	ServerName string
	Balancer   BalancerType
}

// FromTarget breaks apart a Target into component data.
func (t *EtcdTarget) FromTarget(rt Target) error {
	*t = EtcdTarget{}

	wantZero := true
	defer func() {
		if wantZero {
			*t = EtcdTarget{}
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

	if !strings.HasSuffix(t.Path, "/") {
		t.Path += "/"
	}
	err := roxyutil.ValidateEtcdPath(t.Path)
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
func (t EtcdTarget) AsTarget() Target {
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
		Scheme:     constants.SchemeEtcd,
		Endpoint:   endpoint,
		Query:      query,
		ServerName: t.ServerName,
		HasSlash:   true,
	}
}

// NewEtcdBuilder constructs a new gRPC resolver.Builder for the "etcd" scheme.
func NewEtcdBuilder(ctx context.Context, rng *rand.Rand, etcd *v3.Client, serviceConfigJSON string) resolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}
	return etcdBuilder{ctx, rng, etcd, serviceConfigJSON}
}

// NewEtcdResolver constructs a new Resolver for the "etcd" scheme.
func NewEtcdResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	etcd := GetEtcdV3Client(opts.Context)
	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}

	var t EtcdTarget
	err := t.FromTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    t.Balancer,
		ResolveFunc: MakeEtcdResolveFunc(etcd, t),
	})
}

// MakeEtcdResolveFunc constructs a WatchingResolveFunc for building your own
// custom WatchingResolver with the "etcd" scheme.
func MakeEtcdResolveFunc(etcdClient *v3.Client, t EtcdTarget) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []Event, error) {
		ch := make(chan []Event)

		resp, err := etcdClient.KV.Get(
			ctx,
			t.Path,
			v3.WithPrefix(),
			v3.WithSerializable())
		err = MapEtcdError(err)
		if err != nil {
			close(ch)
			return nil, err
		}

		wch := etcdClient.Watcher.Watch(
			ctx,
			t.Path,
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
					if ev := etcdMapEvent(t.Port, t.ServerName, v3.EventTypePut, kv); ev.Type != NoOpEvent {
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
						if ev := etcdMapEvent(t.Port, t.ServerName, wev.Type, wev.Kv); ev.Type != NoOpEvent {
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

// MapEtcdError converts an etcd.io-specific error to a generic error.
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
	return constants.SchemeEtcd
}

func (b etcdBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	var rt Target
	if err := rt.FromGRPCTarget(target); err != nil {
		return nil, err
	}

	var t EtcdTarget
	err := t.FromTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		ResolveFunc:       MakeEtcdResolveFunc(b.etcd, t),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

// }}}

func etcdMapEvent(namedPort string, serverName string, t mvccpb.Event_EventType, kv *mvccpb.KeyValue) Event {
	key := string(kv.Key)
	switch t {
	case v3.EventTypePut:
		return parseMembershipData(namedPort, serverName, key, kv.Value)

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
