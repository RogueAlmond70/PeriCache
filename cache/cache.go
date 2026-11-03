package cache

import (
	"PeriCache/internal"
	"PeriCache/loyalty"
	"PeriCache/metrics"
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNoKey      = errors.New("failed to complete operation. Key not provided")
	ErrNoData     = errors.New("failed to complete operation. No data provided")
	ErrInvalidTTL = errors.New("failed to complete operation. Invalid TTL")
	ErrNotFound   = errors.New("cache entry not found")
)

const defaultCleanupIntervalSeconds = 30

// Cache is the exported service API used by HTTP handlers and callers outside this package.
type Cache interface {
	Set(ctx context.Context, key string, data loyalty.LoyaltyProfile, ttlSeconds int) error
	Get(ctx context.Context, key string) (loyalty.LoyaltyProfile, error)
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
	Size(ctx context.Context) (int, error)
	Keys(ctx context.Context) ([]string, error)
	SetMany(ctx context.Context, batch []BatchRequest) error
	GetMany(ctx context.Context, batch []BatchRequest) ([]loyalty.LoyaltyProfile, error)
	DeleteMany(ctx context.Context, batch []BatchRequest) error
	Stats() StatsSnapshot
	StartCleanup(ctx context.Context, cleanupIntervalSeconds int) <-chan struct{}
}

// StatsSnapshot is the JSON-facing stats view (backed by the metrics recorder).
type StatsSnapshot = metrics.Snapshot

// OperationStats is the per-operation stats view.
type OperationStats = metrics.OperationStats

// BatchRequest is an exported batch item for set/get/delete many.
type BatchRequest struct {
	Key         string                 `json:"key"`
	Data        loyalty.LoyaltyProfile `json:"data"`
	TTL_Seconds int                    `json:"ttl_seconds"`
}

type record struct {
	data         loyalty.LoyaltyProfile
	ttl          int
	expiry       time.Time
	lastAccessed time.Time
}

type ourCache struct {
	logger  internal.Logger
	store   Store
	metrics metrics.Recorder
	maxSize int
}

func (c *ourCache) recordSuccess(op metrics.Operation) {
	c.metrics.RecordOperation(op, metrics.OutcomeSuccess)
}

func (c *ourCache) recordFailure(op metrics.Operation) {
	c.metrics.RecordOperation(op, metrics.OutcomeFailure)
}

// Stats returns a snapshot when the metrics backend supports it.
func (c *ourCache) Stats() StatsSnapshot {
	if snap, ok := metrics.SnapshotOf(c.metrics); ok {
		return snap
	}
	return StatsSnapshot{}
}

func (c *ourCache) Set(ctx context.Context, key string, data loyalty.LoyaltyProfile, ttlSeconds int) error {
	return c.set(ctx, key, data, ttlSeconds)
}

func (c *ourCache) Get(ctx context.Context, key string) (loyalty.LoyaltyProfile, error) {
	rec, err := c.get(ctx, key)
	if err != nil {
		return loyalty.LoyaltyProfile{}, err
	}
	return rec.data, nil
}

func (c *ourCache) Delete(ctx context.Context, key string) error {
	return c.delete(ctx, key)
}

func (c *ourCache) Clear(ctx context.Context) error {
	return c.clear(ctx)
}

func (c *ourCache) Size(ctx context.Context) (int, error) {
	return c.size(ctx)
}

func (c *ourCache) Keys(ctx context.Context) ([]string, error) {
	return c.keys(ctx)
}

func (c *ourCache) SetMany(ctx context.Context, batch []BatchRequest) error {
	return c.setMany(ctx, batch)
}

func (c *ourCache) GetMany(ctx context.Context, batch []BatchRequest) ([]loyalty.LoyaltyProfile, error) {
	recs, err := c.getMany(ctx, batch)
	out := make([]loyalty.LoyaltyProfile, len(recs))
	for i, rec := range recs {
		out[i] = rec.data
	}
	return out, err
}

func (c *ourCache) DeleteMany(ctx context.Context, batch []BatchRequest) error {
	return c.deleteMany(ctx, batch)
}

func (c *ourCache) StartCleanup(ctx context.Context, cleanupIntervalSeconds int) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := c.automaticCleanup(ctx, cleanupIntervalSeconds); err != nil {
			c.logger.Error("automatic cleanup stopped with error", "error", err)
		}
	}()
	return done
}

// NewCache creates a cache. nil store defaults to memory; nil rec defaults to memory metrics.
func NewCache(ctx context.Context, logger internal.Logger, maxSize int, rec metrics.Recorder, store Store) (*ourCache, error) {
	if err := ctx.Err(); err != nil {
		logger.Error("failed to create new cache", "error", err)
		return nil, fmt.Errorf("failed to create new cache %w", err)
	}
	if maxSize < 1 {
		maxSize = 100
	}
	if rec == nil {
		rec = metrics.NewMemoryRecorder()
	}
	if store == nil {
		store = NewMemoryStore()
	}
	logger.Info("New Cache created")
	return &ourCache{
		logger:  logger,
		store:   store,
		metrics: rec,
		maxSize: maxSize,
	}, nil
}

