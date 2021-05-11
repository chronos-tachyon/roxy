package misc

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
	"sync"
)

var gTCPAddrCanonicalizer tcpAddrCanonicalizer

type tcpAddrCanonicalizer struct {
	mu    sync.Mutex
	byKey map[string]*net.TCPAddr
}

func (self *tcpAddrCanonicalizer) Canonicalize(addr *net.TCPAddr) *net.TCPAddr {
	if addr == nil {
		panic(errors.New("*net.TCPAddr is nil"))
	}

	key := makeAddrKey(addr)

	self.mu.Lock()
	if self.byKey == nil {
		self.byKey = make(map[string]*net.TCPAddr, 8)
	}
	canonicalAddr, found := self.byKey[key]
	if found {
		addr = canonicalAddr
	} else {
		self.byKey[key] = addr
	}
	self.mu.Unlock()

	return addr
}

func CanonicalizeTCPAddr(addr *net.TCPAddr) *net.TCPAddr {
	return gTCPAddrCanonicalizer.Canonicalize(addr)
}

func makeAddrKey(addr *net.TCPAddr) string {
	ip := addr.IP
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}

	buf := make([]byte, 0, 4+len(ip)+len(addr.Zone))
	buf = append(buf, byte(len(ip)))
	buf = append(buf, []byte(ip)...)
	buf = append(buf, byte(len(addr.Zone)))
	buf = append(buf, []byte(addr.Zone)...)
	n := len(buf)
	buf = append(buf, 0, 0)
	binary.BigEndian.PutUint16(buf[n:], uint16(addr.Port))
	return base64.StdEncoding.EncodeToString(buf)
}
