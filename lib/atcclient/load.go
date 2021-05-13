package atcclient

import (
	"sync"
)

type LoadFunc func() float32

var DefaultLoadFunc LoadFunc = defaultLoadFunc

var (
	gLoadMu    sync.Mutex
	gLoadValue float32
)

func defaultLoadFunc() float32 {
	gLoadMu.Lock()
	load := gLoadValue
	gLoadMu.Unlock()
	return load
}

func AddLoad(value float32) {
	gLoadMu.Lock()
	gLoadValue += value
	gLoadMu.Unlock()
}

func SubLoad(value float32) {
	AddLoad(-value)
}
