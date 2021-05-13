package misc

import (
	"net"
	"strings"
)

// CompareIP returns {a < b: -1, a == b: 0, a > b: +1}.
func CompareIP(a net.IP, b net.IP) int {
	a, b = a.To16(), b.To16()
	if a == nil && b != nil {
		return -1
	}
	if a != nil && b == nil {
		return 1
	}
	if a == nil {
		return 0
	}
	for i := 0; i < 16; i++ {
		ax, bx := a[i], b[i]
		if ax < bx {
			return -1
		}
		if ax > bx {
			return 1
		}
	}
	return 0
}

// CompareTCPAddr returns {a < b: -1, a == b: 0, a > b: +1}.
func CompareTCPAddr(a *net.TCPAddr, b *net.TCPAddr) int {
	if a == nil && b != nil {
		return -1
	}
	if a != nil && b == nil {
		return 1
	}
	if a == nil {
		return 0
	}
	if cmp := strings.Compare(a.Zone, b.Zone); cmp != 0 {
		return cmp
	}
	if cmp := CompareIP(a.IP, b.IP); cmp != 0 {
		return cmp
	}
	if a.Port < b.Port {
		return -1
	}
	if a.Port > b.Port {
		return 1
	}
	return 0
}
