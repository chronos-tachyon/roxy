package announcer

import (
	"context"
	"errors"
	"os"
	"path"
	"sync"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func NewZK(zkconn *zk.Conn, path, unique, namedPort string, format Format) (Interface, error) {
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
		state:     StateReady,
	}, nil
}

type zkImpl struct {
	zkconn    *zk.Conn
	path      string
	unique    string
	namedPort string
	format    Format

	mu     sync.Mutex
	state  State
	actual string
}

func (impl *zkImpl) Announce(ctx context.Context, r *membership.Roxy) error {
	payload, err := convertToJSON(r, impl.format, impl.namedPort)
	if err != nil {
		return err
	}

	impl.mu.Lock()
	defer impl.mu.Unlock()

	checkAnnounce(impl.state)

	file := path.Join(impl.path, impl.unique)

	actual, err := impl.zkconn.CreateProtectedEphemeralSequential(file, payload, zk.WorldACL(zk.PermAll))
	err = roxyresolver.MapZKError(err)
	if err != nil {
		return err
	}

	impl.state = StateRunning
	impl.actual = actual
	return nil
}

func (impl *zkImpl) Withdraw(ctx context.Context) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	checkWithdraw(impl.state)

	err := impl.zkconn.Delete(impl.actual, -1)
	err = roxyresolver.MapZKError(err)
	impl.state = StateReady
	return err
}

func (impl *zkImpl) Close() error {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	err := checkClose(impl.state)
	impl.state = StateClosed
	return err
}

var _ Interface = (*zkImpl)(nil)
