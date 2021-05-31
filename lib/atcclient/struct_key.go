package atcclient

import (
	"strconv"
)

// Key represents a (ServiceName, ShardID) tuple, where the ShardID is
// optional.  If HasShardID is false, then ShardID should be zero.
type Key struct {
	ServiceName string
	ShardID     uint32
	HasShardID  bool
}

// ForceCanonical ensures that this Key is in the canonical format, i.e. that
// SharID is zero if HasShardID is false.
func (key *Key) ForceCanonical() {
	if !key.HasShardID {
		key.ShardID = 0
	}
}

// String returns a human-readable string representation of this Key.
func (key Key) String() string {
	if key.HasShardID {
		return key.ServiceName + "/" + strconv.FormatUint(uint64(key.ShardID), 10)
	}
	return key.ServiceName
}
