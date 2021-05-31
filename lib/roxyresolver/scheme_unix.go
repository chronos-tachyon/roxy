package roxyresolver

import (
	"net"
	"net/url"
	"strings"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// UnixTarget represents a parsed target spec for the "unix" or "unix-abstract"
// schemes.
type UnixTarget struct {
	Addr       *net.UnixAddr
	IsAbstract bool
	ServerName string
	Balancer   BalancerType
}

// FromTarget breaks apart a Target into component data.
func (t *UnixTarget) FromTarget(rt Target) error {
	*t = UnixTarget{}

	wantZero := true
	defer func() {
		if wantZero {
			*t = UnixTarget{}
		}
	}()

	if rt.Authority != "" && !strings.EqualFold(rt.Authority, "localhost") {
		err := roxyutil.AuthorityError{Authority: rt.Authority, Err: roxyutil.ErrExpectEmptyOrLocalhost}
		return err
	}

	rawUnixPath := rt.Endpoint
	if rawUnixPath == "" {
		err := roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return err
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

	rawUnixPath, err := fn(rawUnixPath)
	if err != nil {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: err}
		return err
	}
	if rawUnixPath == "" {
		err = roxyutil.EndpointError{Endpoint: rt.Endpoint, Err: roxyutil.ErrExpectNonEmpty}
		return err
	}

	var unixPath string
	if isUnixAbstract {
		unixPath = "\x00" + rawUnixPath
		t.IsAbstract = true
	} else if rawUnixPath[0] == '\x00' || rawUnixPath[0] == '@' {
		unixPath = "\x00" + rawUnixPath[1:]
		t.IsAbstract = true
	} else {
		unixPath = rawUnixPath
	}

	t.Addr = &net.UnixAddr{
		Net:  constants.NetUnix,
		Name: unixPath,
	}

	t.ServerName = rt.Query.Get("serverName")
	if t.ServerName == "" {
		t.ServerName = "localhost"
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
func (t UnixTarget) AsTarget() Target {
	query := make(url.Values, 2)
	query.Set("balancer", t.Balancer.String())
	if t.ServerName != "localhost" {
		query.Set("serverName", t.ServerName)
	}

	if t.IsAbstract {
		return Target{
			Scheme:     constants.SchemeUnixAbstract,
			Endpoint:   t.Addr.Name[1:],
			Query:      query,
			ServerName: t.ServerName,
			HasSlash:   true,
		}
	}

	return Target{
		Scheme:     constants.SchemeUnix,
		Endpoint:   t.Addr.Name,
		Query:      query,
		ServerName: t.ServerName,
		HasSlash:   true,
	}
}

// NewUnixResolver constructs a new Resolver for the "unix" scheme.
func NewUnixResolver(opts Options) (Resolver, error) {
	var t UnixTarget
	err := t.FromTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	grpcAddr := resolver.Address{
		Addr:       t.Addr.String(),
		ServerName: t.ServerName,
	}

	// https://github.com/grpc/grpc-go/blob/v1.37.0/internal/resolver/unix/unix.go
	//
	// We would like to call the following code here:
	//
	// >  import "google.golang.org/grpc/internal/transport/networktype"
	// >  grpcAddr = networktype.Set(grpcAddr, "unix")
	//
	// but networktype is an internal package.
	//
	// Thankfully, grpcAddr is only used by resolvers in gRPC mode, i.e.
	// when FooResolverOptions contains a ClientConn.  We don't create or
	// register our own resolver.Builder for "unix" or "unix-abstract", so
	// we rely on the standard gRPC resolver for those schemes.

	data := Resolved{
		Unique:     constants.NetUnix + ":" + t.Addr.String(),
		ServerName: t.ServerName,
		Addr:       t.Addr,
		Address:    grpcAddr,
	}

	return NewStaticResolver(StaticResolverOptions{
		Random:   opts.Random,
		Balancer: t.Balancer,
		Records:  []Resolved{data},
	})
}
