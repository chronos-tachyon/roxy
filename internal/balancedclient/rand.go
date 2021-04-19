package balancedclient

import (
	"math/rand"
	"sync"
)

func NewThreadSafeSource(seed int64) rand.Source64 {
	var rawSource rand.Source64 = rand.NewSource(seed).(rand.Source64)
	return &lockedSource{inner: rawSource}
}

func NewThreadSafeRandom(seed int64) *rand.Rand {
	return rand.New(NewThreadSafeSource(seed))
}

// type lockedSource {{{

type lockedSource struct {
	mu    sync.Mutex
	inner rand.Source64
}

func (src *lockedSource) Seed(seed int64) {
	src.mu.Lock()
	defer src.mu.Unlock()
	src.inner.Seed(seed)
}

func (src *lockedSource) Int63() int64 {
	src.mu.Lock()
	defer src.mu.Unlock()
	return src.inner.Int63()
}

func (src *lockedSource) Uint64() uint64 {
	src.mu.Lock()
	defer src.mu.Unlock()
	return src.inner.Uint64()
}

var _ rand.Source64 = (*lockedSource)(nil)

// }}}
