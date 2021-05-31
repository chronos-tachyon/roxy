package roxyresolver

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/url"
	"time"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// DNSTarget represents a parsed target spec for the "dns" scheme.
type DNSTarget struct {
	ResolverAddr     *net.TCPAddr
	Host             string
	Port             string
	ServerName       string
	Balancer         BalancerType
	PollInterval     time.Duration
	CooldownInterval time.Duration
}

// FromTarget breaks apart a Target into component data.
func (t *DNSTarget) FromTarget(rt Target, defaultPort string) error {
	*t = DNSTarget{}

	wantZero := true
	defer func() {
		if wantZero {
			*t = DNSTarget{}
		}
	}()

	var err error

	t.ResolverAddr, err = parseNetResolver(rt.Authority)
	if err != nil {
		err = roxyutil.AuthorityError{Authority: rt.Authority, Err: err}
		return err
	}

	hostPort := rt.Endpoint
	if hostPort == "" {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return err
	}

	t.Host, t.Port, err = misc.SplitHostPort(hostPort, defaultPort)
	if err != nil {
		err = roxyutil.HostPortError{HostPort: hostPort, Err: err}
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return err
	}

	t.ServerName = rt.Query.Get("serverName")
	if t.ServerName == "" {
		t.ServerName = t.Host
	}

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
func (t DNSTarget) AsTarget() Target {
	query := make(url.Values, 4)
	query.Set("balancer", t.Balancer.String())
	if t.ServerName != t.Host {
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

	return Target{
		Scheme:     constants.SchemeDNS,
		Authority:  authority,
		Endpoint:   net.JoinHostPort(t.Host, t.Port),
		Query:      query,
		ServerName: t.ServerName,
		HasSlash:   true,
	}
}

// NetResolver returns the appropriate *net.Resolver for this target.
func (t DNSTarget) NetResolver() *net.Resolver {
	return makeNetResolver(t.ResolverAddr)
}

// NewDNSBuilder constructs a new gRPC resolver.Builder for the "dns" scheme.
func NewDNSBuilder(ctx context.Context, rng *rand.Rand, serviceConfigJSON string) resolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	return dnsBuilder{ctx, rng, serviceConfigJSON}
}

// NewDNSResolver constructs a new Resolver for the "dns" scheme.
func NewDNSResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	defaultPort := constants.PortHTTP
	if opts.IsTLS {
		defaultPort = constants.PortHTTPS
	}

	var t DNSTarget
	err := t.FromTarget(opts.Target, defaultPort)
	if err != nil {
		return nil, err
	}

	if records := makeStaticRecordsForIP(t.Host, t.Port, t.ServerName); records != nil {
		return NewStaticResolver(StaticResolverOptions{
			Random:   opts.Random,
			Balancer: t.Balancer,
			Records:  records,
		})
	}
	return NewPollingResolver(PollingResolverOptions{
		Context:          opts.Context,
		Random:           opts.Random,
		PollInterval:     t.PollInterval,
		CooldownInterval: t.CooldownInterval,
		Balancer:         t.Balancer,
		ResolveFunc:      MakeDNSResolveFunc(opts.Context, t),
	})
}

// MakeDNSResolveFunc constructs a PollingResolveFunc for building your own
// custom PollingResolver with the "dns" scheme.
func MakeDNSResolveFunc(ctx context.Context, t DNSTarget) PollingResolveFunc {
	res := t.NetResolver()

	return func() ([]Resolved, error) {
		// Resolve the port number.
		portNum, err := roxyutil.LookupPort(ctx, res, constants.NetTCP, t.Port)
		if err != nil {
			return nil, err
		}

		// Resolve the A/AAAA records.
		ipList, err := roxyutil.LookupHost(ctx, res, t.Host)
		if err != nil {
			return nil, err
		}

		// Synthesize a Resolved for each IP address.
		out := make([]Resolved, len(ipList))
		for index, ip := range ipList {
			tcpAddr := &net.TCPAddr{
				IP:   ip,
				Port: int(portNum),
			}
			grpcAddr := resolver.Address{
				Addr:       tcpAddr.String(),
				ServerName: t.ServerName,
			}
			out[index] = Resolved{
				Unique:     tcpAddr.String(),
				ServerName: t.ServerName,
				Addr:       tcpAddr,
				Address:    grpcAddr,
			}
		}
		return out, nil
	}
}

// type dnsBuilder {{{

type dnsBuilder struct {
	ctx               context.Context
	rng               *rand.Rand
	serviceConfigJSON string
}

func (b dnsBuilder) Scheme() string {
	return constants.SchemeDNS
}

func (b dnsBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	var rt Target
	if err := rt.FromGRPCTarget(target); err != nil {
		return nil, err
	}

	var t DNSTarget
	err := t.FromTarget(rt, constants.PortHTTPS)
	if err != nil {
		return nil, err
	}

	if records := makeStaticRecordsForIP(t.Host, t.Port, t.ServerName); records != nil {
		return NewStaticResolver(StaticResolverOptions{
			Random:            b.rng,
			Records:           records,
			ClientConn:        cc,
			ServiceConfigJSON: b.serviceConfigJSON,
		})
	}
	return NewPollingResolver(PollingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		PollInterval:      t.PollInterval,
		CooldownInterval:  t.CooldownInterval,
		ResolveFunc:       MakeDNSResolveFunc(b.ctx, t),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

var _ resolver.Builder = dnsBuilder{}

// }}}
