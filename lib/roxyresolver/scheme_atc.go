package roxyresolver

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sync"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// NewATCBuilder constructs a new gRPC resolver.Builder for the "atc" scheme.
func NewATCBuilder(ctx context.Context, rng *rand.Rand, client *atcclient.ATCClient) resolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if client == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}
	return atcBuilder{ctx, rng, client}
}

// NewATCResolver constructs a new Resolver for the "atc" scheme.
func NewATCResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	client := GetATCClient(opts.Context)
	if client == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}

	lbName, lbLocation, lbUnique, balancer, isDSC, serverName, err := ParseATCTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeATCResolveFunc(client, lbName, lbLocation, lbUnique, isDSC, serverName),
	})
}

// ParseATCTarget breaks apart a Target into component data.
func ParseATCTarget(rt Target) (lbName, lbLocation, lbUnique string, balancer BalancerType, isDSC bool, serverName string, err error) {
	if rt.Authority != "" {
		err = roxyutil.AuthorityError{Authority: rt.Authority, Err: roxyutil.ErrExpectEmpty}
		return
	}

	lbName = rt.Endpoint
	if lbName == "" {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}
	err = roxyutil.ValidateATCServiceName(lbName)
	if err != nil {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return
	}

	lbLocation = rt.Query.Get("location")
	if lbLocation == "" {
		lbLocation = rt.Query.Get("loc")
	}
	err = roxyutil.ValidateATCLocation(lbLocation)
	if err != nil {
		err = roxyutil.QueryParamError{Name: "location", Value: lbLocation, Err: err}
		return
	}

	lbUnique = rt.Query.Get("unique")
	err = roxyutil.ValidateATCUnique(lbUnique)
	if err != nil {
		err = roxyutil.QueryParamError{Name: "unique", Value: lbUnique, Err: err}
		return
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	if str := rt.Query.Get("disableServiceConfig"); str != "" {
		isDSC, err = misc.ParseBool(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "disableServiceConfig", Value: str, Err: err}
			return
		}
	}

	serverName = rt.Query.Get("serverName")

	return
}

// MakeATCResolveFunc constructs a WatchingResolveFunc for building your own
// custom WatchingResolver with the "atc" scheme.
func MakeATCResolveFunc(client *atcclient.ATCClient, lbName, lbLocation, lbUnique string, dsc bool, serverName string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []Event, error) {
		cancelFn, eventCh, errCh, err := client.ClientAssign(ctx, &roxy_v0.ClientData{
			ServiceName: lbName,
			ShardId:     0,
			HasShardId:  false,
			Location:    lbLocation,
			Unique:      lbUnique,
		})
		if err != nil {
			return nil, err
		}

		ch := make(chan []Event)

		wg.Add(1)
		go func() {
			defer func() {
				cancelFn()
				wg.Done()
			}()
			for err := range errCh {
				ch <- []Event{{Type: ErrorEvent, Err: err}}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			byUnique := make(map[string]Resolved, 16)
			for inList := range eventCh {
				outList := mapATCEventsToEvents(serverName, byUnique, inList)
				if len(outList) != 0 {
					ch <- outList
				}
			}
		}()

		return ch, nil
	}
}

// type atcBuilder {{{

type atcBuilder struct {
	ctx    context.Context
	rng    *rand.Rand
	client *atcclient.ATCClient
}

func (b atcBuilder) Scheme() string {
	return constants.SchemeATC
}

func (b atcBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	var rt Target
	if err := rt.FromGRPCTarget(target); err != nil {
		return nil, err
	}

	lbName, lbLocation, lbUnique, _, _, serverName, err := ParseATCTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     b.ctx,
		Random:      b.rng,
		ResolveFunc: MakeATCResolveFunc(b.client, lbName, lbLocation, lbUnique, opts.DisableServiceConfig, serverName),
		ClientConn:  cc,
	})
}

var _ resolver.Builder = atcBuilder{}

// }}}

func mapATCEventsToEvents(serverName string, byUnique map[string]Resolved, in []*roxy_v0.Event) []Event {
	out := make([]Event, 0, len(in))
	for _, event := range in {
		out = mapATCEventToEvents(out, serverName, byUnique, event)
	}
	return out
}

func mapATCEventToEvents(out []Event, serverName string, byUnique map[string]Resolved, event *roxy_v0.Event) []Event {
	switch event.EventType {
	case roxy_v0.Event_INSERT_IP:
		tcpAddr := &net.TCPAddr{
			IP:   net.IP(event.Ip),
			Port: int(event.Port),
			Zone: event.Zone,
		}

		myServerName := event.ServerName
		if myServerName == "" {
			myServerName = serverName
		}
		if myServerName == "" {
			myServerName = tcpAddr.IP.String()
		}

		grpcAddr := resolver.Address{
			Addr:       tcpAddr.String(),
			ServerName: myServerName,
		}

		data := Resolved{
			Unique:     event.Unique,
			Location:   event.Location,
			ServerName: myServerName,
			Weight:     event.Weight,
			HasWeight:  true,
			Addr:       tcpAddr,
			Address:    grpcAddr,
		}

		byUnique[data.Unique] = data

		out = append(out, Event{
			Type: UpdateEvent,
			Key:  data.Unique,
			Data: data,
		})

	case roxy_v0.Event_DELETE_IP:
		delete(byUnique, event.Unique)

		out = append(out, Event{
			Type: DeleteEvent,
			Key:  event.Unique,
		})

	case roxy_v0.Event_UPDATE_WEIGHT:
		old := byUnique[event.Unique]

		data := Resolved{
			Unique:     old.Unique,
			Location:   old.Location,
			ServerName: old.ServerName,
			ShardID:    old.ShardID,
			Weight:     event.Weight,
			HasShardID: old.HasShardID,
			HasWeight:  true,
			Addr:       old.Addr,
			Address:    old.Address,
			Dynamic:    old.Dynamic,
		}

		byUnique[data.Unique] = data

		out = append(out, Event{
			Type: StatusChangeEvent,
			Key:  data.Unique,
			Data: data,
		})

	case roxy_v0.Event_NEW_SERVICE_CONFIG:
		out = append(out, Event{
			Type:              NewServiceConfigEvent,
			ServiceConfigJSON: event.ServiceConfigJson,
		})
	}

	return out
}
