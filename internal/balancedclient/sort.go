package balancedclient

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

func ipCompare(a net.IP, b net.IP) int {
	switch len(a) {
	case 4:
		switch len(b) {
		case 4:
			for i := 0; i < 4; i++ {
				ax, bx := a[i], b[i]
				if ax != bx {
					return int(ax) - int(bx)
				}
			}
			return 0
		case 16:
			return -1
		default:
			panic(fmt.Errorf("bad IP address: %#v", []byte(b)))
		}

	case 16:
		switch len(b) {
		case 4:
			return 1
		case 16:
			for i := 0; i < 16; i++ {
				ax, bx := a[i], b[i]
				if ax != bx {
					return int(ax) - int(bx)
				}
			}
			return 0
		default:
			panic(fmt.Errorf("bad IP address: %#v", []byte(b)))
		}

	default:
		panic(fmt.Errorf("bad IP address: %#v", []byte(a)))
	}
}

func tcpAddrCompare(a net.TCPAddr, b net.TCPAddr) int {
	if cmp := strings.Compare(a.Zone, b.Zone); cmp != 0 {
		return cmp
	}
	if cmp := ipCompare(a.IP, b.IP); cmp != 0 {
		return cmp
	}
	return a.Port - b.Port
}

func srvCompare(a ResolvedSRV, b ResolvedSRV) int {
	if a.Priority != b.Priority {
		return int(a.Priority) - int(b.Priority)
	}
	if a.RawWeight != b.RawWeight {
		return int(b.RawWeight) - int(a.RawWeight)
	}
	return tcpAddrCompare(a.Addr, b.Addr)
}

type tcpAddrList []net.Addr

func (list tcpAddrList) Len() int {
	return len(list)
}

func (list tcpAddrList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list tcpAddrList) Less(i, j int) bool {
	a := list[i].(*net.TCPAddr)
	b := list[i].(*net.TCPAddr)
	return tcpAddrCompare(*a, *b) < 0
}

func (list tcpAddrList) Sort() {
	sort.Sort(list)
}

var _ sort.Interface = tcpAddrList(nil)
