package cache

import (
	"Nandos-Pair-Coding/loyalty"
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type mongoStore struct {
	client     *mongo.Client
	collection *mongo.Collection
}

type mongoDocument struct {
	Key          string                 `bson:"_id"`
	Data         loyalty.LoyaltyProfile `bson:"data"`
	TTL          int                    `bson:"ttl"`
	Expiry       time.Time              `bson:"expiry"`
	LastAccessed time.Time              `bson:"last_accessed"`
}

func newMongoStore(ctx context.Context, uri, database, collection string) (*mongoStore, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		if closeErr := client.Disconnect(ctx); closeErr != nil {
			return nil, fmt.Errorf("ping mongo: %w (also failed to disconnect: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	coll := client.Database(database).Collection(collection)
	store := &mongoStore{client: client, collection: coll}
	if err := store.ensureIndexes(ctx); err != nil {
		if closeErr := client.Disconnect(ctx); closeErr != nil {
			return nil, fmt.Errorf("%w (also failed to disconnect: %v)", err, closeErr)
		}
		return nil, err
	}
	return store, nil
}

func (m *mongoStore) ensureIndexes(ctx context.Context) error {
	// TTL index: Mongo removes docs when expiry is in the past.
	_, err := m.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiry", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return fmt.Errorf("create expiry ttl index: %w", err)
	}
	_, err = m.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "last_accessed", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("create last_accessed index: %w", err)
	}
	return nil
}

func (m *mongoStore) Put(ctx context.Context, key string, rec record) error {
	doc := mongoDocument{
		Key:          key,
		Data:         rec.data,
		TTL:          rec.ttl,
		Expiry:       rec.expiry,
		LastAccessed: rec.lastAccessed,
	}
	opts := options.Replace().SetUpsert(true)
	_, err := m.collection.ReplaceOne(ctx, bson.M{"_id": key}, doc, opts)
	if err != nil {
		return fmt.Errorf("mongo put: %w", err)
	}
	return nil
}

func (m *mongoStore) Get(ctx context.Context, key string) (record, bool, error) {
	var doc mongoDocument
	err := m.collection.FindOne(ctx, bson.M{"_id": key}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return record{}, false, nil
		}
		return record{}, false, fmt.Errorf("mongo get: %w", err)
	}
	if time.Now().After(doc.Expiry) {
		if _, err := m.collection.DeleteOne(ctx, bson.M{"_id": key}); err != nil {
			return record{}, false, fmt.Errorf("mongo delete expired: %w", err)
		}
		return record{}, false, nil
	}
	return record{
		data:         doc.Data,
		ttl:          doc.TTL,
		expiry:       doc.Expiry,
		lastAccessed: doc.LastAccessed,
	}, true, nil
}

func (m *mongoStore) Delete(ctx context.Context, key string) error {
	_, err := m.collection.DeleteOne(ctx, bson.M{"_id": key})
	if err != nil {
		return fmt.Errorf("mongo delete: %w", err)
	}
	return nil
}

func (m *mongoStore) Clear(ctx context.Context) error {
	_, err := m.collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("mongo clear: %w", err)
	}
	return nil
}

func (m *mongoStore) Keys(ctx context.Context) ([]string, error) {
	if _, err := m.DeleteExpired(ctx); err != nil {
		return nil, err
	}
	cursor, err := m.collection.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, fmt.Errorf("mongo keys: %w", err)
	}
	defer cursor.Close(ctx)

	var keys []string
	for cursor.Next(ctx) {
		var doc struct {
			Key string `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("mongo keys decode: %w", err)
		}
		keys = append(keys, doc.Key)
	}
	return keys, cursor.Err()
}

func (m *mongoStore) Size(ctx context.Context) (int, error) {
	if _, err := m.DeleteExpired(ctx); err != nil {
		return 0, err
	}
	n, err := m.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("mongo size: %w", err)
	}
	return int(n), nil
}

func (m *mongoStore) DeleteExpired(ctx context.Context) (int, error) {
	res, err := m.collection.DeleteMany(ctx, bson.M{"expiry": bson.M{"$lte": time.Now().UTC()}})
	if err != nil {
		return 0, fmt.Errorf("mongo delete expired: %w", err)
	}
	return int(res.DeletedCount), nil
}

func (m *mongoStore) EvictIfNeeded(ctx context.Context, maxSize int) error {
	for {
		n, err := m.collection.CountDocuments(ctx, bson.M{})
		if err != nil {
			return fmt.Errorf("mongo evict count: %w", err)
		}
		if int(n) <= maxSize {
			return nil
		}
		if err := m.evictOne(ctx); err != nil {
			return err
		}
	}
}

func (m *mongoStore) evictOne(ctx context.Context) error {
	// Prefer least-recently-accessed among documents that have been accessed.
	accessedFilter := bson.M{"last_accessed": bson.M{"$gt": time.Time{}}}
	opts := options.FindOneAndDelete().SetSort(bson.D{{Key: "last_accessed", Value: 1}})
	err := m.collection.FindOneAndDelete(ctx, accessedFilter, opts).Err()
	if err == nil {
		return nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("mongo lru evict: %w", err)
	}

	// Fallback: earliest expiry among never-accessed / remaining docs.
	opts = options.FindOneAndDelete().SetSort(bson.D{{Key: "expiry", Value: 1}})
	err = m.collection.FindOneAndDelete(ctx, bson.M{}, opts).Err()
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("mongo expiry evict: %w", err)
	}
	return nil
}

func (m *mongoStore) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}
