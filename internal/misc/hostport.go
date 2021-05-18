package misc

import (
	"net"
	"strconv"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// SplitHostPort splits a string in "<host>:<port>" format, except that if a
// port is not present in the string and defaultPort is non-empty, then
// defaultPort is used instead.
func SplitHostPort(str, defaultPort string) (host string, port string, err error) {
	if str == "" {
		return "", "", roxyutil.HostPortError{HostPort: str, Err: roxyutil.ErrExpectNonEmpty}
	}

	host, port, err = net.SplitHostPort(str)
	if err != nil && defaultPort != "" {
		h, p, err2 := net.SplitHostPort(str + ":" + defaultPort)
		if err2 == nil {
			host, port, err = h, p, nil
		}
	}
	if err != nil {
		return "", "", roxyutil.HostPortError{HostPort: str, Err: err}
	}
	if host == "" {
		return "", "", roxyutil.HostError{Host: host, Err: roxyutil.ErrExpectNonEmpty}
	}
	return host, port, nil
}

// ParseTCPAddrList parses a string in "<ip>:<port>,<ip>:<port>,..." format.
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

// ParseTCPAddr parses a string in "<ip>:<port>" format.
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

// ParseIPAndZone parses a string in "<ip>%<zone>" format.
func ParseIPAndZone(host string) (net.IP, string, error) {
	var zone string
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host, zone = host[:i], host[i+1:]
	}
	ip, err := ParseIP(host)
	return ip, zone, err
}

// ParseIP parses an IP address.
func ParseIP(host string) (net.IP, error) {
	if host == "" {
		return nil, roxyutil.HostError{Host: host, Err: roxyutil.ErrExpectNonEmpty}
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, roxyutil.HostError{Host: host, Err: roxyutil.IPError{IP: host, Err: roxyutil.ErrFailedToMatch}}
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return ip, nil
}

// ParsePort parses a port number.
func ParsePort(port string) (uint16, error) {
	u64, err0 := strconv.ParseUint(port, 10, 16)
	if err0 == nil {
		return uint16(u64), nil
	}

	p, err1 := net.LookupPort(constants.NetTCP, port)
	if err1 == nil {
		return uint16(p), nil
	}

	return 0, roxyutil.PortError{Port: "port", Err: err0, NamedOK: true}
}
