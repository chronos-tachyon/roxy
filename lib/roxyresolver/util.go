package roxyresolver

import (
	"net"
	"strconv"
	"strings"
)

func makeAddressList(resolved []*Resolved) []Address {
	list := make([]Address, 0, len(resolved))
	for _, data := range resolved {
		if data.Addr != nil {
			list = append(list, WithResolved(data.Address, data))
		}
	}
	return list
}

func parseIPAndPort(host, port string) *net.TCPAddr {
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
		return nil
	}

	u64, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil
	}

	return &net.TCPAddr{
		IP:   ip,
		Port: int(u64),
		Zone: zone,
	}
}
