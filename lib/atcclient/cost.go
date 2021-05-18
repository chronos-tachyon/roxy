package atcclient

import (
	"fmt"
	"sync"
	"time"
)

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

type CostData struct {
	Counter   uint64
	PerSecond float64
}

func GetCostData() CostData {
	var out CostData

	gCostMu.Lock()
	out.Counter = gCostCounterNow
	out.PerSecond = gCostPerSec
	gCostMu.Unlock()

	return out
}

func Spend(cost uint) {
	if cost > 65536 {
		panic(fmt.Errorf("cost %d is out of range", cost))
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
