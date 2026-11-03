package cache

import (
	"PeriCache/loyalty"
	"context"
	"time"
)

// Record is one persisted cache entry. Exported so alternative Store
// implementations (e.g. Mongo) can be written outside this package.
type Record struct {
	Data         loyalty.LoyaltyProfile
	TTL          int
	Expiry       time.Time
	LastAccessed time.Time
}

// Store is the persistence port for cache entries.
// Implementations must be safe for concurrent use.
type Store interface {
	Put(ctx context.Context, key string, rec Record) error
	// PutAndEvict stores rec then enforces maxSize under one lock where possible.
	PutAndEvict(ctx context.Context, key string, rec Record, maxSize int) error
	Get(ctx context.Context, key string) (Record, bool, error)
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
