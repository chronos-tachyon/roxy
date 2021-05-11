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
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/roxypb"
)

type LoadFunc func() float32

var DefaultLoadFunc LoadFunc = func() float32 { return 0.0 }

func (a *Announcer) AddATC(lbcc *grpc.ClientConn, serviceName, location, unique, namedPort string, loadFn LoadFunc) error {
	impl, err := NewATC(lbcc, serviceName, location, unique, namedPort, loadFn)
	if err != nil {
		return err
	}
	a.Add(impl)
	return nil
}

func NewATC(lbcc *grpc.ClientConn, serviceName, location, unique, namedPort string, loadFn LoadFunc) (Impl, error) {
	if lbcc == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}
	if err := roxyutil.ValidateATCServiceName(serviceName); err != nil {
		return nil, err
	}
	if err := roxyutil.ValidateATCLocation(location); err != nil {
		return nil, err
	}
	if err := roxyutil.ValidateATCUnique(unique); err != nil {
		return nil, err
	}
	if namedPort != "" {
		if err := roxyutil.ValidateNamedPort(namedPort); err != nil {
			return nil, fmt.Errorf("invalid named port %q: %w", namedPort, err)
		}
	}
	if loadFn == nil {
		loadFn = DefaultLoadFunc
	}
	impl := &atcImpl{
		lbcc:        lbcc,
		atc:         roxypb.NewAirTrafficControlClient(lbcc),
		serviceName: serviceName,
		location:    location,
		unique:      unique,
		namedPort:   namedPort,
		loadFn:      loadFn,
	}
	impl.cv = sync.NewCond(&impl.mu)
	return impl, nil
}

type atcImpl struct {
	wg          sync.WaitGroup
	lbcc        *grpc.ClientConn
	atc         roxypb.AirTrafficControlClient
	serviceName string
	location    string
	unique      string
	namedPort   string
	loadFn      LoadFunc

	mu     sync.Mutex
	cv     *sync.Cond
	alive  bool
	rc     roxypb.AirTrafficControl_ServerAnnounceClient
	doneCh chan struct{}
	errs   multierror.Error
}

func (impl *atcImpl) Announce(ctx context.Context, r *membership.Roxy) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	if impl.alive {
		return errors.New("Announce called twice")
	}

	tcpAddr := r.NamedAddr(impl.namedPort)
	if tcpAddr == nil {
		return fmt.Errorf("unknown port name %q", impl.namedPort)
	}

	serverName := r.ServerName

	var (
		shardID    uint32
		hasShardID bool
	)
	if r.ShardID != nil {
		shardID = *r.ShardID
		hasShardID = true
	}

	rc, err := impl.atc.ServerAnnounce(ctx)
	if err != nil {
		return err
	}

	err = rc.Send(&roxypb.ServerAnnounceRequest{
		ServiceName: impl.serviceName,
		ShardId:     shardID,
		Location:    impl.location,
		Unique:      impl.unique,
		ServerName:  serverName,
		Ip:          []byte(tcpAddr.IP),
		Zone:        tcpAddr.Zone,
		Port:        uint32(tcpAddr.Port),
		HasShardId:  hasShardID,
		Load:        impl.loadFn(),
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
				err := rc.Send(&roxypb.ServerAnnounceRequest{
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
