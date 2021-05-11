package roxyresolver

import (
	"context"
	"net"

	"github.com/chronos-tachyon/roxy/internal/misc"
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

func makeStaticRecordsForIP(host string, port string, serverName string) []Resolved {
	tcpAddr, err := misc.ParseTCPAddr(net.JoinHostPort(host, port), "")
	if err != nil {
		panic(err)
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

	tcpAddr, err := misc.ParseTCPAddr(str, dnsPort)
	if err != nil {
		return nil, err
	}

	address := tcpAddr.String()
	dialFunc := func(ctx context.Context, network string, _ string) (net.Conn, error) {
		var defaultDialer net.Dialer
		return defaultDialer.DialContext(ctx, network, address)
	}

	return &net.Resolver{PreferGo: true, Dial: dialFunc}, nil
}
