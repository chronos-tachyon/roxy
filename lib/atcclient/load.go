package atcclient

import (
	"sync"
)

var (
	gLoadMu    sync.Mutex
	gLoadValue float32
)

// LoadFunc is a function that returns the current load each time it is called.
// It must be thread-safe.
type LoadFunc func() float32

// DefaultLoadFunc is the default implementation of LoadFunc.
var DefaultLoadFunc LoadFunc = DefaultDefaultLoadFunc

// DefaultDefaultLoadFunc is the default value for DefaultLoadFunc.
func DefaultDefaultLoadFunc() float32 {
	gLoadMu.Lock()
	load := gLoadValue
	gLoadMu.Unlock()
	return load
}

// AddLoad adds the given value to the current load.  Only affects the value
// returned by DefaultDefaultLoadFunc.
func AddLoad(value float32) {
	gLoadMu.Lock()
	gLoadValue += value
	gLoadMu.Unlock()
}

// SubLoad subtracts the given value from the current load.  Only affects the
// value returned by DefaultDefaultLoadFunc.
func SubLoad(value float32) {
	AddLoad(-value)
}
