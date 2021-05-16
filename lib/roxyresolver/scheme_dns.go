package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func NewDNSBuilder(ctx context.Context, rng *rand.Rand, serviceConfigJSON string) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	return dnsBuilder{ctx, rng, serviceConfigJSON}
}

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

func ParseDNSTarget(rt RoxyTarget, defaultPort string) (res *net.Resolver, host string, port string, balancer BalancerType, pollInterval time.Duration, cdInterval time.Duration, serverName string, err error) {
	res, err = parseNetResolver(rt.Authority)
	if err != nil {
		err = roxyutil.BadAuthorityError{Authority: rt.Authority, Err: err}
		return
	}

	hostPort := rt.Endpoint
	if hostPort == "" {
		err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}

	host, port, err = misc.SplitHostPort(hostPort, defaultPort)
	if err != nil {
		err = roxyutil.BadEndpointError{
			Endpoint: rt.Endpoint,
			Err:      roxyutil.BadHostPortError{HostPort: hostPort, Err: err},
		}
		return
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = roxyutil.BadQueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	if str := rt.Query.Get("pollInterval"); str != "" {
		pollInterval, err = time.ParseDuration(str)
		if err != nil {
			err = roxyutil.BadQueryParamError{Name: "pollInterval", Value: str, Err: err}
			return
		}
	}

	if str := rt.Query.Get("cooldownInterval"); str != "" {
		cdInterval, err = time.ParseDuration(str)
		if err != nil {
			err = roxyutil.BadQueryParamError{Name: "cooldownInterval", Value: str, Err: err}
			return
		}
	}

	serverName = rt.Query.Get("serverName")
	if serverName == "" {
		serverName = host
	}

	return
}

func MakeDNSResolveFunc(ctx context.Context, res *net.Resolver, host string, port string, serverName string) PollingResolveFunc {
	return func() ([]Resolved, error) {
		// Resolve the port number.
		portNum, err := res.LookupPort(ctx, constants.NetTCP, port)
		if err != nil {
			return nil, fmt.Errorf("LookupPort(%q, %q) failed: %w", constants.NetTCP, port, err)
		}

		// Resolve the A/AAAA records.
		ipStrList, err := res.LookupHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("LookupHost(%q) failed: %w", host, err)
		}

		// Synthesize a Resolved for each IP address.
		out := make([]Resolved, len(ipStrList))
		for index, ipStr := range ipStrList {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				return nil, fmt.Errorf("LookupHost returned invalid IP address %q", ipStr)
			}
			tcpAddr := &net.TCPAddr{
				IP:   ip,
				Port: int(portNum),
			}
			resAddr := Address{
				Addr:       tcpAddr.String(),
				ServerName: serverName,
			}
			out[index] = Resolved{
				Unique:     tcpAddr.String(),
				ServerName: serverName,
				Addr:       tcpAddr,
				Address:    resAddr,
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

func (b dnsBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	rt, err := RoxyTargetFromTarget(target)
	if err != nil {
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

var _ grpcresolver.Builder = dnsBuilder{}

// }}}
