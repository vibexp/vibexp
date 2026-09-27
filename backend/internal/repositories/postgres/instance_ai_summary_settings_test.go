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

// --- Audited writes (#1199) ---
//
// The transaction mechanics (begin, lock, commit and their failures) live in
// runAuditedSingletonTx and are pinned by the instance search suite; these
// cases pin what is specific to this table: its lock, its read with the
// millisecond conversion, and its statements.

const (
	instanceAISummaryWriteLock  = "LOCK TABLE instance_ai_summary_settings IN SHARE ROW EXCLUSIVE MODE"
	lockedInstanceAISummaryRead = "SELECT (.+) FROM instance_ai_summary_settings"
)

var instanceAISummarySettingsColumns = []string{
	"enabled", "top_n", "style", "max_output_tokens", "per_document_chars",
	"total_context_chars", "request_timeout_ms", "created_at", "updated_at", "updated_by", "version",
}

// testAISummaryAudit builds a minimal, valid entry and records what it was given.
type testAISummaryAudit struct {
	calls         int
	before, after *models.InstanceAISummarySettings
	err           error
}

func (a *testAISummaryAudit) build(
	before, after *models.InstanceAISummarySettings,
) (*models.InstanceSettingsAuditEntry, error) {
	a.calls++
	a.before, a.after = before, after
	if a.err != nil {
		return nil, a.err
	}
	return &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingAISummary,
		Action:  models.InstanceSettingsAuditActionUpsert,
		After:   []byte(`{"top_n":5}`),
	}, nil
}

// expectLockedInstanceAISummaryRow expects the transaction start, its write
// lock and the current-row read returning one stored row.
func expectLockedInstanceAISummaryRow(mock sqlmock.Sqlmock, version int64) {
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec(instanceAISummaryWriteLock).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockedInstanceAISummaryRead).
		WillReturnRows(sqlmock.NewRows(instanceAISummarySettingsColumns).
			AddRow(false, 3, "concise", 600, 5000, 15000, int64(2500), now, now, nil, version))
}

// expectLockedNoInstanceAISummaryRow is expectLockedInstanceAISummaryRow with
// nothing stored.
func expectLockedNoInstanceAISummaryRow(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(instanceAISummaryWriteLock).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockedInstanceAISummaryRead).WillReturnError(sql.ErrNoRows)
}

func TestInstanceAISummarySettingsRepository_UpsertAudited(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()
	expectLockedInstanceAISummaryRow(mock, 1)
	mock.ExpectQuery("INSERT INTO instance_ai_summary_settings (.+) ON CONFLICT \\(id\\)\\s+DO UPDATE").
		WithArgs(true, 5, models.AISummaryStyleBalanced, 1024, 4000, 20000, int64(45000), nil).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, int64(2)))
	expectInstanceAuditInsert(mock)
	mock.ExpectCommit()

	audit := &testAISummaryAudit{}
	s := instanceAISummarySettingsFixture()
	require.NoError(t, NewInstanceAISummarySettingsRepository(db).UpsertAudited(context.Background(), s, audit.build))

	assert.Equal(t, 1, audit.calls)
	require.NotNil(t, audit.before, "the current row is handed over as before")
	assert.Equal(t, 2500*time.Millisecond, audit.before.RequestTimeout, "before is read with the ms conversion")
	assert.Same(t, s, audit.after, "after is the row as written")
	assert.Equal(t, int64(2), s.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceAISummarySettingsRepository_UpsertAudited_NoRowGivesNilBefore(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()
	expectLockedNoInstanceAISummaryRow(mock)
	mock.ExpectQuery("INSERT INTO instance_ai_summary_settings").
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, int64(1)))
	expectInstanceAuditInsert(mock)
	mock.ExpectCommit()

	audit := &testAISummaryAudit{}
	require.NoError(t, NewInstanceAISummarySettingsRepository(db).
		UpsertAudited(context.Background(), instanceAISummarySettingsFixture(), audit.build))

	assert.Nil(t, audit.before)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A failing audit append (or upsert) rolls the whole change back: the row is
// never written without its audit entry.
func TestInstanceAISummarySettingsRepository_UpsertAudited_FailuresRollBack(t *testing.T) {
	now := time.Now().UTC()
	cases := map[string]func(mock sqlmock.Sqlmock){
		"upsert fails": func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("INSERT INTO instance_ai_summary_settings").WillReturnError(errInstanceSettingsDB)
		},
		"audit insert fails": func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("INSERT INTO instance_ai_summary_settings").
				WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
					AddRow(now, now, int64(1)))
			mock.ExpectQuery("INSERT INTO instance_settings_audit").WillReturnError(errInstanceSettingsDB)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			db, mock := newInstanceSettingsMock(t)
			expectLockedNoInstanceAISummaryRow(mock)
			setup(mock)
			mock.ExpectRollback()

			err := NewInstanceAISummarySettingsRepository(db).
				UpsertAudited(context.Background(), instanceAISummarySettingsFixture(), (&testAISummaryAudit{}).build)

			assert.ErrorIs(t, err, errInstanceSettingsDB)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestInstanceAISummarySettingsRepository_DeleteAudited(t *testing.T) {
	t.Run("a stored row is deleted and audited", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		expectLockedInstanceAISummaryRow(mock, 3)
		mock.ExpectExec("DELETE FROM instance_ai_summary_settings").WillReturnResult(sqlmock.NewResult(0, 1))
		expectInstanceAuditInsert(mock)
		mock.ExpectCommit()

		audit := &testAISummaryAudit{}
		deleted, err := NewInstanceAISummarySettingsRepository(db).DeleteAudited(context.Background(), audit.build)

		require.NoError(t, err)
		assert.True(t, deleted)
		require.NotNil(t, audit.before)
		assert.Equal(t, int64(3), audit.before.Version)
		assert.Nil(t, audit.after)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no row writes nothing and builds no entry", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		expectLockedNoInstanceAISummaryRow(mock)
		mock.ExpectRollback()

		audit := &testAISummaryAudit{}
		deleted, err := NewInstanceAISummarySettingsRepository(db).DeleteAudited(context.Background(), audit.build)

		require.NoError(t, err)
		assert.False(t, deleted)
		assert.Zero(t, audit.calls)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a delete failure rolls back", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		expectLockedInstanceAISummaryRow(mock, 3)
		mock.ExpectExec("DELETE FROM instance_ai_summary_settings").WillReturnError(errInstanceSettingsDB)
		mock.ExpectRollback()

		deleted, err := NewInstanceAISummarySettingsRepository(db).
			DeleteAudited(context.Background(), (&testAISummaryAudit{}).build)

		assert.ErrorIs(t, err, errInstanceSettingsDB)
		assert.False(t, deleted)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
