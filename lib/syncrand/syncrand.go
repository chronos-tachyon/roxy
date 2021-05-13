package syncrand

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

var (
	gMu     sync.Mutex
	gSource rand.Source64
	gRand   *rand.Rand
)

func init() {
	gSource = NewSource(time.Now().UnixNano())
	gRand = rand.New(gSource)
}

// WrappedSource represents a source of randomness which is itself backed by
// another source of randomness.  The Unwrap() method provides access to the
// underlying Source.
type WrappedSource interface {
	rand.Source64
	Unwrap() rand.Source64
}

// Global returns this package's global instance of "math/rand".(*Rand).  It is
// guaranteed to be thread-safe.
func Global() *rand.Rand {
	gMu.Lock()
	rng := gRand
	gMu.Unlock()
	return rng
}

// GlobalSource returns this package's global instance of "math/rand".Source64,
// which backs the "math/rand".(*Rand) returned by Global().  It is guaranteed
// to be thread-safe.
func GlobalSource() rand.Source64 {
	gMu.Lock()
	source := gSource
	gMu.Unlock()
	return source
}

// SetGlobalSource atomically replaces the global instance of
// "math/rand".Source64 with the provided one.  If it is not thread-safe, then
// it is wrapped in an implementation of WrappedSource that _is_ thread-safe.
func SetGlobalSource(source rand.Source64) {
	gMu.Lock()
	gSource = WrapSource(source)
	gRand = rand.New(gSource)
	gMu.Unlock()
}

// New returns a new "math/rand".(*Rand) seeded with the given seed.  It is
// guaranteed to be thread-safe.
func New(seed int64) *rand.Rand {
	return rand.New(NewSource(seed))
}

// NewFromSource returns a new "math/rand".(*Rand) that is backed by the given
// source.  It is guaranteed to be thread-safe.
func NewFromSource(source rand.Source64) *rand.Rand {
	return rand.New(WrapSource(source))
}

// NewSource returns a new "math/rand".Source64 that is seeded with the given
// seed.  It is guaranteed to be thread-safe.
func NewSource(seed int64) rand.Source64 {
	return WrapSource(rand.NewSource(seed).(rand.Source64))
}

// WrapSource returns an instance of "math/rand".Source64 that is thread-safe.
// It may return its argument unchanged, if that argument is already known to
// be thread-safe.
func WrapSource(source rand.Source64) rand.Source64 {
	if IsThreadSafe(source) {
		return source
	}
	return &syncSource{s64: source}
}

// IsThreadSafe returns true if its argument is known to be thread-safe.
func IsThreadSafe(source rand.Source64) bool {
	if source == nil {
		panic(errors.New("source is nil"))
	}

	for source != nil {
		if _, ok := source.(*syncSource); ok {
			return true
		}

		wrapper, ok := source.(WrappedSource)
		if !ok {
			return false
		}

		source = wrapper.Unwrap()
	}
	return false
}

type syncSource struct {
	mu  sync.Mutex
	s64 rand.Source64
}

func (ss *syncSource) Seed(seed int64) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.s64.Seed(seed)
}

func (ss *syncSource) Int63() int64 {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.s64.Int63()
}

func (ss *syncSource) Uint64() uint64 {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.s64.Uint64()
}

func (ss *syncSource) Unwrap() rand.Source64 {
	return ss.s64
}

var _ rand.Source64 = (*syncSource)(nil)
