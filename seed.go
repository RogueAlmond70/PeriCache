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

type seedSpec struct {
	ID, First, Last, Email, Status, Tier string
	Points, Lifetime                     int
	Rewards                              []loyalty.Reward
}

// seedSpecs defines the 40 profiles loaded on startup (keys cust-001 … cust-040).
var seedSpecs = []seedSpec{
	{"cust-001", "Alex", "Flame", "alex.flame@example.com", "active", "Hot", 120, 400, []loyalty.Reward{{RewardID: "r-chips", Name: "Free PERi-PERi Chips", PointsRequired: 50}}},
	{"cust-002", "Sam", "Extra", "sam.extra@example.com", "active", "Extra Hot", 340, 1200, []loyalty.Reward{{RewardID: "r-butterfly", Name: "Free Butterfly Chicken", PointsRequired: 200}}},
	{"cust-003", "Jordan", "Lemon", "jordan.lemon@example.com", "active", "Lemon & Herb", 75, 180, []loyalty.Reward{{RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30}}},
	{"cust-004", "Riley", "Mild", "riley.mild@example.com", "inactive", "Mild", 10, 10, nil},
	{"cust-005", "Casey", "Spice", "casey.spice@example.com", "active", "Hot", 210, 650, []loyalty.Reward{{RewardID: "r-wings", Name: "Free 4 Wings", PointsRequired: 80}}},
	{"cust-006", "Morgan", "Grill", "morgan.grill@example.com", "active", "Extra Hot", 455, 2100, []loyalty.Reward{{RewardID: "r-burger", Name: "Free Chicken Burger", PointsRequired: 250}}},
	{"cust-007", "Avery", "Herb", "avery.herb@example.com", "active", "Lemon & Herb", 95, 320, []loyalty.Reward{{RewardID: "r-chips", Name: "Free PERi-PERi Chips", PointsRequired: 50}}},
	{"cust-008", "Quinn", "Cool", "quinn.cool@example.com", "active", "Mild", 40, 90, nil},
	{"cust-009", "Reese", "Blaze", "reese.blaze@example.com", "active", "Hot", 180, 540, []loyalty.Reward{{RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30}, {RewardID: "r-chips", Name: "Free PERi-PERi Chips", PointsRequired: 50}}},
	{"cust-010", "Skyler", "Inferno", "skyler.inferno@example.com", "active", "Extra Hot", 600, 3000, []loyalty.Reward{{RewardID: "r-platter", Name: "Free Sharing Platter", PointsRequired: 500}}},
	{"cust-011", "Jamie", "Citrus", "jamie.citrus@example.com", "active", "Lemon & Herb", 130, 410, []loyalty.Reward{{RewardID: "r-wings", Name: "Free 4 Wings", PointsRequired: 80}}},
	{"cust-012", "Taylor", "Soft", "taylor.soft@example.com", "suspended", "Mild", 5, 5, nil},
	{"cust-013", "Drew", "Pepper", "drew.pepper@example.com", "active", "Hot", 155, 480, []loyalty.Reward{{RewardID: "r-chips", Name: "Free PERi-PERi Chips", PointsRequired: 50}}},
	{"cust-014", "Cameron", "Volcano", "cameron.volcano@example.com", "active", "Extra Hot", 290, 980, []loyalty.Reward{{RewardID: "r-butterfly", Name: "Free Butterfly Chicken", PointsRequired: 200}}},
	{"cust-015", "Parker", "Zest", "parker.zest@example.com", "active", "Lemon & Herb", 88, 220, []loyalty.Reward{{RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30}}},
	{"cust-016", "Finley", "Gentle", "finley.gentle@example.com", "active", "Mild", 55, 140, nil},
	{"cust-017", "Hayden", "Chili", "hayden.chili@example.com", "active", "Hot", 200, 700, []loyalty.Reward{{RewardID: "r-wings", Name: "Free 4 Wings", PointsRequired: 80}}},
	{"cust-018", "Rowan", "Magma", "rowan.magma@example.com", "active", "Extra Hot", 375, 1500, []loyalty.Reward{{RewardID: "r-burger", Name: "Free Chicken Burger", PointsRequired: 250}}},
	{"cust-019", "Sage", "Lime", "sage.lime@example.com", "inactive", "Lemon & Herb", 20, 60, nil},
	{"cust-020", "Blake", "Warm", "blake.warm@example.com", "active", "Mild", 70, 190, []loyalty.Reward{{RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30}}},
	{"cust-021", "Emerson", "Scorch", "emerson.scorch@example.com", "active", "Hot", 145, 430, []loyalty.Reward{{RewardID: "r-chips", Name: "Free PERi-PERi Chips", PointsRequired: 50}}},
	{"cust-022", "Kendall", "Phoenix", "kendall.phoenix@example.com", "active", "Extra Hot", 520, 2400, []loyalty.Reward{{RewardID: "r-platter", Name: "Free Sharing Platter", PointsRequired: 500}}},
	{"cust-023", "Logan", "Basil", "logan.basil@example.com", "active", "Lemon & Herb", 110, 350, []loyalty.Reward{{RewardID: "r-wings", Name: "Free 4 Wings", PointsRequired: 80}}},
	{"cust-024", "Peyton", "Calm", "peyton.calm@example.com", "active", "Mild", 35, 80, nil},
	{"cust-025", "River", "Ember", "river.ember@example.com", "active", "Hot", 260, 820, []loyalty.Reward{{RewardID: "r-butterfly", Name: "Free Butterfly Chicken", PointsRequired: 200}}},
	{"cust-026", "Dakota", "Lava", "dakota.lava@example.com", "active", "Extra Hot", 410, 1750, []loyalty.Reward{{RewardID: "r-burger", Name: "Free Chicken Burger", PointsRequired: 250}}},
	{"cust-027", "Charlie", "Thyme", "charlie.thyme@example.com", "active", "Lemon & Herb", 65, 160, []loyalty.Reward{{RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30}}},
	{"cust-028", "Harper", "Mellow", "harper.mellow@example.com", "inactive", "Mild", 15, 40, nil},
	{"cust-029", "Jordan", "Sizzle", "jordan.sizzle@example.com", "active", "Hot", 175, 560, []loyalty.Reward{{RewardID: "r-chips", Name: "Free PERi-PERi Chips", PointsRequired: 50}}},
	{"cust-030", "Alexis", "Torch", "alexis.torch@example.com", "active", "Extra Hot", 330, 1100, []loyalty.Reward{{RewardID: "r-wings", Name: "Free 4 Wings", PointsRequired: 80}}},
	{"cust-031", "Elliot", "Tang", "elliot.tang@example.com", "active", "Lemon & Herb", 100, 300, []loyalty.Reward{{RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30}}},
	{"cust-032", "Frankie", "Softly", "frankie.softly@example.com", "suspended", "Mild", 8, 12, nil},
	{"cust-033", "Gray", "Roast", "gray.roast@example.com", "active", "Hot", 190, 610, []loyalty.Reward{{RewardID: "r-chips", Name: "Free PERi-PERi Chips", PointsRequired: 50}, {RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30}}},
	{"cust-034", "Hunter", "Wildfire", "hunter.wildfire@example.com", "active", "Extra Hot", 480, 2000, []loyalty.Reward{{RewardID: "r-platter", Name: "Free Sharing Platter", PointsRequired: 500}}},
	{"cust-035", "Indigo", "Marinade", "indigo.marinade@example.com", "active", "Lemon & Herb", 125, 390, []loyalty.Reward{{RewardID: "r-wings", Name: "Free 4 Wings", PointsRequired: 80}}},
	{"cust-036", "Jules", "Breezy", "jules.breezy@example.com", "active", "Mild", 45, 110, nil},
	{"cust-037", "Kai", "Char", "kai.char@example.com", "active", "Hot", 225, 750, []loyalty.Reward{{RewardID: "r-butterfly", Name: "Free Butterfly Chicken", PointsRequired: 200}}},
	{"cust-038", "Lane", "Ash", "lane.ash@example.com", "active", "Extra Hot", 305, 1050, []loyalty.Reward{{RewardID: "r-burger", Name: "Free Chicken Burger", PointsRequired: 250}}},
	{"cust-039", "Micah", "Garden", "micah.garden@example.com", "active", "Lemon & Herb", 85, 240, []loyalty.Reward{{RewardID: "r-drink", Name: "Free Soft Drink", PointsRequired: 30}}},
	{"cust-040", "Noel", "Plain", "noel.plain@example.com", "inactive", "Mild", 12, 25, nil},
}

// seedEntries is derived from seedSpecs for startup loading and logging.
var seedEntries []struct {
	Key     string
	Profile loyalty.LoyaltyProfile
}

func init() {
	seedEntries = make([]struct {
		Key     string
		Profile loyalty.LoyaltyProfile
	}, 0, len(seedSpecs))
	for _, s := range seedSpecs {
		seedEntries = append(seedEntries, struct {
			Key     string
			Profile loyalty.LoyaltyProfile
		}{
			Key:     s.ID,
			Profile: seedProfile(s.ID, s.First, s.Last, s.Email, s.Status, s.Tier, s.Points, s.Lifetime, s.Rewards...),
		})
	}
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
