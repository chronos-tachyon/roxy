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
	}
	impl.cv = sync.NewCond(&impl.mu)
	return impl, nil
}

type atcImpl struct {
	wg          sync.WaitGroup
	client      *atcclient.ATCClient
	serviceName string
	location    string
	unique      string
	namedPort   string
	loadFn      atcclient.LoadFunc

	mu       sync.Mutex
	cv       *sync.Cond
	alive    bool
	cancelFn context.CancelFunc
	errs     multierror.Error
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

	impl.wg.Add(1)
	go func() {
		for err := range errCh {
			impl.mu.Lock()
			impl.errs.Errors = append(impl.errs.Errors, err)
			impl.mu.Unlock()
		}
		impl.mu.Lock()
		impl.alive = false
		impl.cv.Broadcast()
		impl.mu.Unlock()
		impl.wg.Done()
	}()

	impl.alive = true
	impl.cancelFn = cancelFn
	return nil
}

func (impl *atcImpl) Withdraw(ctx context.Context) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	if !impl.alive {
		return nil
	}

	impl.cancelFn()

	for impl.alive {
		impl.cv.Wait()
	}

	err := impl.errs.ErrorOrNil()
	impl.cancelFn = nil
	impl.errs.Errors = nil
	return err
}

func (impl *atcImpl) Close() error {
	impl.wg.Wait()
	return nil
}

var _ Impl = (*atcImpl)(nil)
