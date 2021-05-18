package roxyresolver

import (
	"fmt"
	"math/rand"
	"net"

	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func NewIPBuilder(rng *rand.Rand, serviceConfigJSON string) grpcresolver.Builder {
	return ipBuilder{rng, serviceConfigJSON}
}

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

func ParseIPTarget(rt RoxyTarget, defaultPort string) (tcpAddrs []*net.TCPAddr, balancer BalancerType, serverName string, err error) {
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

func (b ipBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	rt, err := RoxyTargetFromTarget(target)
	if err != nil {
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

var _ grpcresolver.Builder = ipBuilder{}

// }}}

func makeIPRecords(tcpAddrs []*net.TCPAddr, serverName string) []Resolved {
	records := make([]Resolved, len(tcpAddrs))
	for index, tcpAddr := range tcpAddrs {
		myServerName := serverName
		if myServerName == "" {
			myServerName = tcpAddr.IP.String()
		}
		resAddr := Address{
			Addr:       tcpAddr.String(),
			ServerName: myServerName,
		}
		records[index] = Resolved{
			Unique:     fmt.Sprintf("%d/%s", index, tcpAddr.String()),
			ServerName: myServerName,
			Addr:       tcpAddr,
			Address:    resAddr,
		}
	}
	return records
}
