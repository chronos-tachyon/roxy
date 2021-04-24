package balancedclient

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
)

func NewUnixResolver(opts Options) (baseresolver.Resolver, error) {
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

	var balancer baseresolver.BalancerType
	if err := balancer.Parse(query.Get("balancer")); err != nil {
		return nil, fmt.Errorf("failed to parse balancer=%q query string: %w", query.Get("balancer"), err)
	}

	unixAddr := &net.UnixAddr{
		Net:  "unix",
		Name: unixpath,
	}

	records := []*baseresolver.AddrData{
		{
			Addr: unixAddr,
			Address: resolver.Address{
				Addr: unixAddr.String(),
			},
		},
	}

	return baseresolver.NewStaticResolver(baseresolver.StaticResolverOptions{
		Random:   opts.Random,
		Balancer: balancer,
		Records:  records,
	})
}
