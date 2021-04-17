package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	zkclient "github.com/go-zookeeper/zk"
	etcdclient "go.etcd.io/etcd/client/v3"
	autocert "golang.org/x/crypto/acme/autocert"
)

var (
	gStorageEngineMu  sync.Mutex
	gStorageEngineMap map[string]StorageEngineCtor
)

// type StorageEngine {{{

type StorageEngine interface {
	autocert.Cache
	Close() error
}

// type FileSystemStorageEngine {{{

type FileSystemStorageEngine struct {
	dc autocert.DirCache
}

func (engine *FileSystemStorageEngine) Get(ctx context.Context, name string) ([]byte, error) {
	data, err := engine.dc.Get(ctx, name)
	if err != nil {
		return nil, StorageEngineOperationError{
			Engine: "fs",
			Op:     "Get",
			Key:    name,
			Err:    err,
		}
	}
	return data, nil
}

func (engine *FileSystemStorageEngine) Put(ctx context.Context, name string, data []byte) error {
	err := engine.dc.Put(ctx, name, data)
	if err != nil {
		return StorageEngineOperationError{
			Engine: "fs",
			Op:     "Put",
			Key:    name,
			Err:    err,
		}
	}
	return nil
}

func (engine *FileSystemStorageEngine) Delete(ctx context.Context, name string) error {
	err := engine.dc.Delete(ctx, name)
	if err != nil {
		return StorageEngineOperationError{
			Engine: "fs",
			Op:     "Delete",
			Key:    name,
			Err:    err,
		}
	}
	return nil
}

func (engine *FileSystemStorageEngine) Close() error {
	return nil
}

var _ StorageEngine = (*FileSystemStorageEngine)(nil)

// }}}

// type EtcdStorageEngine {{{

type EtcdStorageEngine struct {
	etcd    *etcdclient.Client
	prefix  string
	timeout time.Duration
}

func (engine *EtcdStorageEngine) Get(ctx context.Context, name string) ([]byte, error) {
	ctx, cancelfn := context.WithTimeout(ctx, engine.timeout)
	defer cancelfn()

	key := engine.prefix + name
	resp, err := engine.etcd.KV.Get(ctx, key)
	if err == nil && len(resp.Kvs) == 0 {
		err = fmt.Errorf("internal error: len(resp.Kvs) == 0")
	}
	if err != nil {
		return nil, StorageEngineOperationError{
			Engine: "etcd",
			Op:     "Get",
			Key:    name,
			Err:    err,
		}
	}
	return resp.Kvs[0].Value, nil
}

func (engine *EtcdStorageEngine) Put(ctx context.Context, name string, data []byte) error {
	ctx, cancelfn := context.WithTimeout(ctx, engine.timeout)
	defer cancelfn()

	key := engine.prefix + name
	_, err := engine.etcd.KV.Put(ctx, key, string(data))
	if err != nil {
		return StorageEngineOperationError{
			Engine: "etcd",
			Op:     "Put",
			Key:    name,
			Err:    err,
		}
	}
	return nil
}

func (engine *EtcdStorageEngine) Delete(ctx context.Context, name string) error {
	ctx, cancelfn := context.WithTimeout(ctx, engine.timeout)
	defer cancelfn()

	key := engine.prefix + name
	_, err := engine.etcd.KV.Delete(ctx, key)
	if err != nil {
		return StorageEngineOperationError{
			Engine: "etcd",
			Op:     "Delete",
			Key:    name,
			Err:    err,
		}
	}
	return nil
}

func (engine *EtcdStorageEngine) Close() error {
	return nil
}

var _ StorageEngine = (*EtcdStorageEngine)(nil)

// }}}

// type ZKStorageEngine {{{

type ZKStorageEngine struct {
	zk     *zkclient.Conn
	prefix string
}

func (engine *ZKStorageEngine) Get(ctx context.Context, name string) ([]byte, error) {
	key := engine.prefix + name
	data, _, err := engine.zk.Get(key)
	if err != nil {
		return nil, StorageEngineOperationError{
			Engine: "zk",
			Op:     "Get",
			Key:    name,
			Err:    err,
		}
	}
	return data, nil
}

