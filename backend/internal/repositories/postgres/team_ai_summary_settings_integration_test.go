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

// Behavior-level suite for TeamAISummarySettingsRepository against real Postgres
// (#1071). It asserts rows in/out, the (nil, nil) miss contract, the version
// bump on conflict, the ON DELETE CASCADE from teams, the ON DELETE SET NULL
// from model_providers, and that each CHECK constraint rejects its degenerate
// input — never SQL text.

// resetTeamAISummarySettingsTables clears this suite's tables. team_ai_summary_
// settings hangs off both teams and model_providers, so this suite truncates
// its own chain rather than relying on the shared resetIntegrationTables.
func resetTeamAISummarySettingsTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, model_providers, team_ai_summary_settings CASCADE")
	require.NoError(t, err)
}

// insertIntegrationModelProvider seeds a model_providers row for teamID and
// returns its id — the FK target for team_ai_summary_settings.model_provider_id.
func insertIntegrationModelProvider(t *testing.T, teamID string) string {
	t.Helper()
	var id string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`INSERT INTO model_providers (team_id, name, provider_type, model)
		 VALUES ($1, $2, 'openai', 'gpt-4o-mini') RETURNING id`,
		teamID, "provider-"+uuid.New().String()).Scan(&id))
	return id
}

func integrationTeamAISummarySettings(teamID string) *models.TeamAISummarySettings {
	return &models.TeamAISummarySettings{
		TeamID:          teamID,
		Enabled:         true,
		TopN:            7,
		Style:           models.AISummaryStyleDetailed,
		MaxOutputTokens: 1200,
	}
}

func TestIntegrationTeamAISummarySettings_Get_NoRow(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)

	got, err := repo.Get(context.Background(), uuid.New().String())

	require.NoError(t, err)
	assert.Nil(t, got, "no override row must yield (nil, nil), not an error")
}

func TestIntegrationTeamAISummarySettings_Upsert_InsertRoundTrip(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)
	providerID := insertIntegrationModelProvider(t, teamID)

	settings := integrationTeamAISummarySettings(teamID)
	settings.ModelProviderID = &providerID
	require.NoError(t, repo.Upsert(context.Background(), settings))
	assert.Equal(t, int64(1), settings.Version)
	assert.False(t, settings.CreatedAt.IsZero())

	got, err := repo.Get(context.Background(), teamID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, teamID, got.TeamID)
	assert.True(t, got.Enabled)
	require.NotNil(t, got.ModelProviderID)
	assert.Equal(t, providerID, *got.ModelProviderID)
	assert.Equal(t, 7, got.TopN)
	assert.Equal(t, models.AISummaryStyleDetailed, got.Style)
	assert.Equal(t, 1200, got.MaxOutputTokens)
	assert.Equal(t, int64(1), got.Version)
}

// A nil ModelProviderID must land as SQL NULL and read back as nil, not as an
// error or an empty string: NULL is the value "use the team default provider".
func TestIntegrationTeamAISummarySettings_Upsert_NilProviderRoundTrips(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)

	require.NoError(t, repo.Upsert(context.Background(), integrationTeamAISummarySettings(teamID)))

	got, err := repo.Get(context.Background(), teamID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.ModelProviderID)
}

func TestIntegrationTeamAISummarySettings_Upsert_ConflictUpdatesAndBumpsVersion(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)
	ctx := context.Background()

	first := integrationTeamAISummarySettings(teamID)
	require.NoError(t, repo.Upsert(ctx, first))

	// Read the stored row back rather than trusting the struct Upsert mutated:
	// afterFirst.UpdatedAt is the baseline the second upsert must advance past.
	// Comparing against created_at instead would be vacuous — the insert sets
	// created_at = updated_at, so an ON CONFLICT clause that never rewrote
	// updated_at would still satisfy it.
	afterFirst, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	require.NotNil(t, afterFirst)

	second := integrationTeamAISummarySettings(teamID)
	second.Enabled = false
	second.TopN = 2
	second.Style = models.AISummaryStyleConcise
	second.MaxOutputTokens = 200
	require.NoError(t, repo.Upsert(ctx, second), "second Upsert must update, not conflict")

	assert.Equal(t, int64(2), second.Version, "version must increment on conflict")
	assert.True(t, second.CreatedAt.Equal(afterFirst.CreatedAt), "created_at must survive the update")

	got, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, teamID, got.TeamID, "team_id stays the identity across upserts")
	assert.False(t, got.Enabled)
	assert.Equal(t, 2, got.TopN)
	assert.Equal(t, models.AISummaryStyleConcise, got.Style)
	assert.Equal(t, int64(2), got.Version)
	assert.True(t, got.UpdatedAt.After(afterFirst.UpdatedAt),
		"updated_at must advance past the previous write, not merely match created_at")
	assert.True(t, got.CreatedAt.Equal(afterFirst.CreatedAt), "created_at must not move")
}

