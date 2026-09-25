//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// Admin saved filter presets (#1147) against real Postgres: the JSON-path
// write, the version lock, and the interaction with the user-facing
// /preferences read and write paths.

func integrationPresets(names ...string) []models.AdminSavedFilterPreset {
	out := make([]models.AdminSavedFilterPreset, 0, len(names))
	for _, n := range names {
		out = append(out, models.AdminSavedFilterPreset{
			ID: uuid.NewString(), Name: n, Query: map[string]string{"status": "active"},
		})
	}
	return out
}

func storedPreferencesDoc(t *testing.T, userID string) map[string]any {
	t.Helper()
	var raw []byte
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT preferences FROM user_preferences WHERE user_id = $1", userID).Scan(&raw))
	doc := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

func TestIntegrationAdminSavedFilters_NoRowReadsEmptyVersionZero(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserPreferencesRepository(integrationDB)

	presets, version, err := repo.GetAdminSavedFilters(context.Background(), insertTestUser(t), "users")

	require.NoError(t, err)
	assert.Empty(t, presets)
	assert.NotNil(t, presets)
	assert.EqualValues(t, 0, version)
}

// TestIntegrationAdminSavedFilters_InsertSeedsDefaults: creating the row via
// the presets path seeds the default preferences, so GET /preferences returns
// exactly what a user with no row gets — and never shows the `admin` key.
func TestIntegrationAdminSavedFilters_InsertSeedsDefaults(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserPreferencesRepository(integrationDB)
	svc := services.NewUserPreferencesService(repo)
	userID := insertTestUser(t)

	saved, version, err := svc.ReplaceAdminSavedFilters(ctx, userID, "teams",
		integrationPresets("Dormant teams"), 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, version)

	got, gotVersion, err := repo.GetAdminSavedFilters(ctx, userID, "teams")
	require.NoError(t, err)
	assert.EqualValues(t, 1, gotVersion)
	assert.Equal(t, saved, got)

	prefs, err := svc.GetPreferences(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, models.DefaultPreferences(), prefs.Preferences)

	body, err := json.Marshal(prefs)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "admin", "GET /preferences must not expose admin.saved_filters")
	assert.NotContains(t, string(body), "saved_filters")
}

// TestIntegrationAdminSavedFilters_VersionLock: two writers holding the same
// version race; exactly one wins, the loser gets the conflict sentinel, and the
// stored presets are the winner's.
func TestIntegrationAdminSavedFilters_VersionLock(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserPreferencesRepository(integrationDB)
	userID := insertTestUser(t)

	v1, err := repo.ReplaceAdminSavedFilters(ctx, userID, "users", integrationPresets("First"),
		models.DefaultPreferences(), 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, v1)

	writers := [][]models.AdminSavedFilterPreset{integrationPresets("Writer A"), integrationPresets("Writer B")}
	errs := make([]error, len(writers))
	versions := make([]int64, len(writers))
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			versions[i], errs[i] = repo.ReplaceAdminSavedFilters(ctx, userID, "users", writers[i],
				models.DefaultPreferences(), v1)
		}(i)
	}
	wg.Wait()

	winner := -1
	for i, err := range errs {
		if err == nil {
			require.Equal(t, -1, winner, "only one writer may win")
			winner = i
			assert.EqualValues(t, 2, versions[i])
		} else {
			require.ErrorIs(t, err, repositories.ErrUserPreferencesVersionConflict)
		}
	}
	require.NotEqual(t, -1, winner, "one writer must win")

	got, version, err := repo.GetAdminSavedFilters(ctx, userID, "users")
	require.NoError(t, err)
	assert.EqualValues(t, 2, version)
	assert.Equal(t, writers[winner], got)

	// A stale version (the one both writers held) now conflicts and changes nothing.
	_, err = repo.ReplaceAdminSavedFilters(ctx, userID, "users", integrationPresets("Stale"),
		models.DefaultPreferences(), v1)
	require.ErrorIs(t, err, repositories.ErrUserPreferencesVersionConflict)
	got, _, err = repo.GetAdminSavedFilters(ctx, userID, "users")
	require.NoError(t, err)
	assert.Equal(t, writers[winner], got)
}

// TestIntegrationAdminSavedFilters_InsertRaceConflicts: version 0 against an
// existing row is a conflict, never an overwrite; a non-zero version against no
// row is a conflict, never an insert.
func TestIntegrationAdminSavedFilters_InsertRaceConflicts(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserPreferencesRepository(integrationDB)
	userID := insertTestUser(t)

	_, err := repo.ReplaceAdminSavedFilters(ctx, userID, "users", integrationPresets("A"),
		models.DefaultPreferences(), 3)
	require.ErrorIs(t, err, repositories.ErrUserPreferencesVersionConflict)
	_, version, err := repo.GetAdminSavedFilters(ctx, userID, "users")
	require.NoError(t, err)
	assert.EqualValues(t, 0, version, "no row may be created by a non-zero version")

	_, err = repo.ReplaceAdminSavedFilters(ctx, userID, "users", integrationPresets("A"),
		models.DefaultPreferences(), 0)
	require.NoError(t, err)
	_, err = repo.ReplaceAdminSavedFilters(ctx, userID, "users", integrationPresets("B"),
		models.DefaultPreferences(), 0)
	require.ErrorIs(t, err, repositories.ErrUserPreferencesVersionConflict)
}

