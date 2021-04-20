package balancedclient

import (
	"context"
	"crypto/tls"
	"math/rand"
	"net"
	"time"

	etcdclient "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

type Options struct {
	Context      context.Context
	Target       string
	Balancer     enums.BalancerType
	PollInterval time.Duration
	Random       *rand.Rand
	Etcd         *etcdclient.Client
	Dialer       *net.Dialer
	TLSConfig    *tls.Config
}
