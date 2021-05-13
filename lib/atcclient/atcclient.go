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
	"github.com/chronos-tachyon/roxy/roxypb"
)

const (
	RetryInterval = 30 * time.Second
	LoadInterval  = 30 * time.Second
)

var gBackoff expbackoff.ExpBackoff = expbackoff.BuildDefault()

type ATCClient struct {
	tc  *tls.Config
	cc  *grpc.ClientConn
	atc roxypb.AirTrafficControlClient

	wg         sync.WaitGroup
	mu         sync.Mutex
	closed     bool
	serviceMap map[serviceKey]*serviceData
	connMap    map[*net.TCPAddr]*connData
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
	ready   bool
}

type connData struct {
	timeout time.Time
	err     error
	cv      *sync.Cond
	cc      *grpc.ClientConn
	atc     roxypb.AirTrafficControlClient
	ready   bool
}

func New(cc *grpc.ClientConn, tlsConfig *tls.Config) (*ATCClient, error) {
	if cc == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}
	c := &ATCClient{
		tc:         tlsConfig,
		cc:         cc,
		atc:        roxypb.NewAirTrafficControlClient(cc),
		serviceMap: make(map[serviceKey]*serviceData, 8),
		connMap:    make(map[*net.TCPAddr]*connData, 4),
	}
	return c, nil
}

func (c *ATCClient) Lookup(ctx context.Context, serviceName string) (*roxypb.LookupResponse, error) {
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

	req := &roxypb.LookupRequest{
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

	req := &roxypb.FindRequest{
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
	} else {
		sd.err = err
		sd.timeout = time.Now().Add(RetryInterval)
	}

	sd.ready = true
	sd.cv.Broadcast()

	c.mu.Unlock()
	// end critical section 2
	// }}}

	return addr, err
}

func (c *ATCClient) Dial(ctx context.Context, addr *net.TCPAddr) (*grpc.ClientConn, roxypb.AirTrafficControlClient, error) {
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

	dialOpts := make([]grpc.DialOption, 1)
	if c.tc == nil {
		dialOpts[0] = grpc.WithInsecure()
	} else {
		tc := c.tc.Clone()
		if tc.ServerName == "" {
			tc.ServerName = addr.IP.String()
		}
		dialOpts[0] = grpc.WithTransportCredentials(credentials.NewTLS(tc))
	}

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
		defer cc2.Close()
		cc = nil
		err = fs.ErrClosed
	}

	var atc roxypb.AirTrafficControlClient
	if err == nil {
		atc = roxypb.NewAirTrafficControlClient(cc)
		cd.cc = cc
		cd.atc = atc
	} else {
		cd.err = err
		cd.timeout = time.Now().Add(RetryInterval)
	}

	cd.ready = true
	cd.cv.Broadcast()

	c.mu.Unlock()
	// end critical section 2
	// }}}

	return cc, atc, err
}

