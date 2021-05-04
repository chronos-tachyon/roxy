package roxyresolver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"

	"github.com/go-zookeeper/zk"
	v3 "go.etcd.io/etcd/client/v3"
	grpcresolver "google.golang.org/grpc/resolver"
)

type (
	Target            = grpcresolver.Target
	Address           = grpcresolver.Address
	ResolveNowOptions = grpcresolver.ResolveNowOptions
)

type WatchID uint32

type WatchFunc func([]*Event)

type UpdateOptions struct {
	Addr       net.Addr
	HasHealthy bool
	HasLoad    bool
	Healthy    bool
	Load       float32
}

// type Resolver {{{

type Resolver interface {
	Err() error
	ResolveAll() ([]*Resolved, error)
	Resolve() (*Resolved, error)

	Update(opts UpdateOptions)

	Watch(WatchFunc) WatchID
	CancelWatch(WatchID)

	ResolveNow(ResolveNowOptions)
	Close()
}

type Options struct {
	Target  Target
	IsTLS   bool
	Context context.Context
	Random  *rand.Rand
	Etcd    *v3.Client
	ZK      *zk.Conn
}

func New(opts Options) (Resolver, error) {
	scheme := strings.ToLower(opts.Target.Scheme)
	switch scheme {
	case "unix":
		fallthrough
	case "unix-abstract":
		return NewUnixResolver(opts)
	case "ip":
		return NewIPResolver(opts)
	case "":
		fallthrough
	case "dns":
		return NewDNSResolver(opts)
	case "srv":
		return NewSRVResolver(opts)
	case "zk":
		return NewZKResolver(opts)
	case "etcd":
		return NewEtcdResolver(opts)
	case "atc":
		return NewATCResolver(opts)
	default:
		return nil, fmt.Errorf("Target.Scheme %q is not supported", opts.Target.Scheme)
	}
}

var _ grpcresolver.Resolver = Resolver(nil)

// }}}

// type Resolved {{{

type Resolved struct {
	Unique      string
	Location    string
	ServerName  string
	SRVPriority uint16
	SRVWeight   uint16
	ShardID     int32
	Weight      float32
	HasSRV      bool
	HasShardID  bool
	HasWeight   bool
	Err         error
	Addr        net.Addr
	Address     Address

	hasHealthy bool
	hasLoad    bool
	healthy    bool
	load       float32
}

func (data *Resolved) Check() {
	if checkDisabled {
		return
	}
	if data == nil {
		panic(errors.New("*Resolved is nil"))
	}
	if data.Unique == "" {
		panic(errors.New("Resolved.Unique is empty"))
	}
	if data.Err == nil && data.Addr == nil {
		panic(errors.New("Resolved.Err is nil but Resolved.Addr is also nil"))
	}
	if data.Addr != nil && data.Address.Addr == "" {
		panic(errors.New("Resolved.Addr is not nil but Resolved.Address.Addr is empty"))
	}
	if data.ServerName != data.Address.ServerName {
		panic(errors.New("Resolved.ServerName is not equal to Resolved.Address.ServerName"))
	}
}

func (data *Resolved) Update(opts UpdateOptions) {
	gDataMutex.Lock()
	data.updateLocked(opts)
	gDataMutex.Unlock()
}

func (data *Resolved) UpdateFrom(source *Resolved) {
	gDataMutex.Lock()
	data.updateLocked(UpdateOptions{
		HasHealthy: source.hasHealthy,
		HasLoad:    source.hasLoad,
		Healthy:    source.healthy,
		Load:       source.load,
	})
	gDataMutex.Unlock()
}

func (data *Resolved) updateLocked(opts UpdateOptions) {
	if opts.HasHealthy {
		data.hasHealthy = true
		data.healthy = opts.Healthy
	}
	if opts.HasLoad {
		data.hasLoad = true
		data.load = opts.Load
	}
}

func (data *Resolved) IsHealthy() bool {
	result := false
	if data.Err == nil && data.Addr != nil {
		gDataMutex.Lock()
		result = data.healthy || !data.hasHealthy
		gDataMutex.Unlock()
	}
	return result
}

func (data *Resolved) GetLoad() (load float32, present bool) {
	load = 1.0
	if data.Err == nil && data.Addr != nil {
		gDataMutex.Lock()
		if data.hasLoad {
			load, present = data.load, true
		}
		gDataMutex.Unlock()
	}
	return
}

// }}}

// type Event {{{

type Event struct {
	Type              EventType
	Err               error
	Key               string
	Data              *Resolved
	ServiceConfigJSON string
}

func (event *Event) Check() {
	if checkDisabled {
		return
	}
	if event == nil {
		panic(errors.New("*Event is nil"))
	}
	var (
		expectErr     bool
		expectKey     bool
		expectData    bool
		expectDataErr bool
		expectSC      bool
	)
	switch event.Type {
	case NoOpEvent:
		// pass
	case ErrorEvent:
		expectErr = true
	case UpdateEvent:
		expectKey = true
		expectData = true
	case DeleteEvent:
		expectKey = true
	case BadDataEvent:
		expectKey = true
		expectData = true
		expectDataErr = true
	case StatusChangeEvent:
		expectKey = true
		expectData = true
	case NewServiceConfigEvent:
		expectSC = true
	}
	if expectErr && event.Err == nil {
		panic(fmt.Errorf("Event.Type is %#v but Event.Err is nil", event.Type))
	}
	if !expectErr && event.Err != nil {
		panic(fmt.Errorf("Event.Type is %#v but Event.Err is non-nil", event.Type))
	}
	if expectKey && event.Key == "" {
		panic(fmt.Errorf("Event.Type is %#v but Event.Key is empty", event.Type))
	}
	if !expectKey && event.Key != "" {
		panic(fmt.Errorf("Event.Type is %#v but Event.Key is non-empty", event.Type))
	}
	if expectData && event.Data == nil {
		panic(fmt.Errorf("Event.Type is %#v but Event.Data is nil", event.Type))
	}
	if !expectData && event.Data != nil {
		panic(fmt.Errorf("Event.Type is %#v but Event.Data is non-nil", event.Type))
	}
	if expectSC && event.ServiceConfigJSON == "" {
		panic(fmt.Errorf("Event.Type is %#v but Event.ServiceConfigJSON is empty", event.Type))
	}
	if !expectSC && event.ServiceConfigJSON != "" {
		panic(fmt.Errorf("Event.Type is %#v but Event.ServiceConfigJSON is non-empty", event.Type))
	}
	if event.Data != nil {
		event.Data.Check()
		if event.Key != event.Data.Unique {
			panic(fmt.Errorf("Event.Key is %q but Event.Data.Unique is %q", event.Key, event.Data.Unique))
		}
		if expectDataErr && event.Data.Err == nil {
			panic(fmt.Errorf("Event.Type is %#v but Event.Data.Err is nil", event.Type))
		}
		if !expectDataErr && event.Data.Err != nil {
			panic(fmt.Errorf("Event.Type is %#v but Event.Data.Err is non-nil", event.Type))
		}
	}
}

// }}}
