package baseresolver

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

func addrDataCompare(a *AddrData, b *AddrData) int {
	ap := (a.SRVPriority == nil)
	bp := (b.SRVPriority == nil)
	aw := (a.SRVWeight == nil)
	bw := (b.SRVWeight == nil)
	if ap != bp || aw != bw {
		panic(fmt.Errorf("SRV records mixed with non-SRV records"))
	}
	if ap {
		if *a.SRVPriority != *b.SRVPriority {
			return int(*a.SRVPriority) - int(*b.SRVPriority)
		}
		if *a.SRVWeight != *b.SRVWeight {
			return int(*b.SRVWeight) - int(*a.SRVWeight)
		}
	}
	return tcpAddrCompare(*a.Addr.(*net.TCPAddr), *b.Addr.(*net.TCPAddr))
}

type addrDataList []*AddrData

func (list addrDataList) Len() int {
	return len(list)
}

func (list addrDataList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list addrDataList) Less(i, j int) bool {
	a, b := list[i], list[j]
	return addrDataCompare(a, b) < 0
}

func (list addrDataList) Sort() {
	sort.Sort(list)
}

var _ sort.Interface = addrDataList(nil)
