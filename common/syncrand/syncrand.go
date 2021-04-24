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

func Global() *rand.Rand {
	gMu.Lock()
	defer gMu.Unlock()
	return gRand
}

func GlobalSource() rand.Source64 {
	gMu.Lock()
	defer gMu.Unlock()
	return gSource
}

func SetGlobalSource(s rand.Source64) {
	gMu.Lock()
	defer gMu.Unlock()
	gSource = WrapSource(s)
	gRand = rand.New(gSource)
}

func New(seed int64) *rand.Rand {
	return rand.New(NewSource(seed))
}

func NewFromSource(s rand.Source64) *rand.Rand {
	return rand.New(WrapSource(s))
}

func NewSource(seed int64) rand.Source64 {
	return WrapSource(rand.NewSource(seed).(rand.Source64))
}

func WrapSource(s rand.Source64) rand.Source64 {
	if IsWrapped(s) {
		return s
	}
	return &wrappedSource{s: s}
}

func IsWrapped(s rand.Source64) bool {
	if s == nil {
		panic(errors.New("source is nil"))
	}

	if _, ok := s.(*wrappedSource); ok {
		return true
	}

	type unwrapper interface{ Unwrap() rand.Source64 }
	for s != nil {
		x, ok := s.(unwrapper)
		if !ok {
			return false
		}
		inner := x.Unwrap()
		if _, ok := inner.(*wrappedSource); ok {
			return true
		}
		s = inner
	}
	return false
}

type wrappedSource struct {
	mu sync.Mutex
	s  rand.Source64
}

func (s *wrappedSource) Seed(seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s.Seed(seed)
}

func (s *wrappedSource) Int63() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s.Int63()
}

func (s *wrappedSource) Uint64() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s.Uint64()
}

func (s *wrappedSource) Unwrap() rand.Source64 {
	return s.s
}

var _ rand.Source64 = (*wrappedSource)(nil)
