package baseresolver

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/membership"
)

var gDataMutex sync.Mutex

type Resolver interface {
	Err() error
	ResolveAll() ([]*AddrData, error)
	Resolve() (*AddrData, error)

	Update(opts UpdateOptions)

	Watch(WatchFunc) WatchID
	CancelWatch(WatchID)

	ResolveNow(resolver.ResolveNowOptions)
	Close()
}

var _ resolver.Resolver = Resolver(nil)

type UpdateOptions struct {
	Addr           net.Addr
	Healthy        *bool
	Load           *float32
	ComputedWeight *float32
}

type WatchID uint32

type WatchFunc func([]*Event)

type Event struct {
	Type              EventType
	Err               error
	Key               string
	Data              *AddrData
	ServiceConfigJSON string
}

type AddrData struct {
	Err            error
	Raw            *membership.ServerSet
	Addr           net.Addr
	UniqueKey      *string
	Location       *string
	ServerName     *string
	SRVPriority    *uint16
	SRVWeight      *uint16
	ShardID        *int32
	AssignedWeight *float32
	Address        resolver.Address

	Healthy        *bool
	Load           *float32
	ComputedWeight *float32
}

func (data *AddrData) Key() string {
	if data.Addr == nil {
		panic(errors.New("Addr is nil"))
	}
	if data.UniqueKey != nil {
		return *data.UniqueKey
	}
	if data.SRVPriority != nil && data.SRVWeight != nil {
		return fmt.Sprintf("%d/%d/%s", *data.SRVPriority, *data.SRVWeight, data.Addr.String())
	}
	return data.Addr.String()
}

func (data *AddrData) Update(opts UpdateOptions) {
	gDataMutex.Lock()
	data.updateLocked(opts)
	gDataMutex.Unlock()
}

func (data *AddrData) UpdateFrom(source *AddrData) {
	gDataMutex.Lock()
	data.updateLocked(UpdateOptions{
		Healthy:        source.Healthy,
		Load:           source.Load,
		ComputedWeight: source.ComputedWeight,
	})
	gDataMutex.Unlock()
}

func (data *AddrData) updateLocked(opts UpdateOptions) {
	if opts.Healthy != nil {
		data.Healthy = opts.Healthy
	}
	if opts.Load != nil {
		data.Load = opts.Load
	}
	if opts.ComputedWeight != nil {
		data.ComputedWeight = opts.ComputedWeight
	}
}

func (data *AddrData) IsHealthy() bool {
	result := false
	if data.Err == nil && data.Addr != nil {
		gDataMutex.Lock()
		if data.Healthy == nil {
			result = true
		} else {
			result = *data.Healthy
		}
		gDataMutex.Unlock()
	}
	return result
}

func (data *AddrData) GetLoad() (load float32, ok bool) {
	load = 1.0
	if data.Err == nil && data.Addr != nil {
		gDataMutex.Lock()
		if data.Load != nil {
			load, ok = *data.Load, true
		}
		gDataMutex.Unlock()
	}
	return
}

func (data *AddrData) GetComputedWeight() (weight float32, ok bool) {
	weight = 1.0
	if data.Err == nil && data.Addr != nil {
		gDataMutex.Lock()
		if data.ComputedWeight != nil {
			weight, ok = *data.ComputedWeight, true
		}
		gDataMutex.Unlock()
	}
	return
}
