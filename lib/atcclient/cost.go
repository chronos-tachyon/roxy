package atcclient

import (
	"sync"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

var (
	gCostMutex   sync.Mutex
	gCostCounter uint64
)

// GetCostCounter captures a snapshot of the cost counter.
func GetCostCounter() uint64 {
	gCostMutex.Lock()
	costCounter := gCostCounter
	gCostMutex.Unlock()
	return costCounter
}

// Spend records that a query just happened and the cost value of that query.
//
// (For clients, this represents demand created by outgoing requests.
// For servers, this represents supply consumed by incoming requests.)
//
// The cost must be a number between 0 and 65536, inclusive.
func Spend(cost uint) {
	roxyutil.Assertf(cost <= 65536, "cost %d is out of range (0..65536)", cost)

	gCostMutex.Lock()
	gCostCounter += uint64(cost)
	gCostMutex.Unlock()
}
