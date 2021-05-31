package atcclient

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// ServerAnnounce starts announcing that a new server is available for the
// given (ServiceName, ShardID) tuple.  If the method returns with no error,
// then the caller must call the returned CancelFunc when the announcement
// should be withdrawn, and the caller must also ensure that the returned error
// channel is drained in a timely manner.  The error channel will be closed
// once all goroutines and other internal resources have been released.
func (c *ATCClient) ServerAnnounce(
	ctx context.Context,
	first *roxy_v0.ServerData,
) (
	context.CancelFunc,
	<-chan error,
	error,
) {
	roxyutil.AssertNotNil(&ctx)
	roxyutil.AssertNotNil(&first)

	first = proto.Clone(first).(*roxy_v0.ServerData)
	if !first.HasShardId && first.ShardId != 0 {
		first.ShardId = 0
	}
	key := Key{first.ServiceName, first.ShardId, first.HasShardId}

	c.wg.Add(1)
	defer c.wg.Done()

	addr, err := c.Find(ctx, key, true)
	if err != nil {
		return nil, nil, err
	}

	cc, atc, err := c.Dial(ctx, addr)
	if err != nil {
		return nil, nil, err
	}

	logger := log.Logger.With().
		Str("type", "ATCClient").
		Str("method", "ServerAnnounce").
		Stringer("key", key).
		Str("uniqueId", first.Unique).
		Logger()

	logger.Trace().
		Str("func", "ServerAnnounce").
		Msg("Call")

	sac, err := atc.ServerAnnounce(ctx)
	if err != nil {
		logger.Trace().
			Str("func", "ServerAnnounce").
			Err(err).
			Msg("Error")
		return nil, nil, err
	}

	active := &activeServerAnnounce{
		logger: logger,
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

	active.doSend(false)

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

type activeServerAnnounce struct {
	logger zerolog.Logger
	c      *ATCClient
	ctx    context.Context
	first  *roxy_v0.ServerData
	key    Key
	stopCh chan struct{}
	syncCh chan struct{}
	errCh  chan error
	wid    WatchID

	mu          sync.Mutex
	cc          *grpc.ClientConn
	atc         roxy_v0.AirTrafficControlClient
	sac         roxy_v0.AirTrafficControl_ServerAnnounceClient
	sentFirst   bool
	lastServing bool
	lastCounter uint64
}

func (active *activeServerAnnounce) isStopped() bool {
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

func (active *activeServerAnnounce) doSend(isFinal bool) {
	active.mu.Lock()
	defer active.mu.Unlock()

	if active.sac == nil {
		return
	}

	first := active.first
	if active.sentFirst {
		first = nil
	}

	costCounter := GetCostCounter()

	isServing := false
	if !isFinal {
		isServing = IsServing()
	}

	if active.sentFirst && isServing == active.lastServing && costCounter == active.lastCounter {
		return
	}

	active.logger.Trace().
		Str("func", "ServerAnnounce.Send").
		Msg("Call")

	err := active.sac.Send(&roxy_v0.ServerAnnounceRequest{
		First:       first,
		CostCounter: costCounter,
		IsServing:   isServing,
	})
	if err != nil {
		active.logger.Trace().
			Str("func", "ServerAnnounce.Send").
			Err(err).
			Msg("Error")
		active.errCh <- err
	}

	active.sentFirst = true
	active.lastServing = isServing
	active.lastCounter = costCounter

	if isFinal {
		active.logger.Trace().
			Str("func", "ServerAnnounce.CloseSend").
			Msg("Call")

		err = active.sac.CloseSend()
		if err != nil {
			active.logger.Trace().
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
		active.doSend(false)

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

	active.doSend(true)
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

	for {
		if active.isStopped() {
			return
		}

		active.logger.Trace().
			Str("func", "ServerAnnounce.Recv").
			Int("counter", counter).
			Int("retries", retries).
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
			active.logger.Trace().
				Str("func", "ServerAnnounce.Recv").
				Int("counter", counter).
				Int("retries", retries).
				Err(err).
				Msg("Error")
			active.errCh <- err
		}

		if active.isStopped() {
			return
		}

		active.mu.Lock()
		active.cc = nil
		active.atc = nil
		active.sac = nil
		active.sentFirst = false
		active.mu.Unlock()

		for {
			if !backoff(active.ctx, retries) {
				return
			}

			counter++
			retries++
			ok := true

			if active.isStopped() {
				return
			}

			active.mu.Lock()

			handleError := func(funcName string, err error) {
				if err != nil {
					active.logger.Trace().
						Str("func", funcName).
						Int("counter", counter).
						Int("retries", retries).
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
				addr, err = active.c.Find(active.ctx, active.key, false)
				handleError("Find", err)
			}

			if ok {
				active.cc, active.atc, err = active.c.Dial(active.ctx, addr)
				handleError("Dial", err)
			}

			if ok {
				active.logger.Trace().
					Str("func", "ServerAnnounce").
					Int("counter", counter).
					Int("retries", retries).
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
