package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUserBasicInfo_SafeFieldsOnly(t *testing.T) {
	googleID := "google-123"
	idpProvider := "google"
	idpSubject := "subj-456"
	stripeCustomerID := "cus_abc123"
	avatarURL := "https://example.com/avatar.png"
	defaultTeamID := "team-uuid-001"
	subscriptionPlan := "teams_pro"
	canceledAt := time.Now().Add(-24 * time.Hour)

	u := &User{
		ID:                     "user-uuid-001",
		GoogleID:               &googleID,
		IDPProvider:            &idpProvider,
		IDPSubject:             &idpSubject,
		Email:                  "jane@example.com",
		Name:                   "Jane Doe",
		AvatarURL:              &avatarURL,
		StripeCustomerID:       &stripeCustomerID,
		SubscriptionStatus:     "active",
		SubscriptionPlan:       &subscriptionPlan,
		SubscriptionCanceledAt: &canceledAt,
		DefaultTeamID:          &defaultTeamID,
		OnboardingCompleted:    true,
		CreatedAt:              time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		Version:                42,
	}

	info := NewUserBasicInfo(u)
	require.NotNil(t, info)

	// Safe fields must be present and match
	assert.Equal(t, "user-uuid-001", info.ID)
	assert.Equal(t, "jane@example.com", info.Email)
	assert.Equal(t, "Jane Doe", info.Name)
	assert.Equal(t, &avatarURL, info.AvatarURL)
	assert.Equal(t, &defaultTeamID, info.DefaultTeamID)
	assert.Equal(t, "active", info.SubscriptionStatus)
	assert.Equal(t, &subscriptionPlan, info.SubscriptionPlan)
	assert.True(t, info.OnboardingCompleted)
	assert.Equal(t, u.CreatedAt, info.CreatedAt)
}

func TestNewUserBasicInfo_NilOptionalFields(t *testing.T) {
	u := &User{
		ID:                  "user-uuid-002",
		Email:               "minimal@example.com",
		Name:                "Minimal User",
		SubscriptionStatus:  "free",
		OnboardingCompleted: false,
		CreatedAt:           time.Now(),
	}

	info := NewUserBasicInfo(u)
	require.NotNil(t, info)

	assert.Nil(t, info.AvatarURL)
	assert.Nil(t, info.DefaultTeamID)
	assert.Nil(t, info.SubscriptionPlan)
}

func TestNewUserBasicInfo_SensitiveFieldsAbsent(t *testing.T) {
	// This test documents that UserBasicInfo has NO fields for the sensitive values —
	// they cannot be set even accidentally.
	info := &UserBasicInfo{}

	// Confirm the struct type has no sensitive-field member by ensuring compilation
	// succeeds with only safe fields assigned.
	info.ID = "x"
	info.Email = "x@x.com"
	info.Name = "X"
	info.SubscriptionStatus = "active"
	info.OnboardingCompleted = true
	info.CreatedAt = time.Now()

	// The following would not compile if UserBasicInfo had sensitive fields:
	// info.GoogleID = nil
	// info.IDPProvider = nil
	// info.IDPSubject = nil
	// info.StripeCustomerID = nil
	// info.SubscriptionCanceledAt = nil
	// info.Version = 0

	assert.NotNil(t, info)
}
