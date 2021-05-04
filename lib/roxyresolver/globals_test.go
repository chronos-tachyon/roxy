package roxyresolver

import (
	"math/rand"
)

func init() {
	EnableCheck()
}

func FakeRandom() *rand.Rand {
	return rand.New(fakeSource{})
}

type fakeSource struct{}

func (fakeSource) Seed(int64)     {}
func (fakeSource) Int63() int64   { return 0 }
func (fakeSource) Uint64() uint64 { return 0 }

var _ rand.Source64 = fakeSource{}
