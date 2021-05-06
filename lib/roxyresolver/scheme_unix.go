package roxyresolver

import (
	"net"
	"path/filepath"
	"strings"
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
		Unique:     "unix:" + unixAddr.String(),
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
		err = BadAuthorityError{Authority: rt.Authority, Err: ErrExpectEmptyOrLocalhost}
		return
	}

	rawUnixPath := rt.Endpoint
	if rawUnixPath == "" {
		err = BadEndpointError{Endpoint: rt.Endpoint, Err: ErrExpectNonEmpty}
		return
	}

	var unixPath string
	if strings.EqualFold(rt.Scheme, "unix-abstract") {
		unixPath = "\x00" + rawUnixPath
	} else if rawUnixPath[0] == '\x00' || rawUnixPath[0] == '@' {
		unixPath = "\x00" + rawUnixPath[1:]
	} else {
		unixPath = filepath.Clean(filepath.FromSlash("/" + rawUnixPath))
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = BadQueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	serverName = rt.Query.Get("serverName")
	if serverName == "" {
		serverName = "localhost"
	}

	unixAddr = &net.UnixAddr{
		Net:  "unix",
		Name: unixPath,
	}

	return
}
