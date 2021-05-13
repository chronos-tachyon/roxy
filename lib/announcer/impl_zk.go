package announcer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"sync"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func NewZK(zkconn *zk.Conn, path, unique, namedPort string, format Format) (Impl, error) {
	if zkconn == nil {
		panic(errors.New("*zk.Conn is nil"))
	}
	if err := roxyutil.ValidateZKPath(path); err != nil {
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
	return &zkImpl{
		zkconn:    zkconn,
		path:      path,
		unique:    unique,
		namedPort: namedPort,
		format:    format,
	}, nil
}

type zkImpl struct {
	zkconn    *zk.Conn
	path      string
	unique    string
	namedPort string
	format    Format

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

	file := path.Join(impl.path, impl.unique)
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
