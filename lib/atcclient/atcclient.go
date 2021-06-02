package atcclient

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// ReportInterval is the time interval between reports sent within
// ServerAnnounce and ClientAssign.
const ReportInterval = 1 * time.Second

var gBackoff expbackoff.ExpBackoff = expbackoff.BuildDefault()

// ATCClient is a client for communicating with the Roxy Air Traffic Controller
// service.
//
// Each ATC tower is responsible for a set of (ServiceName, ShardNumber)
// tuples, which are exclusive to that tower.  The client automatically
// determines which tower it needs to speak with, by asking any tower within
// the service to provide instructions.
type ATCClient struct {
	tc      *tls.Config
	cc      *grpc.ClientConn
	atc     roxy_v0.AirTrafficControlClient
	closeCh chan struct{}

	wg         sync.WaitGroup
	mu         sync.Mutex
	serviceMap map[Key]*serviceData
	connMap    map[*net.TCPAddr]*connData
	closed     bool
}

type serviceData struct {
	timeout time.Time
	err     error
	cv      *sync.Cond
	addr    *net.TCPAddr
	retries uint32
	ready   bool
}

type connData struct {
	timeout time.Time
	err     error
	cv      *sync.Cond
	cc      *grpc.ClientConn
	atc     roxy_v0.AirTrafficControlClient
	retries uint32
	ready   bool
}

// New constructs and returns a new ATCClient.  The cc argument is a gRPC
// ClientConn configured to speak to any/all ATC towers.  The tlsConfig
// argument specifies the TLS client configuration to use when speaking to
// individual ATC towers, or nil for gRPC with no TLS.
func New(cc *grpc.ClientConn, tlsConfig *tls.Config) (*ATCClient, error) {
	roxyutil.AssertNotNil(&cc)

	c := &ATCClient{
		tc:         tlsConfig,
		cc:         cc,
		atc:        roxy_v0.NewAirTrafficControlClient(cc),
		closeCh:    make(chan struct{}),
		serviceMap: make(map[Key]*serviceData, 8),
		connMap:    make(map[*net.TCPAddr]*connData, 4),
	}
	return c, nil
}

func (c *ATCClient) updateServiceData(key Key, addr *net.TCPAddr) {
	c.mu.Lock()
	sd := c.serviceMap[key]
	if sd == nil {
		sd = new(serviceData)
		sd.cv = sync.NewCond(&c.mu)
		c.serviceMap[key] = sd
	} else {
		sd.Wait()
	}
	sd.ready = true
	sd.addr = addr
	sd.err = nil
	sd.timeout = time.Time{}
	sd.retries = 0
	sd.cv.Broadcast()
	c.mu.Unlock()
}

func (sd *serviceData) Wait() {
	for !sd.ready {
		sd.cv.Wait()
	}
}

func (cd *connData) Wait() {
	for !cd.ready {
		cd.cv.Wait()
	}
}

func goAwayToTCPAddr(goAway *roxy_v0.GoAway) *net.TCPAddr {
	roxyutil.AssertNotNil(&goAway)

	addr := &net.TCPAddr{
		IP:   net.IP(goAway.Ip),
		Port: int(goAway.Port),
		Zone: goAway.Zone,
	}
	addr = misc.CanonicalizeTCPAddr(addr)
	return addr
}

func backoff(ctx context.Context, counter int) bool {
	t := time.NewTimer(gBackoff.Backoff(counter))
	select {
	case <-ctx.Done():
		t.Stop()
		return false

	case <-t.C:
		return true
	}
}

func sendSync(syncCh chan<- struct{}) {
	select {
	case syncCh <- struct{}{}:
	default:
	}
}

func drainSyncChannel(syncCh <-chan struct{}) {
	looping := true
	for looping {
		select {
		case _, ok := <-syncCh:
			looping = ok
		default:
			looping = false
		}
	}
}
