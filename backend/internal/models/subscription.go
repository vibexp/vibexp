package models

import (
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Subscription models
type Subscription struct {
	ID                   string     `json:"id" db:"id"`
	UserID               string     `json:"user_id" db:"user_id"`
	StripeSubscriptionID *string    `json:"stripe_subscription_id,omitempty" db:"stripe_subscription_id"`
	StripeCustomerID     *string    `json:"stripe_customer_id,omitempty" db:"stripe_customer_id"`
	Status               string     `json:"status" db:"status"`
	PlanName             *string    `json:"plan_name,omitempty" db:"plan_name"`
	CurrentPeriodStart   *time.Time `json:"current_period_start,omitempty" db:"current_period_start"`
	CurrentPeriodEnd     *time.Time `json:"current_period_end,omitempty" db:"current_period_end"`
	TrialEnd             *time.Time `json:"trial_end,omitempty" db:"trial_end"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

type CreateSubscriptionRequest struct {
	PriceID             string `json:"price_id" validate:"required"`
	GA4ClientID         string `json:"ga4_client_id,omitempty"`
	AllowPromotionCodes bool   `json:"allow_promotion_codes,omitempty"`
}

type CreateSubscriptionResponse struct {
	CheckoutURL string `json:"checkout_url"`
	SessionID   string `json:"session_id"`
}

type SubscriptionStatusResponse struct {
	Status             string     `json:"status"`
	PlanName           *string    `json:"plan_name,omitempty"`
	TrialEnd           *time.Time `json:"trial_end,omitempty"`
	CurrentPeriodStart *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd  bool       `json:"cancel_at_period_end"`
	CanceledAt         *time.Time `json:"canceled_at,omitempty"`
	IsTrialActive      bool       `json:"is_trial_active"`
	CanAccessService   bool       `json:"can_access_service"`
}

type CreatePortalSessionResponse struct {
	URL string `json:"url"`
}

type ProductConfiguration struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	PriceID           string   `json:"price_id"`
	Currency          string   `json:"currency"`
	Amount            float64  `json:"amount"` // Amount in currency units (e.g., 19.99 for €19.99)
	Popular           bool     `json:"popular"`
	MarketingFeatures []string `json:"marketing_features"`
}

type ProductConfigurationResponse struct {
	Products []ProductConfiguration `json:"products"`
}

// TeamQuota represents resource quotas for team subscriptions
type TeamQuota struct {
	Prompts              int `json:"prompts"`               // -1 means unlimited
	Artifacts            int `json:"artifacts"`             // -1 means unlimited
	Memories             int `json:"memories"`              // -1 means unlimited
	AIToolSessions       int `json:"ai_tool_sessions"`      // -1 means unlimited
	AgenticConversations int `json:"agentic_conversations"` // -1 means unlimited
	SpecLibraryItems     int `json:"spec_library_items"`    // -1 means unlimited
}

// TeamSubscriptionPlanPrice represents pricing information for a team plan
type TeamSubscriptionPlanPrice struct {
	PriceID  string `json:"price_id"`
	Interval string `json:"interval"` // "month" or "year"
	Amount   int64  `json:"amount"`   // Amount in cents
	Currency string `json:"currency"`
}

// TeamSubscriptionPlan represents a team subscription plan configuration
type TeamSubscriptionPlan struct {
	ID           string                      `json:"id"`
	Name         string                      `json:"name"`
	Tier         string                      `json:"tier"`
	Description  string                      `json:"description"`
	MinSeats     int                         `json:"min_seats"`
	MaxSeats     int                         `json:"max_seats"` // 0 means unlimited
	Prices       []TeamSubscriptionPlanPrice `json:"prices"`
	Features     []string                    `json:"features"`
	BaseQuota    TeamQuota                   `json:"base_quota"`
	PerSeatQuota TeamQuota                   `json:"per_seat_quota"`
}

// TeamSubscriptionPlansResponse represents the response containing all team plans
type TeamSubscriptionPlansResponse struct {
	Plans []TeamSubscriptionPlan `json:"plans"`
}

// Subscription constants
const (
	SubscriptionStatusBasic       = "basic"
	SubscriptionStatusNone        = "none"
	SubscriptionStatusTrialActive = "trial_active"
	SubscriptionStatusActive      = "active"
	SubscriptionStatusCancelled   = "cancelled"
	SubscriptionStatusPastDue     = "past_due"
	SubscriptionStatusUnpaid      = "unpaid"
)

// Subscription plan constants
const (
	PlanBasic     = "basic"
	PlanStarter   = "starter"
	PlanPro       = "professional"
	PlanPowerUser = "power_user"

	// Team subscription plan constants
	PlanTeamsStarter      = "teams_starter"
	PlanTeamsProfessional = "teams_professional"
	PlanTeamsEnterprise   = "teams_enterprise"
)

// NormalizePlanName normalizes subscription plan names from various formats to standard constants
// Examples:
// - "VibeXP - Starter" -> "starter"
// - "VibeXP - Professional" -> "professional"
// - "VibeXP - Power User" -> "power_user"
func NormalizePlanName(stripePlanName string) string {
	// Remove common prefixes
	planName := stripePlanName
	prefixes := []string{"VibeXP - ", "vibexp - ", "VIBEXP - "}
	for _, prefix := range prefixes {
		if len(planName) >= len(prefix) &&
			(planName[:len(prefix)] == prefix ||
				planName[:len(prefix)] == strings.ToLower(prefix) ||
				planName[:len(prefix)] == strings.ToUpper(prefix)) {
			planName = planName[len(prefix):]
			break
		}
	}

	// Convert to lowercase and replace spaces with underscores
	planName = strings.ToLower(strings.TrimSpace(planName))
	planName = strings.ReplaceAll(planName, " ", "_")

	// Validate against known plan constants
	switch planName {
	case PlanBasic, "free": // Support legacy "free" for backward compatibility
		return PlanBasic
	case PlanStarter:
		return PlanStarter
	case PlanPro, "pro":
		return PlanPro
	case PlanPowerUser, "poweruser":
		return PlanPowerUser
	default:
		logrus.WithField("stripe_plan_name", stripePlanName).
			WithField("normalized_plan_name", planName).
			Warn("Unknown plan name from Stripe, falling back to starter")
		return PlanStarter
	}
}
