package main

import (
	"fmt"
)

type Range struct {
	Lo Key
	Hi Key
}

func (r Range) String() string {
	return fmt.Sprintf("[%v,%v]", r.Lo, r.Hi)
}

func (r Range) Contains(key Key) bool {
	return key.Less(r.Hi) && !key.Less(r.Lo)
}
