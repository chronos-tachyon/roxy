package roxyresolver

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// ATCTarget represents a parsed target spec for the "atc" scheme.
type ATCTarget struct {
	ServiceName string
	ShardID     uint32
	HasShardID  bool
	Unique      string
	Location    string
	ServerName  string
	Balancer    BalancerType
	CPS         float64
}

// ParseATCTarget breaks apart a Target into component data.
func (t *ATCTarget) FromTarget(rt Target) error {
	*t = ATCTarget{}

	wantZero := true
	defer func() {
		if wantZero {
			*t = ATCTarget{}
		}
	}()

	if rt.Authority != "" {
		err := roxyutil.AuthorityError{Authority: rt.Authority, Err: roxyutil.ErrExpectEmpty}
		return err
	}

	ep := rt.Endpoint
	if ep == "" {
		err := roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return err
	}

	var shardStr string
	var hasShardStr bool
	if i := strings.IndexByte(ep, '/'); i >= 0 {
		ep, shardStr, hasShardStr = ep[:i], ep[i+1:], true
	}

	t.ServiceName = ep
	err := roxyutil.ValidateATCServiceName(t.ServiceName)
	if err != nil {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return err
	}

	if hasShardStr {
		u64, err := strconv.ParseUint(shardStr, 10, 32)
		if err != nil {
			err = roxyutil.ATCShardIDError{ShardID: shardStr, Err: err}
			err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
			return err
		}
		t.ShardID = uint32(u64)
		t.HasShardID = true
	}

	t.Unique = rt.Query.Get("unique")
	err = roxyutil.ValidateATCUnique(t.Unique)
	if err != nil {
		err = roxyutil.QueryParamError{Name: "unique", Value: t.Unique, Err: err}
		return err
	}

	t.Location = rt.Query.Get("location")
	if t.Location == "" {
		t.Location = rt.Query.Get("loc")
	}
	err = roxyutil.ValidateATCLocation(t.Location)
	if err != nil {
		err = roxyutil.QueryParamError{Name: "location", Value: t.Location, Err: err}
		return err
	}

	t.ServerName = rt.Query.Get("serverName")

	if str := rt.Query.Get("balancer"); str != "" {
		err = t.Balancer.Parse(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "balancer", Value: str, Err: err}
			return err
		}
	}

	if str := rt.Query.Get("cps"); str != "" {
		t.CPS, err = strconv.ParseFloat(str, 64)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "cps", Value: str, Err: err}
			return err
		}
	}

	wantZero = false
	return nil
}

// AsTarget recombines the component data into a Target.
func (t ATCTarget) AsTarget() Target {
	query := make(url.Values, 5)
	query.Set("balancer", t.Balancer.String())
	if t.ServerName != "" {
		query.Set("serverName", t.ServerName)
	}
	if t.Unique != "" {
		query.Set("unique", t.Unique)
	}
	if t.Location != "" {
		query.Set("location", t.Location)
	}
	if t.CPS != 0.0 {
		query.Set("cps", strconv.FormatFloat(t.CPS, 'f', -1, 64))
	}

	var endpoint string
	endpoint = t.ServiceName
	if t.HasShardID {
		endpoint = t.ServiceName + "/" + strconv.FormatUint(uint64(t.ShardID), 10)
	}

	return Target{
		Scheme:     constants.SchemeATC,
		Endpoint:   endpoint,
		Query:      query,
		ServerName: t.ServerName,
		HasSlash:   true,
	}
}

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

	var t ATCTarget
	err := t.FromTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    t.Balancer,
		ResolveFunc: MakeATCResolveFunc(client, t),
	})
}

// MakeATCResolveFunc constructs a WatchingResolveFunc for building your own
// custom WatchingResolver with the "atc" scheme.
func MakeATCResolveFunc(client *atcclient.ATCClient, t ATCTarget) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []Event, error) {
		cancelFn, eventCh, errCh, err := client.ClientAssign(ctx, &roxy_v0.ClientData{
			ServiceName:           t.ServiceName,
			ShardId:               t.ShardID,
			HasShardId:            t.HasShardID,
			Unique:                t.Unique,
			Location:              t.Location,
			DeclaredCostPerSecond: t.CPS,
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
				outList := mapATCEventsToEvents(t.ServerName, byUnique, inList)
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

	var t ATCTarget
	err := t.FromTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     b.ctx,
		Random:      b.rng,
		ResolveFunc: MakeATCResolveFunc(b.client, t),
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
