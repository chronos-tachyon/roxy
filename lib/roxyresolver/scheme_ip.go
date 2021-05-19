package roxyresolver

import (
	"fmt"
	"math/rand"
	"net"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// NewIPBuilder constructs a new gRPC resolver.Builder for the "ip" scheme.
func NewIPBuilder(rng *rand.Rand, serviceConfigJSON string) resolver.Builder {
	return ipBuilder{rng, serviceConfigJSON}
}

// NewIPResolver constructs a new Resolver for the "ip" scheme.
func NewIPResolver(opts Options) (Resolver, error) {
	defaultPort := constants.PortHTTP
	if opts.IsTLS {
		defaultPort = constants.PortHTTPS
	}

	tcpAddrs, balancer, serverName, err := ParseIPTarget(opts.Target, defaultPort)
	if err != nil {
		return nil, err
	}

	records := makeIPRecords(tcpAddrs, serverName)
	return NewStaticResolver(StaticResolverOptions{
		Random:   opts.Random,
		Balancer: balancer,
		Records:  records,
	})
}

// ParseIPTarget breaks apart a Target into component data.
func ParseIPTarget(rt Target, defaultPort string) (tcpAddrs []*net.TCPAddr, balancer BalancerType, serverName string, err error) {
	if rt.Authority != "" {
		err = roxyutil.AuthorityError{Authority: rt.Authority, Err: roxyutil.ErrExpectEmpty}
		return
	}

	ipPortListStr, err := roxyutil.ExpandString(rt.Endpoint)
	if err != nil {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return
	}
	if ipPortListStr == "" {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}

	tcpAddrs, err = misc.ParseTCPAddrList(ipPortListStr, defaultPort)
	if err != nil {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return
	}
	if len(tcpAddrs) == 0 {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	serverName = rt.Query.Get("serverName")

	return
}

// type ipBuilder {{{

type ipBuilder struct {
	rng               *rand.Rand
	serviceConfigJSON string
}

func (b ipBuilder) Scheme() string {
	return constants.SchemeIP
}

func (b ipBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	var rt Target
	if err := rt.FromGRPCTarget(target); err != nil {
		return nil, err
	}

	tcpAddrs, _, serverName, err := ParseIPTarget(rt, constants.PortHTTPS)
	if err != nil {
		return nil, err
	}

	records := makeIPRecords(tcpAddrs, serverName)
	return NewStaticResolver(StaticResolverOptions{
		Random:            b.rng,
		Records:           records,
		ClientConn:        cc,
		ServiceConfigJSON: b.serviceConfigJSON,
	})
}

var _ resolver.Builder = ipBuilder{}

// }}}

func makeIPRecords(tcpAddrs []*net.TCPAddr, serverName string) []Resolved {
	records := make([]Resolved, len(tcpAddrs))
	for index, tcpAddr := range tcpAddrs {
		myServerName := serverName
		if myServerName == "" {
			myServerName = tcpAddr.IP.String()
		}
		grpcAddr := resolver.Address{
			Addr:       tcpAddr.String(),
			ServerName: myServerName,
		}
		records[index] = Resolved{
			Unique:     fmt.Sprintf("%d/%s", index, tcpAddr.String()),
			ServerName: myServerName,
			Addr:       tcpAddr,
			Address:    grpcAddr,
		}
	}
	return records
}
