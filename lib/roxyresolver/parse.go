package roxyresolver

import (
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

func parseMembershipData(namedPort string, serverName string, pathKey string, bytes []byte) Event {
	r := new(membership.Roxy)

	if err := r.UnmarshalJSON(bytes); err != nil {
		err = ResolveError{UniqueID: pathKey, Err: err}
		return Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: Resolved{
				UniqueID: pathKey,
				Err:      err,
			},
		}
	}

	if !r.Ready {
		var err error = StatusError{membership.StatusDead}
		err = ResolveError{UniqueID: pathKey, Err: err}
		return Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: Resolved{
				UniqueID: pathKey,
				Err:      err,
			},
		}
	}

	tcpAddr := r.NamedAddr(namedPort)

	myServerName := r.ServerName
	if myServerName == "" {
		myServerName = serverName
	}
	if myServerName == "" {
		myServerName = tcpAddr.IP.String()
	}

	grpcAddr := resolver.Address{
		Addr:       tcpAddr.String(),
		ServerName: myServerName,
	}
	grpcAddr = WithMembership(grpcAddr, r)

	return Event{
		Type: UpdateEvent,
		Key:  pathKey,
		Data: Resolved{
			UniqueID:       pathKey,
			ServerName:     myServerName,
			ShardNumber:    r.ShardNumber,
			HasShardNumber: r.HasShardNumber,
			Addr:           tcpAddr,
			Address:        grpcAddr,
		},
	}
}
