package announcer

import (
	"context"
	"errors"
	"fmt"
	"sync"

	multierror "github.com/hashicorp/go-multierror"

	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/roxypb"
)

func NewATC(client *atcclient.ATCClient, serviceName, location, unique, namedPort string, loadFn atcclient.LoadFunc) (Impl, error) {
	if client == nil {
		panic(errors.New("*atcclient.ATCClient is nil"))
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
			return nil, err
		}
	}
	impl := &atcImpl{
		client:      client,
		serviceName: serviceName,
		location:    location,
		unique:      unique,
		namedPort:   namedPort,
		loadFn:      loadFn,
		state:       stateInit,
	}
	impl.cv = sync.NewCond(&impl.mu)
	return impl, nil
}

type atcImpl struct {
	client      *atcclient.ATCClient
	serviceName string
	location    string
	unique      string
	namedPort   string
	loadFn      atcclient.LoadFunc

	mu       sync.Mutex
	cv       *sync.Cond
	state    stateType
	cancelFn context.CancelFunc
	errs     multierror.Error
}

func (impl *atcImpl) Announce(ctx context.Context, r *membership.Roxy) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	checkAnnounce(impl.state)

	tcpAddr := r.NamedAddr(impl.namedPort)
	if tcpAddr == nil {
		return fmt.Errorf("unknown port name %q", impl.namedPort)
	}

	var shardID uint32
	var hasShardID bool
	if r.ShardID != nil {
		shardID = *r.ShardID
		hasShardID = true
	}

	cancelFn, errCh, err := impl.client.ServerAnnounce(
		ctx,
		&roxypb.ServerAnnounceRequest{
			ServiceName: impl.serviceName,
			ShardId:     shardID,
			Location:    impl.location,
			Unique:      impl.unique,
			ServerName:  r.ServerName,
			Ip:          []byte(tcpAddr.IP),
			Zone:        tcpAddr.Zone,
			Port:        uint32(tcpAddr.Port),
			HasShardId:  hasShardID,
		},
		impl.loadFn,
	)
	if err != nil {
		return err
	}

	go func() {
		for err := range errCh {
			impl.mu.Lock()
			impl.errs.Errors = append(impl.errs.Errors, err)
			impl.mu.Unlock()
		}

		impl.mu.Lock()
		impl.state = stateDead
		impl.cv.Broadcast()
		impl.mu.Unlock()
	}()

	impl.state = stateRunning
	impl.cancelFn = cancelFn
	return nil
}

func (impl *atcImpl) Withdraw(ctx context.Context) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	checkWithdraw(impl.state)

	impl.cancelFn()

	for impl.state == stateRunning {
		impl.cv.Wait()
	}

	err := impl.errs.ErrorOrNil()
	impl.cancelFn = nil
	impl.errs.Errors = nil
	impl.state = stateInit
	return err
}

func (impl *atcImpl) Close() error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	err := checkClose(impl.state)
	impl.state = stateClosed
	return err
}

var _ Impl = (*atcImpl)(nil)
