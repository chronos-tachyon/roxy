package roxyresolver

import (
	"sync"

	"github.com/rs/zerolog"
)

const checkDisabled = false

const (
	minLoad = 1.0 / float32(1024.0)
	maxLoad = float32(1024.0)

	minWeight = 1.0 / float32(65536.0)
	maxWeight = float32(65536.0)
)

var gDataMutex sync.Mutex

var (
	gMu     sync.Mutex
	gLogger *zerolog.Logger = newNop()
)

func newNop() *zerolog.Logger {
	logger := zerolog.Nop()
	return &logger
}

func SetLogger(logger zerolog.Logger) {
	gMu.Lock()
	gLogger = &logger
	gMu.Unlock()
}

func Logger() *zerolog.Logger {
	gMu.Lock()
	logger := gLogger
	gMu.Unlock()
	return logger
}
