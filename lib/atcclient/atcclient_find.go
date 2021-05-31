package atcclient

import (
	"context"
	"io/fs"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// Find queries any ATC tower for information about which ATC tower is
// responsible for the given (ServiceName, ShardID) tuple.  If useCache is
// false, then the local cache will not be consulted.
func (c *ATCClient) Find(
	ctx context.Context,
	key Key,
	useCache bool,
) (
	*net.TCPAddr,
	error,
) {
	roxyutil.AssertNotNil(&ctx)

	c.wg.Add(1)
	defer c.wg.Done()

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
				Str("type", "ATCClient").
				Str("method", "Find").
				Stringer("key", key).
				Stringer("addr", addr).
				Msg("Call (positive cache hit)")

			return addr, nil
		}
		if useCache && time.Now().Before(sd.timeout) {
			err := sd.err
			c.mu.Unlock()

			log.Logger.Trace().
				Str("type", "ATCClient").
				Str("method", "Find").
				Stringer("key", key).
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
		ServiceName: key.ServiceName,
		ShardId:     key.ShardID,
		HasShardId:  key.HasShardID,
	}

	log.Logger.Trace().
		Str("type", "ATCClient").
		Str("method", "Find").
		Stringer("key", key).
		Msg("Call")

	resp, err := c.atc.Find(ctx, req)

	if err != nil {
		log.Logger.Trace().
			Str("type", "ATCClient").
			Str("method", "Find").
			Stringer("key", key).
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
