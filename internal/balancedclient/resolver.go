package balancedclient

import (
	"net"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

type Resolver interface {
	ServerHostname() string
	ResolveAll() ([]net.Addr, error)
	Resolve() (net.Addr, error)
	MarkHealthy(addr net.Addr, healthy bool)
	Watch(WatchFunc) WatchID
	CancelWatch(WatchID)
	Close() error
}

type WatchID uint32

type WatchFunc func([]*Event)

type Event struct {
	Type     enums.EventType
	Key      string
	Addr     net.Addr
	Metadata interface{}
	Err      error
}
