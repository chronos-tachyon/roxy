package roxyresolver

import (
	"fmt"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

func parseMembershipData(namedPort string, serverName string, pathKey string, bytes []byte) Event {
	r := new(membership.Roxy)

	if err := r.UnmarshalJSON(bytes); err != nil {
		err = fmt.Errorf("%s: %w", pathKey, err)
		return Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: Resolved{
				Unique: pathKey,
				Err:    err,
			},
		}
	}

	if !r.Ready {
		var err error = BadStatusError{membership.StatusDead}
		err = fmt.Errorf("%s: %w", pathKey, err)
		return Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: Resolved{
				Unique: pathKey,
				Err:    err,
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

	resAddr := Address{
		Addr:       tcpAddr.String(),
		ServerName: myServerName,
	}
	resAddr = WithMembership(resAddr, r)

	return Event{
		Type: UpdateEvent,
		Key:  pathKey,
		Data: Resolved{
			Unique:     pathKey,
			ServerName: myServerName,
			ShardID:    r.ShardID,
			HasShardID: r.HasShardID,
			Addr:       tcpAddr,
			Address:    resAddr,
		},
	}
}
