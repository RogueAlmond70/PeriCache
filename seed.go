package main

import (
	"PeriCache/cache"
	"PeriCache/loyalty"
	"context"
	"fmt"
	"time"
)

// seedTTLSeconds is long enough for local API exploration without entries expiring mid-session.
const seedTTLSeconds = 3600

// seedEntries are loaded into the cache on startup. Keys match customer_id.
var seedEntries = []struct {
	Key     string
	Profile loyalty.LoyaltyProfile
}{
	{
		Key: "cust-001",
		Profile: seedProfile(
			"cust-001", "Alex", "Flame", "alex@example.com", "active",
			"Hot", 120, 400,
			loyalty.Reward{RewardID: "r-chips", Name: "Free PERi-PERi Chips", PointsRequired: 50},
		),
	},
	{
		Key: "cust-002",
		Profile: seedProfile(
			"cust-002", "Sam", "Extra", "sam@example.com", "active",
			"Extra Hot", 340, 1200,
			loyalty.Reward{RewardID: "r-butterfly", Name: "Free Butterfly Chicken", PointsRequired: 200},
		),
	},
	{
		Key: "cust-003",
		Profile: seedProfile(
			"cust-003", "Jordan", "Lemon", "jordan@example.com", "active",
			"Lemon & Herb", 75, 180,
			loyalty.Reward{RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30},
		),
	},
	{
		Key: "cust-004",
		Profile: seedProfile(
			"cust-004", "Riley", "Mild", "riley@example.com", "inactive",
			"Mild", 10, 10,
		),
	},
}

func seedCache(ctx context.Context, c cache.Cache) error {
	batch := make([]cache.BatchRequest, 0, len(seedEntries))
	for _, entry := range seedEntries {
		batch = append(batch, cache.BatchRequest{
			Key:         entry.Key,
			Data:        entry.Profile,
			TTL_Seconds: seedTTLSeconds,
		})
	}
	if err := c.SetMany(ctx, batch); err != nil {
		return fmt.Errorf("seed cache: %w", err)
	}
	return nil
}

func seedProfile(id, first, last, email, status, tier string, points, lifetime int, rewards ...loyalty.Reward) loyalty.LoyaltyProfile {
	now := time.Now().UTC()
	if rewards == nil {
		rewards = []loyalty.Reward{}
	}
	return loyalty.LoyaltyProfile{
		CustomerID:    id,
		FirstName:     first,
		LastName:      last,
		Email:         email,
		AccountStatus: status,
		CreatedAt:     now,
		LastActivity:  now,
		Loyalty: loyalty.Loyalty{
			Tier:             tier,
			PointsBalance:    points,
			LifetimePoints:   lifetime,
			RewardsAvailable: rewards,
		},
	}
}
