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
	ServiceName    string
	ShardNumber    uint32
	HasShardNumber bool
	UniqueID       string
	Location       string
	ServerName     string
	Balancer       BalancerType
	CPS            float64
}

// FromTarget breaks apart a Target into component data.
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
			err = roxyutil.ATCShardNumberError{ShardNumber: shardStr, Err: err}
			err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
			return err
		}
		t.ShardNumber = uint32(u64)
		t.HasShardNumber = true
	}

	t.UniqueID = rt.Query.Get("unique")
	if t.UniqueID == "" {
		t.UniqueID, err = atcclient.UniqueID()
		if err != nil {
			err = roxyutil.ATCLoadUniqueError{Err: err}
			return err
		}
	} else {
		err = roxyutil.ValidateATCUniqueID(t.UniqueID)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "unique", Value: t.UniqueID, Err: err}
			return err
		}
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

	t.Balancer = WeightedRoundRobinBalancer
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
	uniqueID, uniqueErr := atcclient.UniqueID()

	query := make(url.Values, 5)
	if t.Balancer != WeightedRoundRobinBalancer {
		query.Set("balancer", t.Balancer.String())
	}
	if t.ServerName != "" {
		query.Set("serverName", t.ServerName)
	}
	if uniqueErr != nil || t.UniqueID != uniqueID {
		query.Set("unique", t.UniqueID)
	}
	if t.Location != "" {
		query.Set("location", t.Location)
	}
	if t.CPS != 0.0 {
		query.Set("cps", strconv.FormatFloat(t.CPS, 'f', -1, 64))
	}

	var endpoint string
	endpoint = t.ServiceName
	if t.HasShardNumber {
		endpoint = t.ServiceName + "/" + strconv.FormatUint(uint64(t.ShardNumber), 10)
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
	return func(ctx context.Context, wg1 *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []Event, error) {
		cancelFn, eventCh, errCh, err := client.ClientAssign(ctx, &roxy_v0.ClientData{
			ServiceName:           t.ServiceName,
			ShardNumber:           t.ShardNumber,
			HasShardNumber:        t.HasShardNumber,
			UniqueId:              t.UniqueID,
			Location:              t.Location,
			DeclaredCostPerSecond: t.CPS,
		})
		if err != nil {
			return nil, err
		}

		wg2 := new(sync.WaitGroup)
		ch := make(chan []Event)

		wg1.Add(3)
		wg2.Add(2)
		go atcResolveThread1(wg1, wg2, errCh, ch)
		go atcResolveThread2(wg1, wg2, t.ServerName, eventCh, ch)
		go atcResolveThread3(wg1, wg2, cancelFn, ctx.Done(), ch)

		return ch, nil
	}
}

func atcResolveThread1(wg1, wg2 *sync.WaitGroup, errCh <-chan error, ch chan<- []Event) {
	for err := range errCh {
		ch <- []Event{{Type: ErrorEvent, Err: err}}
	}
	wg2.Done()
	wg1.Done()
}

func atcResolveThread2(wg1, wg2 *sync.WaitGroup, serverName string, eventCh <-chan []*roxy_v0.Event, ch chan<- []Event) {
	byUniqueID := make(map[string]Resolved, 16)
	for inList := range eventCh {
		outList := mapATCEventsToEvents(serverName, byUniqueID, inList)
		if len(outList) != 0 {
			ch <- outList
		}
	}
	wg2.Done()
	wg1.Done()
}

func atcResolveThread3(wg1, wg2 *sync.WaitGroup, cancelFn context.CancelFunc, doneCh <-chan struct{}, ch chan<- []Event) {
	<-doneCh
	cancelFn()
	wg2.Wait()
	close(ch)
	wg1.Done()
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

func mapATCEventsToEvents(serverName string, byUniqueID map[string]Resolved, in []*roxy_v0.Event) []Event {
	out := make([]Event, 0, len(in))
	for _, event := range in {
		out = mapATCEventToEvents(out, serverName, byUniqueID, event)
	}
	return out
}

func mapATCEventToEvents(out []Event, serverName string, byUniqueID map[string]Resolved, event *roxy_v0.Event) []Event {
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
			UniqueID:   event.UniqueId,
			Location:   event.Location,
			ServerName: myServerName,
			Weight:     float32(event.AssignedCostPerSecond),
			HasWeight:  true,
			Addr:       tcpAddr,
			Address:    grpcAddr,
		}

		byUniqueID[data.UniqueID] = data

		out = append(out, Event{
			Type: UpdateEvent,
			Key:  data.UniqueID,
			Data: data,
		})

	case roxy_v0.Event_DELETE_IP:
		delete(byUniqueID, event.UniqueId)

		out = append(out, Event{
			Type: DeleteEvent,
			Key:  event.UniqueId,
		})

	case roxy_v0.Event_UPDATE_WEIGHT:
		old := byUniqueID[event.UniqueId]

		data := Resolved{
			UniqueID:       old.UniqueID,
			Location:       old.Location,
			ServerName:     old.ServerName,
			ShardNumber:    old.ShardNumber,
			Weight:         float32(event.AssignedCostPerSecond),
			HasShardNumber: old.HasShardNumber,
			HasWeight:      true,
			Addr:           old.Addr,
			Address:        old.Address,
			Dynamic:        old.Dynamic,
		}

		byUniqueID[data.UniqueID] = data

		out = append(out, Event{
			Type: StatusChangeEvent,
			Key:  data.UniqueID,
			Data: data,
		})

	case roxy_v0.Event_NEW_SERVICE_CONFIG:
		out = append(out, Event{
			Type:              NewServiceConfigEvent,
			ServiceConfigJSON: event.ServiceConfigJson,
		})

	case roxy_v0.Event_DELETE_ALL_IPS:
		for uniqueID := range byUniqueID {
			delete(byUniqueID, uniqueID)
			out = append(out, Event{
				Type: DeleteEvent,
				Key:  uniqueID,
			})
		}
	}

	return out
}
