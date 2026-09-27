package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Statement-level suite for InstanceAISummarySettingsRepository: the error
// paths, argument plumbing and the request_timeout_ms conversion. The database
// behaviour is pinned by the integration suite.

func instanceAISummarySettingsFixture() *models.InstanceAISummarySettings {
	return &models.InstanceAISummarySettings{
		Enabled:           true,
		TopN:              5,
		Style:             models.AISummaryStyleBalanced,
		MaxOutputTokens:   1024,
		PerDocumentChars:  4000,
		TotalContextChars: 20000,
		RequestTimeout:    45 * time.Second,
	}
}

func TestInstanceAISummarySettingsRepository_Get(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT (.+) FROM instance_ai_summary_settings").
		WillReturnRows(sqlmock.NewRows([]string{
			"enabled", "top_n", "style", "max_output_tokens", "per_document_chars",
			"total_context_chars", "request_timeout_ms", "created_at", "updated_at", "updated_by", "version",
		}).AddRow(true, 5, "concise", 1024, 4000, 20000, int64(1500), now, now, nil, int64(3)))

	got, err := NewInstanceAISummarySettingsRepository(db).Get(context.Background())

	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Equal(t, 5, got.TopN)
	assert.Equal(t, "concise", got.Style)
	assert.Equal(t, 20000, got.TotalContextChars)
	assert.Equal(t, 1500*time.Millisecond, got.RequestTimeout, "milliseconds convert to a Duration")
	assert.Nil(t, got.UpdatedBy)
	assert.Equal(t, int64(3), got.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceAISummarySettingsRepository_Get_Errors(t *testing.T) {
	t.Run("no row maps to the sentinel", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery("SELECT (.+) FROM instance_ai_summary_settings").WillReturnError(sql.ErrNoRows)

		got, err := NewInstanceAISummarySettingsRepository(db).Get(context.Background())

		assert.Nil(t, got)
		assert.ErrorIs(t, err, repositories.ErrInstanceAISummarySettingsNotFound)
	})

	t.Run("a database failure is not reported as not-found", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery("SELECT (.+) FROM instance_ai_summary_settings").WillReturnError(errInstanceSettingsDB)

		_, err := NewInstanceAISummarySettingsRepository(db).Get(context.Background())

		assert.ErrorIs(t, err, errInstanceSettingsDB)
		assert.NotErrorIs(t, err, repositories.ErrInstanceAISummarySettingsNotFound)
	})
}

func TestInstanceAISummarySettingsRepository_Upsert_StoresTimeoutInMilliseconds(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()
	editor := "11111111-1111-1111-1111-111111111111"

	mock.ExpectQuery("INSERT INTO instance_ai_summary_settings (.+) ON CONFLICT \\(id\\)\\s+DO UPDATE").
		WithArgs(true, 5, "balanced", 1024, 4000, 20000, int64(45000), &editor).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, int64(2)))

	s := instanceAISummarySettingsFixture()
	s.UpdatedBy = &editor
	require.NoError(t, NewInstanceAISummarySettingsRepository(db).Upsert(context.Background(), s))

	assert.Equal(t, int64(2), s.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceAISummarySettingsRepository_InsertIfAbsent(t *testing.T) {
	const query = "INSERT INTO instance_ai_summary_settings (.+) ON CONFLICT \\(id\\) DO NOTHING"

	t.Run("a returned row means inserted", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		now := time.Now().UTC()
		mock.ExpectQuery(query).
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
				AddRow(now, now, int64(1)))

		s := instanceAISummarySettingsFixture()
		inserted, err := NewInstanceAISummarySettingsRepository(db).InsertIfAbsent(context.Background(), s)

		require.NoError(t, err)
		assert.True(t, inserted)
		assert.Equal(t, int64(1), s.Version)
	})

	t.Run("no returned row means a row already exists", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery(query).WillReturnError(sql.ErrNoRows)

		inserted, err := NewInstanceAISummarySettingsRepository(db).
			InsertIfAbsent(context.Background(), instanceAISummarySettingsFixture())

		require.NoError(t, err)
		assert.False(t, inserted)
	})

	t.Run("a database failure is propagated", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery(query).WillReturnError(errInstanceSettingsDB)

		inserted, err := NewInstanceAISummarySettingsRepository(db).
			InsertIfAbsent(context.Background(), instanceAISummarySettingsFixture())

		assert.False(t, inserted)
		assert.ErrorIs(t, err, errInstanceSettingsDB)
	})
}

func TestInstanceAISummarySettingsRepository_WriteErrors(t *testing.T) {
	t.Run("Upsert", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery("INSERT INTO instance_ai_summary_settings").WillReturnError(errInstanceSettingsDB)

		err := NewInstanceAISummarySettingsRepository(db).
			Upsert(context.Background(), instanceAISummarySettingsFixture())

		assert.ErrorIs(t, err, errInstanceSettingsDB)
	})

	t.Run("Delete", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectExec("DELETE FROM instance_ai_summary_settings").WillReturnError(errInstanceSettingsDB)

		assert.ErrorIs(t, NewInstanceAISummarySettingsRepository(db).Delete(context.Background()),
			errInstanceSettingsDB)
	})
}

func TestInstanceAISummarySettingsRepository_Delete_NoRowIsNotAnError(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	mock.ExpectExec("DELETE FROM instance_ai_summary_settings").WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, NewInstanceAISummarySettingsRepository(db).Delete(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}
