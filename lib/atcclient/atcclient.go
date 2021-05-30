package atcclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// ReportInterval is the time interval between reports sent within
// ServerAnnounce and ClientAssign.
const ReportInterval = 1 * time.Second

var gBackoff expbackoff.ExpBackoff = expbackoff.BuildDefault()

// ATCClient is a client for communicating with the Roxy Air Traffic Controller
// service.
//
// Each ATC tower is responsible for a set of (ServiceName, ShardID) tuples,
// which are exclusive to that tower.  The client automatically determines
// which tower it needs to speak with, by asking any tower within the service
// to provide instructions.
type ATCClient struct {
	tc      *tls.Config
	cc      *grpc.ClientConn
	atc     roxy_v0.AirTrafficControlClient
	closeCh chan struct{}

	wg         sync.WaitGroup
	mu         sync.Mutex
	serviceMap map[serviceKey]*serviceData
	connMap    map[*net.TCPAddr]*connData
	closed     bool
}

type serviceKey struct {
	name    string
	shardID uint32
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
	if cc == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}
	c := &ATCClient{
		tc:         tlsConfig,
		cc:         cc,
		atc:        roxy_v0.NewAirTrafficControlClient(cc),
		closeCh:    make(chan struct{}),
		serviceMap: make(map[serviceKey]*serviceData, 8),
		connMap:    make(map[*net.TCPAddr]*connData, 4),
	}
	return c, nil
}

// Lookup queries any ATC tower for information about the given service.
func (c *ATCClient) Lookup(ctx context.Context, serviceName string) (*roxy_v0.LookupResponse, error) {
	c.wg.Add(1)
	defer c.wg.Done()

	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}

	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()

	if closed {
		return nil, fs.ErrClosed
	}

	req := &roxy_v0.LookupRequest{
		ServiceName: serviceName,
	}

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "Lookup").
		Interface("req", req).
		Msg("Call")

	resp, err := c.atc.Lookup(ctx, req)

	if err != nil {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "Lookup").
			Err(err).
			Msg("Error")
	}

	return resp, err
}

// Find queries any ATC tower for information about which ATC tower is
// responsible for the given (ServiceName, ShardID) tuple.  If useCache is
// false, then the local cache will not be consulted.
func (c *ATCClient) Find(ctx context.Context, serviceName string, shardID uint32, useCache bool) (*net.TCPAddr, error) {
	c.wg.Add(1)
	defer c.wg.Done()

	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}

	key := serviceKey{serviceName, shardID}

	// begin critical section 1
	// {{{
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return nil, fs.ErrClosed
	}

	sd := c.serviceMap[key]
	if sd == nil {
		sd = new(serviceData)
		sd.cv = sync.NewCond(&c.mu)
		c.serviceMap[key] = sd
	} else {
		sd.Wait()
		if useCache && sd.err == nil {
			addr := sd.addr
			c.mu.Unlock()

			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "Find").
				Str("serviceName", serviceName).
				Uint32("shardID", shardID).
				Str("addr", addr.String()).
				Msg("Call (positive cache hit)")

			return addr, nil
		}
		if useCache && time.Now().Before(sd.timeout) {
			err := sd.err
			c.mu.Unlock()

			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "Find").
				Str("serviceName", serviceName).
				Uint32("shardID", shardID).
				Err(err).
				Msg("Call (negative cache hit)")

			return nil, err
		}
		sd.ready = false
		sd.addr = nil
		sd.err = nil
		sd.timeout = time.Time{}
	}

	c.mu.Unlock()
	// end critical section 1
	// }}}

	req := &roxy_v0.FindRequest{
		ServiceName: serviceName,
		ShardId:     shardID,
	}

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "Find").
		Interface("req", req).
		Msg("Call")

	resp, err := c.atc.Find(ctx, req)

	if err != nil {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "Find").
			Err(err).
			Msg("Error")
	}

	var addr *net.TCPAddr

	// begin critical section 2
	// {{{
	c.mu.Lock()

	if err == nil && c.closed {
		addr = nil
		err = fs.ErrClosed
	}

	if err == nil {
		addr = goAwayToTCPAddr(resp.GoAway)
		sd.addr = addr
		sd.retries = 0
	} else {
		sd.err = err
		sd.timeout = time.Now().Add(gBackoff.Backoff(int(sd.retries)))
		sd.retries++
	}

	sd.ready = true
	sd.cv.Broadcast()

	c.mu.Unlock()
	// end critical section 2
	// }}}

	return addr, err
}

