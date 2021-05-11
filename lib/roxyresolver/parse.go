package roxyresolver

import (
	"errors"
	"fmt"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

func parseMembershipData(namedPort string, serverName string, pathKey string, bytes []byte) Event {
	r := new(membership.Roxy)
	if err := r.Parse(bytes); err != nil {
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

	var (
		shardID    uint32
		hasShardID bool
	)
	if r.ShardID != nil {
		shardID, hasShardID = *r.ShardID, true
	}

	tcpAddr := r.NamedAddr(namedPort)
	if tcpAddr == nil {
		var err error
		if namedPort == "" {
			err = errors.New("missing host:port data")
		} else {
			err = fmt.Errorf("no such named port %q", namedPort)
		}
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
			ShardID:    shardID,
			HasShardID: hasShardID,
			Addr:       tcpAddr,
			Address:    resAddr,
		},
	}
}
