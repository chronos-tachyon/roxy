package membership

import (
	"google.golang.org/grpc/resolver"
)

type key string

const serverSetKey = key("roxy.ServerSet")

func GetServerSet(addr resolver.Address) *ServerSet {
	if attrs := addr.Attributes; attrs != nil {
		if value, ok := attrs.Value(serverSetKey).(*ServerSet); ok {
			return value
		}
	}
	return nil
}

func WithServerSet(addr resolver.Address, value *ServerSet) resolver.Address {
	addr.Attributes = addr.Attributes.WithValues(serverSetKey, value)
	return addr
}
