package cache

import (
	"Nandos-Pair-Coding/loyalty"
	"Nandos-Pair-Coding/metrics"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLogger satisfies internal.Logger for unit tests.
type mockLogger struct {
	mu     sync.Mutex
	infos  []string
	errors []string
	warns  []string
}

func (m *mockLogger) Info(msg string, _ ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.infos = append(m.infos, msg)
}

func (m *mockLogger) Error(msg string, _ ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, msg)
}

func (m *mockLogger) Warn(msg string, _ ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warns = append(m.warns, msg)
}

func testProfile(id string) loyalty.LoyaltyProfile {
	now := time.Now().UTC()
	return loyalty.LoyaltyProfile{
		CustomerID:    id,
		FirstName:     "Test",
		LastName:      "User",
		Email:         id + "@example.com",
		AccountStatus: "active",
		CreatedAt:     now,
		LastActivity:  now,
		Loyalty: loyalty.Loyalty{
			Tier:          "Hot",
			PointsBalance: 50,
			LifetimePoints: 50,
		},
	}
}

func newTestCache(t *testing.T, maxSize int) (*ourCache, *mockLogger) {
	t.Helper()
	logger := &mockLogger{}
	c, err := NewCache(context.Background(), logger, maxSize, metrics.NewMemoryRecorder(), newMemoryStore())
	require.NoError(t, err)
	require.NotNil(t, c)
	return c, logger
}

func TestNewCache(t *testing.T) {
	t.Run("success with explicit max size", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		assert.Equal(t, 10, c.maxSize)
		assert.NotNil(t, c.memoryStoreOrNil())
	})

	t.Run("defaults max size when less than 1", func(t *testing.T) {
		c, _ := newTestCache(t, 0)
		assert.Equal(t, 100, c.maxSize)

		c2, _ := newTestCache(t, -5)
		assert.Equal(t, 100, c2.maxSize)
	})

	t.Run("cancelled context", func(t *testing.T) {
		logger := &mockLogger{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		c, err := NewCache(ctx, logger, 10, nil, nil)
		assert.Nil(t, c)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotEmpty(t, logger.errors)
	})
}

func TestSet(t *testing.T) {
	t.Run("stores loyalty profile", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		profile := testProfile("cust-1")
		err := c.set(context.Background(), "k1", profile, 60)
		require.NoError(t, err)

		rec, ok := c.getRaw(t, "k1")
		require.True(t, ok)
		assert.Equal(t, profile.CustomerID, rec.data.CustomerID)
		assert.Equal(t, 60, rec.ttl)
		assert.True(t, rec.expiry.After(time.Now()))
		assert.True(t, rec.lastAccessed.IsZero())
		assert.Equal(t, 1, c.Stats().Set.Attempted)
		assert.Equal(t, 1, c.Stats().Set.Success)
		assert.Equal(t, 0, c.Stats().Set.Failure)
		assert.InDelta(t, 1.0, c.Stats().Set.SuccessRatio, 0.001)
		assert.InDelta(t, 0.0, c.Stats().Set.FailureRatio, 0.001)
	})

	t.Run("rejects empty key", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		err := c.set(context.Background(), "", testProfile("cust-1"), 60)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoKey)
		assert.Equal(t, 1, c.Stats().Set.Failure)
		assert.InDelta(t, 1.0, c.Stats().Set.FailureRatio, 0.001)
	})

	t.Run("rejects profile without customer id", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		err := c.set(context.Background(), "k1", loyalty.LoyaltyProfile{}, 60)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoData)
		assert.Equal(t, 1, c.Stats().Set.Failure)
	})

	t.Run("rejects invalid ttl", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		err := c.set(context.Background(), "k1", testProfile("cust-1"), 0)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidTTL)

		err = c.set(context.Background(), "k1", testProfile("cust-1"), -1)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidTTL)
		assert.Equal(t, 2, c.Stats().Set.Failure)
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.set(ctx, "k1", testProfile("cust-1"), 60)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, c.Stats().Set.Failure)
	})
}

