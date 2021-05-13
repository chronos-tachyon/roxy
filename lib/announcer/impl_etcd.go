package announcer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"

	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func NewEtcd(etcd *v3.Client, path, unique, namedPort string, format Format) (Impl, error) {
	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}
	if err := roxyutil.ValidateEtcdPath(path); err != nil {
		return nil, err
	}
	if unique == "" {
		unique = os.Getenv("HOSTNAME")
	}
	if namedPort != "" {
		if err := roxyutil.ValidateNamedPort(namedPort); err != nil {
			return nil, err
		}
	}
	return &etcdImpl{
		etcd:      etcd,
		path:      path,
		unique:    unique,
		namedPort: namedPort,
		format:    format,
	}, nil
}

type etcdImpl struct {
	wg        sync.WaitGroup
	etcd      *v3.Client
	path      string
	unique    string
	namedPort string
	format    Format

	mu      sync.Mutex
	alive   bool
	leaseID v3.LeaseID
}

func (impl *etcdImpl) Announce(ctx context.Context, r *membership.Roxy) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	var v interface{}
	var err error
	switch impl.format {
	case FinagleFormat:
		v, err = r.AsServerSet()
	case GRPCFormat:
		v, err = r.AsGRPC(impl.namedPort)
	default:
		v, err = r.AsRoxyJSON()
	}
	if err != nil {
		return err
	}

	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}

	lease, err := impl.etcd.Lease.Grant(ctx, 30)
	err = roxyresolver.MapEtcdError(err)
	if err != nil {
		return err
	}

	ch, err := impl.etcd.Lease.KeepAlive(ctx, lease.ID)
	err = roxyresolver.MapEtcdError(err)
	if err != nil {
		return err
	}

	impl.wg.Add(1)
	go func() {
		for range ch {
		}
		impl.wg.Done()
	}()

	key := impl.path + impl.unique
	_, err = impl.etcd.KV.Put(ctx, key, string(payload), v3.WithLease(lease.ID))
	err = roxyresolver.MapEtcdError(err)
	if err != nil {
		_, _ = impl.etcd.Lease.Revoke(ctx, lease.ID)
		return err
	}
	impl.alive = true
	impl.leaseID = lease.ID
	return nil
}

func (impl *etcdImpl) Withdraw(ctx context.Context) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	if !impl.alive {
		return nil
	}

	impl.alive = false
	_, err := impl.etcd.Lease.Revoke(ctx, impl.leaseID)
	err = roxyresolver.MapEtcdError(err)
	return err
}

func (impl *etcdImpl) Close() error {
	impl.wg.Wait()
	return nil
}

var _ Impl = (*etcdImpl)(nil)
