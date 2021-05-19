package roxyresolver

import (
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

type attrKey string

const (
	// MembershipAttrKey is the gRPC attributes.Attributes key for
	// retrieving the original *membership.Roxy, if any.
	MembershipAttrKey = attrKey("roxy.Membership")

	// ResolvedAttrKey is the gRPC attributes.Attributes key for retrieving
	// the Resolved address.
	ResolvedAttrKey = attrKey("roxy.Resolved")
)

// WithMembership returns a copy of addr with the given *membership.Roxy attached.
func WithMembership(addr resolver.Address, r *membership.Roxy) resolver.Address {
	addr.Attributes = addr.Attributes.WithValues(MembershipAttrKey, r)
	return addr
}

// GetMembership retrieves the attached *membership.Roxy, or nil if none attached.
func GetMembership(addr resolver.Address) *membership.Roxy {
	r, _ := addr.Attributes.Value(MembershipAttrKey).(*membership.Roxy)
	return r
}

// WithResolved returns a copy of addr with the given Resolved attached.
func WithResolved(addr resolver.Address, data Resolved) resolver.Address {
	addr.Attributes = addr.Attributes.WithValues(ResolvedAttrKey, data)
	return addr
}

// GetResolved retrieves the attached Resolved.
func GetResolved(addr resolver.Address) (data Resolved, ok bool) {
	data, ok = addr.Attributes.Value(ResolvedAttrKey).(Resolved)
	return
}