func TestGet(t *testing.T) {
	t.Run("hit returns profile and updates lastAccessed", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		profile := testProfile("cust-1")
		require.NoError(t, c.set(context.Background(), "k1", profile, 60))

		rec, err := c.get(context.Background(), "k1")
		require.NoError(t, err)
		assert.Equal(t, "cust-1", rec.data.CustomerID)
		assert.False(t, rec.lastAccessed.IsZero())
		assert.Equal(t, 1, c.Stats().Hit)
		assert.Equal(t, 0, c.Stats().Miss)
		assert.InDelta(t, 1.0, c.Stats().HitRatio, 0.001)
		assert.InDelta(t, 0.0, c.Stats().MissRatio, 0.001)
		assert.Equal(t, 1, c.Stats().Get.Success)
	})

	t.Run("miss returns ErrNotFound", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		rec, err := c.get(context.Background(), "missing")
		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, record{}, rec)
		assert.Equal(t, 0, c.Stats().Hit)
		assert.Equal(t, 1, c.Stats().Miss)
		assert.InDelta(t, 0.0, c.Stats().HitRatio, 0.001)
		assert.InDelta(t, 1.0, c.Stats().MissRatio, 0.001)
		assert.Equal(t, 1, c.Stats().Get.Success)
	})

	t.Run("expired entry is not returned", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "k1", testProfile("cust-1"), 60))

		c.expireKey(t, "k1", time.Now().Add(-time.Second))

		got, err := c.get(context.Background(), "k1")
		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, record{}, got)
		assert.Equal(t, 1, c.Stats().Miss)
	})

	t.Run("rejects empty key as failure", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		_, err := c.get(context.Background(), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoKey)
		assert.Equal(t, 1, c.Stats().Get.Failure)
		assert.InDelta(t, 1.0, c.Stats().Get.FailureRatio, 0.001)
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := c.get(ctx, "k1")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, c.Stats().Get.Failure)
	})
}

func TestDelete(t *testing.T) {
	t.Run("removes existing key", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "k1", testProfile("cust-1"), 60))
		require.NoError(t, c.delete(context.Background(), "k1"))

		assert.False(t, c.hasKey(t, "k1"))
		assert.Equal(t, 1, c.Stats().Delete.Success)
	})

	t.Run("deleting missing key is a no-op success", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.delete(context.Background(), "missing"))
		assert.Equal(t, 1, c.Stats().Delete.Success)
	})

	t.Run("rejects empty key", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		err := c.delete(context.Background(), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoKey)
		assert.Equal(t, 1, c.Stats().Delete.Failure)
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.delete(ctx, "k1")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, c.Stats().Delete.Failure)
	})
}

func TestClear(t *testing.T) {
	t.Run("removes all entries", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "k1", testProfile("cust-1"), 60))
		require.NoError(t, c.set(context.Background(), "k2", testProfile("cust-2"), 60))
		require.NoError(t, c.clear(context.Background()))

		assert.Equal(t, 0, c.entryCount(t))
		assert.Equal(t, 1, c.Stats().Clear.Success)
	})

	t.Run("cancelled context does not clear data", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "k1", testProfile("cust-1"), 60))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.clear(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.True(t, c.hasKey(t, "k1"))
		assert.Equal(t, 1, c.Stats().Clear.Failure)
	})
}


func TestSize(t *testing.T) {
	t.Run("counts active entries only", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "k1", testProfile("cust-1"), 60))
		require.NoError(t, c.set(context.Background(), "k2", testProfile("cust-2"), 60))

		c.expireKey(t, "k2", time.Now().Add(-time.Second))

		n, err := c.size(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, 1, c.Stats().Size.Success)
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		n, err := c.size(ctx)
		assert.Equal(t, -1, n)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, c.Stats().Size.Failure)
	})
}

func TestKeys(t *testing.T) {
	t.Run("lists active keys", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "a", testProfile("cust-a"), 60))
		require.NoError(t, c.set(context.Background(), "b", testProfile("cust-b"), 60))

		c.expireKey(t, "b", time.Now().Add(-time.Second))

		keys, err := c.keys(context.Background())
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"a"}, keys)
		assert.Equal(t, 1, c.Stats().Keys.Success)
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		keys, err := c.keys(ctx)
		assert.Nil(t, keys)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, c.Stats().Keys.Failure)
	})
}

