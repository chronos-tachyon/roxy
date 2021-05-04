package roxyresolver

import (
	"github.com/chronos-tachyon/roxy/lib/membership"
)

type attrKey string

const (
	ServerSetAttrKey = attrKey("roxy.ServerSet")
	ResolvedAttrKey  = attrKey("roxy.Resolved")
)

func WithServerSet(addr Address, ss *membership.ServerSet) Address {
	addr.Attributes = addr.Attributes.WithValues(ServerSetAttrKey, ss)
	return addr
}

func GetServerSet(addr Address) *membership.ServerSet {
	ss, _ := addr.Attributes.Value(ServerSetAttrKey).(*membership.ServerSet)
	return ss
}

func WithResolved(addr Address, data Resolved) Address {
	addr.Attributes = addr.Attributes.WithValues(ResolvedAttrKey, data)
	return addr
}

func GetResolved(addr Address) (data Resolved, ok bool) {
	data, ok = addr.Attributes.Value(ResolvedAttrKey).(Resolved)
	return
}
