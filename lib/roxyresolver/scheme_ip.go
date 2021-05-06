package roxyresolver

import (
	"fmt"
	"math/rand"
	"net"
	"strings"

	grpcresolver "google.golang.org/grpc/resolver"
)

func NewIPBuilder(rng *rand.Rand, serviceConfigJSON string) grpcresolver.Builder {
	return ipBuilder{rng, serviceConfigJSON}
}

func NewIPResolver(opts Options) (Resolver, error) {
	defaultPort := httpPort
	if opts.IsTLS {
		defaultPort = httpsPort
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
		err = BadAuthorityError{Authority: rt.Authority, Err: ErrExpectEmpty}
		return
	}

	ipPortListStr := rt.Endpoint
	if ipPortListStr == "" {
		err = BadEndpointError{Endpoint: rt.Endpoint, Err: ErrExpectNonEmpty}
		return
	}

	ipPortList := strings.Split(ipPortListStr, ",")
	tcpAddrs = make([]*net.TCPAddr, 0, len(ipPortList))
	for _, ipPort := range ipPortList {
		if ipPort == "" {
			continue
		}
		var (
			host string
			port string
		)
		host, port, err = net.SplitHostPort(ipPort)
		if err != nil {
			h, p, err2 := net.SplitHostPort(ipPort + ":" + defaultPort)
			if err2 == nil {
				host, port, err = h, p, nil
			}
			if err != nil {
				err = BadEndpointError{
					Endpoint: rt.Endpoint,
					Err:      BadHostPortError{HostPort: ipPort, Err: err},
				}
				return
			}
		}
		var tcpAddr *net.TCPAddr
		tcpAddr, err = parseIPAndPort(host, port)
		if err != nil {
			err = BadEndpointError{
				Endpoint: rt.Endpoint,
				Err:      BadHostPortError{HostPort: ipPort, Err: err},
			}
			return
		}
		tcpAddrs = append(tcpAddrs, tcpAddr)
	}

	if len(tcpAddrs) == 0 {
		err = BadEndpointError{Endpoint: rt.Endpoint, Err: ErrExpectNonEmpty}
		return
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = BadQueryParamError{Name: "balancer", Value: str, Err: err}
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
	return ipScheme
}

func (b ipBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	rt, err := RoxyTargetFromTarget(target)
	if err != nil {
		return nil, err
	}

	tcpAddrs, _, serverName, err := ParseIPTarget(rt, httpsPort)
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
