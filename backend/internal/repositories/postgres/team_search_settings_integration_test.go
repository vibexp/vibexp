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

// Behavior-level suite for TeamSearchSettingsRepository against real Postgres
// (#488). It asserts rows in/out, the (nil, nil) miss contract, the version
// bump on conflict, the ON DELETE CASCADE from teams, and that each CHECK
// constraint rejects its degenerate input — never SQL text.

// resetTeamSearchSettingsTables clears this suite's tables. The shared
// resetIntegrationTables does not name teams, and team_search_settings hangs
// off teams rather than users, so this suite truncates its own chain.
func resetTeamSearchSettingsTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, team_search_settings CASCADE")
	require.NoError(t, err)
}

// newIntegrationTeam seeds a users row and a teams row owned by it, returning
// the team ID — the FK target for team_search_settings.
func newIntegrationTeam(t *testing.T) string {
	t.Helper()
	return insertTestTeam(t, insertTestUser(t))
}

func integrationTeamSearchSettings(teamID string) *models.TeamSearchSettings {
	return &models.TeamSearchSettings{
		TeamID:                teamID,
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.7,
		RankWeightCreated:     0.2,
		RankWeightUpdated:     0.1,
		RankHalfLifeDays:      30,
	}
}

func TestIntegrationTeamSearchSettings_Get_NoRow(t *testing.T) {
	resetTeamSearchSettingsTables(t)
	repo := NewTeamSearchSettingsRepository(integrationDB)

	got, err := repo.Get(context.Background(), uuid.New().String())

	require.NoError(t, err)
	assert.Nil(t, got, "no override row must yield (nil, nil), not an error")
}

func TestIntegrationTeamSearchSettings_Upsert_InsertRoundTrip(t *testing.T) {
	resetTeamSearchSettingsTables(t)
	repo := NewTeamSearchSettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)

	settings := integrationTeamSearchSettings(teamID)
	require.NoError(t, repo.Upsert(context.Background(), settings))
	assert.Equal(t, int64(1), settings.Version)
	assert.False(t, settings.CreatedAt.IsZero())

	got, err := repo.Get(context.Background(), teamID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, teamID, got.TeamID)
	assert.True(t, got.RecencyRankingEnabled)
	assert.InDelta(t, 0.7, got.RankWeightRelevance, 1e-9)
	assert.InDelta(t, 0.2, got.RankWeightCreated, 1e-9)
	assert.InDelta(t, 0.1, got.RankWeightUpdated, 1e-9)
	assert.InDelta(t, 30.0, got.RankHalfLifeDays, 1e-9)
	assert.Equal(t, int64(1), got.Version)
}

func TestIntegrationTeamSearchSettings_Upsert_ConflictUpdatesAndBumpsVersion(t *testing.T) {
	resetTeamSearchSettingsTables(t)
	repo := NewTeamSearchSettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)
	ctx := context.Background()

	first := integrationTeamSearchSettings(teamID)
	require.NoError(t, repo.Upsert(ctx, first))

	second := integrationTeamSearchSettings(teamID)
	second.RecencyRankingEnabled = false
	second.RankWeightRelevance = 1
	second.RankWeightCreated = 0
	second.RankWeightUpdated = 0
	second.RankHalfLifeDays = 90
	require.NoError(t, repo.Upsert(ctx, second), "second Upsert must update, not conflict")

	assert.Equal(t, int64(2), second.Version, "version must increment on conflict")
	assert.Equal(t, first.CreatedAt, second.CreatedAt, "created_at must survive the update")

	got, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, teamID, got.TeamID, "team_id stays the identity across upserts")
	assert.False(t, got.RecencyRankingEnabled)
	assert.InDelta(t, 90.0, got.RankHalfLifeDays, 1e-9)
	assert.Equal(t, int64(2), got.Version)
	assert.True(t, got.UpdatedAt.After(got.CreatedAt) || got.UpdatedAt.Equal(got.CreatedAt),
		"updated_at must advance to at least created_at")
}

func TestIntegrationTeamSearchSettings_Delete(t *testing.T) {
	resetTeamSearchSettingsTables(t)
	repo := NewTeamSearchSettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, integrationTeamSearchSettings(teamID)))
	require.NoError(t, repo.Delete(ctx, teamID))

	got, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	assert.Nil(t, got, "after Delete the team inherits the instance defaults again")
}

func TestIntegrationTeamSearchSettings_Delete_MissingRowIsNoOp(t *testing.T) {
	resetTeamSearchSettingsTables(t)
	repo := NewTeamSearchSettingsRepository(integrationDB)

	assert.NoError(t, repo.Delete(context.Background(), uuid.New().String()))
}

func TestIntegrationTeamSearchSettings_TeamDeleteCascades(t *testing.T) {
	resetTeamSearchSettingsTables(t)
	repo := NewTeamSearchSettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, integrationTeamSearchSettings(teamID)))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	require.NoError(t, err)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM team_search_settings WHERE team_id = $1", teamID).Scan(&count))
	assert.Equal(t, 0, count, "deleting the owning team must cascade the settings row away")
}

func TestIntegrationTeamSearchSettings_Upsert_RejectsUnknownTeam(t *testing.T) {
	resetTeamSearchSettingsTables(t)
	repo := NewTeamSearchSettingsRepository(integrationDB)

	err := repo.Upsert(context.Background(), integrationTeamSearchSettings(uuid.New().String()))

	assert.Error(t, err, "the teams FK must reject settings for a team that does not exist")
}

// degenerateProfiles enumerates one violation per CHECK constraint on the
// table. Each must be rejected by Postgres, mirroring validateSearchRankingConfig.
func degenerateProfiles() map[string]func(*models.TeamSearchSettings) {
	return map[string]func(*models.TeamSearchSettings){
		"negative relevance weight": func(s *models.TeamSearchSettings) { s.RankWeightRelevance = -0.1 },
		"negative created weight":   func(s *models.TeamSearchSettings) { s.RankWeightCreated = -1 },
		"negative updated weight":   func(s *models.TeamSearchSettings) { s.RankWeightUpdated = -1 },
		"all weights zero": func(s *models.TeamSearchSettings) {
			s.RankWeightRelevance, s.RankWeightCreated, s.RankWeightUpdated = 0, 0, 0
		},
		"zero half-life":     func(s *models.TeamSearchSettings) { s.RankHalfLifeDays = 0 },
		"negative half-life": func(s *models.TeamSearchSettings) { s.RankHalfLifeDays = -1 },
		"half-life above the ceiling": func(s *models.TeamSearchSettings) {
			s.RankHalfLifeDays = 36501
		},
	}
}

func TestIntegrationTeamSearchSettings_Upsert_RejectsDegenerateProfiles(t *testing.T) {
	resetTeamSearchSettingsTables(t)
	repo := NewTeamSearchSettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)

	for name, degrade := range degenerateProfiles() {
		t.Run(name, func(t *testing.T) {
			settings := integrationTeamSearchSettings(teamID)
			degrade(settings)

			err := repo.Upsert(context.Background(), settings)

			assert.Error(t, err, "a CHECK constraint must reject this profile")
		})
	}
}
