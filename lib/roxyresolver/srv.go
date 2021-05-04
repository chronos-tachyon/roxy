package roxyresolver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

func NewSRVResolver(opts Options) (Resolver, error) {
	res, name, service, query, err := ParseSRVTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	var balancer BalancerType = SRVBalancer
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

	return NewPollingResolver(PollingResolverOptions{
		Context:          opts.Context,
		Random:           opts.Random,
		PollInterval:     pollInterval,
		CooldownInterval: cdInterval,
		Balancer:         balancer,
		ResolveFunc:      MakeSRVResolveFunc(opts.Context, res, name, service, serverName),
	})
}

func MakeSRVResolveFunc(ctx context.Context, res *net.Resolver, name string, service string, serverName string) PollingResolveFunc {
	return func() ([]*Resolved, error) {
		// Resolve the SRV records.
		_, records, err := res.LookupSRV(ctx, service, "tcp", name)
		if err != nil {
			return nil, fmt.Errorf("LookupSRV(%q, %q, %q) failed: %w", service, "tcp", name, err)
		}

		// Generate the *Resolved records.
		out := make([]*Resolved, 0, len(records))
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

			// Synthesize a separate *Resolved record for each IP address.
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
				out = append(out, &Resolved{
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