func TestRemoveExpiredEntries(t *testing.T) {
	t.Run("removes only expired", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "live", testProfile("live"), 60))
		require.NoError(t, c.set(context.Background(), "dead", testProfile("dead"), 60))

		c.expireKey(t, "dead", time.Now().Add(-time.Millisecond))

		require.NoError(t, c.removeExpiredEntries(context.Background()))
		assert.True(t, c.hasKey(t, "live"))
		assert.False(t, c.hasKey(t, "dead"))
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.removeExpiredEntries(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestLRUEviction(t *testing.T) {
	t.Run("evicts least recently used when over capacity", func(t *testing.T) {
		c, _ := newTestCache(t, 2)
		require.NoError(t, c.set(context.Background(), "a", testProfile("a"), 60))
		require.NoError(t, c.set(context.Background(), "b", testProfile("b"), 60))

		_, err := c.get(context.Background(), "a")
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
		_, err = c.get(context.Background(), "b")
		require.NoError(t, err)

		require.NoError(t, c.set(context.Background(), "c", testProfile("c"), 60))

		assert.False(t, c.hasKey(t, "a"))
		assert.True(t, c.hasKey(t, "b"))
		assert.True(t, c.hasKey(t, "c"))
		assert.Equal(t, 2, c.entryCount(t))
	})

	t.Run("fallback evicts earliest expiry when nothing accessed", func(t *testing.T) {
		c, _ := newTestCache(t, 2)
		require.NoError(t, c.set(context.Background(), "short", testProfile("short"), 60))
		require.NoError(t, c.set(context.Background(), "long", testProfile("long"), 60))

		c.expireKey(t, "short", time.Now().Add(5 * time.Second))
		c.expireKey(t, "long", time.Now().Add(30 * time.Second))

		require.NoError(t, c.set(context.Background(), "new", testProfile("new"), 60))

		assert.False(t, c.hasKey(t, "short"))
		assert.True(t, c.hasKey(t, "long"))
		assert.True(t, c.hasKey(t, "new"))
		assert.Equal(t, 2, c.entryCount(t))
	})

	t.Run("direct EvictIfNeeded respects cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 2)
		require.NoError(t, c.set(context.Background(), "a", testProfile("a"), 60))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.store.EvictIfNeeded(ctx, 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("direct EvictIfNeeded on empty cache is no-op", func(t *testing.T) {
		c, _ := newTestCache(t, 2)
		err := c.store.EvictIfNeeded(context.Background(), 2)
		require.NoError(t, err)
	})
}

func TestOpStatsRatios(t *testing.T) {
	t.Run("mixed set outcomes", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "ok", testProfile("ok"), 60))
		require.Error(t, c.set(context.Background(), "", testProfile("bad"), 60))
		require.Error(t, c.set(context.Background(), "x", loyalty.LoyaltyProfile{}, 60))

		stats := c.Stats()
		assert.Equal(t, 3, stats.Set.Attempted)
		assert.Equal(t, 1, stats.Set.Success)
		assert.Equal(t, 2, stats.Set.Failure)
		assert.InDelta(t, 1.0/3.0, stats.Set.SuccessRatio, 0.001)
		assert.InDelta(t, 2.0/3.0, stats.Set.FailureRatio, 0.001)
	})
}

