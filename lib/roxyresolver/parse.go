package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/chronos-tachyon/roxy/lib/membership"
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
	"off":   false,
	"n":     false,
	"no":    false,
	"f":     false,
	"false": false,

	"1":    true,
	"on":   true,
	"y":    true,
	"yes":  true,
	"t":    true,
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

func ParseIPTarget(target Target, defaultPort string) (tcpAddrs []*net.TCPAddr, query url.Values, err error) {
	if target.Authority != "" {
		err = fmt.Errorf("invalid non-empty Target.Authority %q", target.Authority)
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

	if ep == "" {
		err = errors.New("invalid Target.Endpoint: empty <ip>:<port> list")
		return
	}

	listStr, err := url.PathUnescape(ep)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", ep, err)
		return
	}

	ipPortList := strings.Split(listStr, ",")
	tcpAddrs = make([]*net.TCPAddr, 0, len(ipPortList))
	for _, ipPort := range ipPortList {
		if ipPort == "" {
			continue
		}
		var (
			host string
			port string
		)
		host, port, err = net.SplitHostPort(ipPort)
		if err != nil {
			h, p, err2 := net.SplitHostPort(ipPort + ":" + defaultPort)
			if err2 == nil {
				host, port, err = h, p, nil
			}
			if err != nil {
				err = fmt.Errorf("invalid Target.Endpoint: failed to parse <ip>:<port> string %q: %w", ipPort, err)
				return
			}
		}
		tcpAddr := parseIPAndPort(host, port)
		if tcpAddr == nil {
			err = fmt.Errorf("invalid Target.Endpoint: failed to parse <ip>:<port> string %q", net.JoinHostPort(host, port))
			return
		}
		tcpAddrs = append(tcpAddrs, tcpAddr)
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("invalid Target.Endpoint: failed to parse query string %q: %w", qs, err)
			return
		}
	}

	return
}

func ParseDNSTarget(target Target, defaultPort string) (res *net.Resolver, host string, port string, query url.Values, err error) {
	res, err = parseNetResolver(target.Authority)
	if err != nil {
		return
	}

	ep := target.Endpoint
	if ep == "" {
		err = errors.New("Target.Endpoint is empty")
		return
	}

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	hostPort, err := url.PathUnescape(ep)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", ep, err)
		return
	}

	host, port, err = net.SplitHostPort(hostPort)
	if err != nil {
		h, p, err2 := net.SplitHostPort(hostPort + ":" + defaultPort)
		if err2 == nil {
			host, port, err = h, p, nil
		}
		if err != nil {
			err = fmt.Errorf("invalid Target.Endpoint: cannot parse <host>:<port> %q: %w", hostPort, err)
			return
		}
	}
	if host == "" {
		err = errors.New("invalid Target.Endpoint: DNS hostname is empty")
		return
	}
	if port == "" {
		err = errors.New("invalid Target.Endpoint: port is empty")
		return
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("invalid Target.Endpoint: failed to parse query string %q: %w", qs, err)
			return
		}
	}

	return
}

func ParseSRVTarget(target Target) (res *net.Resolver, name string, service string, query url.Values, err error) {
	res, err = parseNetResolver(target.Authority)
	if err != nil {
		return
	}

	ep := target.Endpoint
	if ep == "" {
		err = errors.New("Target.Endpoint is empty")
		return
	}

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	i := strings.IndexByte(ep, '/')
	if i < 0 {
		err = fmt.Errorf("Target.Endpoint %q cannot be parsed as <name>/<service>: missing '/'", ep)
		return
	}
	str1, str2 := ep[:i], ep[i+1:]
	if j := strings.IndexByte(str2, '/'); j >= 0 {
		err = fmt.Errorf("Target.Endpoint %q contains multiple slashes", ep)
		return
	}

	name, err = url.PathUnescape(str1)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", str1, err)
		return
	}

	service, err = url.PathUnescape(str2)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", str2, err)
		return
	}

	if name == "" {
		err = errors.New("invalid Target.Endpoint: DNS domain name is empty")
		return
	}
	if service == "" {
		err = errors.New("invalid Target.Endpoint: DNS service name is empty")
		return
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("invalid Target.Endpoint: failed to parse query string %q: %w", qs, err)
			return
		}
	}

	return
}

func ParseZKTarget(target Target) (zkPath string, zkPort string, query url.Values, err error) {
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

	var (
		str1    string
		str2    string
		hasPort bool
	)
	if i := strings.IndexByte(ep, ':'); i >= 0 {
		str1, str2, hasPort = ep[:i], ep[i+1:], false
	} else {
		str1, str2, hasPort = ep, "", true
	}

	zkPath, err = url.PathUnescape(str1)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", str1, err)
		return
	}

	zkPort, err = url.PathUnescape(str2)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", str2, err)
		return
	}

	if !strings.HasPrefix(zkPath, "/") {
		zkPath = "/" + zkPath
	}
	if zkPath != "/" && strings.HasSuffix(zkPath, "/") {
		err = fmt.Errorf("invalid Target.Endpoint: path %q must not end with slash", zkPath)
		return
	}
	if strings.Contains(zkPath, "//") {
		err = fmt.Errorf("invalid Target.Endpoint: path %q must not contain two or more consecutive slashes", zkPath)
		return
	}
	if strings.Contains(zkPath+"/", "/./") {
		err = fmt.Errorf("invalid Target.Endpoint: path %q must not contain \".\"", zkPath)
		return
	}
	if strings.Contains(zkPath+"/", "/../") {
		err = fmt.Errorf("invalid Target.Endpoint: path %q must not contain \"..\"", zkPath)
		return
	}

	if hasPort && zkPort == "" {
		err = errors.New("invalid Target.Endpoint: port is empty")
		return
	}
	if hasPort && !reNamedPort.MatchString(zkPort) {
		err = fmt.Errorf("invalid Target.Endpoint: failed to parse port %q", zkPort)
		return
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("invalid Target.Endpoint: failed to parse query string %q: %w", qs, err)
			return
		}
	}

	return
}

