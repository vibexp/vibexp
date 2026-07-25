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

const selectTeamSearchSettingsQuery = `SELECT team_id, recency_ranking_enabled, ` +
	`rank_weight_relevance, rank_weight_created, rank_weight_updated, rank_half_life_days, ` +
	`created_at, updated_at, version FROM team_search_settings WHERE team_id = \$1`

// teamSearchSettingsColumns mirrors the SELECT projection order.
func teamSearchSettingsColumns() []string {
	return []string{
		"team_id", "recency_ranking_enabled", "rank_weight_relevance", "rank_weight_created",
		"rank_weight_updated", "rank_half_life_days", "created_at", "updated_at", "version",
	}
}

func setupTeamSearchSettingsTest(t *testing.T) (*TeamSearchSettingsRepository, sqlmock.Sqlmock) {
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

	return NewTeamSearchSettingsRepository(&database.DB{DB: mockDB}), mock
}

func TestTeamSearchSettingsRepository_Get_Found(t *testing.T) {
	repo, mock := setupTeamSearchSettingsTest(t)
	now := time.Now()

	rows := sqlmock.NewRows(teamSearchSettingsColumns()).
		AddRow("team-123", true, 0.5, 0.3, 0.2, 30.0, now, now, int64(3))
	mock.ExpectQuery(selectTeamSearchSettingsQuery).WithArgs("team-123").WillReturnRows(rows)

	got, err := repo.Get(context.Background(), "team-123")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "team-123", got.TeamID)
	assert.True(t, got.RecencyRankingEnabled)
	assert.InDelta(t, 0.5, got.RankWeightRelevance, 1e-9)
	assert.InDelta(t, 0.3, got.RankWeightCreated, 1e-9)
	assert.InDelta(t, 0.2, got.RankWeightUpdated, 1e-9)
	assert.InDelta(t, 30.0, got.RankHalfLifeDays, 1e-9)
	assert.Equal(t, int64(3), got.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSearchSettingsRepository_Get_NoRowReturnsNilNil(t *testing.T) {
	repo, mock := setupTeamSearchSettingsTest(t)

	mock.ExpectQuery(selectTeamSearchSettingsQuery).
		WithArgs("team-missing").
		WillReturnError(sql.ErrNoRows)

	got, err := repo.Get(context.Background(), "team-missing")

	assert.NoError(t, err, "a missing row must not be an error — callers fall back to instance defaults")
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSearchSettingsRepository_Get_DatabaseError(t *testing.T) {
	repo, mock := setupTeamSearchSettingsTest(t)

	mock.ExpectQuery(selectTeamSearchSettingsQuery).
		WithArgs("team-err").
		WillReturnError(sql.ErrConnDone)

	got, err := repo.Get(context.Background(), "team-err")

	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func sampleTeamSearchSettings(teamID string) *models.TeamSearchSettings {
	return &models.TeamSearchSettings{
		TeamID:                teamID,
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.6,
		RankWeightCreated:     0.25,
		RankWeightUpdated:     0.15,
		RankHalfLifeDays:      45,
	}
}

func TestTeamSearchSettingsRepository_Upsert_PopulatesCreatedAtAndVersion(t *testing.T) {
	repo, mock := setupTeamSearchSettingsTest(t)
	created := time.Now().Add(-time.Hour)

	mock.ExpectQuery(`INSERT INTO team_search_settings`).
		WithArgs("team-1", true, 0.6, 0.25, 0.15, 45.0, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "version"}).AddRow(created, int64(2)))

	settings := sampleTeamSearchSettings("team-1")
	err := repo.Upsert(context.Background(), settings)

	require.NoError(t, err)
	assert.Equal(t, created, settings.CreatedAt)
	assert.Equal(t, int64(2), settings.Version)
	assert.False(t, settings.UpdatedAt.IsZero(), "Upsert must stamp UpdatedAt on the passed struct")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSearchSettingsRepository_Upsert_DatabaseError(t *testing.T) {
	repo, mock := setupTeamSearchSettingsTest(t)

	mock.ExpectQuery(`INSERT INTO team_search_settings`).
		WithArgs("team-1", true, 0.6, 0.25, 0.15, 45.0, sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	err := repo.Upsert(context.Background(), sampleTeamSearchSettings("team-1"))

	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSearchSettingsRepository_Delete(t *testing.T) {
	repo, mock := setupTeamSearchSettingsTest(t)

	mock.ExpectExec(`DELETE FROM team_search_settings WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	assert.NoError(t, repo.Delete(context.Background(), "team-1"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSearchSettingsRepository_Delete_DatabaseError(t *testing.T) {
	repo, mock := setupTeamSearchSettingsTest(t)

	mock.ExpectExec(`DELETE FROM team_search_settings WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnError(sql.ErrConnDone)

	assert.ErrorIs(t, repo.Delete(context.Background(), "team-1"), sql.ErrConnDone)
	assert.NoError(t, mock.ExpectationsWereMet())
}
