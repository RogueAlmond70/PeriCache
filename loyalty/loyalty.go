package loyalty

import "time"

type LoyaltyProfile struct {
	CustomerID   string    `json:"customer_id"`
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	Email        string    `json:"email"`
	Loyalty      Loyalty   `json:"loyalty"`
	AccountStatus string   `json:"account_status"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
}

type Loyalty struct {
	Tier             string   `json:"tier"`
	PointsBalance    int      `json:"points_balance"`
	LifetimePoints   int      `json:"lifetime_points"`
	RewardsAvailable []Reward `json:"rewards_available"`
}

type Reward struct {
	RewardID       string `json:"reward_id"`
	Name           string `json:"name"`
	PointsRequired int    `json:"points_required"`
}