// Dial returns a gRPC ClientConn connected directly to the given ATC tower.
//
// The caller should _not_ call Close() on it.  It is owned by the ATCClient
// and will be re-used until the ATCClient itself is Close()'d.
func (c *ATCClient) Dial(ctx context.Context, addr *net.TCPAddr) (*grpc.ClientConn, roxy_v0.AirTrafficControlClient, error) {
	c.wg.Add(1)
	defer c.wg.Done()

	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if addr == nil {
		panic(errors.New("*net.TCPAddr is nil"))
	}

	addr = misc.CanonicalizeTCPAddr(addr)

	// begin critical section 1
	// {{{
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return nil, nil, fs.ErrClosed
	}

	cd := c.connMap[addr]
	if cd == nil {
		cd = new(connData)
		cd.cv = sync.NewCond(&c.mu)
		c.connMap[addr] = cd
	} else {
		cd.Wait()
		if cd.cc != nil {
			cc := cd.cc
			atc := cd.atc
			c.mu.Unlock()

			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "Dial").
				Str("addr", addr.String()).
				Str("cc", fmt.Sprintf("%p", cc)).
				Msg("Call (positive cache hit)")

			return cc, atc, nil
		}
		if time.Now().Before(cd.timeout) {
			err := cd.err
			c.mu.Unlock()

			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "Dial").
				Str("addr", addr.String()).
				Err(err).
				Msg("Call (negative cache hit)")

			return nil, nil, err
		}
		cd.ready = false
		cd.cc = nil
		cd.atc = nil
		cd.err = nil
		cd.timeout = time.Time{}
	}

	c.mu.Unlock()
	// end critical section 1
	// }}}

	dialOpts := make([]grpc.DialOption, 3)
	if c.tc == nil {
		dialOpts[0] = grpc.WithInsecure()
	} else {
		tc := c.tc.Clone()
		if tc.ServerName == "" {
			tc.ServerName = addr.IP.String()
		}
		dialOpts[0] = grpc.WithTransportCredentials(credentials.NewTLS(tc))
	}
	dialOpts[1] = grpc.WithBlock()
	dialOpts[2] = grpc.FailOnNonTempDialError(true)

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "Dial").
		Str("addr", addr.String()).
		Msg("Call")

	cc, err := grpc.DialContext(ctx, addr.String(), dialOpts...)

	if err != nil {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "Dial").
			Err(err).
			Msg("Error")
	}

	// begin critical section 2
	// {{{
	c.mu.Lock()

	if err == nil && c.closed {
		cc2 := cc
		defer func() {
			_ = cc2.Close()
		}()
		cc = nil
		err = fs.ErrClosed
	}

	var atc roxy_v0.AirTrafficControlClient
	if err == nil {
		atc = roxy_v0.NewAirTrafficControlClient(cc)
		cd.cc = cc
		cd.atc = atc
		cd.retries = 0
	} else {
		cd.err = err
		cd.timeout = time.Now().Add(gBackoff.Backoff(int(cd.retries)))
		cd.retries++
	}

	cd.ready = true
	cd.cv.Broadcast()

	c.mu.Unlock()
	// end critical section 2
	// }}}

	return cc, atc, err
}

