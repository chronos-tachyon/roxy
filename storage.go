package main

import (
	"fmt"
	"path/filepath"
	"sync"

	autocert "golang.org/x/crypto/acme/autocert"
)

type StorageEngineCtor func(cfg *StorageConfig) (autocert.Cache, error)

var (
	gStorageEngineMu  sync.Mutex
	gStorageEngineMap map[string]StorageEngineCtor
)

func RegisterStorageEngine(name string, ctor StorageEngineCtor) {
	gStorageEngineMu.Lock()
	if gStorageEngineMap == nil {
		gStorageEngineMap = make(map[string]StorageEngineCtor, 10)
	}
	gStorageEngineMap[name] = ctor
	gStorageEngineMu.Unlock()
}

func NewStorageEngine(cfg *StorageConfig) (autocert.Cache, error) {
	engine := cfg.Engine

	gStorageEngineMu.Lock()
	ctor := gStorageEngineMap[cfg.Engine]
	gStorageEngineMu.Unlock()

	if ctor == nil {
		return nil, fmt.Errorf("roxy: unknown storage engine %q", engine)
	}

	return ctor(cfg)
}

func init() {
	RegisterStorageEngine("local", func(cfg *StorageConfig) (autocert.Cache, error) {
		if cfg.Path == "" {
			return nil, fmt.Errorf("roxy: storage engine \"local\": expected non-empty path, got %q", cfg.Path)
		}

		if len(cfg.Servers) != 0 {
			return nil, fmt.Errorf("roxy: storage engine \"local\": expected empty server list, got %q", cfg.Servers)
		}

		abs, err := filepath.Abs(cfg.Path)
		if err != nil {
			return nil, fmt.Errorf("roxy: storage engine \"local\": failed to make path %q absolute: %w", cfg.Path, err)
		}

		abs = filepath.Clean(abs)
		return autocert.DirCache(abs), nil
	})
}
