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

	grpcresolver "google.golang.org/grpc/resolver"
)

func NewSRVBuilder(ctx context.Context, rng *rand.Rand, serviceConfigJSON string) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	return srvBuilder{ctx, rng, serviceConfigJSON}
}

func NewSRVResolver(opts Options) (Resolver, error) {
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

func ParseSRVTarget(target Target) (res *net.Resolver, name string, service string, balancer BalancerType, pollInterval time.Duration, cdInterval time.Duration, serverName string, err error) {
	res, err = parseNetResolver(target.Authority)
	if err != nil {
		err = BadAuthorityError{Authority: target.Authority, Err: err}
		return
	}

	ep := target.Endpoint
	if ep == "" {
		err = BadEndpointError{Endpoint: target.Endpoint, Err: ErrExpectNonEmpty}
		return
	}

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	if ep == "" {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err:      BadPathError{Path: ep, Err: ErrExpectNonEmpty},
		}
		return
	}

	unescaped, err := url.PathUnescape(ep)
	if err != nil {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err:      BadPathError{Path: ep, Err: err},
		}
		return
	}

	i := strings.IndexByte(unescaped, '/')
	if i >= 0 {
		name, service = unescaped[:i], unescaped[i+1:]
	} else {
		name, service = unescaped, ""
	}
	if j := strings.IndexByte(service, '/'); j >= 0 {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: unescaped,
				Err:  ErrExpectOneSlash,
			},
		}
		return
	}

	if name == "" {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: unescaped,
				Err: BadHostError{
					Host: name,
					Err:  ErrExpectNonEmpty,
				},
			},
		}
		return
	}

	var query url.Values
	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err:      BadQueryStringError{QueryString: qs, Err: err},
			}
			return
		}
	}

	if str := query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err: BadQueryStringError{
					QueryString: qs,
					Err: BadQueryParamError{
						Name:  "balancer",
						Value: str,
						Err:   err,
					},
				},
			}
			return
		}
	}

	if str := query.Get("pollInterval"); str != "" {
		pollInterval, err = time.ParseDuration(str)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err: BadQueryStringError{
					QueryString: qs,
					Err: BadQueryParamError{
						Name:  "pollInterval",
						Value: str,
						Err:   err,
					},
				},
			}
			return
		}
	}

	if str := query.Get("cooldownInterval"); str != "" {
		cdInterval, err = time.ParseDuration(str)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err: BadQueryStringError{
					QueryString: qs,
					Err: BadQueryParamError{
						Name:  "cooldownInterval",
						Value: str,
						Err:   err,
					},
				},
			}
			return
		}
	}

	serverName = query.Get("serverName")

	return
}

func MakeSRVResolveFunc(ctx context.Context, res *net.Resolver, name string, service string, serverName string) PollingResolveFunc {
	proto := "tcp"
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
	return srvScheme
}

func (b srvBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	res, name, service, _, pollInterval, cdInterval, serverName, err := ParseSRVTarget(target)
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
