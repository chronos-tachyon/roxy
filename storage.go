package main

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	v3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/crypto/acme/autocert"

	"github.com/chronos-tachyon/roxy/lib/mainutil"
)

type StorageEngine interface {
	autocert.Cache
	Close() error
}

type StorageEngineCtor func(impl *Impl, cfg *StorageConfig) (StorageEngine, error)

var (
	gStorageEngineMu  sync.Mutex
	gStorageEngineMap map[string]StorageEngineCtor
)

func NewStorageEngine(impl *Impl, cfg *StorageConfig) (StorageEngine, error) {
	engine := cfg.Engine

	gStorageEngineMu.Lock()
	ctor := gStorageEngineMap[cfg.Engine]
	gStorageEngineMu.Unlock()

	if ctor == nil {
		return nil, StorageEngineCreateError{
			Engine: engine,
			Err:    errors.New("unknown storage engine"),
		}
	}

	return ctor(impl, cfg)
}

func RegisterStorageEngine(name string, ctor StorageEngineCtor) {
	if ctor == nil {
		panic(errors.New("ctor is nil"))
	}
	gStorageEngineMu.Lock()
	if gStorageEngineMap == nil {
		gStorageEngineMap = make(map[string]StorageEngineCtor, 10)
	}
	gStorageEngineMap[name] = ctor
	gStorageEngineMu.Unlock()
}

// type FileSystemStorageEngine {{{

type FileSystemStorageEngine struct {
	dc autocert.DirCache
}

func (engine *FileSystemStorageEngine) Get(ctx context.Context, name string) ([]byte, error) {
	data, err := engine.dc.Get(ctx, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, autocert.ErrCacheMiss
	}
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
	etcd    *v3.Client
	prefix  string
	timeout time.Duration
}

func (engine *EtcdStorageEngine) Get(ctx context.Context, name string) ([]byte, error) {
	ctx, cancelfn := context.WithTimeout(ctx, engine.timeout)
	defer cancelfn()

	key := engine.prefix + name
	resp, err := engine.etcd.KV.Get(ctx, key)
	if err != nil {
		return nil, StorageEngineOperationError{
			Engine: "etcd",
			Op:     "Get",
			Key:    name,
			Err:    err,
		}
	}
	if len(resp.Kvs) == 0 {
		return nil, autocert.ErrCacheMiss
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
	zkconn *zk.Conn
	prefix string
}

func (engine *ZKStorageEngine) Get(ctx context.Context, name string) ([]byte, error) {
	key := engine.prefix + name
	data, _, err := engine.zkconn.Get(key)
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
	_, err := engine.zkconn.Create(key, data, 0, zk.WorldACL(zk.PermAll))
	if err == nil {
		return nil
	}
	if errors.Is(err, zk.ErrNodeExists) {
		_, err = engine.zkconn.Set(key, data, -1)
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
	err := engine.zkconn.Delete(key, -1)
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

func init() {
	RegisterStorageEngine("fs", func(_ *Impl, cfg *StorageConfig) (StorageEngine, error) {
		if cfg.Path == "" {
			return nil, StorageEngineCreateError{
				Engine: "fs",
				Err:    errors.New("missing required field \"path\""),
			}
		}

		abs, err := mainutil.ProcessPath(cfg.Path)
		if err != nil {
			return nil, StorageEngineCreateError{
				Engine: "fs",
				Err:    err,
			}
		}

		return &FileSystemStorageEngine{autocert.DirCache(abs)}, nil
	})
	RegisterStorageEngine("etcd", func(impl *Impl, cfg *StorageConfig) (StorageEngine, error) {
		if impl.etcd == nil {
			return nil, StorageEngineCreateError{
				Engine: "etcd",
				Err:    errors.New("missing configuration for \"global.etcd\" section"),
			}
		}

		if cfg.Path == "" {
			return nil, StorageEngineCreateError{
				Engine: "etcd",
				Err:    errors.New("missing required field \"path\""),
			}
		}

		return &EtcdStorageEngine{impl.etcd, cfg.Path, 5 * time.Second}, nil
	})
	RegisterStorageEngine("zk", func(impl *Impl, cfg *StorageConfig) (StorageEngine, error) {
		if impl.zkconn == nil {
			return nil, StorageEngineCreateError{
				Engine: "zk",
				Err:    errors.New("missing configuration for \"global.zookeeper\" section"),
			}
		}

		if cfg.Path == "" {
			return nil, StorageEngineCreateError{
				Engine: "zk",
				Err:    errors.New("missing required field \"path\""),
			}
		}

		return &ZKStorageEngine{impl.zkconn, cfg.Path}, nil
	})
}
