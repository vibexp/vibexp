package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTeamSubscriptionConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		// Tier constants
		{"TeamTierStarter", TeamTierStarter, "starter"},
		{"TeamTierProfessional", TeamTierProfessional, "professional"},
		{"TeamTierEnterprise", TeamTierEnterprise, "enterprise"},

		// Status constants
		{"TeamSubscriptionStatusTrialing", TeamSubscriptionStatusTrialing, "trialing"},
		{"TeamSubscriptionStatusActive", TeamSubscriptionStatusActive, "active"},
		{"TeamSubscriptionStatusPastDue", TeamSubscriptionStatusPastDue, "past_due"},
		{"TeamSubscriptionStatusCanceled", TeamSubscriptionStatusCanceled, "canceled"},
		{"TeamSubscriptionStatusUnpaid", TeamSubscriptionStatusUnpaid, "unpaid"},

		// Billing interval constants
		{"BillingIntervalMonth", BillingIntervalMonth, "month"},
		{"BillingIntervalYear", BillingIntervalYear, "year"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}

func TestTeamSubscriptionStructFields(t *testing.T) {
	now := time.Now()
	trialEnd := now.Add(7 * 24 * time.Hour)
	canceledAt := now.Add(30 * 24 * time.Hour)

	subscription := TeamSubscription{
		ID:                   "sub-123",
		TeamID:               "team-456",
		StripeSubscriptionID: "sub_stripe_789",
		StripeCustomerID:     "cus_stripe_abc",
		Tier:                 TeamTierProfessional,
		SeatCount:            5,
		Status:               TeamSubscriptionStatusActive,
		BillingInterval:      BillingIntervalMonth,
		CurrentPeriodStart:   now,
		CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
		TrialEnd:             &trialEnd,
		CanceledAt:           &canceledAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	assert.Equal(t, "sub-123", subscription.ID)
	assert.Equal(t, "team-456", subscription.TeamID)
	assert.Equal(t, "sub_stripe_789", subscription.StripeSubscriptionID)
	assert.Equal(t, "cus_stripe_abc", subscription.StripeCustomerID)
	assert.Equal(t, TeamTierProfessional, subscription.Tier)
	assert.Equal(t, 5, subscription.SeatCount)
	assert.Equal(t, TeamSubscriptionStatusActive, subscription.Status)
	assert.Equal(t, BillingIntervalMonth, subscription.BillingInterval)
	assert.False(t, subscription.CurrentPeriodStart.IsZero())
	assert.False(t, subscription.CurrentPeriodEnd.IsZero())
	assert.False(t, subscription.CreatedAt.IsZero())
	assert.False(t, subscription.UpdatedAt.IsZero())
	assert.NotNil(t, subscription.TrialEnd)
	assert.NotNil(t, subscription.CanceledAt)
}

func TestCreateTeamSubscriptionRequestValidation(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		request CreateTeamSubscriptionRequest
		wantErr bool
	}{
		{
			name: "valid request with all required fields",
			request: CreateTeamSubscriptionRequest{
				TeamID:               "550e8400-e29b-41d4-a716-446655440000",
				StripeSubscriptionID: "sub_123",
				StripeCustomerID:     "cus_123",
				Tier:                 TeamTierStarter,
				SeatCount:            3,
				Status:               TeamSubscriptionStatusActive,
				BillingInterval:      BillingIntervalMonth,
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
			wantErr: false,
		},
		{
			name: "valid request with trial end",
			request: CreateTeamSubscriptionRequest{
				TeamID:               "550e8400-e29b-41d4-a716-446655440000",
				StripeSubscriptionID: "sub_456",
				StripeCustomerID:     "cus_456",
				Tier:                 TeamTierProfessional,
				SeatCount:            10,
				Status:               TeamSubscriptionStatusTrialing,
				BillingInterval:      BillingIntervalYear,
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(365 * 24 * time.Hour),
				TrialEnd:             &now,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the struct can be created without errors
			assert.NotEmpty(t, tt.request.TeamID)
			assert.NotEmpty(t, tt.request.StripeSubscriptionID)
			assert.NotEmpty(t, tt.request.StripeCustomerID)
			assert.NotEmpty(t, tt.request.Tier)
			assert.Greater(t, tt.request.SeatCount, 0)
			assert.NotEmpty(t, tt.request.Status)
			assert.NotEmpty(t, tt.request.BillingInterval)
			assert.False(t, tt.request.CurrentPeriodStart.IsZero())
			assert.False(t, tt.request.CurrentPeriodEnd.IsZero())
		})
	}
}

func TestTeamSubscriptionJSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second) // Truncate for comparison
	trialEnd := now.Add(7 * 24 * time.Hour)

	subscription := TeamSubscription{
		ID:                   "sub-123",
		TeamID:               "team-456",
		StripeSubscriptionID: "sub_stripe_789",
		StripeCustomerID:     "cus_stripe_abc",
		Tier:                 TeamTierProfessional,
		SeatCount:            5,
		Status:               TeamSubscriptionStatusActive,
		BillingInterval:      BillingIntervalMonth,
		CurrentPeriodStart:   now,
		CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
		TrialEnd:             &trialEnd,
		CanceledAt:           nil,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(subscription)
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal back to struct
	var decoded TeamSubscription
	err = json.Unmarshal(jsonData, &decoded)
	assert.NoError(t, err)

	// Verify fields
	assert.Equal(t, subscription.ID, decoded.ID)
	assert.Equal(t, subscription.TeamID, decoded.TeamID)
	assert.Equal(t, subscription.StripeSubscriptionID, decoded.StripeSubscriptionID)
	assert.Equal(t, subscription.StripeCustomerID, decoded.StripeCustomerID)
	assert.Equal(t, subscription.Tier, decoded.Tier)
	assert.Equal(t, subscription.SeatCount, decoded.SeatCount)
	assert.Equal(t, subscription.Status, decoded.Status)
	assert.Equal(t, subscription.BillingInterval, decoded.BillingInterval)
	assert.WithinDuration(t, subscription.CurrentPeriodStart, decoded.CurrentPeriodStart, time.Second)
	assert.WithinDuration(t, subscription.CurrentPeriodEnd, decoded.CurrentPeriodEnd, time.Second)
	assert.WithinDuration(t, subscription.CreatedAt, decoded.CreatedAt, time.Second)
	assert.WithinDuration(t, subscription.UpdatedAt, decoded.UpdatedAt, time.Second)
	assert.NotNil(t, decoded.TrialEnd)
	assert.Nil(t, decoded.CanceledAt)
}

