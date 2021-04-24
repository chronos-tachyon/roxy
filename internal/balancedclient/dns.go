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

func NewDNSResolver(opts Options) (baseresolver.Resolver, error) {
	if opts.Target.Authority != "" {
		return nil, fmt.Errorf("non-empty Target.Authority %q is not supported", opts.Target.Authority)
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

	host, port, err := net.SplitHostPort(ep)
	if err != nil {
		defaultPort := ":80"
		if opts.TLSConfig != nil {
			defaultPort = ":443"
		}
		h, p, err2 := net.SplitHostPort(ep + defaultPort)
		if err2 == nil {
			host, port, err = h, p, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("Target.Endpoint %q cannot be parsed as <host>:<port>: %w", ep, err)
	}
	if host == "" {
		return nil, fmt.Errorf("Target.Endpoint %q contains empty <host>", ep)
	}
	if port == "" {
		return nil, fmt.Errorf("Target.Endpoint %q contains empty <port>", ep)
	}

	var query url.Values
	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Target.Endpoint query string %q", qs, err)
		}
	}

	var balancer baseresolver.BalancerType
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
			// Resolve the port number.
			portNum, err := net.LookupPort("tcp", port)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve port %q: %w", port, err)
			}

			// Resolve the A/AAAA records.
			ipStrList, err := net.LookupHost(host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve host %q: %w", host, err)
			}

			// Synthesize an AddrData for each IP address.
			out := make([]*baseresolver.AddrData, len(ipStrList))
			for index, ipStr := range ipStrList {
				ip := net.ParseIP(ipStr)
				if ip == nil {
					return nil, fmt.Errorf("failed to parse IP %q", ipStr)
				}

				tcpAddr := &net.TCPAddr{IP: ip, Port: int(portNum)}

				serverName := new(string)
				*serverName = host

				out[index] = &baseresolver.AddrData{
					Addr:       tcpAddr,
					ServerName: serverName,
					Address: resolver.Address{
						Addr: tcpAddr.String(),
					},
				}
			}
			return out, nil
		},
	})
}
