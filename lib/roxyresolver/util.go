package roxyresolver

import (
	"context"
	"net"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
)

func makeAddressList(resolved []Resolved) []resolver.Address {
	list := make([]resolver.Address, 0, len(resolved))
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
	grpcAddr := resolver.Address{
		Addr:       tcpAddr.String(),
		ServerName: serverName,
	}
	records := make([]Resolved, 1)
	records[0] = Resolved{
		Unique:     tcpAddr.String(),
		ServerName: serverName,
		Addr:       tcpAddr,
		Address:    grpcAddr,
	}
	return records
}

func parseNetResolver(str string) (*net.TCPAddr, error) {
	if str == "" {
		return nil, nil
	}

	tcpAddr, err := misc.ParseTCPAddr(str, constants.PortDNS)
	if err != nil {
		return nil, err
	}

	return tcpAddr, nil
}

func makeNetResolver(tcpAddr *net.TCPAddr) *net.Resolver {
	res := &net.Resolver{}
	if tcpAddr != nil {
		address := tcpAddr.String()
		res.PreferGo = true
		res.Dial = func(ctx context.Context, network string, _ string) (net.Conn, error) {
			var defaultDialer net.Dialer
			return defaultDialer.DialContext(ctx, network, address)
		}
	}
	return res
}
