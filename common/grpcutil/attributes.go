package grpcutil

import (
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/membership"
	"github.com/chronos-tachyon/roxy/roxypb"
)

type attrKey string

const (
	serverSetKey = attrKey("roxy.ServerSet")
	endpointKey  = attrKey("roxy.Endpoint")
)

func WithServerSet(addr resolver.Address, ss *membership.ServerSet) resolver.Address {
	addr.Attributes = addr.Attributes.WithValues(serverSetKey, ss)
	return addr
}

func GetServerSet(addr resolver.Address) *membership.ServerSet {
	ss, _ := addr.Attributes.Value(serverSetKey).(*membership.ServerSet)
	return ss
}

func WithEndpoint(addr resolver.Address, ep *roxypb.Endpoint) resolver.Address {
	addr.Attributes = addr.Attributes.WithValues(endpointKey, ep)
	return addr
}

func GetEndpoint(addr resolver.Address) *roxypb.Endpoint {
	ep, _ := addr.Attributes.Value(endpointKey).(*roxypb.Endpoint)
	return ep
}
