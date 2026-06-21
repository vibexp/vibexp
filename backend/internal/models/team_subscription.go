package models

import "time"

// TeamSubscription represents a team's subscription to VibeXP
type TeamSubscription struct {
	ID                   string     `json:"id" db:"id"`
	TeamID               string     `json:"team_id" db:"team_id"`
	StripeSubscriptionID string     `json:"stripe_subscription_id" db:"stripe_subscription_id"`
	StripeCustomerID     string     `json:"stripe_customer_id" db:"stripe_customer_id"`
	Tier                 string     `json:"tier" db:"tier"`
	SeatCount            int        `json:"seat_count" db:"seat_count"`
	Status               string     `json:"status" db:"status"`
	BillingInterval      string     `json:"billing_interval" db:"billing_interval"`
	CurrentPeriodStart   time.Time  `json:"current_period_start" db:"current_period_start"`
	CurrentPeriodEnd     time.Time  `json:"current_period_end" db:"current_period_end"`
	TrialEnd             *time.Time `json:"trial_end,omitempty" db:"trial_end"`
	CanceledAt           *time.Time `json:"canceled_at,omitempty" db:"canceled_at"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

// Team subscription tier constants
const (
	TeamTierStarter      = "starter"
	TeamTierProfessional = "professional"
	TeamTierEnterprise   = "enterprise"
)

// Team subscription status constants
const (
	TeamSubscriptionStatusIncomplete        = "incomplete"         // Payment required to activate (Stripe initial state)
	TeamSubscriptionStatusIncompleteExpired = "incomplete_expired" // First payment not made in time
	TeamSubscriptionStatusTrialing          = "trialing"
	TeamSubscriptionStatusActive            = "active"
	TeamSubscriptionStatusPastDue           = "past_due"
	TeamSubscriptionStatusCanceled          = "canceled"
	TeamSubscriptionStatusUnpaid            = "unpaid"
)

// Team subscription billing interval constants
const (
	BillingIntervalMonth = "month"
	BillingIntervalYear  = "year"
)

// CreateTeamSubscriptionRequest represents the request to create a team subscription
type CreateTeamSubscriptionRequest struct {
	TeamID               string     `json:"team_id" validate:"required,uuid"`
	StripeSubscriptionID string     `json:"stripe_subscription_id" validate:"required"`
	StripeCustomerID     string     `json:"stripe_customer_id" validate:"required"`
	Tier                 string     `json:"tier" validate:"required,oneof=starter professional enterprise"`
	SeatCount            int        `json:"seat_count" validate:"required,min=1"`
	Status               string     `json:"status" validate:"required,oneof=trialing active past_due canceled unpaid"`
	BillingInterval      string     `json:"billing_interval" validate:"required,oneof=month year"`
	CurrentPeriodStart   time.Time  `json:"current_period_start" validate:"required"`
	CurrentPeriodEnd     time.Time  `json:"current_period_end" validate:"required"`
	TrialEnd             *time.Time `json:"trial_end,omitempty"`
}

// CreateTeamSubscriptionResponse represents the response after creating a team subscription
type CreateTeamSubscriptionResponse struct {
	ID                   string    `json:"id"`
	TeamID               string    `json:"team_id"`
	StripeSubscriptionID string    `json:"stripe_subscription_id"`
	Tier                 string    `json:"tier"`
	SeatCount            int       `json:"seat_count"`
	Status               string    `json:"status"`
	BillingInterval      string    `json:"billing_interval"`
	CurrentPeriodStart   time.Time `json:"current_period_start"`
	CurrentPeriodEnd     time.Time `json:"current_period_end"`
	CreatedAt            time.Time `json:"created_at"`
}

// TeamSubscriptionResponse represents detailed team subscription information with quota types
type TeamSubscriptionResponse struct {
	ID                   string                `json:"id"`
	TeamID               string                `json:"team_id"`
	StripeSubscriptionID string                `json:"stripe_subscription_id"`
	StripeCustomerID     string                `json:"stripe_customer_id"`
	Tier                 string                `json:"tier"`
	SeatCount            int                   `json:"seat_count"`
	Status               string                `json:"status"`
	BillingInterval      string                `json:"billing_interval"`
	CurrentPeriodStart   time.Time             `json:"current_period_start"`
	CurrentPeriodEnd     time.Time             `json:"current_period_end"`
	TrialEnd             *time.Time            `json:"trial_end,omitempty"`
	CanceledAt           *time.Time            `json:"canceled_at,omitempty"`
	IsTrialActive        bool                  `json:"is_trial_active"`
	CanAccessService     bool                  `json:"can_access_service"`
	QuotaTypes           []TeamQuotaTypeDetail `json:"quota_types"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
}

// TeamQuotaTypeDetail represents quota details for a specific resource type (prompts, projects, etc.)
type TeamQuotaTypeDetail struct {
	ResourceType string `json:"resource_type"` // "prompts", "projects", "artifacts", "blueprint"
	Limit        int    `json:"limit"`         // Maximum allowed (e.g., 100)
	Used         int    `json:"used"`          // Current usage (e.g., 45)
	Remaining    int    `json:"remaining"`     // Available quota (e.g., 55)
}

// IsActiveForQuotas returns true if the subscription status counts towards resource quotas.
// Active and Trialing subscriptions grant quota benefits.
func (ts *TeamSubscription) IsActiveForQuotas() bool {
	return ts.Status == TeamSubscriptionStatusActive ||
		ts.Status == TeamSubscriptionStatusTrialing
}
