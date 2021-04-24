package baseresolver

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/grpcutil"
	"github.com/chronos-tachyon/roxy/common/membership"
	"github.com/chronos-tachyon/roxy/roxypb"
)

var (
	reScheme    = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:[+-][0-9A-Za-z]+)*$`)
	reNamedPort = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:[._+-][0-9A-Za-z]+)*$`)
	reLBHost    = regexp.MustCompile(`^[0-9A-Za-z]+(?:[._-][0-9A-Za-z]+)*\.?$`)
	reLBPort    = regexp.MustCompile(`^(?:[0-9]+|[A-Za-z][0-9A-Za-z]*(?:[_-][0-9A-Za-z]+)*)$`)
	reLBName    = regexp.MustCompile(`^[0-9A-Za-z]+(?:[._-][0-9A-Za-z]+)*$`)
)

var boolMap = map[string]bool{
	"0":     false,
	"n":     false,
	"no":    false,
	"false": false,

	"1":    true,
	"y":    true,
	"yes":  true,
	"true": true,
}

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

func ParseTargetString(str string) (resolver.Target, error) {
	var target resolver.Target

	i := strings.IndexByte(str, ':')
	if i >= 0 {
		target.Scheme = str[:i]
		str = str[i+1:]
		if len(str) >= 2 && str[0] == '/' && str[1] == '/' {
			str = str[2:]
			j := strings.IndexByte(str, '/')
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

	if target.Scheme != "" && !reScheme.MatchString(target.Scheme) {
		return resolver.Target{}, fmt.Errorf("invalid Target.Scheme %q", target.Scheme)
	}

	return target, nil
}

func ParseEtcdTarget(target resolver.Target) (etcdPrefix string, etcdPort string, query url.Values, err error) {
	if target.Authority != "" {
		err = fmt.Errorf("unexpected non-empty Target.Authority %q", target.Authority)
		return
	}

	ep := target.Endpoint

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	var hasPort bool
	if i := strings.IndexByte(ep, ':'); i >= 0 {
		etcdPrefix, etcdPort, hasPort = ep[:i], ep[i+1:], true
	} else {
		etcdPrefix, etcdPort, hasPort = ep, "", false
	}

	if !strings.HasSuffix(etcdPrefix, "/") {
		etcdPrefix += "/"
	}
	if strings.Contains(etcdPrefix, "//") {
		err = fmt.Errorf("Target.Endpoint path %q must not contain two or more consecutive slashes", etcdPrefix)
		return
	}

	if hasPort && etcdPort == "" {
		err = errors.New("Target.Endpoint port must be non-empty (if present)")
		return
	}
	if hasPort && !reNamedPort.MatchString(etcdPort) {
		err = fmt.Errorf("failed to parse Target.Endpoint port %q", etcdPort)
		return
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("failed to parse Target.Endpoint query string %q: %w", qs, err)
			return
		}
	}

	return
}

func ParseZKTarget(target resolver.Target) (zkPath string, zkPort string, query url.Values, err error) {
	if target.Authority != "" {
		err = fmt.Errorf("unexpected non-empty Target.Authority %q", target.Authority)
		return
	}

	ep := target.Endpoint
	if !strings.HasPrefix(ep, "/") {
		ep = "/" + ep
	}

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	var hasPort bool
	if i := strings.IndexByte(ep, ':'); i >= 0 {
		zkPath, zkPort, hasPort = ep[:i], ep[i+1:], false
	} else {
		zkPath, zkPort, hasPort = ep, "", true
	}

	if zkPath != "/" && strings.HasSuffix(zkPath, "/") {
		err = fmt.Errorf("Target.Endpoint path %q must not end with slash", zkPath)
		return
	}
	if strings.Contains(zkPath, "//") {
		err = fmt.Errorf("Target.Endpoint path %q must not contain two or more consecutive slashes", zkPath)
		return
	}
	if strings.Contains(zkPath+"/", "/./") {
		err = fmt.Errorf("Target.Endpoint path %q must not contain \".\"", zkPath)
		return
	}
	if strings.Contains(zkPath+"/", "/../") {
		err = fmt.Errorf("Target.Endpoint path %q must not contain \"..\"", zkPath)
		return
	}

	if hasPort && zkPort == "" {
		err = errors.New("Target.Endpoint port must be non-empty (if present)")
		return
	}
	if hasPort && !reNamedPort.MatchString(zkPort) {
		err = fmt.Errorf("failed to parse Target.Endpoint port %q", zkPort)
		return
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("failed to parse Target.Endpoint query string %q: %w", qs, err)
			return
		}
	}

	return
}

