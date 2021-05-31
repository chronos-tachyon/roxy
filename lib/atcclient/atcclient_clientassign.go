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

// ClientAssign starts a subscription for assignment Events.  If the method
// returns with no error, then the caller must call the returned CancelFunc
// when it is no longer interested in receiving Events, and the caller is also
// responsible for draining both channels in a timely manner.
func (c *ATCClient) ClientAssign(
	ctx context.Context,
	first *roxy_v0.ClientData,
) (
	context.CancelFunc,
	<-chan []*roxy_v0.Event,
	<-chan error,
	error,
) {
	roxyutil.AssertNotNil(&ctx)
	roxyutil.AssertNotNil(&first)

	first = proto.Clone(first).(*roxy_v0.ClientData)
	if !first.HasShardId && first.ShardId != 0 {
		first.ShardId = 0
	}
	key := Key{first.ServiceName, first.ShardId, first.HasShardId}

	c.wg.Add(1)
	defer c.wg.Done()

	addr, err := c.Find(ctx, key, true)
	if err != nil {
		return nil, nil, nil, err
	}

	cc, atc, err := c.Dial(ctx, addr)
	if err != nil {
		return nil, nil, nil, err
	}

	logger := log.Logger.With().
		Str("type", "ATCClient").
		Str("method", "ClientAssign").
		Stringer("key", key).
		Str("uniqueId", first.Unique).
		Logger()

	logger.Trace().
		Str("func", "ClientAssign").
		Msg("Call")

	cac, err := atc.ClientAssign(ctx)
	if err != nil {
		logger.Trace().
			Str("func", "ClientAssign").
			Err(err).
			Msg("Error")
		return nil, nil, nil, err
	}

	active := &activeClientAssign{
		logger:  logger,
		c:       c,
		ctx:     ctx,
		first:   first,
		key:     key,
		stopCh:  make(chan struct{}),
		syncCh:  make(chan struct{}, 1),
		eventCh: make(chan []*roxy_v0.Event),
		errCh:   make(chan error),
		cc:      cc,
		atc:     atc,
		cac:     cac,
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
	return cancelFn, active.eventCh, active.errCh, nil
}

type activeClientAssign struct {
	logger  zerolog.Logger
	c       *ATCClient
	ctx     context.Context
	first   *roxy_v0.ClientData
	key     Key
	stopCh  chan struct{}
	syncCh  chan struct{}
	eventCh chan []*roxy_v0.Event
	errCh   chan error
	wid     WatchID

	mu          sync.Mutex
	cc          *grpc.ClientConn
	atc         roxy_v0.AirTrafficControlClient
	cac         roxy_v0.AirTrafficControl_ClientAssignClient
	sentFirst   bool
	lastServing bool
	lastCounter uint64
}

func (active *activeClientAssign) isStopped() bool {
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

func (active *activeClientAssign) doSend(isFinal bool) {
	active.mu.Lock()
	defer active.mu.Unlock()

	if active.cac == nil {
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
		Str("func", "ClientAssign.Send").
		Msg("Call")

	err := active.cac.Send(&roxy_v0.ClientAssignRequest{
		First:       first,
		CostCounter: costCounter,
		IsServing:   isServing,
	})
	if err != nil {
		active.logger.Trace().
			Str("func", "ClientAssign.Send").
			Err(err).
			Msg("Error")
		active.errCh <- err
	}

	active.sentFirst = true
	active.lastServing = isServing
	active.lastCounter = costCounter

	if isFinal {
		active.logger.Trace().
			Str("func", "ClientAssign.CloseSend").
			Msg("Call")

		err = active.cac.CloseSend()
		if err != nil {
			active.logger.Trace().
				Str("func", "ClientAssign.CloseSend").
				Err(err).
				Msg("Error")
			active.errCh <- err
		}
	}
}

func (active *activeClientAssign) sendThread() {
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

func (active *activeClientAssign) recvThread() {
	defer func() {
		active.mu.Lock()
		if active.cac != nil {
			active.c.wg.Add(1)
			go caRecvUntilEOF(&active.c.wg, active.cac)
		}
		CancelWatchIsServing(active.wid)
		drainSyncChannel(active.syncCh)
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
		if active.isStopped() {
			return
		}

		active.logger.Trace().
			Str("func", "ClientAssign.Recv").
			Int("counter", counter).
			Int("retries", retries).
			Msg("Call")

		resp, err := active.cac.Recv()

		switch {
		case err == nil:
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

		case err == io.EOF:
			// pass

		case errors.Is(err, context.Canceled):
			// pass

		case errors.Is(err, context.DeadlineExceeded):
			// pass

		default:
			active.logger.Trace().
				Str("func", "ClientAssign.Recv").
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
		active.cac = nil
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
				addr, err = active.c.Find(active.ctx, active.key, false)
				handleError("Find", err)
			}

			if ok {
				active.cc, active.atc, err = active.c.Dial(active.ctx, addr)
				handleError("Dial", err)
			}

			if ok {
				active.logger.Trace().
					Str("func", "ClientAssign").
					Int("counter", counter).
					Int("retries", retries).
					Msg("Call")

				active.cac, err = active.atc.ClientAssign(active.ctx)
				handleError("ClientAssign", err)
			}

			active.mu.Unlock()

			if ok {
				sendSync(active.syncCh)
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