func (c *ATCClient) ServerAnnounce(ctx context.Context, req *roxypb.ServerAnnounceRequest, loadFn LoadFunc) (context.CancelFunc, chan error, error) {
	c.wg.Add(1)
	defer c.wg.Done()

	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if req == nil {
		panic(errors.New("*roxypb.ServerAnnounceRequest is nil"))
	}
	if loadFn == nil {
		loadFn = DefaultLoadFunc
	}

	req = proto.Clone(req).(*roxypb.ServerAnnounceRequest)
	key := serviceKey{req.ServiceName, req.ShardId}

	addr, err := c.Find(ctx, req.ServiceName, req.ShardId, true)
	if err != nil {
		return nil, nil, err
	}

	_, atc, err := c.Dial(ctx, addr)
	if err != nil {
		return nil, nil, err
	}

	var counter int
	var retries int

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
		Str("method", "ServerAnnounce").
		Str("func", "ServerAnnounce").
		Int("counter", counter).
		Msg("Call")

	sac, err := atc.ServerAnnounce(ctx)
	if err != nil {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "ServerAnnounce").
			Str("func", "ServerAnnounce").
			Int("counter", counter).
			Err(err).
			Msg("Error")

		return nil, nil, err
	}

	req.Load = loadFn()
	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "ServerAnnounce").
		Str("func", "ServerAnnounce.Send").
		Int("counter", counter).
		Interface("req", req).
		Msg("Call")

	err = sac.Send(req)
	if err != nil {
		log.Logger.Trace().
			Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
			Str("type", "ATCClient").
			Str("method", "ServerAnnounce").
			Str("func", "ServerAnnounce.Send").
			Int("counter", counter).
			Err(err).
			Msg("Error")

		return nil, nil, err
	}

	recvUntilEOF := func(sac roxypb.AirTrafficControl_ServerAnnounceClient) {
		defer c.wg.Done()
		_ = sac.CloseSend()
		for {
			_, err := sac.Recv()
			if err != nil {
				break
			}
		}
	}

	mu := new(sync.Mutex)
	syncCh := make(chan struct{}, 1)
	errCh := make(chan error)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		for {
			t := time.NewTimer(LoadInterval)

			ok := true
			select {
			case <-ctx.Done():
				t.Stop()
				return

			case <-syncCh:
				t.Stop()
				ok = false

			case <-t.C:
				// pass
			}

			mu.Lock()
			if ok && sac != nil {
				req := &roxypb.ServerAnnounceRequest{
					Load: loadFn(),
				}

				log.Logger.Trace().
					Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
					Str("type", "ATCClient").
					Str("method", "ServerAnnounce").
					Str("func", "ServerAnnounce.Send").
					Interface("req", req).
					Msg("Call")

				err := sac.Send(req)
				if err != nil {
					log.Logger.Trace().
						Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
						Str("type", "ATCClient").
						Str("method", "ServerAnnounce").
						Str("func", "ServerAnnounce.Send").
						Err(err).
						Msg("Error")

					errCh <- err
				}
			}
			mu.Unlock()
		}
	}()

	c.wg.Add(1)
	go func() {
		defer func() {
			mu.Lock()
			if sac != nil {
				c.wg.Add(1)
				go recvUntilEOF(sac)
			}
			close(syncCh)
			close(errCh)
			sac = nil
			mu.Unlock()

			c.wg.Done()
		}()

		addr = nil

		for {
			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "ServerAnnounce").
				Str("func", "ServerAnnounce.Recv").
				Int("counter", counter).
				Msg("Call")

			resp, err := sac.Recv()

			if err == nil {
				retries = 0
				addr = goAwayToTCPAddr(resp.GoAway)
				c.updateServiceData(key, addr)
				c.wg.Add(1)
				go recvUntilEOF(sac)
			} else {
				log.Logger.Trace().
					Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
					Str("type", "ATCClient").
					Str("method", "ServerAnnounce").
					Str("func", "ServerAnnounce.Recv").
					Int("counter", counter).
					Err(err).
					Msg("Error")

				if err != io.EOF {
					errCh <- err
				}
			}

			mu.Lock()
			atc = nil
			sac = nil
			mu.Unlock()

			for {
				if !backoff(ctx, retries) {
					return
				}

				counter++
				retries++
				ok := true

				mu.Lock()

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

						errCh <- err

						if sac != nil {
							c.wg.Add(1)
							go recvUntilEOF(sac)
						}

						ok = false
						addr = nil
						atc = nil
						sac = nil
					}
				}

				if addr == nil {
					addr, err = c.Find(ctx, req.ServiceName, req.ShardId, false)
					handleError("Find", err)
				}

				if ok {
					_, atc, err = c.Dial(ctx, addr)
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

					sac, err = atc.ServerAnnounce(ctx)
					handleError("ServerAnnounce", err)
				}

				if ok {
					req.Load = loadFn()
					log.Logger.Trace().
						Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
						Str("type", "ATCClient").
						Str("method", "ServerAnnounce").
						Str("func", "ServerAnnounce.Send").
						Int("counter", counter).
						Interface("req", req).
						Msg("Call")

					err = sac.Send(req)
					handleError("ServerAnnounce.Send", err)
				}

				mu.Unlock()

				if ok {
					syncCh <- struct{}{}
					break
				}
			}
		}
	}()

	needCancel = false
	return cancelFn, errCh, nil
}

