package roxyresolver

import (
	"fmt"
	"math/rand"

	grpcresolver "google.golang.org/grpc/resolver"
)

func NewIPBuilder(rng *rand.Rand, serviceConfigJSON string) grpcresolver.Builder {
	return ipBuilder{rng, serviceConfigJSON}
}

func NewIPResolver(opts Options) (Resolver, error) {
	defaultPort := "80"
	if opts.IsTLS {
		defaultPort = "443"
	}

	tcpAddrs, query, err := ParseIPTarget(opts.Target, defaultPort)
	if err != nil {
		return nil, err
	}

	var balancer BalancerType
	if str := query.Get("balancer"); str != "" {
		if err := balancer.Parse(str); err != nil {
			return nil, fmt.Errorf("failed to parse balancer=%q query string: %w", str, err)
		}
	}

	serverName := query.Get("serverName")

	records := make([]*Resolved, len(tcpAddrs))
	for index, tcpAddr := range tcpAddrs {
		myServerName := serverName
		if myServerName == "" {
			myServerName = tcpAddr.IP.String()
		}
		resAddr := Address{
			Addr:       tcpAddr.String(),
			ServerName: myServerName,
		}
		data := &Resolved{
			Unique:     fmt.Sprintf("%d/%s", index, tcpAddr.String()),
			ServerName: myServerName,
			Addr:       tcpAddr,
			Address:    resAddr,
		}
		records[index] = data
	}

	return NewStaticResolver(StaticResolverOptions{
		Random:   opts.Random,
		Balancer: balancer,
		Records:  records,
	})
}

// type ipBuilder {{{

type ipBuilder struct {
	rng               *rand.Rand
	serviceConfigJSON string
}

func (b ipBuilder) Scheme() string {
	return "ip"
}

func (b ipBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	tcpAddrs, query, err := ParseIPTarget(target, "443")
	if err != nil {
		return nil, err
	}

	serverName := query.Get("serverName")

	records := make([]*Resolved, len(tcpAddrs))
	for index, tcpAddr := range tcpAddrs {
		myServerName := serverName
		if myServerName == "" {
			myServerName = tcpAddr.IP.String()
		}
		resAddr := Address{
			Addr:       tcpAddr.String(),
			ServerName: myServerName,
		}
		data := &Resolved{
			Unique:     fmt.Sprintf("%d/%s", index, tcpAddr.String()),
			ServerName: myServerName,
			Addr:       tcpAddr,
			Address:    resAddr,
		}
		records[index] = data
	}

	return NewStaticResolver(StaticResolverOptions{
		Random:            b.rng,
		Records:           records,
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

var _ grpcresolver.Builder = ipBuilder{}

// }}}
