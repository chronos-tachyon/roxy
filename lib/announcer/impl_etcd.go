package announcer

import (
	"context"
	"errors"
	"fmt"
	"sync"

	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
)

func (a *Announcer) AddEtcd(etcd *v3.Client, etcdPath string, unique string, format Format, namedPort string) error {
	impl, err := NewEtcd(etcd, etcdPath, unique, format, namedPort)
	if err != nil {
		return err
	}
	a.Add(impl)
	return nil
}

func NewEtcd(etcd *v3.Client, etcdPath string, unique string, format Format, namedPort string) (Impl, error) {
	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}
	if err := roxyresolver.ValidateEtcdPath(etcdPath); err != nil {
		return nil, fmt.Errorf("invalid etcd path %q: %w", etcdPath, err)
	}
	if namedPort != "" {
		if err := roxyresolver.ValidateServerSetPort(namedPort); err != nil {
			return nil, fmt.Errorf("invalid named port %q: %w", namedPort, err)
		}
	}
	if unique == "" {
		return nil, fmt.Errorf("invalid unique string %q", unique)
	}
	return &etcdImpl{
		etcd:      etcd,
		etcdPath:  etcdPath,
		unique:    unique,
		format:    format,
		namedPort: namedPort,
	}, nil
}

type etcdImpl struct {
	wg        sync.WaitGroup
	etcd      *v3.Client
	etcdPath  string
	unique    string
	format    Format
	namedPort string

	mu      sync.Mutex
	alive   bool
	leaseID v3.LeaseID
}

func (impl *etcdImpl) Announce(ctx context.Context, ss *membership.ServerSet) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	var payload []byte
	if impl.format == GRPCFormat {
		payload = ss.AsGRPC(impl.namedPort).AsJSON()
	} else {
		payload = ss.AsJSON()
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

	key := impl.etcdPath + impl.unique
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
