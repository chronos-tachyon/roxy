package roxyresolver

import (
	"context"
	"math/rand"
	"net"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// WatchID uniquely identifies a call to Resolver.Watch.
type WatchID uint64

// WatchFunc denotes a callback which will be called whenever a Resolver's
// state changes.
type WatchFunc func([]Event)

// type Resolver {{{

// Resolver is an interface for obtaining resolved addresses and/or watching
// for address resolution changes.
type Resolver interface {
	// Err returns any errors encountered since the last call to Err or
	// ResolveAll.
	Err() error

	// ResolveAll returns all resolved addresses, plus any errors
	// encountered since the last call to Err or ResolveAll.
	ResolveAll() ([]Resolved, error)

	// Resolve returns the resolved address of a healthy backend, if one is
	// available, or else returns the error that prevented it from doing
	// so.
	Resolve() (Resolved, error)

	// Update changes the status of a server.
	Update(opts UpdateOptions)

	// Watch registers a WatchFunc.  The WatchFunc will be called
	// immediately with synthetic events for each resolved address
	// currently known, plus it will be called whenever the Resolver's
	// state changes.
	Watch(WatchFunc) WatchID

	// CancelWatch cancels a previous call to Watch.
	CancelWatch(WatchID)

	// ResolveNow requests that the resolver issue a re-resolve poll ASAP,
	// if that makes sense for the particular Resolver implementation.
	ResolveNow(resolver.ResolveNowOptions)

	// Close stops the resolver and frees all resources.
	Close()
}

// UpdateOptions holds options for the Resolver.Update method.
type UpdateOptions struct {
	// Addr is the address of the server to act upon.
	Addr net.Addr

	// HasHealthy is true if Healthy has a value.
	HasHealthy bool

	// Healthy is true if the server is ready to serve, false if it is not.
	Healthy bool
}

// Options holds options related to constructing a new Resolver.
type Options struct {
	// Context is the context within which the resolver runs.  If this
	// context is cancelled or reaches its deadline, the resolver will
	// stop.
	//
	// This field is mandatory.
	Context context.Context

	// Random is the source of randomness for balancer algorithms that need
	// one.
	//
	// If provided, it MUST be thread-safe; see the syncrand package for
	// more information.  If nil, the syncrand.Global() instance will be
	// used.
	Random *rand.Rand

	// Target is the resolve target.  The target's Scheme field determines
	// which Resolver implementation will be used.
	//
	// This field is mandatory.
	Target Target

	// IsTLS should be set to true iff the consumer of the resolved
	// addresses will be using TLS to communicate with them.  It is used
	// for automatic port guessing.
	IsTLS bool
}

// New constructs a new Resolver.
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
		return nil, roxyutil.SchemeError{Scheme: opts.Target.Scheme, Err: roxyutil.ErrNotExist}
	}
}

var _ resolver.Resolver = Resolver(nil)

// }}}