func (c *ourCache) keys(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		c.recordFailure(metrics.OpKeys)
		c.logger.Error("failed to retrieve keys", "error", err)
		return nil, fmt.Errorf("failed to retrieve keys %w", err)
	}

	ourKeys, err := c.store.Keys(ctx)
	if err != nil {
		c.recordFailure(metrics.OpKeys)
		c.logger.Error("failed to retrieve keys", "error", err)
		return nil, fmt.Errorf("failed to retrieve keys %w", err)
	}

	c.recordSuccess(metrics.OpKeys)
	c.logger.Info("successfully retrieved keys", "count of keys", len(ourKeys))
	return ourKeys, nil
}

func (c *ourCache) set(ctx context.Context, key string, data loyalty.LoyaltyProfile, ttlSeconds int) error {
	if err := ctx.Err(); err != nil {
		c.recordFailure(metrics.OpSet)
		c.logger.Error("failed to set cache", "key", key, "error", err)
		return fmt.Errorf("failed to set cache %w", err)
	}
	if key == "" {
		c.recordFailure(metrics.OpSet)
		c.logger.Error("failed to set cache", "key", key, "error", ErrNoKey)
		return fmt.Errorf("failed to set cache: %w", ErrNoKey)
	}
	if data.CustomerID == "" {
		c.recordFailure(metrics.OpSet)
		c.logger.Error("failed to set cache", "key", key, "error", ErrNoData)
		return fmt.Errorf("failed to set cache %w", ErrNoData)
	}
	if ttlSeconds < 1 {
		c.recordFailure(metrics.OpSet)
		c.logger.Error("failed to set cache", "key", key, "error", ErrInvalidTTL)
		return fmt.Errorf("failed to set cache %w", ErrInvalidTTL)
	}

	rec := record{
		data:         data,
		ttl:          ttlSeconds,
		expiry:       time.Now().Add(time.Duration(ttlSeconds) * time.Second),
		lastAccessed: time.Time{},
	}
	if err := c.store.Put(ctx, key, rec); err != nil {
		c.recordFailure(metrics.OpSet)
		c.logger.Error("failed to set cache", "key", key, "error", err)
		return fmt.Errorf("failed to set cache %w", err)
	}
	c.logger.Info("cache set", "key", key, "ttl", ttlSeconds)

	if err := c.store.EvictIfNeeded(ctx, c.maxSize); err != nil {
		c.recordFailure(metrics.OpSet)
		c.logger.Error("failed to complete housekeeping after set operation", "error", err)
		return fmt.Errorf("failed to complete housekeeping after set operation %w", err)
	}

	c.recordSuccess(metrics.OpSet)
	return nil
}

func (c *ourCache) get(ctx context.Context, key string) (record, error) {
	if err := ctx.Err(); err != nil {
		c.recordFailure(metrics.OpGet)
		c.logger.Error("failed to complete get request", "error", err)
		return record{}, fmt.Errorf("failed to complete get request %w", err)
	}
	if key == "" {
		c.recordFailure(metrics.OpGet)
		c.logger.Error("failed to complete get request", "error", ErrNoKey)
		return record{}, fmt.Errorf("failed to complete get request %w", ErrNoKey)
	}

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		c.recordFailure(metrics.OpGet)
		c.logger.Error("failed to complete get request", "error", err)
		return record{}, fmt.Errorf("failed to complete get request %w", err)
	}
	if !ok {
		c.metrics.RecordGetResult(metrics.ResultMiss)
		c.recordSuccess(metrics.OpGet)
		return record{}, ErrNotFound
	}

	updatedRecord := record{
		data:         value.data,
		ttl:          value.ttl,
		expiry:       value.expiry,
		lastAccessed: time.Now(),
	}
	if err := c.store.Put(ctx, key, updatedRecord); err != nil {
		c.recordFailure(metrics.OpGet)
		c.logger.Error("failed to complete get request", "error", err)
		return record{}, fmt.Errorf("failed to complete get request %w", err)
	}

	c.metrics.RecordGetResult(metrics.ResultHit)
	c.recordSuccess(metrics.OpGet)
	return updatedRecord, nil
}

func (c *ourCache) delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		c.recordFailure(metrics.OpDelete)
		c.logger.Error("failed to complete delete operation", "error", err)
		return fmt.Errorf("failed to complete delete operation %w", err)
	}
	if key == "" {
		c.recordFailure(metrics.OpDelete)
		c.logger.Error("failed to complete delete operation", "error", ErrNoKey)
		return fmt.Errorf("failed to complete delete operation %w", ErrNoKey)
	}

	if err := c.store.Delete(ctx, key); err != nil {
		c.recordFailure(metrics.OpDelete)
		c.logger.Error("failed to complete delete operation", "error", err)
		return fmt.Errorf("failed to complete delete operation %w", err)
	}
	c.recordSuccess(metrics.OpDelete)
	return nil
}