func TestCreateTeamSubscriptionResponse(t *testing.T) {
	now := time.Now()

	response := CreateTeamSubscriptionResponse{
		ID:                   "sub-123",
		TeamID:               "team-456",
		StripeSubscriptionID: "sub_stripe_789",
		Tier:                 TeamTierStarter,
		SeatCount:            3,
		Status:               TeamSubscriptionStatusActive,
		BillingInterval:      BillingIntervalMonth,
		CurrentPeriodStart:   now,
		CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
		CreatedAt:            now,
	}

	assert.Equal(t, "sub-123", response.ID)
	assert.Equal(t, "team-456", response.TeamID)
	assert.Equal(t, "sub_stripe_789", response.StripeSubscriptionID)
	assert.Equal(t, TeamTierStarter, response.Tier)
	assert.Equal(t, 3, response.SeatCount)
	assert.Equal(t, TeamSubscriptionStatusActive, response.Status)
	assert.Equal(t, BillingIntervalMonth, response.BillingInterval)
	assert.False(t, response.CurrentPeriodStart.IsZero())
	assert.False(t, response.CurrentPeriodEnd.IsZero())
	assert.False(t, response.CreatedAt.IsZero())
}

func TestTeamSubscriptionResponse(t *testing.T) {
	now := time.Now()
	trialEnd := now.Add(7 * 24 * time.Hour)

	quotaTypes := []TeamQuotaTypeDetail{
		{
			ResourceType: "prompts",
			Limit:        100,
			Used:         45,
			Remaining:    55,
		},
		{
			ResourceType: "projects",
			Limit:        50,
			Used:         12,
			Remaining:    38,
		},
	}

	response := TeamSubscriptionResponse{
		ID:                   "sub-123",
		TeamID:               "team-456",
		StripeSubscriptionID: "sub_stripe_789",
		StripeCustomerID:     "cus_stripe_abc",
		Tier:                 TeamTierProfessional,
		SeatCount:            5,
		Status:               TeamSubscriptionStatusActive,
		BillingInterval:      BillingIntervalMonth,
		CurrentPeriodStart:   now,
		CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
		TrialEnd:             &trialEnd,
		CanceledAt:           nil,
		IsTrialActive:        false,
		CanAccessService:     true,
		QuotaTypes:           quotaTypes,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	assert.Equal(t, "sub-123", response.ID)
	assert.Equal(t, "team-456", response.TeamID)
	assert.Equal(t, "sub_stripe_789", response.StripeSubscriptionID)
	assert.Equal(t, "cus_stripe_abc", response.StripeCustomerID)
	assert.Equal(t, TeamTierProfessional, response.Tier)
	assert.Equal(t, 5, response.SeatCount)
	assert.Equal(t, TeamSubscriptionStatusActive, response.Status)
	assert.Equal(t, BillingIntervalMonth, response.BillingInterval)
	assert.False(t, response.CurrentPeriodStart.IsZero())
	assert.False(t, response.CurrentPeriodEnd.IsZero())
	assert.False(t, response.CreatedAt.IsZero())
	assert.False(t, response.UpdatedAt.IsZero())
	assert.NotNil(t, response.TrialEnd)
	assert.Nil(t, response.CanceledAt)
	assert.True(t, response.CanAccessService)
	assert.False(t, response.IsTrialActive)
	assert.Len(t, response.QuotaTypes, 2)
	assert.Equal(t, "prompts", response.QuotaTypes[0].ResourceType)
	assert.Equal(t, 100, response.QuotaTypes[0].Limit)
}

func TestTeamQuotaTypeDetail(t *testing.T) {
	quota := TeamQuotaTypeDetail{
		ResourceType: "artifacts",
		Limit:        200,
		Used:         150,
		Remaining:    50,
	}

	assert.Equal(t, "artifacts", quota.ResourceType)
	assert.Equal(t, 200, quota.Limit)
	assert.Equal(t, 150, quota.Used)
	assert.Equal(t, 50, quota.Remaining)
	assert.Equal(t, quota.Limit, quota.Used+quota.Remaining)
}
