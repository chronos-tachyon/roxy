package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func NewSRVBuilder(ctx context.Context, rng *rand.Rand, serviceConfigJSON string) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	return srvBuilder{ctx, rng, serviceConfigJSON}
}

func NewSRVResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	res, name, service, balancer, pollInterval, cdInterval, serverName, err := ParseSRVTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewPollingResolver(PollingResolverOptions{
		Context:          opts.Context,
		Random:           opts.Random,
		PollInterval:     pollInterval,
		CooldownInterval: cdInterval,
		Balancer:         balancer,
		ResolveFunc:      MakeSRVResolveFunc(opts.Context, res, name, service, serverName),
	})
}

func ParseSRVTarget(rt RoxyTarget) (res *net.Resolver, name string, service string, balancer BalancerType, pollInterval time.Duration, cdInterval time.Duration, serverName string, err error) {
	res, err = parseNetResolver(rt.Authority)
	if err != nil {
		err = roxyutil.BadAuthorityError{Authority: rt.Authority, Err: err}
		return
	}

	nameAndService := rt.Endpoint
	if nameAndService == "" {
		err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}

	i := strings.IndexByte(nameAndService, '/')
	if i >= 0 {
		name, service = nameAndService[:i], nameAndService[i+1:]
	} else {
		name, service = nameAndService, ""
	}
	if j := strings.IndexByte(service, '/'); j >= 0 {
		err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectOneSlash}
		return
	}

	if name == "" {
		err = roxyutil.BadEndpointError{
			Endpoint: rt.Endpoint,
			Err:      roxyutil.BadHostError{Host: name, Err: roxyutil.ErrExpectNonEmpty},
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

	return
}

func MakeSRVResolveFunc(ctx context.Context, res *net.Resolver, name string, service string, serverName string) PollingResolveFunc {
	proto := constants.NetTCP
	if service == "" {
		proto = ""
	}
	return func() ([]Resolved, error) {
		// Resolve the SRV records.
		_, records, err := res.LookupSRV(ctx, service, proto, name)
		if err != nil {
			return nil, fmt.Errorf("LookupSRV(%q, %q, %q) failed: %w", service, proto, name, err)
		}

		// Generate the Resolved records.
		out := make([]Resolved, 0, len(records))
		for _, record := range records {
			// Resolve the A/AAAA records.
			ipStrList, err := res.LookupHost(ctx, record.Target)
			if err != nil {
				return nil, fmt.Errorf("LookupHost(%q) failed: %w", record.Target, err)
			}

			srvServerName := serverName
			if srvServerName == "" {
				srvServerName = strings.TrimRight(record.Target, ".")
			}

			srvPriority := record.Priority
			srvWeight := record.Weight

			// Divide the weight evenly across all IP addresses.
			computedWeight := float32(record.Weight) / float32(len(ipStrList))

			// Synthesize a separate Resolved record for each IP address.
			for _, ipStr := range ipStrList {
				ip := net.ParseIP(ipStr)
				if ip == nil {
					return nil, fmt.Errorf("LookupHost returned invalid IP address %q", ipStr)
				}
				tcpAddr := &net.TCPAddr{
					IP:   ip,
					Port: int(record.Port),
				}
				resAddr := Address{
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
					Address:     resAddr,
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

func (b srvBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	rt, err := RoxyTargetFromTarget(target)
	if err != nil {
		return nil, err
	}

	res, name, service, _, pollInterval, cdInterval, serverName, err := ParseSRVTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewPollingResolver(PollingResolverOptions{
		Context:           b.ctx,
		Random:            b.rng,
		PollInterval:      pollInterval,
		CooldownInterval:  cdInterval,
		ResolveFunc:       MakeSRVResolveFunc(b.ctx, res, name, service, serverName),
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

var _ grpcresolver.Builder = srvBuilder{}

// }}}
