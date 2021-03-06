package announcer

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// NewEtcd creates a new etcd.io announcer.
func NewEtcd(etcd *v3.Client, path, hostID, namedPort string, format Format) (Interface, error) {
	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}
	if err := roxyutil.ValidateEtcdPath(path); err != nil {
		return nil, err
	}
	if hostID == "" {
		hostID = os.Getenv("HOSTNAME")
	}
	if namedPort != "" {
		if err := roxyutil.ValidateNamedPort(namedPort); err != nil {
			return nil, err
		}
	}
	impl := &etcdImpl{
		etcd:      etcd,
		path:      path,
		hostID:    hostID,
		namedPort: namedPort,
		format:    format,
		state:     StateReady,
	}
	impl.cv = sync.NewCond(&impl.mu)
	return impl, nil
}

type etcdImpl struct {
	etcd      *v3.Client
	path      string
	hostID    string
	namedPort string
	format    Format

	mu       sync.Mutex
	cv       *sync.Cond
	state    State
	leaseID  v3.LeaseID
	cancelFn context.CancelFunc
	errs     multierror.Error
}

func (impl *etcdImpl) Announce(ctx context.Context, r *membership.Roxy) error {
	payload, err := convertToJSON(r, impl.format, impl.namedPort)
	if err != nil {
		return err
	}

	impl.mu.Lock()
	defer impl.mu.Unlock()

	checkAnnounce(impl.state)

	lease, err := impl.etcd.Lease.Grant(ctx, 30)
	err = roxyresolver.MapEtcdError(err)
	if err != nil {
		return err
	}

	key := impl.path + impl.hostID

	_, err = impl.etcd.KV.Put(ctx, key, string(payload), v3.WithLease(lease.ID))
	err = roxyresolver.MapEtcdError(err)
	if err != nil {
		_, _ = impl.etcd.Lease.Revoke(ctx, lease.ID)
		return err
	}

	ctx, cancelFn := context.WithCancel(ctx)

	go func() {
		defer func() {
			impl.mu.Lock()
			impl.state = StateDead
			impl.cv.Broadcast()
			impl.mu.Unlock()
		}()

		for {
			t := time.NewTimer(10 * time.Second)

			select {
			case <-ctx.Done():
				t.Stop()
				return

			case <-t.C:
				// pass
			}

			_, err := impl.etcd.Lease.KeepAliveOnce(ctx, lease.ID)
			err = roxyresolver.MapEtcdError(err)
			if err != nil {
				impl.mu.Lock()
				impl.errs.Errors = append(impl.errs.Errors, err)
				impl.mu.Unlock()
			}
		}
	}()

	impl.state = StateRunning
	impl.leaseID = lease.ID
	impl.cancelFn = cancelFn
	return nil
}

func (impl *etcdImpl) Withdraw(ctx context.Context) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	checkWithdraw(impl.state)

	impl.cancelFn()

	_, err := impl.etcd.Lease.Revoke(ctx, impl.leaseID)
	err = roxyresolver.MapEtcdError(err)
	if err != nil {
		impl.errs.Errors = append(impl.errs.Errors, err)
	}

	for impl.state == StateRunning {
		impl.cv.Wait()
	}

	err = misc.ErrorOrNil(impl.errs)
	impl.errs.Errors = nil
	impl.cancelFn = nil
	impl.leaseID = 0
	impl.state = StateReady
	return err
}

func (impl *etcdImpl) Close() error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	err := checkClose(impl.state)
	impl.state = StateClosed
	return err
}

var _ Interface = (*etcdImpl)(nil)
