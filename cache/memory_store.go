package cache

import (
	"context"
	"sync"
	"time"
)

type memoryStore struct {
	mu   sync.Mutex
	data map[string]record
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: make(map[string]record)}
}

func (m *memoryStore) Put(_ context.Context, key string, rec record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = rec
	return nil
}

func (m *memoryStore) PutAndEvict(ctx context.Context, key string, rec record, maxSize int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = rec
	m.deleteExpiredLocked()
	for len(m.data) > maxSize {
		if err := m.evictOneLocked(); err != nil {
			return err
		}
	}
	return nil
}

func (m *memoryStore) Get(_ context.Context, key string) (record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.data[key]
	if !ok {
		return record{}, false, nil
	}
	if time.Now().After(rec.expiry) {
		delete(m.data, key)
		return record{}, false, nil
	}
	// Touch under the same lock so concurrent Set cannot be overwritten by a
	// separate read-modify-write of lastAccessed in the cache layer.
	rec.lastAccessed = time.Now()
	m.data[key] = rec
	return rec, true, nil
}

func (m *memoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memoryStore) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]record)
	return nil
}

func (m *memoryStore) Keys(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteExpiredLocked()
	keys := make([]string, 0, len(m.data))
	for key := range m.data {
		keys = append(keys, key)
	}
	return keys, nil
}

func (m *memoryStore) Size(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteExpiredLocked()
	return len(m.data), nil
}

func (m *memoryStore) DeleteExpired(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := len(m.data)
	m.deleteExpiredLocked()
	return before - len(m.data), nil
}

func (m *memoryStore) EvictIfNeeded(ctx context.Context, maxSize int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteExpiredLocked()
	for len(m.data) > maxSize {
		if err := m.evictOneLocked(); err != nil {
			return err
		}
	}
	return nil
}

func (m *memoryStore) Close(_ context.Context) error {
	return nil
}

func (m *memoryStore) deleteExpiredLocked() {
	now := time.Now()
	for key, rec := range m.data {
		if now.After(rec.expiry) {
			delete(m.data, key)
		}
	}
}

func (m *memoryStore) evictOneLocked() error {
	if len(m.data) == 0 {
		return nil
	}

	victimKey := ""
	var victim record
	found := false

	for key, rec := range m.data {
		if !found {
			victimKey = key
			victim = rec
			found = true
			continue
		}
		// Least recently accessed wins. Equal access times break ties on earliest expiry
		// (covers inserts that share the same timestamp).
		if rec.lastAccessed.Before(victim.lastAccessed) ||
			(rec.lastAccessed.Equal(victim.lastAccessed) && rec.expiry.Before(victim.expiry)) {
			victimKey = key
			victim = rec
		}
	}

	delete(m.data, victimKey)
	return nil
}
