package roxyresolver

import (
	"context"
	"net"
	"strconv"
	"strings"
)

func makeAddressList(resolved []Resolved) []Address {
	list := make([]Address, 0, len(resolved))
	for _, data := range resolved {
		if data.Addr != nil {
			list = append(list, WithResolved(data.Address, data))
		}
	}
	return list
}

func parseIPAndPort(host, port string) (*net.TCPAddr, error) {
	var (
		ipStr string
		zone  string
	)
	i := strings.IndexByte(host, '%')
	if i >= 0 {
		ipStr, zone = host[:i], host[i+1:]
	} else {
		ipStr = host
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, BadIPError{IP: ipStr}
	}

	u64, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, BadPortError{Port: port, Err: err}
	}

	tcpAddr := &net.TCPAddr{
		IP:   ip,
		Port: int(u64),
		Zone: zone,
	}
	return tcpAddr, nil
}

func makeStaticRecordsForIP(host string, port string, serverName string) []Resolved {
	tcpAddr, err := parseIPAndPort(host, port)
	if err != nil {
		return nil
	}
	if serverName == "" {
		serverName = tcpAddr.IP.String()
	}
	resAddr := Address{
		Addr:       tcpAddr.String(),
		ServerName: serverName,
	}
	records := make([]Resolved, 1)
	records[0] = Resolved{
		Unique:     tcpAddr.String(),
		ServerName: serverName,
		Addr:       tcpAddr,
		Address:    resAddr,
	}
	return records
}

func parseNetResolver(str string) (*net.Resolver, error) {
	if str == "" {
		return &net.Resolver{}, nil
	}

	host, port, err := net.SplitHostPort(str)
	if err != nil {
		h, p, err2 := net.SplitHostPort(str + ":" + dnsPort)
		if err2 == nil {
			host, port, err = h, p, nil
		}
		if err != nil {
			return nil, BadHostPortError{HostPort: str, Err: err}
		}
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, BadIPError{IP: host}
	}

	portNum, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, BadPortError{Port: port, Err: err}
	}

	host = ip.String()
	port = strconv.FormatUint(portNum, 10)
	hostPort := net.JoinHostPort(host, port)

	dialFunc := func(ctx context.Context, network string, address string) (net.Conn, error) {
		var defaultDialer net.Dialer
		return defaultDialer.DialContext(ctx, network, hostPort)
	}

	return &net.Resolver{PreferGo: true, Dial: dialFunc}, nil
}
