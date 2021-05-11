package roxyresolver

import (
	"errors"
	"net"
	"sync"
)

type Resolved struct {
	Unique      string
	Location    string
	ServerName  string
	SRVPriority uint16
	SRVWeight   uint16
	ShardID     uint32
	Weight      float32
	HasSRV      bool
	HasShardID  bool
	HasWeight   bool
	Err         error
	Addr        net.Addr
	Address     Address
	Dynamic     *Dynamic
}

func (data Resolved) Check() {
	if checkDisabled {
		return
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
	if data.Addr != nil && data.Dynamic == nil {
		panic(errors.New("Resolved.Addr is not nil but Resolved.Dynamic is nil"))
	}
	if data.ServerName != data.Address.ServerName {
		panic(errors.New("Resolved.ServerName is not equal to Resolved.Address.ServerName"))
	}
}

func (data Resolved) Equal(other Resolved) bool {
	equal := true
	equal = equal && (data.Unique == other.Unique)
	equal = equal && (data.Location == other.Location)
	equal = equal && (data.ServerName == other.ServerName)
	equal = equal && (data.HasSRV == other.HasSRV)
	equal = equal && (data.HasShardID == other.HasShardID)
	equal = equal && (data.HasWeight == other.HasWeight)
	if equal && data.HasSRV {
		equal = equal && (data.SRVPriority == other.SRVPriority)
		equal = equal && (data.SRVWeight == other.SRVWeight)
	}
	if equal && data.HasShardID {
		equal = equal && (data.ShardID == other.ShardID)
	}
	if equal && data.HasWeight {
		equal = equal && (data.Weight == other.Weight)
	}
	equal = equal && (data.Err == other.Err)
	equal = equal && (data.Addr == other.Addr)
	equal = equal && (data.Address.Addr == other.Address.Addr)
	equal = equal && (data.Address.ServerName == other.Address.ServerName)
	equal = equal && (data.Dynamic == other.Dynamic)
	return equal
}

func (data Resolved) IsHealthy() bool {
	result := false
	if data.Dynamic != nil && data.Err == nil {
		result = data.Dynamic.IsHealthy()
	}
	return result
}

func (data Resolved) GetLoad() (load float32, ok bool) {
	load, ok = 1.0, false
	if data.Dynamic != nil && data.Err == nil {
		load, ok = data.Dynamic.GetLoad()
	}
	return
}

type Dynamic struct {
	mu         sync.Mutex
	load       float32
	healthy    bool
	hasHealthy bool
	hasLoad    bool
}

func (dynamic *Dynamic) Update(opts UpdateOptions) {
	dynamic.mu.Lock()
	if opts.HasHealthy {
		dynamic.hasHealthy = true
		dynamic.healthy = opts.Healthy
	}
	if opts.HasLoad {
		dynamic.hasLoad = true
		dynamic.load = opts.Load
	}
	dynamic.mu.Unlock()
}

func (dynamic *Dynamic) IsHealthy() bool {
	dynamic.mu.Lock()
	result := dynamic.healthy || !dynamic.hasHealthy
	dynamic.mu.Unlock()
	return result
}

func (dynamic *Dynamic) GetLoad() (load float32, ok bool) {
	load, ok = 1.0, false
	dynamic.mu.Lock()
	if dynamic.hasLoad {
		load, ok = dynamic.load, true
	}
	dynamic.mu.Unlock()
	return
}
