package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// teamFreshnessSettingsCols mirrors the teamFreshnessSettingsColumns
// projection order.
func teamFreshnessSettingsCols() []string {
	return []string{
		"team_id", "interval_seconds", "reversibility_enabled",
		"created_at", "updated_at", "version",
	}
}

func setupTeamFreshnessSettingsTest(t *testing.T) (*TeamFreshnessSettingsRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewTeamFreshnessSettingsRepository(&database.DB{DB: mockDB})
	return repo.(*TeamFreshnessSettingsRepository), mock
}

func TestTeamFreshnessSettingsRepository_Get_Found(t *testing.T) {
	repo, mock := setupTeamFreshnessSettingsTest(t)
	now := time.Now().UTC()

	mock.ExpectQuery(`FROM team_freshness_settings WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows(teamFreshnessSettingsCols()).
			AddRow("team-1", 7200, false, now, now, int64(4)))

	got, err := repo.Get(context.Background(), "team-1")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "team-1", got.TeamID)
	assert.Equal(t, 7200, got.IntervalSeconds)
	assert.False(t, got.ReversibilityEnabled)
	assert.Equal(t, int64(4), got.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An absent row means "inherit the defaults", which callers detect from a nil
// result — so it must not be an error.
func TestTeamFreshnessSettingsRepository_Get_NoRowReturnsNilNil(t *testing.T) {
	repo, mock := setupTeamFreshnessSettingsTest(t)

	mock.ExpectQuery(`FROM team_freshness_settings`).WillReturnError(sql.ErrNoRows)

	got, err := repo.Get(context.Background(), "team-1")

	require.NoError(t, err)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamFreshnessSettingsRepository_Get_Error(t *testing.T) {
	repo, mock := setupTeamFreshnessSettingsTest(t)

	mock.ExpectQuery(`FROM team_freshness_settings`).WillReturnError(errors.New("boom"))

	_, err := repo.Get(context.Background(), "team-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get team freshness settings")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The upsert must increment version from the stored row (not from whatever the
// caller happened to hold) and hand the new value back.
func TestTeamFreshnessSettingsRepository_Upsert_IncrementsVersion(t *testing.T) {
	repo, mock := setupTeamFreshnessSettingsTest(t)
	now := time.Now().UTC()

	mock.ExpectQuery(
		`INSERT INTO team_freshness_settings .* ON CONFLICT \(team_id\) DO UPDATE SET `+
			`interval_seconds = EXCLUDED\.interval_seconds, `+
			`reversibility_enabled = EXCLUDED\.reversibility_enabled, `+
			`updated_at = now\(\), version = team_freshness_settings\.version \+ 1 RETURNING`,
	).
		WithArgs("team-1", 3600, true).
		WillReturnRows(sqlmock.NewRows(teamFreshnessSettingsCols()).
			AddRow("team-1", 3600, true, now, now, int64(9)))

	settings := &models.TeamFreshnessSettings{
		TeamID: "team-1", IntervalSeconds: 3600, ReversibilityEnabled: true, Version: 1,
	}
	require.NoError(t, repo.Upsert(context.Background(), settings))

	assert.Equal(t, int64(9), settings.Version, "version comes back from the stored row")
	assert.False(t, settings.CreatedAt.IsZero())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamFreshnessSettingsRepository_Upsert_Error(t *testing.T) {
	repo, mock := setupTeamFreshnessSettingsTest(t)

	mock.ExpectQuery(`INSERT INTO team_freshness_settings`).WillReturnError(errors.New("boom"))

	err := repo.Upsert(context.Background(), &models.TeamFreshnessSettings{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upsert team freshness settings")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamFreshnessSettingsRepository_Delete(t *testing.T) {
	repo, mock := setupTeamFreshnessSettingsTest(t)

	mock.ExpectExec(`DELETE FROM team_freshness_settings WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.Delete(context.Background(), "team-1"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Reset-to-defaults is idempotent: deleting settings a team never had is a
// no-op, not an error.
func TestTeamFreshnessSettingsRepository_Delete_MissingRowIsNoOp(t *testing.T) {
	repo, mock := setupTeamFreshnessSettingsTest(t)

	mock.ExpectExec(`DELETE FROM team_freshness_settings`).
		WithArgs("team-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, repo.Delete(context.Background(), "team-1"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamFreshnessSettingsRepository_Delete_Error(t *testing.T) {
	repo, mock := setupTeamFreshnessSettingsTest(t)

	mock.ExpectExec(`DELETE FROM team_freshness_settings`).WillReturnError(errors.New("boom"))

	err := repo.Delete(context.Background(), "team-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete team freshness settings")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDefaultTeamFreshnessSettings(t *testing.T) {
	got := models.DefaultTeamFreshnessSettings("team-1")

	assert.Equal(t, "team-1", got.TeamID)
	assert.Equal(t, models.DefaultFreshnessIntervalSeconds, got.IntervalSeconds)
	assert.True(t, got.ReversibilityEnabled)
	assert.GreaterOrEqual(t, got.IntervalSeconds, models.MinFreshnessIntervalSeconds,
		"the default interval must satisfy the schema's CHECK, or the first write of an "+
			"untouched team's defaults would be rejected by the database")
}