func ParseATCTarget(target resolver.Target) (lbHost string, lbPort string, lbName string, query url.Values, err error) {
	const defaultPort = ":2987"

	if target.Authority == "" {
		err = fmt.Errorf("Authority must not be empty")
		return
	}

	lbHost, lbPort, err = net.SplitHostPort(target.Authority)
	if err != nil {
		h, p, err2 := net.SplitHostPort(target.Authority + defaultPort)
		if err2 == nil {
			lbHost, lbPort, err = h, p, nil
		}
	}
	if err != nil {
		err = fmt.Errorf("failed to parse Target.Authority %q as <lbhost>:<lbport>: %w", target.Authority, err)
		return
	}
	if !reLBHost.MatchString(lbHost) {
		err = fmt.Errorf("failed to parse Target.Authority <lbhost> %q as DNS hostname", lbHost)
		return
	}
	if !reLBPort.MatchString(lbPort) {
		err = fmt.Errorf("failed to parse Target.Authority <lbport> %q as port number or named port", lbPort)
		return
	}

	ep := target.Endpoint
	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	lbName = ep
	if !reLBName.MatchString(lbName) {
		err = fmt.Errorf("failed to parse Target.Endpoint %q as <lbname>", ep)
		return
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("failed to parse Target.Endpoint query string %q: %w", qs, err)
			return
		}
	}

	return
}

func ParseServerSetData(portName string, pathKey string, value []byte) *Event {
	ss := new(membership.ServerSet)
	if err := ss.Parse(value); err != nil {
		err = fmt.Errorf("%s: %w", pathKey, err)
		return &Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: &AddrData{Err: err, Raw: ss},
		}
	}

	if !ss.IsAlive() {
		var err error = BadStatusError{ss.Status}
		err = fmt.Errorf("%s: %w", pathKey, err)
		return &Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: &AddrData{Err: err, Raw: ss},
		}
	}

	tcpAddr := ss.TCPAddrForPort(portName)
	if tcpAddr == nil {
		var err error
		if portName == "" {
			err = fmt.Errorf("missing host:port data")
		} else {
			err = fmt.Errorf("no such named port %q", portName)
		}
		err = fmt.Errorf("%s: %w", pathKey, err)
		return &Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: &AddrData{Err: err, Raw: ss},
		}
	}

	unique := new(string)
	*unique = pathKey

	var shardID *int32
	if ss.ShardID >= 0 {
		shardID = new(int32)
		*shardID = ss.ShardID
	}

	var serverNamePtr *string
	serverName := ss.Metadata["ServerName"]
	if serverName != "" {
		*serverNamePtr = serverName
	}

	return &Event{
		Type: UpdateEvent,
		Key:  pathKey,
		Data: &AddrData{
			Raw:        ss,
			Addr:       tcpAddr,
			UniqueKey:  unique,
			ServerName: serverNamePtr,
			ShardID:    shardID,
			Address: grpcutil.WithServerSet(
				resolver.Address{
					Addr:       tcpAddr.String(),
					ServerName: serverName,
				},
				ss),
		},
	}
}

func ParseATCEndpointData(ep *roxypb.Endpoint) *Event {
	tcpAddr := &net.TCPAddr{
		IP:   net.IP(ep.Ip),
		Zone: ep.Zone,
		Port: int(ep.Port),
	}

	unique := new(string)
	*unique = ep.Unique

	location := new(string)
	*location = ep.Location

	var serverName *string
	if ep.ServerName != "" {
		serverName = new(string)
		*serverName = ep.ServerName
	}

	var shardID *int32
	if ep.ShardId >= 0 {
		shardID = new(int32)
		*shardID = ep.ShardId
	}

	weight := new(float32)
	*weight = ep.Weight

	return &Event{
		Type: UpdateEvent,
		Key:  ep.Unique,
		Data: &AddrData{
			Addr:           tcpAddr,
			UniqueKey:      unique,
			Location:       location,
			ServerName:     serverName,
			ShardID:        shardID,
			AssignedWeight: weight,
			Address: grpcutil.WithEndpoint(
				resolver.Address{
					Addr:       tcpAddr.String(),
					ServerName: ep.ServerName,
				},
				ep),
		},
	}
}
