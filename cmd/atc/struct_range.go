package main

import (
	"fmt"
)

// Range represents a range of Keys.  Matching Keys are greater than or equal
// to Lo but strictly less than Hi.
type Range struct {
	Lo Key
	Hi Key
}

// String returns a human-readable string representation of this Range.
func (r Range) String() string {
	return fmt.Sprintf("[%v,%v]", r.Lo, r.Hi)
}

// Contains returns true if the given Key is contained within this Range.
func (r Range) Contains(key Key) bool {
	return key.Less(r.Hi) && !key.Less(r.Lo)
}
