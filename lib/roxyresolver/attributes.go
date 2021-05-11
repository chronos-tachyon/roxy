package roxyresolver

import (
	"github.com/chronos-tachyon/roxy/lib/membership"
)

type attrKey string

const (
	MembershipAttrKey = attrKey("roxy.Membership")
	ResolvedAttrKey   = attrKey("roxy.Resolved")
)

func WithMembership(addr Address, r *membership.Roxy) Address {
	addr.Attributes = addr.Attributes.WithValues(MembershipAttrKey, r)
	return addr
}

func GetMembership(addr Address) *membership.Roxy {
	r, _ := addr.Attributes.Value(MembershipAttrKey).(*membership.Roxy)
	return r
}

func WithResolved(addr Address, data Resolved) Address {
	addr.Attributes = addr.Attributes.WithValues(ResolvedAttrKey, data)
	return addr
}

func GetResolved(addr Address) (data Resolved, ok bool) {
	data, ok = addr.Attributes.Value(ResolvedAttrKey).(Resolved)
	return
}
