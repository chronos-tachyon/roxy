package announcer

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

func (a *Announcer) AddZK(zkconn *zk.Conn, dir string, unique string) error {
	impl, err := NewZK(zkconn, dir, unique)
	if err != nil {
		return err
	}
	a.Add(impl)
	return nil
}

func NewZK(zkconn *zk.Conn, dir string, unique string) (Impl, error) {
	if zkconn == nil {
		panic(errors.New("*zk.Conn is nil"))
	}
	if dir == "" || dir[0] != '/' || (dir != "/" && dir[len(dir)-1] == '/') {
		return nil, fmt.Errorf("invalid ZooKeeper path %q", dir)
	}
	if unique == "" {
		return nil, fmt.Errorf("invalid unique string %q", unique)
	}
	return &zkImpl{
		zkconn: zkconn,
		dir:    dir,
		unique: unique,
	}, nil
}

type zkImpl struct {
	zkconn *zk.Conn
	dir    string
	unique string
	actual string
}

func (impl *zkImpl) Announce(ctx context.Context, ss *membership.ServerSet) error {
	payload := ss.AsJSON()
	file := path.Join(impl.dir, impl.unique)
	actual, err := impl.zkconn.CreateProtectedEphemeralSequential(file, payload, zk.WorldACL(zk.PermAll))
	if err != nil {
		return err
	}
	impl.actual = actual
	return nil
}

func (impl *zkImpl) Withdraw(ctx context.Context) error {
	return impl.zkconn.Delete(impl.actual, -1)
}

func (impl *zkImpl) Close() error {
	return nil
}

var _ Impl = (*zkImpl)(nil)
