package misc

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
	"sync"
)

var (
	gCanonMu  sync.Mutex
	gCanonMap map[string]*net.TCPAddr
)

// CanonicalizeTCPAddr takes a TCPAddr and returns a TCPAddr with the same
// value, but with the guarantee that two calls to CanonicalizeTCPAddr with the
// same TCPAddr value will return the same pointer.
func CanonicalizeTCPAddr(addr *net.TCPAddr) *net.TCPAddr {
	if addr == nil {
		panic(errors.New("*net.TCPAddr is nil"))
	}

	key := makeAddrKey(addr)

	gCanonMu.Lock()
	if gCanonMap == nil {
		gCanonMap = make(map[string]*net.TCPAddr, 8)
	}
	canonicalAddr, found := gCanonMap[key]
	if found {
		addr = canonicalAddr
	} else {
		addr = maybeIPv4(addr)
		gCanonMap[key] = addr
	}
	gCanonMu.Unlock()

	return addr
}

func makeAddrKey(addr *net.TCPAddr) string {
	ip := []byte(addr.IP.To16())
	port := uint16(addr.Port)
	zone := []byte(addr.Zone)

	buf := make([]byte, 0, 4+len(ip)+len(zone))
	buf = append(buf, byte(len(ip)))
	buf = append(buf, ip...)
	buf = append(buf, byte(len(zone)))
	buf = append(buf, zone...)
	n := len(buf)
	buf = append(buf, 0, 0)
	binary.BigEndian.PutUint16(buf[n:], port)
	return base64.StdEncoding.EncodeToString(buf)
}

func maybeIPv4(addr *net.TCPAddr) *net.TCPAddr {
	ipv4 := addr.IP.To4()
	if ipv4 == nil {
		return addr
	}

	clone := &net.TCPAddr{
		IP:   ipv4,
		Port: addr.Port,
		Zone: addr.Zone,
	}
	return clone
}
