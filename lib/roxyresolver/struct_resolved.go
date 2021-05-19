package roxyresolver

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc/resolver"
)

// Resolved represents a resolved address.
type Resolved struct {
	// Unique is a stable unique identifier for this server.
	Unique string

	// Location is a string denoting the geographic location of this server.
	//
	// This field is only set by the ATC resolver.
	Location string

	// ServerName is either the empty string or the recommended value of
	// the "crypto/tls".(*Config).ServerName field.
	ServerName string

	// SRVPriority is the priority field of this SRV record.
	//
	// This field is only set by the SRV resolver.
	SRVPriority uint16

	// SRVWeight is the weight field of this SRV record.
	//
	// This field is only set by the SRV resolver.
	SRVWeight uint16

	// ShardID is the shard ID number.
	//
	// This field is only set by some resolvers.
	ShardID uint32

	// Weight is the proportional weight for this server.
	//
	// This field is only set by some resolvers.
	Weight float32

	// HasSRV is true if both SRVPriority and SRVWeight are set.
	HasSRV bool

	// HasShardID is true if ShardID is set.
	HasShardID bool

	// HasWeight is true if Weight is set.
	HasWeight bool

	// Err is the error encountered while resolving this server's address.
	Err error

	// Addr is the address of this server.
	Addr net.Addr

	// Address is the address of this server, in gRPC format.
	Address resolver.Address

	// Dynamic points to mutable, mutex-protected data associated with this
	// server.  Two Resolved addresses can share the same *Dynamic if they
	// point to the same server (e.g. have the same IP address).
	Dynamic *Dynamic
}

// Check verifies the data integrity of all fields.
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

// Equal returns true iff the two Resolved addresses are identical.
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

// IsHealthy returns true if this server is healthy.
func (data Resolved) IsHealthy() bool {
	result := false
	if data.Dynamic != nil && data.Err == nil {
		result = data.Dynamic.IsHealthy()
	}
	return result
}

// Dynamic represents the mutable, mutex-protected data associated with one or
// more Resolved addresses.
type Dynamic struct {
	_ sync.Mutex // prevent copying, force alignment

	healthy uint32
}

// Update changes the status of this server.
func (dynamic *Dynamic) Update(opts UpdateOptions) {
	if opts.HasHealthy {
		newValue := uint32(1)
		if opts.Healthy {
			newValue = 2
		}
		atomic.StoreUint32(&dynamic.healthy, newValue)
	}
}

// IsHealthy returns true if this server is healthy.
func (dynamic *Dynamic) IsHealthy() bool {
	value := atomic.LoadUint32(&dynamic.healthy)
	result := (value == 0 || value == 2)
	return result
}
