package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// SRVTarget represents a parsed target spec for the "srv" scheme.
type SRVTarget struct {
	ResolverAddr     *net.TCPAddr
	Domain           string
	Service          string
	ServerName       string
	Balancer         BalancerType
	PollInterval     time.Duration
	CooldownInterval time.Duration
}

// FromTarget breaks apart a Target into component data.
func (t *SRVTarget) FromTarget(rt Target) error {
	*t = SRVTarget{}

	wantZero := true
	defer func() {
		if wantZero {
			*t = SRVTarget{}
		}
	}()

	var err error

	t.ResolverAddr, err = parseNetResolver(rt.Authority)
	if err != nil {
		err = roxyutil.AuthorityError{Authority: rt.Authority, Err: err}
		return err
	}

	domainAndService := rt.Endpoint
	if domainAndService == "" {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return err
	}

	if i := strings.IndexByte(domainAndService, '/'); i >= 0 {
		t.Domain, t.Service = domainAndService[:i], domainAndService[i+1:]
	} else {
		t.Domain, t.Service = domainAndService, ""
	}

	if j := strings.IndexByte(t.Service, '/'); j >= 0 {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectOneSlash}
		return err
	}

	if t.Domain == "" {
		err = roxyutil.HostError{Host: t.Domain, Err: roxyutil.ErrExpectNonEmpty}
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
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

	if str := rt.Query.Get("pollInterval"); str != "" {
		t.PollInterval, err = time.ParseDuration(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "pollInterval", Value: str, Err: err}
			return err
		}
	}

	if str := rt.Query.Get("cooldownInterval"); str != "" {
		t.CooldownInterval, err = time.ParseDuration(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "cooldownInterval", Value: str, Err: err}
			return err
		}
	}

	wantZero = false
	return nil
}

// AsTarget recombines the component data into a Target.
func (t SRVTarget) AsTarget() Target {
	query := make(url.Values, 4)
	query.Set("balancer", t.Balancer.String())
	if t.ServerName != "" {
		query.Set("serverName", t.ServerName)
	}
	if t.PollInterval != 0 {
		query.Set("pollInterval", t.PollInterval.String())
	}
	if t.CooldownInterval != 0 {
		query.Set("cooldownInterval", t.CooldownInterval.String())
	}

	var authority string
	if t.ResolverAddr != nil {
		authority = t.ResolverAddr.String()
	}

	var endpoint string
	endpoint = t.Domain
	if t.Service != "" {
		endpoint = t.Domain + "/" + t.Service
	}

	return Target{
		Scheme:     constants.SchemeSRV,
		Authority:  authority,
		Endpoint:   endpoint,
		Query:      query,
		ServerName: t.ServerName,
		HasSlash:   true,
	}
}

func (t SRVTarget) NetResolver() *net.Resolver {
	return makeNetResolver(t.ResolverAddr)
}

// NewSRVBuilder constructs a new gRPC resolver.Builder for the "srv" scheme.
func NewSRVBuilder(ctx context.Context, rng *rand.Rand, serviceConfigJSON string) resolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	return srvBuilder{ctx, rng, serviceConfigJSON}
}

// NewSRVResolver constructs a new Resolver for the "srv" scheme.
func NewSRVResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	var t SRVTarget
	err := t.FromTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewPollingResolver(PollingResolverOptions{
		Context:          opts.Context,
		Random:           opts.Random,
		PollInterval:     t.PollInterval,
		CooldownInterval: t.CooldownInterval,
		Balancer:         t.Balancer,
		ResolveFunc:      MakeSRVResolveFunc(opts.Context, t),
	})
}

// MakeSRVResolveFunc constructs a PollingResolveFunc for building your own
// custom PollingResolver with the "srv" scheme.
func MakeSRVResolveFunc(ctx context.Context, t SRVTarget) PollingResolveFunc {
	res := t.NetResolver()
	domain := t.Domain
	proto := constants.NetTCP
	service := t.Service
	if service == "" {
		proto = ""
	}
	return func() ([]Resolved, error) {
		// Resolve the SRV records.
		_, records, err := roxyutil.LookupSRV(ctx, res, service, proto, domain)
		if err != nil {
			return nil, err
		}

		// Generate the Resolved records.
		out := make([]Resolved, 0, len(records))
		for _, record := range records {
			// Resolve the A/AAAA records.
			ipList, err := roxyutil.LookupHost(ctx, res, record.Target)
			if err != nil {
				return nil, err
			}

			srvServerName := t.ServerName
			if srvServerName == "" {
				srvServerName = strings.TrimRight(record.Target, ".")
			}

			srvPriority := record.Priority
			srvWeight := record.Weight

			// Divide the weight evenly across all IP addresses.
			computedWeight := float32(record.Weight) / float32(len(ipList))

			// Synthesize a separate Resolved record for each IP address.
			for _, ip := range ipList {
				tcpAddr := &net.TCPAddr{
					IP:   ip,
					Port: int(record.Port),
				}
				grpcAddr := resolver.Address{
					Addr:       tcpAddr.String(),
					ServerName: srvServerName,
				}
				out = append(out, Resolved{
					Unique:      fmt.Sprintf("%03d/%03d/%s", srvPriority, srvWeight, tcpAddr.String()),
					ServerName:  srvServerName,
					SRVPriority: srvPriority,
					SRVWeight:   srvWeight,
					Weight:      computedWeight,
					HasSRV:      true,
					HasWeight:   true,
					Addr:        tcpAddr,
					Address:     grpcAddr,
				})
			}
		}
		return out, nil
	}
}

// type srvBuilder {{{

type srvBuilder struct {
	ctx               context.Context
	rng               *rand.Rand
	serviceConfigJSON string
}

func (b srvBuilder) Scheme() string {
	return constants.SchemeSRV
}

func (b srvBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	var rt Target
	if err := rt.FromGRPCTarget(target); err != nil {
		return nil, err
	}

	var t SRVTarget
	err := t.FromTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewPollingResolver(PollingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		PollInterval:      t.PollInterval,
		CooldownInterval:  t.CooldownInterval,
		ResolveFunc:       MakeSRVResolveFunc(b.ctx, t),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

var _ resolver.Builder = srvBuilder{}

// }}}
