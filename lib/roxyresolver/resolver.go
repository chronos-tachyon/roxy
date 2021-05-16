package roxyresolver

import (
	"context"
	"fmt"
	"math/rand"
	"net"

	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
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
	case constants.SchemeUnix:
		fallthrough
	case constants.SchemeUnixAbstract:
		return NewUnixResolver(opts)
	case constants.SchemeIP:
		return NewIPResolver(opts)
	case constants.SchemeDNS:
		return NewDNSResolver(opts)
	case constants.SchemeSRV:
		return NewSRVResolver(opts)
	case constants.SchemeZK:
		return NewZKResolver(opts)
	case constants.SchemeEtcd:
		return NewEtcdResolver(opts)
	case constants.SchemeATC:
		return NewATCResolver(opts)
	default:
		return nil, fmt.Errorf("Target.Scheme %q is not supported", opts.Target.Scheme)
	}
}

var _ grpcresolver.Resolver = Resolver(nil)

// }}}
