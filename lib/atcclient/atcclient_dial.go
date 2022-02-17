package atcclient

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// Dial returns a gRPC ClientConn connected directly to the given ATC tower.
//
// The caller should _not_ call Close() on it.  It is owned by the ATCClient
// and will be re-used until the ATCClient itself is Close()'d.
func (c *ATCClient) Dial(ctx context.Context, addr *net.TCPAddr) (*grpc.ClientConn, roxy_v0.AirTrafficControlClient, error) {
	roxyutil.AssertNotNil(&ctx)
	roxyutil.AssertNotNil(&addr)

	c.wg.Add(1)
	defer c.wg.Done()

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
				Str("type", "ATCClient").
				Str("method", "Dial").
				Stringer("addr", addr).
				Str("cc", fmt.Sprintf("%p", cc)).
				Msg("Call (positive cache hit)")

			return cc, atc, nil
		}
		if time.Now().Before(cd.timeout) {
			err := cd.err
			c.mu.Unlock()

			log.Logger.Trace().
				Str("type", "ATCClient").
				Str("method", "Dial").
				Stringer("addr", addr).
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
		dialOpts[0] = grpc.WithTransportCredentials(insecure.NewCredentials())
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
		Str("type", "ATCClient").
		Str("method", "Dial").
		Stringer("addr", addr).
		Msg("Call")

	cc, err := grpc.DialContext(ctx, addr.String(), dialOpts...)

	if err != nil {
		log.Logger.Trace().
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
