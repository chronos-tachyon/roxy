package announcer

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sync"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
)

func (a *Announcer) AddZK(zkconn *zk.Conn, zkPath string, unique string, format Format, namedPort string) error {
	impl, err := NewZK(zkconn, zkPath, unique, format, namedPort)
	if err != nil {
		return err
	}
	a.Add(impl)
	return nil
}

func NewZK(zkconn *zk.Conn, zkPath string, unique string, format Format, namedPort string) (Impl, error) {
	if zkconn == nil {
		panic(errors.New("*zk.Conn is nil"))
	}
	if err := roxyresolver.ValidateZKPath(zkPath); err != nil {
		return nil, fmt.Errorf("invalid ZooKeeper path %q: %w", zkPath, err)
	}
	if namedPort != "" {
		if err := roxyresolver.ValidateServerSetPort(namedPort); err != nil {
			return nil, fmt.Errorf("invalid named port %q: %w", namedPort, err)
		}
	}
	if unique == "" {
		return nil, fmt.Errorf("invalid unique string %q", unique)
	}
	return &zkImpl{
		zkconn:    zkconn,
		zkPath:    zkPath,
		unique:    unique,
		format:    format,
		namedPort: namedPort,
	}, nil
}

type zkImpl struct {
	zkconn    *zk.Conn
	zkPath    string
	unique    string
	format    Format
	namedPort string

	mu     sync.Mutex
	alive  bool
	actual string
}

func (impl *zkImpl) Announce(ctx context.Context, ss *membership.ServerSet) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	var payload []byte
	if impl.format == GRPCFormat {
		payload = ss.AsGRPC(impl.namedPort).AsJSON()
	} else {
		payload = ss.AsJSON()
	}

	file := path.Join(impl.zkPath, impl.unique)
	actual, err := impl.zkconn.CreateProtectedEphemeralSequential(file, payload, zk.WorldACL(zk.PermAll))
	err = roxyresolver.MapZKError(err)
	if err != nil {
		return err
	}
	impl.alive = true
	impl.actual = actual
	return nil
}

func (impl *zkImpl) Withdraw(ctx context.Context) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	if !impl.alive {
		return nil
	}
	impl.alive = false
	return roxyresolver.MapZKError(impl.zkconn.Delete(impl.actual, -1))
}

func (impl *zkImpl) Close() error {
	return nil
}

var _ Impl = (*zkImpl)(nil)
