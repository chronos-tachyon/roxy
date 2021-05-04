package roxyresolver

import (
	"errors"
	"net"
	"sort"
	"strings"
)

// type ResolvedList {{{

type ResolvedList []Resolved

func (list ResolvedList) Len() int {
	return len(list)
}

func (list ResolvedList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list ResolvedList) Less(i, j int) bool {
	a, b := list[i], list[j]
	return resolvedCompare(a, b) < 0
}

func (list ResolvedList) Sort() {
	sort.Sort(list)
}

var _ sort.Interface = ResolvedList(nil)

// }}}

func resolvedCompare(a Resolved, b Resolved) int {
	if a.HasSRV != b.HasSRV {
		panic(errors.New("SRV records mixed with non-SRV records"))
	}
	if a.HasSRV {
		if a.SRVPriority != b.SRVPriority {
			return int(a.SRVPriority) - int(b.SRVPriority)
		}
		if a.SRVWeight != b.SRVWeight {
			return int(b.SRVWeight) - int(a.SRVWeight)
		}
	}
	if cmp := tcpAddrCompare(*a.Addr.(*net.TCPAddr), *b.Addr.(*net.TCPAddr)); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.Unique, b.Unique)
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

func ipCompare(a net.IP, b net.IP) int {
	a, b = a.To16(), b.To16()
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	default:
		for i := 0; i < 16; i++ {
			ax, bx := a[i], b[i]
			if ax != bx {
				return int(ax) - int(bx)
			}
		}
		return 0
	}
}