// ServerAnnounce starts announcing that a new server is available for the
// given (ServiceName, ShardID) tuple.  If the method returns with no error,
// then the caller must call the returned CancelFunc when the announcement
// should be withdrawn, and the caller must also ensure that the returned error
// channel is drained in a timely manner.  The error channel will be closed
// once all goroutines and other internal resources have been released.
func (c *ATCClient) ServerAnnounce(ctx context.Context, first *roxy_v0.ServerData) (context.CancelFunc, <-chan error, error) {
	c.wg.Add(1)
	defer c.wg.Done()

	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if first == nil {
		panic(errors.New("*roxy_v0.ServerData is nil"))
	}

	first = proto.Clone(first).(*roxy_v0.ServerData)
	key := serviceKey{first.ServiceName, first.ShardId}

	addr, err := c.Find(ctx, key.name, key.shardID, true)
	if err != nil {
		return nil, nil, err
	}

	cc, atc, err := c.Dial(ctx, addr)
	if err != nil {
		return nil, nil, err
	}

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "ServerAnnounce").
		Str("func", "ServerAnnounce").
		Int("counter", 0).
		Msg("Call")

	sac, err := atc.ServerAnnounce(ctx)
	if err != nil {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "ServerAnnounce").
			Str("func", "ServerAnnounce").
			Int("counter", 0).
			Err(err).
			Msg("Error")

		return nil, nil, err
	}

	active := &activeServerAnnounce{
		c:      c,
		ctx:    ctx,
		first:  first,
		key:    key,
		stopCh: make(chan struct{}),
		syncCh: make(chan struct{}, 1),
		errCh:  make(chan error),
		cc:     cc,
		atc:    atc,
		sac:    sac,
	}

	active.doSend(first, false)

	active.wid = WatchIsServing(func(bool) {
		sendSync(active.syncCh)
	})

	c.wg.Add(2)
	go active.sendThread()
	go active.recvThread()

	cancelFn := context.CancelFunc(func() {
		close(active.stopCh)
	})
	return cancelFn, active.errCh, nil
}

// ClientAssign starts a subscription for assignment Events.  If the method
// returns with no error, then the caller must call the returned CancelFunc
// when it is no longer interested in receiving Events, and the caller is also
// responsible for draining both channels in a timely manner.
func (c *ATCClient) ClientAssign(ctx context.Context, first *roxy_v0.ClientData) (context.CancelFunc, <-chan []*roxy_v0.Event, <-chan error, error) {
	c.wg.Add(1)
	defer c.wg.Done()

	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if first == nil {
		panic(errors.New("*roxy_v0.ClientData is nil"))
	}

	first = proto.Clone(first).(*roxy_v0.ClientData)
	key := serviceKey{first.ServiceName, first.ShardId}

	addr, err := c.Find(ctx, key.name, key.shardID, true)
	if err != nil {
		return nil, nil, nil, err
	}

	cc, atc, err := c.Dial(ctx, addr)
	if err != nil {
		return nil, nil, nil, err
	}

	ctx, cancelFn := context.WithCancel(ctx)
	needCancel := true
	defer func() {
		if needCancel {
			cancelFn()
		}
	}()

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "ClientAssign").
		Str("func", "ClientAssign.Send").
		Int("counter", 0).
		Msg("Call")

	cac, err := atc.ClientAssign(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "ClientAssign").
		Str("func", "ClientAssign.Send").
		Int("counter", 0).
		Interface("req", first).
		Msg("Call")

	err = cac.Send(&roxy_v0.ClientAssignRequest{
		First:       first,
		CostCounter: GetCostCounter(),
	})
	if err != nil {
		return nil, nil, nil, err
	}

	active := &activeClientAssign{
		c:       c,
		ctx:     ctx,
		first:   first,
		key:     key,
		syncCh:  make(chan struct{}, 1),
		eventCh: make(chan []*roxy_v0.Event),
		errCh:   make(chan error),
		cc:      cc,
		atc:     atc,
		cac:     cac,
	}

	c.wg.Add(2)
	go active.sendThread()
	go active.recvThread()

	needCancel = false
	return cancelFn, active.eventCh, active.errCh, nil
}

