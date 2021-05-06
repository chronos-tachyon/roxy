package roxyresolver

import (
	"context"
	"fmt"
	"math/rand"
	"net"

	grpcresolver "google.golang.org/grpc/resolver"
)

type (
	Target            = grpcresolver.Target
	Address           = grpcresolver.Address
	ResolveNowOptions = grpcresolver.ResolveNowOptions
)

type WatchID uint32

type WatchFunc func([]Event)

type UpdateOptions struct {
	Addr       net.Addr
	HasHealthy bool
	HasLoad    bool
	Healthy    bool
	Load       float32
}

// type Resolver {{{

type Resolver interface {
	Err() error
	ResolveAll() ([]Resolved, error)
	Resolve() (Resolved, error)

	Update(opts UpdateOptions)

	Watch(WatchFunc) WatchID
	CancelWatch(WatchID)

	ResolveNow(ResolveNowOptions)
	Close()
}

type Options struct {
	Target  RoxyTarget
	IsTLS   bool
	Context context.Context
	Random  *rand.Rand
}

func New(opts Options) (Resolver, error) {
	switch opts.Target.Scheme {
	case unixScheme:
		fallthrough
	case unixAbstractScheme:
		return NewUnixResolver(opts)
	case ipScheme:
		return NewIPResolver(opts)
	case dnsScheme:
		return NewDNSResolver(opts)
	case srvScheme:
		return NewSRVResolver(opts)
	case zkScheme:
		return NewZKResolver(opts)
	case etcdScheme:
		return NewEtcdResolver(opts)
	case atcScheme:
		return NewATCResolver(opts)
	default:
		return nil, fmt.Errorf("Target.Scheme %q is not supported", opts.Target.Scheme)
	}
}

var _ grpcresolver.Resolver = Resolver(nil)

// }}}
