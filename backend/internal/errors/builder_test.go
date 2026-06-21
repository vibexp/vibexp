package errors

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResourceLimitExceededErrorWithMetadata_WithSubscriptionData(t *testing.T) {
	detail := "Team requires an active subscription"
	metadata := map[string]any{
		"team_id":     "team-123",
		"feature":     "team_invitations",
		"upgrade_url": "/settings/teams/team-123/subscription",
	}

	got := NewResourceLimitExceededErrorWithMetadata(detail, metadata)

	require.NotNil(t, got)
	// Default neutral base ("about:blank") yields a bare "about:blank" type.
	assert.Equal(t, "about:blank", got.Type)
	assert.Equal(t, "Resource Limit Exceeded", got.Title)
	assert.Equal(t, http.StatusForbidden, got.Status)
	assert.Equal(t, detail, got.Detail)
	assert.Equal(t, CodeResourceLimitExceeded, got.Code)
	assert.Equal(t, metadata, got.Metadata)
	assert.NotEmpty(t, got.Timestamp)
}

func TestNewResourceLimitExceededErrorWithMetadata_WithSeatLimitData(t *testing.T) {
	detail := "Team has reached seat limit"
	metadata := map[string]any{
		"team_id":                 "team-456",
		"total_seats":             5,
		"occupied_seats":          5,
		"current_members":         3,
		"pending_invitations":     2,
		"requested_invitations":   3,
		"available_seats":         0,
		"additional_seats_needed": 3,
		"upgrade_url":             "/settings/teams/team-456/subscription",
	}

	got := NewResourceLimitExceededErrorWithMetadata(detail, metadata)

	require.NotNil(t, got)
	assert.Equal(t, http.StatusForbidden, got.Status)
	assert.Equal(t, detail, got.Detail)
	assert.Equal(t, metadata, got.Metadata)
	assert.NotEmpty(t, got.Timestamp)
}

func TestNewResourceLimitExceededErrorWithMetadata_EmptyMetadata(t *testing.T) {
	detail := "Resource limit exceeded"
	metadata := map[string]any{}

	got := NewResourceLimitExceededErrorWithMetadata(detail, metadata)

	require.NotNil(t, got)
	assert.Equal(t, http.StatusForbidden, got.Status)
	assert.Equal(t, detail, got.Detail)
	assert.Equal(t, metadata, got.Metadata)
}

func TestNewResourceLimitExceededErrorWithMetadata_NilMetadata(t *testing.T) {
	detail := "Resource limit exceeded"

	got := NewResourceLimitExceededErrorWithMetadata(detail, nil)

	require.NotNil(t, got)
	assert.Equal(t, http.StatusForbidden, got.Status)
	assert.Equal(t, detail, got.Detail)
	assert.Nil(t, got.Metadata)
}

func TestTypeURI_ConfigurableBase(t *testing.T) {
	// Default neutral base: "about:blank" with no code appended.
	assert.Equal(t, "about:blank", typeURI(CodeResourceLimitExceeded))

	// Restore the default after the test so other tests see the neutral base.
	t.Cleanup(func() { SetTypeBaseURI("") })

	// A configured base joins "<base>/<code>".
	SetTypeBaseURI("https://example.com/errors")
	assert.Equal(t, "https://example.com/errors/RESOURCE_LIMIT_EXCEEDED", typeURI(CodeResourceLimitExceeded))
	assert.Equal(
		t,
		"https://example.com/errors/RESOURCE_LIMIT_EXCEEDED",
		NewResourceLimitExceededError("limit").Type,
	)

	// Empty resets to the neutral default.
	SetTypeBaseURI("")
	assert.Equal(t, "about:blank", typeURI(CodeResourceLimitExceeded))
}

func TestNewResourceLimitExceededError_BackwardCompatibility(t *testing.T) {
	// Ensure the original function still works without metadata
	err := NewResourceLimitExceededError("Rate limit exceeded")

	require.NotNil(t, err)
	assert.Equal(t, "about:blank", err.Type)
	assert.Equal(t, "Resource Limit Exceeded", err.Title)
	assert.Equal(t, http.StatusForbidden, err.Status)
	assert.Equal(t, "Rate limit exceeded", err.Detail)
	assert.Equal(t, CodeResourceLimitExceeded, err.Code)
	assert.Nil(t, err.Metadata)
	assert.NotEmpty(t, err.Timestamp)
}