// Close closes all gRPC channels and blocks until all resources are freed.
func (c *ATCClient) Close() error {
	c.mu.Lock()
	closed := c.closed
	connList := make([]*grpc.ClientConn, 0, len(c.connMap))
	for _, cd := range c.connMap {
		if cd.ready && cd.cc != nil {
			connList = append(connList, cd.cc)
			cd.cc = nil
			cd.err = fs.ErrClosed
		}
	}
	c.closed = true
	c.mu.Unlock()

	if closed {
		return fs.ErrClosed
	}

	close(c.closeCh)

	var errs multierror.Error

	for _, cc := range connList {
		if err := cc.Close(); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}

	if err := c.cc.Close(); err != nil {
		errs.Errors = append(errs.Errors, err)
	}

	c.wg.Wait()

	return misc.ErrorOrNil(errs)
}

func (c *ATCClient) updateServiceData(key serviceKey, addr *net.TCPAddr) {
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

type activeServerAnnounce struct {
	c      *ATCClient
	ctx    context.Context
	first  *roxy_v0.ServerData
	key    serviceKey
	stopCh chan struct{}
	syncCh chan struct{}
	errCh  chan error
	wid    WatchID

	mu  sync.Mutex
	cc  *grpc.ClientConn
	atc roxy_v0.AirTrafficControlClient
	sac roxy_v0.AirTrafficControl_ServerAnnounceClient
}

func (active *activeServerAnnounce) doSend(first *roxy_v0.ServerData, final bool) {
	active.mu.Lock()
	defer active.mu.Unlock()

	if active.sac == nil {
		return
	}

	costCounter := GetCostCounter()
	isServing := false
	if !final {
		isServing = IsServing()
	}

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "ServerAnnounce").
		Str("func", "ServerAnnounce.Send").
		Msg("Call")

	err := active.sac.Send(&roxy_v0.ServerAnnounceRequest{
		First:       first,
		CostCounter: costCounter,
		IsServing:   isServing,
	})
	if err != nil {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "ServerAnnounce").
			Str("func", "ServerAnnounce.Send").
			Err(err).
			Msg("Error")

		active.errCh <- err
	}

	if final {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "ServerAnnounce").
			Str("func", "ServerAnnounce.CloseSend").
			Msg("Call")

		err = active.sac.CloseSend()
		if err != nil {
			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "ServerAnnounce").
				Str("func", "ServerAnnounce.CloseSend").
				Err(err).
				Msg("Error")

			active.errCh <- err
		}
	}
}

func (active *activeServerAnnounce) sendThread() {
	defer active.c.wg.Done()

	looping := true
	for looping {
		active.doSend(nil, false)

		t := time.NewTimer(ReportInterval)
		select {
		case <-active.ctx.Done():
			t.Stop()
			return

		case <-active.c.closeCh:
			t.Stop()
			looping = false

		case <-active.stopCh:
			t.Stop()
			looping = false

		case <-active.syncCh:
			t.Stop()

		case <-t.C:
			// pass
		}
		drainSyncChannel(active.syncCh)
	}

	active.doSend(nil, true)
}

