package roxyresolver

import (
	"net"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func NewUnixResolver(opts Options) (Resolver, error) {
	unixAddr, balancer, serverName, err := ParseUnixTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	resAddr := Address{
		Addr:       unixAddr.String(),
		ServerName: serverName,
	}

	// https://github.com/grpc/grpc-go/blob/v1.37.0/internal/resolver/unix/unix.go
	//
	// We would like to call the following code here:
	//
	// >  import "google.golang.org/grpc/internal/transport/networktype"
	// >  resAddr = networktype.Set(resAddr, "unix")
	//
	// but networktype is an internal package.
	//
	// Thankfully, resAddr is only used by resolvers in gRPC mode, i.e.
	// when FooResolverOptions contains a ClientConn.  We don't create or
	// register our own grpcresolver.Builder for "unix" or "unix-abstract",
	// so we rely on the standard gRPC resolver for those schemes.

	data := Resolved{
		Unique:     constants.NetUnix + ":" + unixAddr.String(),
		ServerName: serverName,
		Addr:       unixAddr,
		Address:    resAddr,
	}

	return NewStaticResolver(StaticResolverOptions{
		Random:   opts.Random,
		Balancer: balancer,
		Records:  []Resolved{data},
	})
}

func ParseUnixTarget(rt RoxyTarget) (unixAddr *net.UnixAddr, balancer BalancerType, serverName string, err error) {
	if rt.Authority != "" && !strings.EqualFold(rt.Authority, "localhost") {
		err = roxyutil.BadAuthorityError{Authority: rt.Authority, Err: roxyutil.ErrExpectEmptyOrLocalhost}
		return
	}

	rawUnixPath := rt.Endpoint
	if rawUnixPath == "" {
		err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}

	isUnixAbstract := strings.EqualFold(rt.Scheme, "unix-abstract")
	fn := roxyutil.ExpandString
	if rawUnixPath != "" && !isUnixAbstract {
		ch := rawUnixPath[0]
		if ch == '\x00' || ch == '@' {
			// pass
		} else if ch == '/' || ch == '$' {
			fn = roxyutil.ExpandPath
		} else {
			rawUnixPath = "/" + rawUnixPath
			fn = roxyutil.ExpandPath
		}
	}

	rawUnixPath, err = fn(rawUnixPath)
	if err != nil {
		err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: err}
		return
	}
	if rawUnixPath == "" {
		err = roxyutil.BadEndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return
	}

	var unixPath string
	if isUnixAbstract {
		unixPath = "\x00" + rawUnixPath
	} else if rawUnixPath[0] == '\x00' || rawUnixPath[0] == '@' {
		unixPath = "\x00" + rawUnixPath[1:]
	} else {
		unixPath = rawUnixPath
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = roxyutil.BadQueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	serverName = rt.Query.Get("serverName")
	if serverName == "" {
		serverName = "localhost"
	}

	unixAddr = &net.UnixAddr{
		Net:  constants.NetUnix,
		Name: unixPath,
	}

	return
}