func TestIntegrationTeamAISummarySettings_Delete(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, integrationTeamAISummarySettings(teamID)))
	require.NoError(t, repo.Delete(ctx, teamID))

	got, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	assert.Nil(t, got, "after Delete the team inherits the instance defaults again")
}

func TestIntegrationTeamAISummarySettings_Delete_MissingRowIsNoOp(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)

	assert.NoError(t, repo.Delete(context.Background(), uuid.New().String()))
}

func TestIntegrationTeamAISummarySettings_TeamDeleteCascades(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, integrationTeamAISummarySettings(teamID)))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	require.NoError(t, err)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM team_ai_summary_settings WHERE team_id = $1", teamID).Scan(&count))
	assert.Equal(t, 0, count, "deleting the owning team must cascade the settings row away")
}

// The headline FK decision: deleting a provider must NOT delete the team's whole
// summary profile (CASCADE) and must NOT leave a dangling id (no FK) — it nulls
// the column, which the rest of the stack already reads as "the team default".
func TestIntegrationTeamAISummarySettings_ProviderDeleteNullsTheColumn(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)
	providerID := insertIntegrationModelProvider(t, teamID)
	ctx := context.Background()

	settings := integrationTeamAISummarySettings(teamID)
	settings.ModelProviderID = &providerID
	require.NoError(t, repo.Upsert(ctx, settings))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM model_providers WHERE id = $1", providerID)
	require.NoError(t, err, "the settings row must not block deleting a provider")

	got, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	require.NotNil(t, got, "the settings row must SURVIVE the provider deletion, not cascade away")
	assert.Nil(t, got.ModelProviderID, "the dangling reference must become NULL, not stay orphaned")
	assert.Equal(t, 7, got.TopN, "the rest of the profile must be untouched")
}

func TestIntegrationTeamAISummarySettings_Upsert_RejectsUnknownTeam(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)

	err := repo.Upsert(context.Background(), integrationTeamAISummarySettings(uuid.New().String()))

	assert.Error(t, err, "the teams FK must reject settings for a team that does not exist")
}

func TestIntegrationTeamAISummarySettings_Upsert_RejectsUnknownProvider(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)

	unknown := uuid.New().String()
	settings := integrationTeamAISummarySettings(teamID)
	settings.ModelProviderID = &unknown

	err := repo.Upsert(context.Background(), settings)

	assert.Error(t, err, "the model_providers FK must reject a provider that does not exist")
}

// invalidProfiles enumerates one violation per CHECK constraint on the table.
// Each must be rejected by Postgres, mirroring services.ValidateAISummarySettings.
func invalidProfiles() map[string]func(*models.TeamAISummarySettings) {
	return map[string]func(*models.TeamAISummarySettings){
		"zero top_n":                 func(s *models.TeamAISummarySettings) { s.TopN = 0 },
		"negative top_n":             func(s *models.TeamAISummarySettings) { s.TopN = -1 },
		"top_n above the ceiling":    func(s *models.TeamAISummarySettings) { s.TopN = 11 },
		"zero max_output_tokens":     func(s *models.TeamAISummarySettings) { s.MaxOutputTokens = 0 },
		"negative max_output_tokens": func(s *models.TeamAISummarySettings) { s.MaxOutputTokens = -1 },
		"style outside the vocabulary": func(s *models.TeamAISummarySettings) {
			s.Style = "verbose"
		},
	}
}

func TestIntegrationTeamAISummarySettings_Upsert_RejectsInvalidProfiles(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)

	for name, degrade := range invalidProfiles() {
		t.Run(name, func(t *testing.T) {
			settings := integrationTeamAISummarySettings(teamID)
			degrade(settings)

			err := repo.Upsert(context.Background(), settings)

			assert.Error(t, err, "a CHECK constraint must reject this profile")
		})
	}
}

// top_n = 10 is the inclusive ceiling the CHECK and config.MaxAISummaryTopN
// share; it must be accepted, not merely "not obviously rejected".
func TestIntegrationTeamAISummarySettings_Upsert_AcceptsTopNAtTheCeiling(t *testing.T) {
	resetTeamAISummarySettingsTables(t)
	repo := NewTeamAISummarySettingsRepository(integrationDB)
	teamID := newIntegrationTeam(t)

	settings := integrationTeamAISummarySettings(teamID)
	settings.TopN = 10

	assert.NoError(t, repo.Upsert(context.Background(), settings))
}