func (c *ourCache) clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		c.recordFailure(metrics.OpClear)
		c.logger.Error("failed to complete clear operation", "error", err)
		return fmt.Errorf("failed to complete clear operation %w", err)
	}

	if err := c.store.Clear(ctx); err != nil {
		c.recordFailure(metrics.OpClear)
		c.logger.Error("failed to complete clear operation", "error", err)
		return fmt.Errorf("failed to complete clear operation %w", err)
	}
	c.recordSuccess(metrics.OpClear)
	return nil
}

func (c *ourCache) size(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		c.recordFailure(metrics.OpSize)
		c.logger.Error("failed to complete size operation", "error", err)
		return -1, fmt.Errorf("failed to complete size operation %w", err)
	}

	n, err := c.store.Size(ctx)
	if err != nil {
		c.recordFailure(metrics.OpSize)
		c.logger.Error("failed to complete size operation", "error", err)
		return -1, fmt.Errorf("failed to complete size operation %w", err)
	}
	c.recordSuccess(metrics.OpSize)
	return n, nil
}

func (c *ourCache) automaticCleanup(ctx context.Context, cleanupIntervalSeconds int) error {
	if err := ctx.Err(); err != nil {
		c.recordFailure(metrics.OpCleanup)
		c.logger.Error("failed to start automatic cleanup", "error", err)
		return fmt.Errorf("failed to start automatic cleanup %w", err)
	}

	if cleanupIntervalSeconds < 1 {
		cleanupIntervalSeconds = defaultCleanupIntervalSeconds
	}

	ticker := time.NewTicker(time.Duration(cleanupIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("shutting down automatic cleanup")
			return nil
		default:
		}

		if _, err := c.store.DeleteExpired(ctx); err != nil {
			c.recordFailure(metrics.OpCleanup)
			c.logger.Error("failed to run automatic cleanup", "error", err)
		} else {
			c.recordSuccess(metrics.OpCleanup)
		}

		select {
		case <-ctx.Done():
			c.logger.Info("shutting down automatic cleanup")
			return nil
		case <-ticker.C:
		}
	}
}

func (c *ourCache) removeExpiredEntries(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		c.logger.Error("failed to remove expired entries", "error", err)
		return fmt.Errorf("failed to remove expired entries %w", err)
	}
	_, err := c.store.DeleteExpired(ctx)
	return err
}

func (c *ourCache) setMany(ctx context.Context, batch []BatchRequest) error {
	if err := ctx.Err(); err != nil {
		c.logger.Error("failed to complete set many request", "error", err)
		return fmt.Errorf("failed to complete set many request %w", err)
	}

	var returnErrors []error
	for _, request := range batch {
		if err := c.set(ctx, request.Key, request.Data, request.TTL_Seconds); err != nil {
			c.logger.Error("failed to complete set many request for key", "key", request.Key, "error", err)
			returnErrors = append(returnErrors, err)
		}
	}
	return errors.Join(returnErrors...)
}

func (c *ourCache) getMany(ctx context.Context, batch []BatchRequest) ([]record, error) {
	if err := ctx.Err(); err != nil {
		c.logger.Error("failed to complete get many request", "error", err)
		return nil, fmt.Errorf("failed to complete get many request %w", err)
	}

	var returnErrors []error
	returnData := make([]record, 0, len(batch))
	for _, request := range batch {
		data, err := c.get(ctx, request.Key)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				returnData = append(returnData, record{})
				continue
			}
			c.logger.Error("failed to complete get many request for key", "key", request.Key, "error", err)
			returnErrors = append(returnErrors, err)
			returnData = append(returnData, record{})
			continue
		}
		returnData = append(returnData, data)
	}
	return returnData, errors.Join(returnErrors...)
}

func (c *ourCache) deleteMany(ctx context.Context, batch []BatchRequest) error {
	if err := ctx.Err(); err != nil {
		c.logger.Error("failed to complete delete many request", "error", err)
		return fmt.Errorf("failed to complete delete many request %w", err)
	}

	var returnErrors []error
	for _, request := range batch {
		if err := c.delete(ctx, request.Key); err != nil {
			c.logger.Error("failed to complete delete many request for key", "key", request.Key, "error", err)
			returnErrors = append(returnErrors, err)
		}
	}
	return errors.Join(returnErrors...)
}

// memoryStoreOrNil returns the underlying memory store when used (tests).
func (c *ourCache) memoryStoreOrNil() *memoryStore {
	ms, ok := c.store.(*memoryStore)
	if !ok {
		return nil
	}
	return ms
}
