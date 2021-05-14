package roxyresolver

import (
	"sync"

	"github.com/rs/zerolog"
)

const (
	minLoad = 1.0 / float32(1024.0)
	maxLoad = float32(1024.0)

	minWeight = 1.0 / float32(65536.0)
	maxWeight = float32(65536.0)

	nullString = "null"

	passthroughScheme  = "passthrough"
	unixScheme         = "unix"
	unixAbstractScheme = "unix-abstract"
	ipScheme           = "ip"
	dnsScheme          = "dns"
	srvScheme          = "srv"
	zkScheme           = "zk"
	etcdScheme         = "etcd"
	atcScheme          = "atc"

	atcBalancerName = "atc_lb"

	dnsPort   = "53"
	httpPort  = "80"
	httpsPort = "443"
)

var (
	checkDisabled = true

	gMu     sync.Mutex
	gLogger *zerolog.Logger = newNop()
)

func newNop() *zerolog.Logger {
	logger := zerolog.Nop()
	return &logger
}

func EnableCheck() {
	checkDisabled = false
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
