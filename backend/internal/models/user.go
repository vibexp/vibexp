package models

import (
	"time"
)

// UserBasicInfo is a safe, minimal DTO that exposes only non-sensitive user
// fields over the MCP protocol. Sensitive fields (GoogleID, IDPProvider,
// IDPSubject, StripeCustomerID, SubscriptionCanceledAt, Version) are
// intentionally excluded.
type UserBasicInfo struct {
	ID                  string    `json:"id"`
	Email               string    `json:"email"`
	Name                string    `json:"name"`
	AvatarURL           *string   `json:"avatar_url,omitempty"`
	DefaultTeamID       *string   `json:"default_team_id,omitempty"`
	SubscriptionStatus  string    `json:"subscription_status"`
	SubscriptionPlan    *string   `json:"subscription_plan,omitempty"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
	CreatedAt           time.Time `json:"created_at"`
}

// NewUserBasicInfo copies the safe subset of fields from u into a new
// UserBasicInfo. Sensitive fields are never copied.
func NewUserBasicInfo(u *User) *UserBasicInfo {
	return &UserBasicInfo{
		ID:                  u.ID,
		Email:               u.Email,
		Name:                u.Name,
		AvatarURL:           u.AvatarURL,
		DefaultTeamID:       u.DefaultTeamID,
		SubscriptionStatus:  u.SubscriptionStatus,
		SubscriptionPlan:    u.SubscriptionPlan,
		OnboardingCompleted: u.OnboardingCompleted,
		CreatedAt:           u.CreatedAt,
	}
}

type User struct {
	ID                     string     `json:"id" db:"id"`
	GoogleID               *string    `json:"google_id,omitempty" db:"google_id"`
	IDPProvider            *string    `json:"idp_provider,omitempty" db:"idp_provider"`
	IDPSubject             *string    `json:"idp_subject,omitempty" db:"idp_subject"`
	Email                  string     `json:"email" db:"email"`
	Name                   string     `json:"name" db:"name"`
	AvatarURL              *string    `json:"avatar_url" db:"avatar_url"`
	StripeCustomerID       *string    `json:"stripe_customer_id,omitempty" db:"stripe_customer_id"`
	SubscriptionStatus     string     `json:"subscription_status" db:"subscription_status"`
	TrialEndsAt            *time.Time `json:"trial_ends_at,omitempty" db:"trial_ends_at"`
	SubscriptionPlan       *string    `json:"subscription_plan,omitempty" db:"subscription_plan"`
	SubscriptionCanceledAt *time.Time `json:"-" db:"subscription_canceled_at"`
	DefaultTeamID          *string    `json:"default_team_id,omitempty" db:"default_team_id"`
	OnboardingCompleted    bool       `json:"onboarding_completed" db:"onboarding_completed"`
	OnboardingCompletedAt  *time.Time `json:"onboarding_completed_at,omitempty" db:"onboarding_completed_at"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
	Version                int64      `json:"version" db:"version"`
}

type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type UserOnboardingProgress struct {
	User                                *User      `json:"user"`
	HasApiKeys                          bool       `json:"has_api_keys"`
	HasPrompts                          bool       `json:"has_prompts"`
	HasArtifacts                        bool       `json:"has_artifacts"`
	HasActiveSubscription               bool       `json:"has_active_subscription"`
	HasClaudeCodeIntegration            bool       `json:"has_claude_code_integration"`
	HasMemories                         bool       `json:"has_memories"`
	ClaudeCodeIntegrationLastAccessedAt *time.Time `json:"claude_code_integration_last_accessed_at,omitempty"`
}
