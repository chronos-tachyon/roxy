package announcer

import (
	"context"
	"errors"
	"fmt"
	"sync"

	multierror "github.com/hashicorp/go-multierror"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// NewATC creates a new Roxy Air Traffic Control announcer.
func NewATC(client *atcclient.ATCClient, serviceName, location, unique, namedPort string) (Interface, error) {
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
		state:       StateReady,
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

	mu       sync.Mutex
	cv       *sync.Cond
	state    State
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

	cancelFn, errCh, err := impl.client.ServerAnnounce(
		ctx,
		&roxy_v0.ServerAnnounceRequest_First{
			ServiceName: impl.serviceName,
			ShardId:     r.ShardID,
			Location:    impl.location,
			Unique:      impl.unique,
			ServerName:  r.ServerName,
			Ip:          []byte(tcpAddr.IP),
			Zone:        tcpAddr.Zone,
			Port:        uint32(tcpAddr.Port),
			HasShardId:  r.HasShardID,
		},
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
		impl.state = StateDead
		impl.cv.Broadcast()
		impl.mu.Unlock()
	}()

	impl.state = StateRunning
	impl.cancelFn = cancelFn
	return nil
}

func (impl *atcImpl) Withdraw(ctx context.Context) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	checkWithdraw(impl.state)

	impl.cancelFn()

	for impl.state == StateRunning {
		impl.cv.Wait()
	}

	err := misc.ErrorOrNil(impl.errs)
	impl.cancelFn = nil
	impl.errs.Errors = nil
	impl.state = StateReady
	return err
}

func (impl *atcImpl) Close() error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	err := checkClose(impl.state)
	impl.state = StateClosed
	return err
}

var _ Interface = (*atcImpl)(nil)
