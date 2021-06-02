package main

import (
	"strconv"
	"strings"
)

// Key represents a (ServiceName, ShardNumber) tuple, where the ShardNumber is
// optional.  If HasShardNumber is false, then ShardNumber should be zero.
type Key struct {
	ServiceName    ServiceName
	ShardNumber    ShardNumber
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
		return string(key.ServiceName) + "/" + strconv.FormatUint(uint64(key.ShardNumber), 10)
	}
	return string(key.ServiceName)
}

// Compare returns x cmp 0 whenever key cmp other, for comparators less than,
// equal to, or greater than.
func (key Key) Compare(other Key) int {
	cmp := strings.Compare(string(key.ServiceName), string(other.ServiceName))
	if cmp == 0 {
		cmp = cmpBool(key.HasShardNumber, other.HasShardNumber)
	}
	if cmp == 0 {
		cmp = cmpShardNumber(key.ShardNumber, other.ShardNumber)
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
	if key.HasShardNumber && key.ShardNumber != ^ShardNumber(0) {
		key.ShardNumber++
	} else {
		key.ServiceName = key.ServiceName + "\x00"
		key.ShardNumber = 0
		key.HasShardNumber = false
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

func cmpShardNumber(a, b ShardNumber) int {
	switch {
	case a == b:
		return 0
	case a < b:
		return -1
	default:
		return 1
	}
}
