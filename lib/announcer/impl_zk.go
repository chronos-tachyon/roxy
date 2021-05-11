package announcer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sync"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
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
	if err := roxyutil.ValidateZKPath(zkPath); err != nil {
		return nil, fmt.Errorf("invalid ZooKeeper path %q: %w", zkPath, err)
	}
	if namedPort != "" {
		if err := roxyutil.ValidateNamedPort(namedPort); err != nil {
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

func (impl *zkImpl) Announce(ctx context.Context, r *membership.Roxy) error {
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
