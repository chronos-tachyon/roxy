package announcer

import (
	"context"
	"errors"
	"fmt"
	"sync"

	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

func (a *Announcer) AddEtcd(etcd *v3.Client, prefix string, unique string) error {
	impl, err := NewEtcd(etcd, prefix, unique)
	if err != nil {
		return err
	}
	a.Add(impl)
	return nil
}

func NewEtcd(etcd *v3.Client, prefix string, unique string) (Impl, error) {
	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}
	if prefix != "" && prefix[len(prefix)-1] == '/' {
		return nil, fmt.Errorf("invalid prefix %q", prefix)
	}
	if unique == "" {
		return nil, fmt.Errorf("invalid unique string %q", unique)
	}
	return &etcdImpl{
		etcd:   etcd,
		prefix: prefix,
		unique: unique,
	}, nil
}

type etcdImpl struct {
	wg      sync.WaitGroup
	etcd    *v3.Client
	prefix  string
	unique  string
	leaseID v3.LeaseID
}

func (impl *etcdImpl) Announce(ctx context.Context, ss *membership.ServerSet) error {
	payload := ss.AsJSON()

	lease, err := impl.etcd.Lease.Grant(ctx, 30)
	if err != nil {
		return err
	}

	ch, err := impl.etcd.Lease.KeepAlive(ctx, lease.ID)
	if err != nil {
		return err
	}

	impl.wg.Add(1)
	go func() {
		for range ch {
		}
		impl.wg.Done()
	}()

	impl.leaseID = lease.ID

	key := impl.prefix + "/" + impl.unique
	_, err = impl.etcd.KV.Put(ctx, key, string(payload), v3.WithLease(impl.leaseID))
	if err != nil {
		return err
	}
	return nil
}

func (impl *etcdImpl) Withdraw(ctx context.Context) error {
	_, err := impl.etcd.Lease.Revoke(ctx, impl.leaseID)
	return err
}

func (impl *etcdImpl) Close() error {
	impl.wg.Wait()
	return nil
}

var _ Impl = (*etcdImpl)(nil)
