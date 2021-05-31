package roxyresolver

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strings"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// IPTarget represents a parsed target spec for the "ip" scheme.
type IPTarget struct {
	Addrs      []*net.TCPAddr
	ServerName string
	Balancer   BalancerType
}

// FromTarget breaks apart a Target into component data.
func (t *IPTarget) FromTarget(rt Target, defaultPort string) error {
	*t = IPTarget{}

	wantZero := true
	defer func() {
		if wantZero {
			*t = IPTarget{}
		}
	}()

	if rt.Authority != "" {
		err := roxyutil.AuthorityError{Authority: rt.Authority, Err: roxyutil.ErrExpectEmpty}
		return err
	}

	ipPortListStr, err := roxyutil.ExpandString(rt.Endpoint)
	if err != nil {
		err := roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return err
	}
	if ipPortListStr == "" {
		err := roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return err
	}

	t.Addrs, err = misc.ParseTCPAddrList(ipPortListStr, defaultPort)
	if err != nil {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return err
	}
	if len(t.Addrs) == 0 {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return err
	}

	t.ServerName = rt.Query.Get("serverName")
	if t.ServerName == "" && len(t.Addrs) == 1 {
		t.ServerName = t.Addrs[0].IP.String()
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = t.Balancer.Parse(str)
		if err != nil {
			err = roxyutil.QueryParamError{Name: "balancer", Value: str, Err: err}
			return err
		}
	}

	wantZero = false
	return nil
}

// AsTarget recombines the component data into a Target.
func (t IPTarget) AsTarget() Target {
	query := make(url.Values, 2)
	query.Set("balancer", t.Balancer.String())
	if t.ServerName != "" {
		query.Set("serverName", t.ServerName)
	}

	addrStrings := make([]string, len(t.Addrs))
	for index, addr := range t.Addrs {
		addrStrings[index] = addr.String()
	}

	return Target{
		Scheme:     constants.SchemeIP,
		Endpoint:   strings.Join(addrStrings, ","),
		Query:      query,
		ServerName: t.ServerName,
		HasSlash:   true,
	}
}

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

	var t IPTarget
	err := t.FromTarget(opts.Target, defaultPort)
	if err != nil {
		return nil, err
	}

	records := makeIPRecords(t.Addrs, t.ServerName)
	return NewStaticResolver(StaticResolverOptions{
		Random:   opts.Random,
		Balancer: t.Balancer,
		Records:  records,
	})
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

	var t IPTarget
	err := t.FromTarget(rt, constants.PortHTTPS)
	if err != nil {
		return nil, err
	}

	records := makeIPRecords(t.Addrs, t.ServerName)
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
