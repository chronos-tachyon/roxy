package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	grpcresolver "google.golang.org/grpc/resolver"
)

func NewDNSBuilder(ctx context.Context, rng *rand.Rand, serviceConfigJSON string) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	return dnsBuilder{ctx, rng, serviceConfigJSON}
}

func NewDNSResolver(opts Options) (Resolver, error) {
	defaultPort := "80"
	if opts.IsTLS {
		defaultPort = "443"
	}

	res, host, port, query, err := ParseDNSTarget(opts.Target, defaultPort)
	if err != nil {
		return nil, err
	}

	var balancer BalancerType
	if str := query.Get("balancer"); str != "" {
		if err = balancer.Parse(str); err != nil {
			return nil, fmt.Errorf("failed to parse balancer=%q query string: %w", str, err)
		}
	}

	var pollInterval time.Duration
	if str := query.Get("pollInterval"); str != "" {
		if pollInterval, err = time.ParseDuration(str); err != nil {
			return nil, fmt.Errorf("failed to parse pollInterval=%q query string: %w", str, err)
		}
	}

	var cdInterval time.Duration
	if str := query.Get("cooldownInterval"); str != "" {
		if cdInterval, err = time.ParseDuration(str); err != nil {
			return nil, fmt.Errorf("failed to parse cooldownInterval=%q query string: %w", str, err)
		}
	}

	serverName := query.Get("serverName")

	if tcpAddr := parseIPAndPort(host, port); tcpAddr != nil {
		if serverName == "" {
			serverName = tcpAddr.IP.String()
		}
		resAddr := Address{
			Addr:       tcpAddr.String(),
			ServerName: serverName,
		}
		data := &Resolved{
			Unique:     tcpAddr.String(),
			ServerName: serverName,
			Addr:       tcpAddr,
			Address:    resAddr,
		}
		return NewStaticResolver(StaticResolverOptions{
			Random:   opts.Random,
			Balancer: balancer,
			Records:  []*Resolved{data},
		})
	}

	if serverName == "" {
		serverName = host
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

func MakeDNSResolveFunc(ctx context.Context, res *net.Resolver, host string, port string, serverName string) PollingResolveFunc {
	return func() ([]*Resolved, error) {
		// Resolve the port number.
		portNum, err := res.LookupPort(ctx, "tcp", port)
		if err != nil {
			return nil, fmt.Errorf("LookupPort(%q, %q) failed: %w", "tcp", port, err)
		}

		// Resolve the A/AAAA records.
		ipStrList, err := res.LookupHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("LookupHost(%q) failed: %w", host, err)
		}

		// Synthesize a *Resolved for each IP address.
		out := make([]*Resolved, len(ipStrList))
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
			out[index] = &Resolved{
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
	return "dns"
}

func (b dnsBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	res, host, port, query, err := ParseDNSTarget(target, "443")
	if err != nil {
		return nil, err
	}

	var pollInterval time.Duration
	if str := query.Get("pollInterval"); str != "" {
		if pollInterval, err = time.ParseDuration(str); err != nil {
			return nil, fmt.Errorf("failed to parse pollInterval=%q query string: %w", str, err)
		}
	}

	var cdInterval time.Duration
	if str := query.Get("cooldownInterval"); str != "" {
		if cdInterval, err = time.ParseDuration(str); err != nil {
			return nil, fmt.Errorf("failed to parse cooldownInterval=%q query string: %w", str, err)
		}
	}

	serverName := query.Get("serverName")

	if tcpAddr := parseIPAndPort(host, port); tcpAddr != nil {
		if serverName == "" {
			serverName = tcpAddr.IP.String()
		}
		resAddr := Address{
			Addr:       tcpAddr.String(),
			ServerName: serverName,
		}
		data := &Resolved{
			Unique:     tcpAddr.String(),
			ServerName: serverName,
			Addr:       tcpAddr,
			Address:    resAddr,
		}
		return NewStaticResolver(StaticResolverOptions{
			Random:            b.rng,
			Records:           []*Resolved{data},
			ClientConn:        cc,
			ServiceConfigJSON: b.serviceConfigJSON,
		})
	}

	if serverName == "" {
		serverName = host
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
