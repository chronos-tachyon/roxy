package main

import (
	"fmt"
	"strings"
)

type Key struct {
	ServiceName ServiceName
	ShardID     ShardID
}

func (key Key) String() string {
	return fmt.Sprintf("%q(%d)", string(key.ServiceName), uint32(key.ShardID))
}

func (key Key) Compare(other Key) int {
	cmp := strings.Compare(string(key.ServiceName), string(other.ServiceName))
	if cmp == 0 {
		switch {
		case key.ShardID < other.ShardID:
			cmp = -1
		case key.ShardID == other.ShardID:
			cmp = 0
		default:
			cmp = 1
		}
	}
	return cmp
}

func (key Key) Equal(other Key) bool {
	return (key.ServiceName == other.ServiceName && key.ShardID == other.ShardID)
}

func (key Key) Less(other Key) bool {
	cmp := strings.Compare(string(key.ServiceName), string(other.ServiceName))
	return (cmp < 0) || (cmp == 0 && key.ShardID < other.ShardID)
}

func (key Key) Next() Key {
	key.ShardID++
	return key
}