func TestSetMany(t *testing.T) {
	t.Run("sets all valid entries", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		err := c.setMany(context.Background(), []BatchRequest{
			{Key: "a", Data: testProfile("a"), TTL_Seconds: 60},
			{Key: "b", Data: testProfile("b"), TTL_Seconds: 60},
		})
		require.NoError(t, err)
		assert.True(t, c.hasKey(t, "a"))
		assert.True(t, c.hasKey(t, "b"))
	})

	t.Run("continues on partial failure and joins errors", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		err := c.setMany(context.Background(), []BatchRequest{
			{Key: "a", Data: testProfile("a"), TTL_Seconds: 60},
			{Key: "", Data: testProfile("b"), TTL_Seconds: 60},
			{Key: "c", Data: testProfile("c"), TTL_Seconds: 60},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoKey)
		assert.True(t, c.hasKey(t, "a"))
		assert.True(t, c.hasKey(t, "c"))
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.setMany(ctx, []BatchRequest{{Key: "a", Data: testProfile("a"), TTL_Seconds: 60}})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestGetMany(t *testing.T) {
	t.Run("returns records for requested keys", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "a", testProfile("a"), 60))
		require.NoError(t, c.set(context.Background(), "b", testProfile("b"), 60))

		recs, err := c.getMany(context.Background(), []BatchRequest{
			{Key: "a"},
			{Key: "b"},
			{Key: "missing"},
		})
		require.NoError(t, err)
		require.Len(t, recs, 3)
		assert.Equal(t, "a", recs[0].data.CustomerID)
		assert.Equal(t, "b", recs[1].data.CustomerID)
		assert.Equal(t, "", recs[2].data.CustomerID)
	})

	t.Run("joins per-key errors", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		recs, err := c.getMany(context.Background(), []BatchRequest{
			{Key: ""},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoKey)
		require.Len(t, recs, 1)
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		recs, err := c.getMany(ctx, []BatchRequest{{Key: "a"}})
		assert.Nil(t, recs)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestDeleteMany(t *testing.T) {
	t.Run("deletes requested keys", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "a", testProfile("a"), 60))
		require.NoError(t, c.set(context.Background(), "b", testProfile("b"), 60))

		err := c.deleteMany(context.Background(), []BatchRequest{
			{Key: "a"},
			{Key: "b"},
		})
		require.NoError(t, err)
		assert.Equal(t, 0, c.entryCount(t))
	})

	t.Run("continues on partial failure", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "a", testProfile("a"), 60))

		err := c.deleteMany(context.Background(), []BatchRequest{
			{Key: "a"},
			{Key: ""},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoKey)
		assert.False(t, c.hasKey(t, "a"))
	})

	t.Run("cancelled context", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.deleteMany(ctx, []BatchRequest{{Key: "a"}})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestAutomaticCleanup(t *testing.T) {
	t.Run("removes expired entries then stops on cancel", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		require.NoError(t, c.set(context.Background(), "live", testProfile("live"), 60))
		require.NoError(t, c.set(context.Background(), "dead", testProfile("dead"), 60))

		c.expireKey(t, "dead", time.Now().Add(-time.Second))

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- c.automaticCleanup(ctx, 1)
		}()

		require.Eventually(t, func() bool {
			return !c.hasKey(t, "dead") && c.hasKey(t, "live") && c.Stats().Cleanup.Success >= 1
		}, 2*time.Second, 20*time.Millisecond)

		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("automaticCleanup did not stop after cancel")
		}
	})

	t.Run("cancelled context before start", func(t *testing.T) {
		c, _ := newTestCache(t, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.automaticCleanup(ctx, 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, c.Stats().Cleanup.Failure)
	})
}

func TestRun(t *testing.T) {
	c, logger := newTestCache(t, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, c)
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not finish")
	}

	assert.Equal(t, 0, c.entryCount(t))
	assert.NotEmpty(t, logger.infos)
}

func TestCacheInterfaceSatisfaction(t *testing.T) {
	c, _ := newTestCache(t, 10)
	var _ Cache = c
}

func TestConcurrentAccess(t *testing.T) {
	c, _ := newTestCache(t, 50)
	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('A'+(i%26))) + string(rune('0'+i%10))
			profile := testProfile(key)
			if err := c.set(context.Background(), key, profile, 60); err != nil {
				errCh <- err
				return
			}
			if _, err := c.get(context.Background(), key); err != nil {
				errCh <- err
			}
			if _, err := c.size(context.Background()); err != nil {
				errCh <- err
			}
			if _, err := c.keys(context.Background()); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	assert.Empty(t, errs, "unexpected concurrent errors: %v", errors.Join(errs...))
}

func TestConcurrentAccessRaceDetector(t *testing.T) {
	c, _ := newTestCache(t, 20)
	ctx := context.Background()
	var wg sync.WaitGroup
	errCh := make(chan error, 500)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + (i % 20)))
			if err := c.set(ctx, key, testProfile(key), 60); err != nil {
				errCh <- err
			}
			if _, err := c.get(ctx, key); err != nil && !errors.Is(err, ErrNotFound) {
				errCh <- err
			}
			if err := c.delete(ctx, key); err != nil {
				errCh <- err
			}
			if _, err := c.size(ctx); err != nil {
				errCh <- err
			}
			if err := c.clear(ctx); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	assert.Empty(t, errs, "unexpected concurrent errors: %v", errors.Join(errs...))
}