func (active *activeServerAnnounce) recvThread() {
	defer func() {
		active.mu.Lock()
		if active.sac != nil {
			active.c.wg.Add(1)
			go saRecvUntilEOF(&active.c.wg, active.sac)
		}
		CancelWatchIsServing(active.wid)
		drainSyncChannel(active.syncCh)
		close(active.syncCh)
		close(active.errCh)
		active.cc = nil
		active.atc = nil
		active.sac = nil
		active.mu.Unlock()

		active.c.wg.Done()
	}()

	var counter int
	var retries int
	var addr *net.TCPAddr

	isStopped := func() bool {
		select {
		case <-active.ctx.Done():
			return true
		case <-active.c.closeCh:
			return true
		case <-active.stopCh:
			return true
		default:
			return false
		}
	}

	for {
		if isStopped() {
			return
		}

		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "ServerAnnounce").
			Str("func", "ServerAnnounce.Recv").
			Int("counter", counter).
			Msg("Call")

		resp, err := active.sac.Recv()

		switch {
		case err == nil:
			retries = 0
			addr = goAwayToTCPAddr(resp.GoAway)
			active.c.updateServiceData(active.key, addr)
			active.c.wg.Add(1)
			go saRecvUntilEOF(&active.c.wg, active.sac)

		case err == io.EOF:
			// pass

		case errors.Is(err, context.Canceled):
			// pass

		case errors.Is(err, context.DeadlineExceeded):
			// pass

		default:
			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "ServerAnnounce").
				Str("func", "ServerAnnounce.Recv").
				Int("counter", counter).
				Err(err).
				Msg("Error")

			if err != io.EOF {
				active.errCh <- err
			}
		}

		if isStopped() {
			return
		}

		active.mu.Lock()
		active.cc = nil
		active.atc = nil
		active.sac = nil
		active.mu.Unlock()

		for {
			if !backoff(active.ctx, retries) {
				return
			}

			counter++
			retries++
			ok := true

			if isStopped() {
				return
			}

			active.mu.Lock()

			handleError := func(funcName string, err error) {
				if err != nil {
					log.Logger.Trace().
						Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
						Str("type", "ATCClient").
						Str("method", "ServerAnnounce").
						Str("func", funcName).
						Int("counter", counter).
						Err(err).
						Msg("Error")

					active.errCh <- err

					if active.sac != nil {
						active.c.wg.Add(1)
						go saRecvUntilEOF(&active.c.wg, active.sac)
					}

					ok = false
					addr = nil
					active.cc = nil
					active.atc = nil
					active.sac = nil
				}
			}

			if addr == nil {
				addr, err = active.c.Find(active.ctx, active.key.name, active.key.shardID, false)
				handleError("Find", err)
			}

			if ok {
				active.cc, active.atc, err = active.c.Dial(active.ctx, addr)
				handleError("Dial", err)
			}

			if ok {
				log.Logger.Trace().
					Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
					Str("type", "ATCClient").
					Str("method", "ServerAnnounce").
					Str("func", "ServerAnnounce").
					Int("counter", counter).
					Msg("Call")

				active.sac, err = active.atc.ServerAnnounce(active.ctx)
				handleError("ServerAnnounce", err)
			}

			active.mu.Unlock()

			if ok {
				sendSync(active.syncCh)
				break
			}
		}
	}
}

func saRecvUntilEOF(wg *sync.WaitGroup, sac roxy_v0.AirTrafficControl_ServerAnnounceClient) {
	_ = sac.CloseSend()
	for {
		_, err := sac.Recv()
		if err != nil {
			break
		}
	}
	wg.Done()
}

type activeClientAssign struct {
	c       *ATCClient
	ctx     context.Context
	first   *roxy_v0.ClientData
	key     serviceKey
	syncCh  chan struct{}
	eventCh chan []*roxy_v0.Event
	errCh   chan error

	mu  sync.Mutex
	cc  *grpc.ClientConn
	atc roxy_v0.AirTrafficControlClient
	cac roxy_v0.AirTrafficControl_ClientAssignClient
}

func (active *activeClientAssign) sendThread() {
	defer active.c.wg.Done()

	for {
		t := time.NewTimer(ReportInterval)

		ok := true
		select {
		case <-active.ctx.Done():
			t.Stop()
			return

		case <-active.c.closeCh:
			t.Stop()
			return

		case <-active.syncCh:
			t.Stop()
			ok = false

		case <-t.C:
			// pass
		}
		drainSyncChannel(active.syncCh)

		active.mu.Lock()
		if ok && active.cac != nil {
			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "ClientAssign").
				Str("func", "ClientAssign.Send").
				Msg("Call")

			err := active.cac.Send(&roxy_v0.ClientAssignRequest{
				CostCounter: GetCostCounter(),
			})
			if err != nil {
				log.Logger.Trace().
					Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
					Str("type", "ATCClient").
					Str("method", "ClientAssign").
					Str("func", "ClientAssign.Send").
					Err(err).
					Msg("Error")

				active.errCh <- err
			}
		}
		active.mu.Unlock()
	}
}

