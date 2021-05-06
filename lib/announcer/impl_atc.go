package announcer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/roxypb"
)

type LoadFunc func() float32

var DefaultLoadFunc LoadFunc = func() float32 { return 0.0 }

func (a *Announcer) AddATC(lbcc *grpc.ClientConn, lbName, lbLoc, unique, namedPort string, loadFn LoadFunc) error {
	impl, err := NewATC(lbcc, lbName, lbLoc, unique, namedPort, loadFn)
	if err != nil {
		return err
	}
	a.Add(impl)
	return nil
}

func NewATC(lbcc *grpc.ClientConn, lbName, lbLoc, unique, namedPort string, loadFn LoadFunc) (Impl, error) {
	if lbcc == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}
	if err := roxyresolver.ValidateATCServiceName(lbName); err != nil {
		return nil, fmt.Errorf("invalid ATC service name %q: %w", lbName, err)
	}
	if err := roxyresolver.ValidateATCLocation(lbLoc); err != nil {
		return nil, fmt.Errorf("invalid ATC location %q: %w", lbLoc, err)
	}
	if namedPort != "" {
		if err := roxyresolver.ValidateServerSetPort(namedPort); err != nil {
			return nil, fmt.Errorf("invalid named port %q: %w", namedPort, err)
		}
	}
	if unique == "" {
		return nil, fmt.Errorf("invalid unique string %q", unique)
	}
	if loadFn == nil {
		loadFn = DefaultLoadFunc
	}
	impl := &atcImpl{
		lbcc:      lbcc,
		atc:       roxypb.NewAirTrafficControlClient(lbcc),
		lbName:    lbName,
		lbLoc:     lbLoc,
		unique:    unique,
		namedPort: namedPort,
		loadFn:    loadFn,
	}
	impl.cv = sync.NewCond(&impl.mu)
	return impl, nil
}

type atcImpl struct {
	wg        sync.WaitGroup
	lbcc      *grpc.ClientConn
	atc       roxypb.AirTrafficControlClient
	lbName    string
	lbLoc     string
	unique    string
	namedPort string
	loadFn    LoadFunc

	mu     sync.Mutex
	cv     *sync.Cond
	alive  bool
	rc     roxypb.AirTrafficControl_ReportClient
	doneCh chan struct{}
	errs   multierror.Error
}

func (impl *atcImpl) Announce(ctx context.Context, ss *membership.ServerSet) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	if impl.alive {
		return errors.New("Announce called twice")
	}

	tcpAddr := ss.TCPAddrForPort(impl.namedPort)
	if tcpAddr == nil {
		return fmt.Errorf("unknown port name %q", impl.namedPort)
	}

	serverName := ss.Metadata["ServerName"]

	var (
		shardID    int32
		hasShardID bool
	)
	if ss.ShardID != nil {
		shardID = *ss.ShardID
		hasShardID = true
	}

	rc, err := impl.atc.Report(ctx)
	if err != nil {
		return err
	}

	err = rc.Send(&roxypb.ReportRequest{
		Load: impl.loadFn(),
		FirstReport: &roxypb.ReportRequest_FirstReport{
			Name:       impl.lbName,
			Location:   impl.lbLoc,
			Unique:     impl.unique,
			ServerName: serverName,
			ShardId:    shardID,
			Ip:         []byte(tcpAddr.IP),
			Zone:       tcpAddr.Zone,
			Port:       uint32(tcpAddr.Port),
			HasShardId: hasShardID,
		},
	})
	if err != nil {
		_ = rc.CloseSend()
		return err
	}

	_, err = rc.Recv()
	if err != nil {
		_ = rc.CloseSend()
		return err
	}

	ctxDoneCh := ctx.Done()
	doneCh1 := make(chan struct{})
	doneCh2 := make(chan struct{})

	impl.wg.Add(1)
	go func() {
		t := time.NewTicker(30 * time.Second)
		looping := true
		for looping {
			select {
			case <-ctxDoneCh:
				looping = false
			case <-doneCh1:
				looping = false
			case <-doneCh2:
				looping = false
			case <-t.C:
				err := rc.Send(&roxypb.ReportRequest{
					Load: impl.loadFn(),
				})
				impl.recordError(err)
				if err != nil {
					looping = false
				}
			}
		}
		t.Stop()
		impl.recordErrorAndDie(rc.CloseSend())
		impl.wg.Done()
	}()

	impl.wg.Add(1)
	go func() {
		looping := true
		for looping {
			_, err := rc.Recv()
			if err != nil {
				impl.recordError(err)
				break
			}
			select {
			case <-ctxDoneCh:
				looping = false
			case <-doneCh2:
				looping = false
			default:
				// pass
			}
		}
		close(doneCh1)
		impl.wg.Done()
	}()

	impl.alive = true
	impl.rc = rc
	impl.doneCh = doneCh2
	return nil
}

func (impl *atcImpl) Withdraw(ctx context.Context) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	if !impl.alive {
		return nil
	}

	close(impl.doneCh)

	for impl.alive {
		impl.cv.Wait()
	}

	err := impl.errs.ErrorOrNil()
	impl.rc = nil
	impl.doneCh = nil
	impl.errs.Errors = nil
	return err
}

func (impl *atcImpl) Close() error {
	impl.wg.Wait()
	return nil
}

func (impl *atcImpl) recordError(err error) {
	if err == nil || err == io.EOF {
		return
	}
	impl.mu.Lock()
	impl.errs.Errors = append(impl.errs.Errors, err)
	impl.mu.Unlock()
}

func (impl *atcImpl) recordErrorAndDie(err error) {
	impl.mu.Lock()
	if err != nil && err != io.EOF {
		impl.errs.Errors = append(impl.errs.Errors, err)
	}
	impl.alive = false
	impl.cv.Broadcast()
	impl.mu.Unlock()
}

var _ Impl = (*atcImpl)(nil)
