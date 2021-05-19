package roxyresolver

import (
	"regexp"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
)

var (
	checkDisabled = true

	gMu     sync.Mutex
	gLogger *zerolog.Logger = newNop()

	gLastWatchID uint64 = 0

	reTargetScheme = regexp.MustCompile(`^([0-9A-Za-z+-]+):`)
)

const (
	minLoad = 1.0 / float32(1024.0)
	maxLoad = float32(1024.0)

	minWeight = 1.0 / float32(65536.0)
	maxWeight = float32(65536.0)

	atcBalancerName = "atc_lb"
)

func generateWatchID() WatchID {
	return WatchID(atomic.AddUint64(&gLastWatchID, 1))
}

func newNop() *zerolog.Logger {
	logger := zerolog.Nop()
	return &logger
}

// EnableCheck enables performance-degrading data integrity checks.  Meant only
// for internal use.
func EnableCheck() {
	checkDisabled = false
}

// SetLogger sets the global logger to use for Resolver background threads.
func SetLogger(logger zerolog.Logger) {
	gMu.Lock()
	gLogger = &logger
	gMu.Unlock()
}

// Logger returns the global logger to use for Resolver background threads.
func Logger() *zerolog.Logger {
	gMu.Lock()
	logger := gLogger
	gMu.Unlock()
	return logger
}