func (engine *ZKStorageEngine) Put(ctx context.Context, name string, data []byte) error {
	key := engine.prefix + name
	_, err := engine.zk.Create(key, data, 0, zkclient.WorldACL(zkclient.PermAll))
	if err == nil {
		return nil
	}
	if errors.Is(err, zkclient.ErrNodeExists) {
		_, err = engine.zk.Set(key, data, -1)
		if err == nil {
			return nil
		}
	}
	return StorageEngineOperationError{
		Engine: "zk",
		Op:     "Put",
		Key:    name,
		Err:    err,
	}
}

func (engine *ZKStorageEngine) Delete(ctx context.Context, name string) error {
	key := engine.prefix + name
	err := engine.zk.Delete(key, -1)
	if err != nil {
		return StorageEngineOperationError{
			Engine: "zk",
			Op:     "Delete",
			Key:    name,
			Err:    err,
		}
	}
	return nil
}

func (engine *ZKStorageEngine) Close() error {
	return nil
}

var _ StorageEngine = (*ZKStorageEngine)(nil)

// }}}

// }}}

type StorageEngineCtor func(impl *Impl, cfg *StorageConfig) (StorageEngine, error)

func RegisterStorageEngine(name string, ctor StorageEngineCtor) {
	gStorageEngineMu.Lock()
	if gStorageEngineMap == nil {
		gStorageEngineMap = make(map[string]StorageEngineCtor, 10)
	}
	gStorageEngineMap[name] = ctor
	gStorageEngineMu.Unlock()
}

func NewStorageEngine(impl *Impl, cfg *StorageConfig) (StorageEngine, error) {
	engine := cfg.Engine

	gStorageEngineMu.Lock()
	ctor := gStorageEngineMap[cfg.Engine]
	gStorageEngineMu.Unlock()

	if ctor == nil {
		return nil, StorageEngineCreateError{
			Engine: engine,
			Err:    fmt.Errorf("unknown storage engine"),
		}
	}

	return ctor(impl, cfg)
}

func init() {
	RegisterStorageEngine("fs", func(_ *Impl, cfg *StorageConfig) (StorageEngine, error) {
		if cfg.Path == "" {
			return nil, StorageEngineCreateError{
				Engine: "fs",
				Err:    fmt.Errorf("expected non-empty path, got %q", cfg.Path),
			}
		}

		abs, err := filepath.Abs(cfg.Path)
		if err != nil {
			return nil, StorageEngineCreateError{
				Engine: "fs",
				Err:    fmt.Errorf("failed to make path %q absolute: %w", cfg.Path, err),
			}
		}

		abs = filepath.Clean(abs)
		return &FileSystemStorageEngine{autocert.DirCache(abs)}, nil
	})
	RegisterStorageEngine("etcd", func(impl *Impl, cfg *StorageConfig) (StorageEngine, error) {
		if impl.etcd == nil {
			return nil, StorageEngineCreateError{
				Engine: "etcd",
				Err:    fmt.Errorf("missing configuration for top-level \"etcd\" section"),
			}
		}

		if cfg.Path == "" {
			return nil, StorageEngineCreateError{
				Engine: "etcd",
				Err:    fmt.Errorf("expected non-empty path, got %q", cfg.Path),
			}
		}

		return &EtcdStorageEngine{impl.etcd, cfg.Path, 5 * time.Second}, nil
	})
	RegisterStorageEngine("zk", func(impl *Impl, cfg *StorageConfig) (StorageEngine, error) {
		if impl.zk == nil {
			return nil, StorageEngineCreateError{
				Engine: "zk",
				Err:    fmt.Errorf("missing configuration for top-level \"zookeeper\" section"),
			}
		}

		if cfg.Path == "" {
			return nil, StorageEngineCreateError{
				Engine: "zk",
				Err:    fmt.Errorf("expected non-empty path, got %q", cfg.Path),
			}
		}

		return &ZKStorageEngine{impl.zk, cfg.Path}, nil
	})
}
