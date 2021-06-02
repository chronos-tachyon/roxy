package roxyresolver

import (
	"errors"
	"net"
	"sort"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/misc"
)

// type ResolvedList {{{

// ResolvedList is a sorted list of Resolved addresses.
type ResolvedList []Resolved

// Len fulfills sort.Interface.
func (list ResolvedList) Len() int {
	return len(list)
}

// Swap fulfills sort.Interface.
func (list ResolvedList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

// Less fulfills sort.Interface.
func (list ResolvedList) Less(i, j int) bool {
	a, b := list[i], list[j]
	return resolvedCompare(a, b) < 0
}

// Sort is a convenience method for sort.Sort(list).
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
	if cmp := misc.CompareTCPAddr(a.Addr.(*net.TCPAddr), b.Addr.(*net.TCPAddr)); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.UniqueID, b.UniqueID)
}
