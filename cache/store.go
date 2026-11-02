package cache

import (
	"context"
	"fmt"
	"strings"
)

// Store is the persistence port for cache entries.
type Store interface {
	Put(ctx context.Context, key string, rec record) error
	Get(ctx context.Context, key string) (record, bool, error)
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
	Keys(ctx context.Context) ([]string, error)
	Size(ctx context.Context) (int, error)
	DeleteExpired(ctx context.Context) (int, error)
	EvictIfNeeded(ctx context.Context, maxSize int) error
	Close(ctx context.Context) error
}

const (
	StoreMemory = "memory"
	StoreMongo  = "mongo"
)

// NewStore builds a Store from a backend name.
// mongoURI is required when backend is mongo (e.g. mongodb://mongo:27017).
func NewStore(ctx context.Context, backend, mongoURI, mongoDatabase string) (Store, error) {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", StoreMemory:
		return newMemoryStore(), nil
	case StoreMongo:
		if mongoURI == "" {
			return nil, fmt.Errorf("mongo store requires a MongoDB URI")
		}
		if mongoDatabase == "" {
			mongoDatabase = "nandos_cache"
		}
		return newMongoStore(ctx, mongoURI, mongoDatabase, "entries")
	default:
		return nil, fmt.Errorf("unknown store backend %q (want memory|mongo)", backend)
	}
}
