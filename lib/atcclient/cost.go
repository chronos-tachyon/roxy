package atcclient

import (
	"fmt"
	"sync"
	"time"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// CostInterval is the span of time over which the average cost-per-second is
// computed, for estimating future cost spend.
const CostInterval = 60 * time.Second

var (
	gCostMu          sync.Mutex
	gCostCounterNow  uint64
	gCostCounterThen uint64
	gCostPerSec      float64
	gCostSamples     []costSample
)

type costSample struct {
	t time.Time
	c uint64
}

// CostData represents a snapshot of this Go process's cost expenditures.
//
// (For clients, this represents demand created by outgoing requests.
// For servers, this represents supply consumed by incoming requests.)
type CostData struct {
	Counter   uint64
	PerSecond float64
}

// GetCostData captures a CostData snapshot.
func GetCostData() CostData {
	var out CostData

	gCostMu.Lock()
	out.Counter = gCostCounterNow
	out.PerSecond = gCostPerSec
	gCostMu.Unlock()

	return out
}

// Spend records that a query just happened and the cost value of that query.
//
// (For clients, this represents demand created by outgoing requests.
// For servers, this represents supply consumed by incoming requests.)
//
// The cost must be a number between 0 and 65536, inclusive.
func Spend(cost uint) {
	if cost == 0 {
		return
	}

	if cost > 65536 {
		panic(roxyutil.CheckError{
			Message: fmt.Sprintf("cost %d is out of range (1..65536)", cost),
		})
	}

	gCostMu.Lock()
	now := time.Now().UTC()
	gCostCounterNow += uint64(cost)
	ejectCostSamples(now.Add(-CostInterval))
	gCostSamples = append(gCostSamples, costSample{now, gCostCounterNow})
	gCostPerSec = float64(gCostCounterNow-gCostCounterThen) / float64(CostInterval/time.Second)
	gCostMu.Unlock()
}

func ejectCostSamples(limit time.Time) {
	length := uint(len(gCostSamples))
	i := uint(0)
	for i < length {
		sample := gCostSamples[i]
		if sample.t.After(limit) {
			break
		}
		i++
	}
	if i != 0 {
		gCostCounterThen = gCostSamples[i-1].c
		gCostSamples = gCostSamples[i:]
	}
}