func ParseEtcdTarget(target Target) (etcdPrefix string, etcdPort string, query url.Values, err error) {
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

	var (
		str1    string
		str2    string
		hasPort bool
	)
	if i := strings.IndexByte(ep, ':'); i >= 0 {
		str1, str2, hasPort = ep[:i], ep[i+1:], true
	} else {
		str1, str2, hasPort = ep, "", false
	}

	etcdPrefix, err = url.PathUnescape(str1)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", str1, err)
		return
	}

	etcdPort, err = url.PathUnescape(str2)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", str2, err)
		return
	}

	if !strings.HasSuffix(etcdPrefix, "/") {
		etcdPrefix += "/"
	}
	if strings.Contains(etcdPrefix, "//") {
		err = fmt.Errorf("invalid Target.Endpoint: path %q must not contain two or more consecutive slashes", etcdPrefix)
		return
	}

	if hasPort && etcdPort == "" {
		err = errors.New("invalid Target.Endpoint: port is empty")
		return
	}
	if hasPort && !reNamedPort.MatchString(etcdPort) {
		err = fmt.Errorf("invalid Target.Endpoint: failed to parse port %q", etcdPort)
		return
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("invalid Target.Endpoint: failed to parse query string %q: %w", qs, err)
			return
		}
	}

	return
}

func ParseATCTarget(target Target) (lbHost string, lbPort string, lbName string, query url.Values, err error) {
	if target.Authority == "" {
		err = errors.New("Target.Authority is empty")
		return
	}

	lbHost, lbPort, err = net.SplitHostPort(target.Authority)
	if err != nil {
		h, p, err2 := net.SplitHostPort(target.Authority + ":2987")
		if err2 == nil {
			lbHost, lbPort, err = h, p, nil
		}
		if err != nil {
			err = fmt.Errorf("Target.Authority %q cannot be parsed as <lbhost>:<lbport>: %w", target.Authority, err)
			return
		}
	}
	if !reLBHost.MatchString(lbHost) {
		err = fmt.Errorf("invalid Target.Authority: failed to parse DNS hostname %q", lbHost)
		return
	}
	if !reLBPort.MatchString(lbPort) {
		err = fmt.Errorf("invalid Target.Authority: failed to parse port number or named port %q", lbPort)
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

	lbName, err = url.PathUnescape(ep)
	if err != nil {
		err = fmt.Errorf("invalid Target.Endpoint: failed to unescape %q: %w", ep, err)
		return
	}
	if !reLBName.MatchString(lbName) {
		err = fmt.Errorf("invalid Target.Endpoint: failed to parse LB target name %q", ep)
		return
	}

	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = fmt.Errorf("invalid Target.Endpoint: failed to parse query string %q: %w", qs, err)
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
			Data: &Resolved{
				Unique: pathKey,
				Err:    err,
			},
		}
	}

	if !ss.IsAlive() {
		var err error = BadStatusError{ss.Status}
		err = fmt.Errorf("%s: %w", pathKey, err)
		return &Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: &Resolved{
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
		return &Event{
			Type: BadDataEvent,
			Key:  pathKey,
			Data: &Resolved{
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

	return &Event{
		Type: UpdateEvent,
		Key:  pathKey,
		Data: &Resolved{
			Unique:     pathKey,
			ServerName: serverName,
			ShardID:    shardID,
			HasShardID: hasShardID,
			Addr:       tcpAddr,
			Address:    resAddr,
		},
	}
}

func parseNetResolver(str string) (*net.Resolver, error) {
	if str == "" {
		return &net.Resolver{}, nil
	}

	dnsHost, dnsPort, err := net.SplitHostPort(str)
	if err != nil {
		h, p, err2 := net.SplitHostPort(str + ":53")
		if err2 == nil {
			dnsHost, dnsPort, err = h, p, nil
		}
		if err != nil {
			return nil, fmt.Errorf("Target.Authority %q cannot be parsed as <ip>:<port>: %w", str, err)
		}
	}

	dnsIP := net.ParseIP(dnsHost)
	if dnsIP == nil {
		return nil, fmt.Errorf("invalid Target.Authority: failed to parse IP address %q", dnsHost)
	}

	dnsPortNum, err := strconv.ParseUint(dnsPort, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid Target.Authority: failed to parse port number %q: %w", dnsPort, err)
	}

	dnsHost = dnsIP.String()
	dnsPort = strconv.FormatUint(dnsPortNum, 10)
	hostPort := net.JoinHostPort(dnsHost, dnsPort)

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network string, address string) (net.Conn, error) {
			var defaultDialer net.Dialer
			return defaultDialer.DialContext(ctx, network, hostPort)
		},
	}, nil
}
