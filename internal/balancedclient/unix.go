package balancedclient

import (
	"fmt"
	"net"
	"path/filepath"

	enums "github.com/chronos-tachyon/roxy/internal/enums"
)

func NewUnixResolver(opts Options) (Resolver, error) {
	if opts.Target == "" {
		return nil, fmt.Errorf("Target is empty")
	}

	unixpath := opts.Target
	switch unixpath[0] {
	case '\x00':
		// pass
	case '@':
		unixpath = "\x00" + unixpath[1:]
	default:
		var err error
		unixpath, err = filepath.Abs(unixpath)
		if err != nil {
			return nil, fmt.Errorf("failed to make Target %q absolute: %w", opts.Target, err)
		}
	}

	if opts.Balancer != enums.RandomBalancer {
		return nil, fmt.Errorf("UnixResolver does not support %#v", opts.Balancer)
	}

	res := &unixResolver{
		singleton: &net.UnixAddr{
			Net:  "unix",
			Name: unixpath,
		},
	}
	return res, nil
}

// type unixResolver {{{

type unixResolver struct {
	singleton net.Addr
}

func (res *unixResolver) ServerHostname() string {
	return "localhost"
}

func (res *unixResolver) ResolveAll() ([]net.Addr, error) {
	return []net.Addr{res.singleton}, nil
}

func (res *unixResolver) Resolve() (net.Addr, error) {
	return res.singleton, nil
}

func (res *unixResolver) MarkHealthy(addr net.Addr, healthy bool) {
	// pass
}

func (res *unixResolver) Watch(fn WatchFunc) WatchID {
	events := []*Event{
		{
			Type: enums.UpdateEvent,
			Key:  "singleton",
			Addr: res.singleton,
		},
	}
	fn(events)
	return 0
}

func (res *unixResolver) CancelWatch(id WatchID) {
	// pass
}

func (res *unixResolver) Close() error {
	return nil
}

var _ Resolver = (*unixResolver)(nil)

// }}}