func (active *activeClientAssign) recvThread() {
	defer func() {
		active.mu.Lock()
		if active.cac != nil {
			active.c.wg.Add(1)
			go caRecvUntilEOF(&active.c.wg, active.cac)
		}
		close(active.syncCh)
		close(active.eventCh)
		close(active.errCh)
		active.cc = nil
		active.atc = nil
		active.cac = nil
		active.mu.Unlock()

		active.c.wg.Done()
	}()

	var counter int
	var retries int
	var addr *net.TCPAddr

	for {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "ClientAssign").
			Str("func", "ClientAssign.Recv").
			Int("counter", counter).
			Msg("Call")

		resp, err := active.cac.Recv()
		if err == nil {
			retries = 0
			if len(resp.Events) != 0 {
				active.eventCh <- resp.Events
			}
			if resp.GoAway == nil {
				continue
			}
			addr = goAwayToTCPAddr(resp.GoAway)
			active.c.updateServiceData(active.key, addr)
			active.c.wg.Add(1)
			go caRecvUntilEOF(&active.c.wg, active.cac)
		} else {
			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "ClientAssign").
				Str("func", "ClientAssign.Recv").
				Int("counter", counter).
				Err(err).
				Msg("Error")

			if err != io.EOF {
				active.errCh <- err
			}
		}

		active.c.mu.Lock()
		closed := active.c.closed
		active.c.mu.Unlock()

		if closed {
			return
		}

		active.mu.Lock()
		active.cc = nil
		active.atc = nil
		active.cac = nil
		active.mu.Unlock()

		for {
			if !backoff(active.ctx, retries) {
				return
			}

			counter++
			retries++
			ok := true

			active.c.mu.Lock()
			closed := active.c.closed
			active.c.mu.Unlock()

			if closed {
				return
			}

			active.mu.Lock()

			handleError := func(funcName string, err error) {
				if err != nil {
					log.Logger.Trace().
						Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
						Str("type", "ATCClient").
						Str("method", "ClientAssign").
						Str("func", funcName).
						Int("counter", counter).
						Err(err).
						Msg("Error")

					active.errCh <- err

					if active.cac != nil {
						active.c.wg.Add(1)
						go caRecvUntilEOF(&active.c.wg, active.cac)
					}

					ok = false
					addr = nil
					active.cc = nil
					active.atc = nil
					active.cac = nil
				}
			}

			if addr == nil {
				addr, err = active.c.Find(active.ctx, active.key.name, active.key.shardID, false)
				handleError("Find", err)
			}

			if ok {
				active.cc, active.atc, err = active.c.Dial(active.ctx, addr)
				handleError("Dial", err)
			}

			if ok {
				log.Logger.Trace().
					Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
					Str("type", "ATCClient").
					Str("method", "ClientAssign").
					Str("func", "ClientAssign").
					Int("counter", counter).
					Msg("Call")

				active.cac, err = active.atc.ClientAssign(active.ctx)
				handleError("ClientAssign", err)
			}

			if ok {
				log.Logger.Trace().
					Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
					Str("type", "ATCClient").
					Str("method", "ClientAssign").
					Str("func", "ClientAssign.Send").
					Int("counter", counter).
					Interface("req", active.first).
					Msg("Call")

				err = active.cac.Send(&roxy_v0.ClientAssignRequest{
					First:       active.first,
					CostCounter: GetCostCounter(),
				})
				handleError("ClientAssign.Send", err)
			}

			active.mu.Unlock()

			if ok {
				break
			}
		}
	}
}

func caRecvUntilEOF(wg *sync.WaitGroup, cac roxy_v0.AirTrafficControl_ClientAssignClient) {
	_ = cac.CloseSend()
	for {
		_, err := cac.Recv()
		if err != nil {
			break
		}
	}
	wg.Done()
}

func goAwayToTCPAddr(goAway *roxy_v0.GoAway) *net.TCPAddr {
	if goAway == nil {
		panic(errors.New("*roxy_v0.GoAway is nil"))
	}
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
