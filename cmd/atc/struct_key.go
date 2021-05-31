package main

import (
	"strconv"
	"strings"
)

// Key represents a (ServiceName, ShardID) tuple, where the ShardID is
// optional.  If HasShardID is false, then ShardID should be zero.
type Key struct {
	ServiceName ServiceName
	ShardID     ShardID
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
		return string(key.ServiceName) + "/" + strconv.FormatUint(uint64(key.ShardID), 10)
	}
	return string(key.ServiceName)
}

// Compare returns x cmp 0 whenever key cmp other, for comparators less than,
// equal to, or greater than.
func (key Key) Compare(other Key) int {
	cmp := strings.Compare(string(key.ServiceName), string(other.ServiceName))
	if cmp == 0 {
		cmp = cmpBool(key.HasShardID, other.HasShardID)
	}
	if cmp == 0 {
		cmp = cmpShardID(key.ShardID, other.ShardID)
	}
	return cmp
}

// Equal returns true if key is equal to other.
func (key Key) Equal(other Key) bool {
	return key.Compare(other) == 0
}

// Less returns true if key is less than other.
func (key Key) Less(other Key) bool {
	return key.Compare(other) < 0
}

// Next returns the smallest key which is greater than the given key.
func (key Key) Next() Key {
	if key.HasShardID && key.ShardID != ^ShardID(0) {
		key.ShardID++
	} else {
		key.ServiceName = key.ServiceName + "\x00"
		key.ShardID = 0
		key.HasShardID = false
	}
	return key
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case b:
		return -1
	default:
		return 1
	}
}

func cmpShardID(a, b ShardID) int {
	switch {
	case a == b:
		return 0
	case a < b:
		return -1
	default:
		return 1
	}
}
