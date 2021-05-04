package roxyresolver

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

func NewUnixResolver(opts Options) (Resolver, error) {
	if opts.Target.Authority != "" && !strings.EqualFold(opts.Target.Authority, "localhost") {
		return nil, fmt.Errorf("Target.Authority %q is not supported", opts.Target.Authority)
	}

	ep := opts.Target.Endpoint

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	if ep == "" {
		return nil, errors.New("Target.Endpoint is empty")
	}

	var unixpath string
	if strings.EqualFold(opts.Target.Scheme, "unix-abstract") {
		unixpath = "\x00" + ep
	} else if strings.EqualFold(opts.Target.Scheme, "unix") {
		if ep[0] == '\x00' || ep[0] == '@' {
			unixpath = "\x00" + ep[1:]
		} else {
			unixpath = filepath.Clean(filepath.FromSlash("/" + ep))
		}
	} else {
		return nil, fmt.Errorf("Target.Scheme %q is not supported", opts.Target.Scheme)
	}

	var query url.Values
	if hasQS {
		var err error
		query, err = url.ParseQuery(qs)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Target.Endpoint query string %q: %w", qs, err)
		}
	}

	var balancer BalancerType
	if err := balancer.Parse(query.Get("balancer")); err != nil {
		return nil, fmt.Errorf("failed to parse balancer=%q query string: %w", query.Get("balancer"), err)
	}

	unixAddr := &net.UnixAddr{
		Net:  "unix",
		Name: unixpath,
	}

	resAddr := Address{
		Addr: unixAddr.String(),
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

	data := &Resolved{
		Unique:  "unix:" + unixAddr.String(),
		Addr:    unixAddr,
		Address: resAddr,
	}

	return NewStaticResolver(StaticResolverOptions{
		Random:   opts.Random,
		Balancer: balancer,
		Records:  []*Resolved{data},
	})
}