func (c *ATCClient) ClientAssign(ctx context.Context, req *roxypb.ClientAssignRequest) (chan []*roxypb.Event, chan error, error) {
	c.wg.Add(1)
	defer c.wg.Done()

	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if req == nil {
		panic(errors.New("*roxypb.ClientAssignRequest is nil"))
	}

	req = proto.Clone(req).(*roxypb.ClientAssignRequest)
	key := serviceKey{req.ServiceName, req.ShardId}

	addr, err := c.Find(ctx, req.ServiceName, req.ShardId, true)
	if err != nil {
		return nil, nil, err
	}

	_, atc, err := c.Dial(ctx, addr)
	if err != nil {
		return nil, nil, err
	}

	var counter int
	var retries int

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "ClientAssign").
		Str("func", "ClientAssign.Send").
		Int("counter", counter).
		Msg("Call")

	cac, err := atc.ClientAssign(ctx)
	if err != nil {
		return nil, nil, err
	}

	log.Logger.Trace().
		Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
		Str("type", "ATCClient").
		Str("method", "ClientAssign").
		Str("func", "ClientAssign.Send").
		Int("counter", counter).
		Interface("req", req).
		Msg("Call")

	err = cac.Send(req)
	if err != nil {
		return nil, nil, err
	}

	recvUntilEOF := func(cac roxypb.AirTrafficControl_ClientAssignClient) {
		defer c.wg.Done()
		_ = cac.CloseSend()
		for {
			_, err := cac.Recv()
			if err != nil {
				break
			}
		}
	}

	eventCh := make(chan []*roxypb.Event)
	errCh := make(chan error)

	c.wg.Add(1)
	go func() {
		defer func() {
			if cac != nil {
				c.wg.Add(1)
				go recvUntilEOF(cac)
			}
			close(eventCh)
			close(errCh)
			c.wg.Done()
		}()

		addr = nil

		for {
			log.Logger.Trace().
				Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
				Str("type", "ATCClient").
				Str("method", "ClientAssign").
				Str("func", "ClientAssign.Recv").
				Int("counter", counter).
				Msg("Call")

			resp, err := cac.Recv()
			if err == nil {
				retries = 0
				if len(resp.Events) != 0 {
					eventCh <- resp.Events
				}
				if resp.GoAway == nil {
					continue
				}
				addr = goAwayToTCPAddr(resp.GoAway)
				c.updateServiceData(key, addr)
				c.wg.Add(1)
				go recvUntilEOF(cac)
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
					errCh <- err
				}
			}

			atc = nil
			cac = nil

			for {
				if !backoff(ctx, retries) {
					return
				}

				counter++
				retries++
				ok := true

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

						errCh <- err

						if cac != nil {
							c.wg.Add(1)
							go recvUntilEOF(cac)
						}

						ok = false
						addr = nil
						atc = nil
						cac = nil
					}
				}

				if addr == nil {
					addr, err = c.Find(ctx, req.ServiceName, req.ShardId, false)
					handleError("Find", err)
				}

				if ok {
					_, atc, err = c.Dial(ctx, addr)
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

					cac, err = atc.ClientAssign(ctx)
					handleError("ClientAssign", err)
				}

				if ok {
					log.Logger.Trace().
						Str("package", "github.com/chronos-tachyon/roxy/lib/atcclient").
						Str("type", "ATCClient").
						Str("method", "ClientAssign").
						Str("func", "ClientAssign.Send").
						Int("counter", counter).
						Interface("req", req).
						Msg("Call")

					err = cac.Send(req)
					handleError("ClientAssign.Send", err)
				}

				if ok {
					break
				}
			}
		}
	}()

	return eventCh, errCh, nil
}

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

	return errs.ErrorOrNil()
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

func goAwayToTCPAddr(goAway *roxypb.GoAway) *net.TCPAddr {
	if goAway == nil {
		panic(errors.New("*roxypb.GoAway is nil"))
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