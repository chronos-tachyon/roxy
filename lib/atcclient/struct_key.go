package atcclient

import (
	"strconv"
)

// Key represents a (ServiceName, ShardNumber) tuple, where the ShardNumber is
// optional.  If HasShardNumber is false, then ShardNumber should be zero.
type Key struct {
	ServiceName    string
	ShardNumber    uint32
	HasShardNumber bool
}

// ForceCanonical ensures that this Key is in the canonical format, i.e. that
// SharID is zero if HasShardNumber is false.
func (key *Key) ForceCanonical() {
	if !key.HasShardNumber {
		key.ShardNumber = 0
	}
}

// String returns a human-readable string representation of this Key.
func (key Key) String() string {
	if key.HasShardNumber {
		return key.ServiceName + "/" + strconv.FormatUint(uint64(key.ShardNumber), 10)
	}
	return key.ServiceName
}