// TestIntegrationAdminSavedFilters_ListsAreIndependent: writing one list keeps
// the others and every non-admin key.
func TestIntegrationAdminSavedFilters_ListsAreIndependent(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserPreferencesRepository(integrationDB)
	userID := insertTestUser(t)

	usersPresets := integrationPresets("Power users")
	v, err := repo.ReplaceAdminSavedFilters(ctx, userID, "users", usersPresets, models.DefaultPreferences(), 0)
	require.NoError(t, err)
	teamsPresets := integrationPresets("Dormant teams", "Big teams")
	_, err = repo.ReplaceAdminSavedFilters(ctx, userID, "teams", teamsPresets, models.DefaultPreferences(), v)
	require.NoError(t, err)

	gotUsers, _, err := repo.GetAdminSavedFilters(ctx, userID, "users")
	require.NoError(t, err)
	assert.Equal(t, usersPresets, gotUsers)
	gotTeams, _, err := repo.GetAdminSavedFilters(ctx, userID, "teams")
	require.NoError(t, err)
	assert.Equal(t, teamsPresets, gotTeams)
	gotProjects, _, err := repo.GetAdminSavedFilters(ctx, userID, "projects")
	require.NoError(t, err)
	assert.Empty(t, gotProjects)

	doc := storedPreferencesDoc(t, userID)
	assert.Contains(t, doc, "email_notification")
	assert.Contains(t, doc, "notifications")
}

// TestIntegrationAdminSavedFilters_PreferencesPutKeepsPresets: the user-facing
// PUT /preferences path (UpdatePreferences → Upsert) must not drop
// admin.saved_filters, while still fully replacing the keys it models.
func TestIntegrationAdminSavedFilters_PreferencesPutKeepsPresets(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserPreferencesRepository(integrationDB)
	svc := services.NewUserPreferencesService(repo)
	userID := insertTestUser(t)

	saved, _, err := svc.ReplaceAdminSavedFilters(ctx, userID, "projects", integrationPresets("Stale projects"), 0)
	require.NoError(t, err)

	notifications := models.NotificationPreferences{
		Channels: models.NotificationChannelPreferences{InApp: true, Email: false},
		Types:    map[string]models.NotificationTypePreference{"team.invitation": {InApp: false, Email: "none"}},
	}
	resp, err := svc.UpdatePreferences(ctx, userID, models.UpdatePreferencesRequest{
		EmailNotification: &models.EmailNotificationPreferences{MarketingPromotional: true},
		Notifications:     &notifications,
	})
	require.NoError(t, err)
	assert.Equal(t, notifications, resp.Preferences.Notifications)

	got, version, err := svc.GetAdminSavedFilters(ctx, userID, "projects")
	require.NoError(t, err)
	assert.Equal(t, saved, got, "PUT /preferences must not drop the admin presets")
	assert.EqualValues(t, 2, version, "the shared row version advanced with the /preferences write")

	// The modelled keys are still fully replaced: the default notification
	// types are gone, not merged with the new ones.
	prefs, err := svc.GetPreferences(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, notifications, prefs.Preferences.Notifications)
	assert.True(t, prefs.Preferences.EmailNotification.MarketingPromotional)
}

// TestIntegrationAdminSavedFilters_PerUser: one admin's presets are invisible
// to another.
func TestIntegrationAdminSavedFilters_PerUser(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserPreferencesRepository(integrationDB)
	adminA, adminB := insertTestUser(t), insertTestUser(t)

	_, err := repo.ReplaceAdminSavedFilters(ctx, adminA, "users", integrationPresets("A only"),
		models.DefaultPreferences(), 0)
	require.NoError(t, err)

	got, version, err := repo.GetAdminSavedFilters(ctx, adminB, "users")
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.EqualValues(t, 0, version)
}

// TestIntegrationAdminSavedFilters_LegacyNullVersion: a legacy row with a NULL
// version reads as 1 and accepts a write carrying 1.
func TestIntegrationAdminSavedFilters_LegacyNullVersion(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserPreferencesRepository(integrationDB)
	userID := insertTestUser(t)
	_, err := integrationDB.ExecContext(ctx,
		`INSERT INTO user_preferences (user_id, preferences, version) VALUES ($1, '{}'::jsonb, NULL)`, userID)
	require.NoError(t, err)

	_, version, err := repo.GetAdminSavedFilters(ctx, userID, "users")
	require.NoError(t, err)
	require.EqualValues(t, 1, version)

	newVersion, err := repo.ReplaceAdminSavedFilters(ctx, userID, "users", integrationPresets("A"),
		models.DefaultPreferences(), 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, newVersion)
}
