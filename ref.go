package main

import (
	"context"
	"net/http"
	"sync"

	autocert "golang.org/x/crypto/acme/autocert"
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

func (ref *Ref) HostPolicy() autocert.HostPolicy {
	return func(ctx context.Context, host string) error {
		return ref.Get().HostPolicyImpl(ctx, host)
	}
}

func (ref *Ref) Cache() autocert.Cache {
	return CacheWrapper{ref}
}

func (ref *Ref) Handler() http.Handler {
	return HandlerWrapper{ref}
}

type CacheWrapper struct {
	ref *Ref
}

func (wrap CacheWrapper) Get(ctx context.Context, key string) ([]byte, error) {
	return wrap.ref.Get().StorageGet(ctx, key)
}

func (wrap CacheWrapper) Put(ctx context.Context, key string, data []byte) error {
	return wrap.ref.Get().StoragePut(ctx, key, data)
}

func (wrap CacheWrapper) Delete(ctx context.Context, key string) error {
	return wrap.ref.Get().StorageDelete(ctx, key)
}

var _ autocert.Cache = CacheWrapper{}

type HandlerWrapper struct {
	ref *Ref
}

func (wrap HandlerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wrap.ref.Get().ServeHTTP(w, r)
}

var _ http.Handler = HandlerWrapper{}
