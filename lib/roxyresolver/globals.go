package roxyresolver

import (
	"regexp"
	"sync"

	"github.com/rs/zerolog"
)

const (
	minLoad = 1.0 / float32(1024.0)
	maxLoad = float32(1024.0)

	minWeight = 1.0 / float32(65536.0)
	maxWeight = float32(65536.0)

	nullString = "null"

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
	atcPort   = "2987"
)

var (
	checkDisabled = true

	gMu     sync.Mutex
	gLogger *zerolog.Logger = newNop()

	reScheme    = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:[+-][0-9A-Za-z]+)*$`)
	reNamedPort = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:[._+-][0-9A-Za-z]+)*$`)
	reLBHost    = regexp.MustCompile(`^[0-9A-Za-z]+(?:[._-][0-9A-Za-z]+)*\.?$`)
	reLBPort    = regexp.MustCompile(`^(?:[0-9]+|[A-Za-z][0-9A-Za-z]*(?:[_-][0-9A-Za-z]+)*)$`)
	reLBName    = regexp.MustCompile(`^[0-9A-Za-z]+(?:[._-][0-9A-Za-z]+)*$`)

	boolMap = map[string]bool{
		"0": false,
		"1": true,

		"off": false,
		"on":  true,

		"n":   false,
		"no":  false,
		"y":   true,
		"yes": true,

		"f":     false,
		"false": false,
		"t":     true,
		"true":  true,
	}
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
