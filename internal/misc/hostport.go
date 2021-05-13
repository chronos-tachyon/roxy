package misc

import (
	"net"
	"strconv"
	"strings"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func SplitHostPort(str, defaultPort string) (host string, port string, err error) {
	if str == "" {
		return "", "", roxyutil.BadHostPortError{HostPort: str, Err: roxyutil.ErrExpectNonEmpty}
	}

	host, port, err = net.SplitHostPort(str)
	if err != nil && defaultPort != "" {
		h, p, err2 := net.SplitHostPort(str + ":" + defaultPort)
		if err2 == nil {
			host, port, err = h, p, nil
		}
	}
	if err != nil {
		return "", "", roxyutil.BadHostPortError{HostPort: str, Err: err}
	}
	if host == "" {
		return "", "", roxyutil.BadHostError{Host: host, Err: roxyutil.ErrExpectNonEmpty}
	}
	return host, port, nil
}

func ParseTCPAddrList(str, defaultPort string) ([]*net.TCPAddr, error) {
	list := strings.Split(str, ",")
	out := make([]*net.TCPAddr, 0, len(list))
	for _, item := range list {
		if item == "" {
			continue
		}
		tcpAddr, err := ParseTCPAddr(item, defaultPort)
		if err != nil {
			return nil, err
		}
		out = append(out, tcpAddr)
	}
	return out, nil
}

func ParseTCPAddr(str, defaultPort string) (*net.TCPAddr, error) {
	host, port, err := SplitHostPort(str, defaultPort)
	if err != nil {
		return nil, err
	}

	ip, zone, err := ParseIPAndZone(host)
	if err != nil {
		return nil, err
	}

	portNum, err := ParsePort(port)
	if err != nil {
		return nil, err
	}

	tcpAddr := &net.TCPAddr{
		IP:   ip,
		Port: int(portNum),
		Zone: zone,
	}
	return tcpAddr, nil
}

func ParseIPAndZone(host string) (net.IP, string, error) {
	var zone string
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host, zone = host[:i], host[i+1:]
	}
	ip, err := ParseIP(host)
	return ip, zone, err
}

func ParseIP(host string) (net.IP, error) {
	if host == "" {
		return nil, roxyutil.BadHostError{Host: host, Err: roxyutil.ErrExpectNonEmpty}
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, roxyutil.BadHostError{Host: host, Err: roxyutil.BadIPError{IP: host}}
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return ip, nil
}

func ParsePort(port string) (uint16, error) {
	u64, err0 := strconv.ParseUint(port, 10, 16)
	if err0 == nil {
		return uint16(u64), nil
	}

	p, err1 := net.LookupPort("tcp", port)
	if err1 == nil {
		return uint16(p), nil
	}

	return 0, roxyutil.BadPortError{Port: "port", Err: err0, NamedOK: true}
}