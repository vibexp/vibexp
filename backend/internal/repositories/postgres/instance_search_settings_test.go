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
	"github.com/vibexp/vibexp/internal/repositories"
)

// Statement-level suite for InstanceSearchSettingsRepository: the error paths
// and argument plumbing. What the database does with the statements (the
// singleton, the CHECKs, ON CONFLICT behaviour) is pinned by the integration
// suite.

var errInstanceSettingsDB = errors.New("connection reset")

func newInstanceSettingsMock(t *testing.T) (*database.DB, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	return &database.DB{DB: mockDB}, mock
}

func TestInstanceSearchSettingsRepository_Get(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	repo := NewInstanceSearchSettingsRepository(db)
	now := time.Now().UTC()
	editor := "11111111-1111-1111-1111-111111111111"

	mock.ExpectQuery("SELECT (.+) FROM instance_search_settings").
		WillReturnRows(sqlmock.NewRows([]string{
			"recency_ranking_enabled", "rank_weight_relevance", "rank_weight_created",
			"rank_weight_updated", "rank_half_life_days", "rank_candidate_cap",
			"created_at", "updated_at", "updated_by", "version",
		}).AddRow(true, 0.7, 0.1, 0.2, 30.0, 200, now, now, editor, int64(4)))

	got, err := repo.Get(context.Background())

	require.NoError(t, err)
	assert.True(t, got.RecencyRankingEnabled)
	assert.InDelta(t, 0.7, got.RankWeightRelevance, 1e-9)
	assert.InDelta(t, 30.0, got.RankHalfLifeDays, 1e-9)
	assert.Equal(t, 200, got.RankCandidateCap)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, editor, *got.UpdatedBy)
	assert.Equal(t, int64(4), got.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSearchSettingsRepository_Get_Errors(t *testing.T) {
	t.Run("no row maps to the sentinel", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery("SELECT (.+) FROM instance_search_settings").WillReturnError(sql.ErrNoRows)

		got, err := NewInstanceSearchSettingsRepository(db).Get(context.Background())

		assert.Nil(t, got)
		assert.ErrorIs(t, err, repositories.ErrInstanceSearchSettingsNotFound)
	})

	t.Run("a database failure is not reported as not-found", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery("SELECT (.+) FROM instance_search_settings").WillReturnError(errInstanceSettingsDB)

		_, err := NewInstanceSearchSettingsRepository(db).Get(context.Background())

		assert.ErrorIs(t, err, errInstanceSettingsDB)
		assert.NotErrorIs(t, err, repositories.ErrInstanceSearchSettingsNotFound)
	})
}

func instanceSearchSettingsFixture() *models.InstanceSearchSettings {
	return &models.InstanceSearchSettings{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.7,
		RankWeightCreated:     0.1,
		RankWeightUpdated:     0.2,
		RankHalfLifeDays:      30,
		RankCandidateCap:      200,
	}
}

func TestInstanceSearchSettingsRepository_Upsert(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()

	mock.ExpectQuery("INSERT INTO instance_search_settings (.+) ON CONFLICT \\(id\\)\\s+DO UPDATE").
		WithArgs(true, 0.7, 0.1, 0.2, 30.0, 200, nil).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, int64(2)))

	s := instanceSearchSettingsFixture()
	require.NoError(t, NewInstanceSearchSettingsRepository(db).Upsert(context.Background(), s))

	assert.Equal(t, int64(2), s.Version)
	assert.True(t, s.CreatedAt.Equal(now))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSearchSettingsRepository_InsertIfAbsent(t *testing.T) {
	const query = "INSERT INTO instance_search_settings (.+) ON CONFLICT \\(id\\) DO NOTHING"

	t.Run("a returned row means inserted", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		now := time.Now().UTC()
		mock.ExpectQuery(query).
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
				AddRow(now, now, int64(1)))

		s := instanceSearchSettingsFixture()
		inserted, err := NewInstanceSearchSettingsRepository(db).InsertIfAbsent(context.Background(), s)

		require.NoError(t, err)
		assert.True(t, inserted)
		assert.Equal(t, int64(1), s.Version)
	})

	t.Run("no returned row means a row already exists", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery(query).WillReturnError(sql.ErrNoRows)

		inserted, err := NewInstanceSearchSettingsRepository(db).
			InsertIfAbsent(context.Background(), instanceSearchSettingsFixture())

		require.NoError(t, err)
		assert.False(t, inserted)
	})

	t.Run("a database failure is propagated", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery(query).WillReturnError(errInstanceSettingsDB)

		inserted, err := NewInstanceSearchSettingsRepository(db).
			InsertIfAbsent(context.Background(), instanceSearchSettingsFixture())

		assert.False(t, inserted)
		assert.ErrorIs(t, err, errInstanceSettingsDB)
	})
}

func TestInstanceSearchSettingsRepository_WriteErrors(t *testing.T) {
	t.Run("Upsert", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery("INSERT INTO instance_search_settings").WillReturnError(errInstanceSettingsDB)

		err := NewInstanceSearchSettingsRepository(db).Upsert(context.Background(), instanceSearchSettingsFixture())

		assert.ErrorIs(t, err, errInstanceSettingsDB)
	})

	t.Run("Delete", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectExec("DELETE FROM instance_search_settings").WillReturnError(errInstanceSettingsDB)

		assert.ErrorIs(t, NewInstanceSearchSettingsRepository(db).Delete(context.Background()), errInstanceSettingsDB)
	})
}

func TestInstanceSearchSettingsRepository_Delete_NoRowIsNotAnError(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	mock.ExpectExec("DELETE FROM instance_search_settings").WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, NewInstanceSearchSettingsRepository(db).Delete(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}
