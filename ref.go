package main

import (
	"sync"
)

type Ref struct {
	mu   sync.RWMutex
	impl *Impl
}

func (ref *Ref) Load(configPath string) error {
	next, err := LoadImpl(configPath)
	if err != nil {
		return err
	}
	if prev := ref.Swap(next); prev != nil {
		if err := prev.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (ref *Ref) Close() error {
	if prev := ref.Swap(nil); prev != nil {
		if err := prev.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (ref *Ref) Get() *Impl {
	ref.mu.RLock()
	impl := ref.impl
	ref.mu.RUnlock()
	return impl
}

func (ref *Ref) Swap(next *Impl) (prev *Impl) {
	ref.mu.Lock()
	prev = ref.impl
	ref.impl = next
	ref.mu.Unlock()
	return prev
}
