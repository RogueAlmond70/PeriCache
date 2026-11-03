package cache

import "context"

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

// NewMemoryStore returns the in-memory Store implementation.
func NewMemoryStore() Store {
	return newMemoryStore()
}
