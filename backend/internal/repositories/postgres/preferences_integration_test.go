//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Behavior-level parity suite for UserPreferencesRepository against real
// Postgres (#1612). These tests are the contract any reimplementation of the
// queries (e.g. the sqlc PoC, #1588) must satisfy — they assert rows in/out
// and error semantics, never SQL text.

func integrationPreferences() models.Preferences {
	return models.Preferences{
		EmailNotification: models.EmailNotificationPreferences{
			PlatformAnnouncement: true,
			AccountSecurity:      true,
			NewFeature:           false,
			MarketingPromotional: false,
		},
		Notifications: models.NotificationPreferences{
			Channels: models.NotificationChannelPreferences{
				InApp:   true,
				Email:   true,
				WebPush: false,
			},
			Types: map[string]models.NotificationTypePreference{
				"team_invitation": {InApp: true, Email: "instant", WebPush: false},
			},
		},
	}
}

func TestIntegrationUserPreferences_GetByUserID_NoRows(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserPreferencesRepository(integrationDB)

	got, err := repo.GetByUserID(context.Background(), uuid.New().String())

	require.NoError(t, err)
	assert.Nil(t, got, "no preferences row must yield (nil, nil), not an error")
}

func TestIntegrationUserPreferences_Upsert_InsertAndJSONBRoundTrip(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserPreferencesRepository(integrationDB)
	userID := insertTestUser(t)

	prefs := &models.UserPreferences{
		UserID:      userID,
		Preferences: integrationPreferences(),
	}
	require.NoError(t, repo.Upsert(context.Background(), prefs))

	assert.NotEmpty(t, prefs.ID)
	assert.EqualValues(t, 1, prefs.Version)
	assert.False(t, prefs.CreatedAt.IsZero())

	got, err := repo.GetByUserID(context.Background(), userID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, prefs.ID, got.ID)
	assert.Equal(t, userID, got.UserID)
	assert.EqualValues(t, 1, got.Version)
	assert.Equal(t, integrationPreferences(), got.Preferences,
		"JSONB round-trip must preserve the full preferences document")
}

func TestIntegrationUserPreferences_Upsert_ConflictIncrementsVersion(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserPreferencesRepository(integrationDB)
	userID := insertTestUser(t)

	first := &models.UserPreferences{
		UserID:      userID,
		Preferences: integrationPreferences(),
	}
	require.NoError(t, repo.Upsert(context.Background(), first))

	updated := integrationPreferences()
	updated.EmailNotification.MarketingPromotional = true
	updated.Notifications.Types["mention"] = models.NotificationTypePreference{
		InApp: true, Email: "digest", WebPush: true,
	}
	// A fresh struct (empty ID) proves the ON CONFLICT (user_id) path: the
	// repository generates a new candidate ID, but the existing row wins.
	conflicting := &models.UserPreferences{
		UserID:      userID,
		Preferences: updated,
	}
	require.NoError(t, repo.Upsert(context.Background(), conflicting))

	assert.Equal(t, first.ID, conflicting.ID, "ON CONFLICT must keep the existing row's ID")
	assert.EqualValues(t, 2, conflicting.Version, "each conflicting upsert must increment version")
	assert.Equal(t, first.CreatedAt, conflicting.CreatedAt, "created_at must not change on update")

	got, err := repo.GetByUserID(context.Background(), userID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.EqualValues(t, 2, got.Version)
	assert.Equal(t, updated, got.Preferences, "the stored document must be fully replaced")
}
