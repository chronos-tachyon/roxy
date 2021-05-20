package roxyresolver

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"time"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

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

	res, host, port, balancer, pollInterval, cdInterval, serverName, err := ParseDNSTarget(opts.Target, defaultPort)
	if err != nil {
		return nil, err
	}

	if records := makeStaticRecordsForIP(host, port, serverName); records != nil {
		return NewStaticResolver(StaticResolverOptions{
			Random:   opts.Random,
			Balancer: balancer,
			Records:  records,
		})
	}
	return NewPollingResolver(PollingResolverOptions{
		Context:          opts.Context,
		Random:           opts.Random,
		PollInterval:     pollInterval,
		CooldownInterval: cdInterval,
		Balancer:         balancer,
		ResolveFunc:      MakeDNSResolveFunc(opts.Context, res, host, port, serverName),
	})
}

// ParseDNSTarget breaks apart a Target into component data.
func ParseDNSTarget(rt Target, defaultPort string) (res *net.Resolver, host string, port string, balancer BalancerType, pollInterval time.Duration, cdInterval time.Duration, serverName string, err error) {
	res, err = parseNetResolver(rt.Authority)
	if err != nil {
		err = roxyutil.AuthorityError{Authority: rt.Authority, Err: err}
		return
	}

	hostPort := rt.Endpoint
	if hostPort == "" {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}

	host, port, err = misc.SplitHostPort(hostPort, defaultPort)
	if err != nil {
		err = roxyutil.EndpointError{
			Endpoint: rt.Endpoint,
			Err:      roxyutil.HostPortError{HostPort: hostPort, Err: err},
		}
		return
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	if str := rt.Query.Get("pollInterval"); str != "" {
		pollInterval, err = time.ParseDuration(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "pollInterval", Value: str, Err: err}
			return
		}
	}

	if str := rt.Query.Get("cooldownInterval"); str != "" {
		cdInterval, err = time.ParseDuration(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "cooldownInterval", Value: str, Err: err}
			return
		}
	}

	serverName = rt.Query.Get("serverName")
	if serverName == "" {
		serverName = host
	}

	return
}

// MakeDNSResolveFunc constructs a PollingResolveFunc for building your own
// custom PollingResolver with the "dns" scheme.
func MakeDNSResolveFunc(ctx context.Context, res *net.Resolver, host string, port string, serverName string) PollingResolveFunc {
	return func() ([]Resolved, error) {
		// Resolve the port number.
		portNum, err := roxyutil.LookupPort(ctx, res, constants.NetTCP, port)
		if err != nil {
			return nil, err
		}

		// Resolve the A/AAAA records.
		ipList, err := roxyutil.LookupHost(ctx, res, host)
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
				ServerName: serverName,
			}
			out[index] = Resolved{
				Unique:     tcpAddr.String(),
				ServerName: serverName,
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

	res, host, port, _, pollInterval, cdInterval, serverName, err := ParseDNSTarget(rt, constants.PortHTTPS)
	if err != nil {
		return nil, err
	}

	if records := makeStaticRecordsForIP(host, port, serverName); records != nil {
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
		PollInterval:      pollInterval,
		CooldownInterval:  cdInterval,
		ResolveFunc:       MakeDNSResolveFunc(b.ctx, res, host, port, serverName),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

var _ resolver.Builder = dnsBuilder{}

// }}}
