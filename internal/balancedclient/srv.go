package balancedclient

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
)

func NewSRVResolver(opts Options) (baseresolver.Resolver, error) {
	netResolver, err := parseNetResolver(opts.Target.Authority)
	if err != nil {
		return nil, fmt.Errorf("invalid Target.Authority: %w", err)
	}

	ep := opts.Target.Endpoint
	if ep == "" {
		return nil, errors.New("Target.Endpoint is empty")
	}

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	i := strings.IndexByte(ep, '/')
	if i < 0 {
		return nil, fmt.Errorf("Target.Endpoint %q cannot be parsed as <name>/<service>: missing '/'", ep)
	}
	name, service := ep[:i], ep[i+1:]
	if name == "" {
		return nil, fmt.Errorf("Target.Endpoint %q contains empty <name>", ep)
	}
	if service == "" {
		return nil, fmt.Errorf("Target.Endpoint %q contains empty <service>", ep)
	}
	if j := strings.IndexByte(service, '/'); j >= 0 {
		return nil, fmt.Errorf("Target.Endpoint %q contains multiple slashes", ep)
	}

	var query url.Values
	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Target.Endpoint query string %q: %w", qs, err)
		}
	}

	var balancer baseresolver.BalancerType = baseresolver.SRVBalancer
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

	return baseresolver.NewPollingResolver(baseresolver.PollingResolverOptions{
		Context:      opts.Context,
		Random:       opts.Random,
		PollInterval: pollInterval,
		Balancer:     balancer,
		ResolveFunc: func() ([]*baseresolver.AddrData, error) {
			// Resolve the SRV records.
			_, records, err := netResolver.LookupSRV(opts.Context, service, "tcp", name)
			if err != nil {
				return nil, err
			}

			// Generate the AddrData records.
			out := make([]*baseresolver.AddrData, 0, len(records))
			for _, record := range records {
				// Resolve the A/AAAA records.
				ipStrList, err := netResolver.LookupHost(opts.Context, record.Target)
				if err != nil {
					return nil, err
				}

				serverName := new(string)
				*serverName = query.Get("serverName")
				if *serverName == "" {
					*serverName = strings.TrimRight(record.Target, ".")
				}

				priority := new(uint16)
				*priority = record.Priority

				weight := new(uint16)
				*weight = record.Weight

				// Divide the weight evenly across all IP addresses.
				computedWeight := new(float32)
				*computedWeight = float32(record.Weight) / float32(len(ipStrList))

				// Synthesize a separate AddrData record for each IP address.
				for _, ipStr := range ipStrList {
					ip := net.ParseIP(ipStr)
					if ip == nil {
						return nil, fmt.Errorf("net.LookupHost returned unparseable IP address %q", ipStr)
					}

					tcpAddr := &net.TCPAddr{IP: ip, Port: int(record.Port)}

					out = append(out, &baseresolver.AddrData{
						Addr:           tcpAddr,
						ServerName:     serverName,
						SRVPriority:    priority,
						SRVWeight:      weight,
						ComputedWeight: computedWeight,
						Address: resolver.Address{
							Addr:       tcpAddr.String(),
							ServerName: *serverName,
						},
					})
				}
			}
			return out, nil
		},
	})
}
