package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

const selectTeamAISummarySettingsQuery = `SELECT team_id, enabled, model_provider_id, top_n, ` +
	`style, max_output_tokens, created_at, updated_at, version ` +
	`FROM team_ai_summary_settings WHERE team_id = \$1`

// teamAISummarySettingsColumns mirrors the SELECT projection order.
func teamAISummarySettingsColumns() []string {
	return []string{
		"team_id", "enabled", "model_provider_id", "top_n", "style", "max_output_tokens",
		"created_at", "updated_at", "version",
	}
}

func setupTeamAISummarySettingsTest(t *testing.T) (*TeamAISummarySettingsRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		// Registered here rather than at setup: sqlmock matches expectations in
		// order, so an ExpectClose declared up front would be expected before
		// the test's own query.
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	return NewTeamAISummarySettingsRepository(&database.DB{DB: mockDB}), mock
}

func TestTeamAISummarySettingsRepository_Get_Found(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)
	now := time.Now()

	rows := sqlmock.NewRows(teamAISummarySettingsColumns()).
		AddRow("team-123", true, "provider-9", 7, models.AISummaryStyleDetailed, 1200, now, now, int64(3))
	mock.ExpectQuery(selectTeamAISummarySettingsQuery).WithArgs("team-123").WillReturnRows(rows)

	got, err := repo.Get(context.Background(), "team-123")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "team-123", got.TeamID)
	assert.True(t, got.Enabled)
	require.NotNil(t, got.ModelProviderID)
	assert.Equal(t, "provider-9", *got.ModelProviderID)
	assert.Equal(t, 7, got.TopN)
	assert.Equal(t, models.AISummaryStyleDetailed, got.Style)
	assert.Equal(t, 1200, got.MaxOutputTokens)
	assert.Equal(t, int64(3), got.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A NULL model_provider_id is a VALUE ("use the team default provider"), not an
// absence, so it must scan cleanly rather than erroring on a non-pointer dest.
func TestTeamAISummarySettingsRepository_Get_NullModelProviderScansAsNil(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)
	now := time.Now()

	rows := sqlmock.NewRows(teamAISummarySettingsColumns()).
		AddRow("team-123", true, nil, 5, models.AISummaryStyleBalanced, 800, now, now, int64(1))
	mock.ExpectQuery(selectTeamAISummarySettingsQuery).WithArgs("team-123").WillReturnRows(rows)

	got, err := repo.Get(context.Background(), "team-123")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.ModelProviderID, "a NULL provider must mean 'the team default', not an error")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamAISummarySettingsRepository_Get_NoRowReturnsNilNil(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)

	mock.ExpectQuery(selectTeamAISummarySettingsQuery).
		WithArgs("team-missing").
		WillReturnError(sql.ErrNoRows)

	got, err := repo.Get(context.Background(), "team-missing")

	assert.NoError(t, err, "a missing row must not be an error — callers fall back to instance defaults")
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamAISummarySettingsRepository_Get_DatabaseError(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)

	mock.ExpectQuery(selectTeamAISummarySettingsQuery).
		WithArgs("team-err").
		WillReturnError(sql.ErrConnDone)

	got, err := repo.Get(context.Background(), "team-err")

	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func sampleTeamAISummarySettings(teamID string) *models.TeamAISummarySettings {
	providerID := "provider-9"
	return &models.TeamAISummarySettings{
		TeamID:          teamID,
		Enabled:         true,
		ModelProviderID: &providerID,
		TopN:            7,
		Style:           models.AISummaryStyleDetailed,
		MaxOutputTokens: 1200,
	}
}

func TestTeamAISummarySettingsRepository_Upsert_PopulatesCreatedAtAndVersion(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)
	created := time.Now().Add(-time.Hour)

	mock.ExpectQuery(`INSERT INTO team_ai_summary_settings`).
		WithArgs("team-1", true, "provider-9", 7, models.AISummaryStyleDetailed, 1200, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "version"}).AddRow(created, int64(2)))

	settings := sampleTeamAISummarySettings("team-1")
	err := repo.Upsert(context.Background(), settings)

	require.NoError(t, err)
	assert.Equal(t, created, settings.CreatedAt)
	assert.Equal(t, int64(2), settings.Version)
	assert.False(t, settings.UpdatedAt.IsZero(), "Upsert must stamp UpdatedAt on the passed struct")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamAISummarySettingsRepository_Upsert_NilModelProviderBindsNull(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)

	mock.ExpectQuery(`INSERT INTO team_ai_summary_settings`).
		WithArgs("team-1", true, nil, 7, models.AISummaryStyleDetailed, 1200, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "version"}).AddRow(time.Now(), int64(1)))

	settings := sampleTeamAISummarySettings("team-1")
	settings.ModelProviderID = nil
	require.NoError(t, repo.Upsert(context.Background(), settings))

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamAISummarySettingsRepository_Upsert_DatabaseError(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)

	mock.ExpectQuery(`INSERT INTO team_ai_summary_settings`).
		WithArgs("team-1", true, "provider-9", 7, models.AISummaryStyleDetailed, 1200, sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	err := repo.Upsert(context.Background(), sampleTeamAISummarySettings("team-1"))

	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamAISummarySettingsRepository_Delete(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)

	mock.ExpectExec(`DELETE FROM team_ai_summary_settings WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	assert.NoError(t, repo.Delete(context.Background(), "team-1"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamAISummarySettingsRepository_Delete_DatabaseError(t *testing.T) {
	repo, mock := setupTeamAISummarySettingsTest(t)

	mock.ExpectExec(`DELETE FROM team_ai_summary_settings WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnError(sql.ErrConnDone)

	assert.ErrorIs(t, repo.Delete(context.Background(), "team-1"), sql.ErrConnDone)
	assert.NoError(t, mock.ExpectationsWereMet())
}
