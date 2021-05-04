package roxyresolver

import (
	"errors"
	"fmt"
	"strings"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

func ParseBool(str string, defaultValue bool) (bool, error) {
	if str == "" {
		return defaultValue, nil
	}
	if value, found := boolMap[str]; found {
		return value, nil
	}
	for name, value := range boolMap {
		if strings.EqualFold(str, name) {
			return value, nil
		}
	}
	return false, fmt.Errorf("failed to parse %q as boolean value", str)
}

func ParseTargetString(str string) Target {
	var target Target

	i := strings.IndexByte(str, ':')
	j := strings.IndexByte(str, '/')
	if i >= 0 && (j < 0 || j > i) && reScheme.MatchString(str[:i]) {
		target.Scheme = str[:i]
		str = str[i+1:]
		if len(str) >= 2 && str[0] == '/' && str[1] == '/' {
			str = str[2:]
			j = strings.IndexByte(str, '/')
			if j >= 0 {
				target.Authority = str[:j]
				target.Endpoint = str[j+1:]
			} else {
				target.Authority = str
			}
		} else {
			target.Endpoint = str
		}
	} else {
		target.Endpoint = str
	}

	return target
}

func ParseServerSetData(portName string, pathKey string, bytes []byte) Event {
	ss := new(membership.ServerSet)
	if err := ss.Parse(bytes); err != nil {
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

	if !ss.IsAlive() {
		var err error = BadStatusError{ss.Status}
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
		shardID    int32
		hasShardID bool
	)
	if ss.ShardID != nil {
		shardID, hasShardID = *ss.ShardID, true
	}

	tcpAddr := ss.TCPAddrForPort(portName)
	if tcpAddr == nil {
		var err error
		if portName == "" {
			err = errors.New("missing host:port data")
		} else {
			err = fmt.Errorf("no such named port %q", portName)
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

	serverName := ss.Metadata["ServerName"]

	resAddr := Address{
		Addr:       tcpAddr.String(),
		ServerName: serverName,
	}
	resAddr = WithServerSet(resAddr, ss)

	return Event{
		Type: UpdateEvent,
		Key:  pathKey,
		Data: Resolved{
			Unique:     pathKey,
			ServerName: serverName,
			ShardID:    shardID,
			HasShardID: hasShardID,
			Addr:       tcpAddr,
			Address:    resAddr,
		},
	}
}
