package balancedclient

import (
	"context"
	"crypto/tls"
	"math/rand"
	"net"
	"time"

	zkclient "github.com/go-zookeeper/zk"
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
	ZK           *zkclient.Conn
	Dialer       *net.Dialer
	TLSConfig    *tls.Config
}
