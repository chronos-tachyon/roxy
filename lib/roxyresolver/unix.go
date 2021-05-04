package roxyresolver

import (
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

func NewUnixResolver(opts Options) (Resolver, error) {
	unixAddr, balancer, err := ParseUnixTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	resAddr := Address{
		Addr:       unixAddr.String(),
		ServerName: "localhost",
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
		ServerName: "localhost",
		Addr:       unixAddr,
		Address:    resAddr,
	}

	return NewStaticResolver(StaticResolverOptions{
		Random:   opts.Random,
		Balancer: balancer,
		Records:  []Resolved{data},
	})
}

func ParseUnixTarget(target Target) (unixAddr *net.UnixAddr, balancer BalancerType, err error) {
	if target.Authority != "" && !strings.EqualFold(target.Authority, "localhost") {
		err = BadAuthorityError{
			Authority: target.Authority,
			Err:       ErrExpectEmptyOrLocalhost,
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

	var unixpath string
	if strings.EqualFold(target.Scheme, "unix-abstract") {
		unixpath = "\x00" + unescaped
	} else if unescaped[0] == '\x00' || unescaped[0] == '@' {
		unixpath = "\x00" + unescaped[1:]
	} else {
		unixpath = filepath.Clean(filepath.FromSlash("/" + unescaped))
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

	unixAddr = &net.UnixAddr{
		Net:  "unix",
		Name: unixpath,
	}
	return
}
