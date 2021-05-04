package roxyresolver

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
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

func ParseIPTarget(target Target, defaultPort string) (tcpAddrs []*net.TCPAddr, balancer BalancerType, serverName string, err error) {
	if target.Authority != "" {
		err = BadAuthorityError{
			Authority: target.Authority,
			Err:       ErrExpectEmpty,
		}
		return
	}

	ep := target.Endpoint
	if ep == "" {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err:      ErrExpectNonEmpty,
		}
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
			Err: BadPathError{
				Path: ep,
				Err:  ErrExpectNonEmpty,
			},
		}
		return
	}

	var unescaped string
	unescaped, err = url.PathUnescape(ep)
	if err != nil {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: ep,
				Err:  err,
			},
		}
		return
	}

	ipPortList := strings.Split(unescaped, ",")
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
					Endpoint: target.Endpoint,
					Err: BadPathError{
						Path: unescaped,
						Err: BadHostPortError{
							HostPort: ipPort,
							Err:      err,
						},
					},
				}
				return
			}
		}
		var tcpAddr *net.TCPAddr
		tcpAddr, err = parseIPAndPort(host, port)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err: BadPathError{
					Path: unescaped,
					Err: BadHostPortError{
						HostPort: ipPort,
						Err:      err,
					},
				},
			}
			return
		}
		tcpAddrs = append(tcpAddrs, tcpAddr)
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

	serverName = query.Get("serverName")

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
	tcpAddrs, _, serverName, err := ParseIPTarget(target, httpsPort)
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
