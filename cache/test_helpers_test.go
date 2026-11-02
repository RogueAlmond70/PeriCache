package cache

import (
	"testing"
	"time"
)

func (c *ourCache) mem(t *testing.T) *memoryStore {
	t.Helper()
	ms := c.memoryStoreOrNil()
	if ms == nil {
		t.Fatal("expected memory store")
	}
	return ms
}

func (c *ourCache) hasKey(t *testing.T, key string) bool {
	t.Helper()
	ms := c.mem(t)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	_, ok := ms.data[key]
	return ok
}

func (c *ourCache) entryCount(t *testing.T) int {
	t.Helper()
	ms := c.mem(t)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return len(ms.data)
}

func (c *ourCache) getRaw(t *testing.T, key string) (record, bool) {
	t.Helper()
	ms := c.mem(t)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	rec, ok := ms.data[key]
	return rec, ok
}

func (c *ourCache) putRaw(t *testing.T, key string, rec record) {
	t.Helper()
	ms := c.mem(t)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.data[key] = rec
}

func (c *ourCache) expireKey(t *testing.T, key string, at time.Time) {
	t.Helper()
	rec, ok := c.getRaw(t, key)
	if !ok {
		t.Fatalf("key %q missing", key)
	}
	rec.expiry = at
	c.putRaw(t, key, rec)
}
