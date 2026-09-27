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

// instanceSearchSettingsColumns is the Get / locked-read projection.
var instanceSearchSettingsColumns = []string{
	"recency_ranking_enabled", "rank_weight_relevance", "rank_weight_created",
	"rank_weight_updated", "rank_half_life_days", "rank_candidate_cap",
	"created_at", "updated_at", "updated_by", "version",
}

const lockedInstanceSearchRead = "SELECT (.+) FROM instance_search_settings FOR UPDATE"

// testSearchAudit builds a minimal, valid entry and records what it was given.
type testSearchAudit struct {
	calls         int
	before, after *models.InstanceSearchSettings
	err           error
}

func (a *testSearchAudit) build(before, after *models.InstanceSearchSettings) (*models.InstanceSettingsAuditEntry, error) {
	a.calls++
	a.before, a.after = before, after
	if a.err != nil {
		return nil, a.err
	}
	return &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingSearch,
		Action:  models.InstanceSettingsAuditActionUpsert,
		After:   []byte(`{"rank_candidate_cap":200}`),
	}, nil
}

func expectInstanceAuditInsert(mock sqlmock.Sqlmock) {
	now := time.Now().UTC()
	mock.ExpectQuery("INSERT INTO instance_settings_audit").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "setting", "action", "actor_user_id", "before", "after", "created_at",
		}).AddRow("aaaaaaaa-0000-0000-0000-000000000001", models.InstanceSettingSearch,
			models.InstanceSettingsAuditActionUpsert, nil, nil, []byte(`{}`), now))
}

func TestInstanceSearchSettingsRepository_UpsertAudited(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(lockedInstanceSearchRead).
		WillReturnRows(sqlmock.NewRows(instanceSearchSettingsColumns).
			AddRow(false, 0.5, 0.3, 0.2, 90.0, 200, now, now, nil, int64(1)))
	mock.ExpectQuery("INSERT INTO instance_search_settings (.+) ON CONFLICT \\(id\\)\\s+DO UPDATE").
		WithArgs(true, 0.7, 0.1, 0.2, 30.0, 200, nil).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, int64(2)))
	expectInstanceAuditInsert(mock)
	mock.ExpectCommit()

	audit := &testSearchAudit{}
	s := instanceSearchSettingsFixture()
	require.NoError(t, NewInstanceSearchSettingsRepository(db).UpsertAudited(context.Background(), s, audit.build))

	assert.Equal(t, 1, audit.calls)
	require.NotNil(t, audit.before, "the locked read is handed over as before")
	assert.InDelta(t, 90.0, audit.before.RankHalfLifeDays, 1e-9)
	assert.Same(t, s, audit.after, "after is the row as written")
	assert.Equal(t, int64(2), s.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSearchSettingsRepository_UpsertAudited_NoRowGivesNilBefore(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(lockedInstanceSearchRead).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("INSERT INTO instance_search_settings").
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, int64(1)))
	expectInstanceAuditInsert(mock)
	mock.ExpectCommit()

	audit := &testSearchAudit{}
	require.NoError(t, NewInstanceSearchSettingsRepository(db).
		UpsertAudited(context.Background(), instanceSearchSettingsFixture(), audit.build))

	assert.Nil(t, audit.before)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Every failure inside the transaction rolls the whole change back: there is
// never a settings write without its audit entry.
func TestInstanceSearchSettingsRepository_UpsertAudited_FailuresRollBack(t *testing.T) {
	now := time.Now().UTC()
	upsertOK := func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery("INSERT INTO instance_search_settings").
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
				AddRow(now, now, int64(1)))
	}
	errBuild := errors.New("snapshot failed")
	cases := []struct {
		name     string
		auditErr error
		setup    func(mock sqlmock.Sqlmock)
		wantErr  error
	}{
		{"locked read fails", nil, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(lockedInstanceSearchRead).WillReturnError(errInstanceSettingsDB)
		}, errInstanceSettingsDB},
		{"upsert fails", nil, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(lockedInstanceSearchRead).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("INSERT INTO instance_search_settings").WillReturnError(errInstanceSettingsDB)
		}, errInstanceSettingsDB},
		{"building the entry fails", errBuild, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(lockedInstanceSearchRead).WillReturnError(sql.ErrNoRows)
			upsertOK(mock)
		}, errBuild},
		{"audit insert fails", nil, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(lockedInstanceSearchRead).WillReturnError(sql.ErrNoRows)
			upsertOK(mock)
			mock.ExpectQuery("INSERT INTO instance_settings_audit").WillReturnError(errInstanceSettingsDB)
		}, errInstanceSettingsDB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newInstanceSettingsMock(t)
			mock.ExpectBegin()
			tc.setup(mock)
			mock.ExpectRollback()

			audit := &testSearchAudit{err: tc.auditErr}
			err := NewInstanceSearchSettingsRepository(db).
				UpsertAudited(context.Background(), instanceSearchSettingsFixture(), audit.build)

			assert.ErrorIs(t, err, tc.wantErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestInstanceSearchSettingsRepository_UpsertAudited_BeginAndCommitErrors(t *testing.T) {
	t.Run("begin", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectBegin().WillReturnError(errInstanceSettingsDB)

		err := NewInstanceSearchSettingsRepository(db).
			UpsertAudited(context.Background(), instanceSearchSettingsFixture(), (&testSearchAudit{}).build)

		assert.ErrorIs(t, err, errInstanceSettingsDB)
	})

	t.Run("commit", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		now := time.Now().UTC()
		mock.ExpectBegin()
		mock.ExpectQuery(lockedInstanceSearchRead).WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery("INSERT INTO instance_search_settings").
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
				AddRow(now, now, int64(1)))
		expectInstanceAuditInsert(mock)
		mock.ExpectCommit().WillReturnError(errInstanceSettingsDB)

		err := NewInstanceSearchSettingsRepository(db).
			UpsertAudited(context.Background(), instanceSearchSettingsFixture(), (&testSearchAudit{}).build)

		assert.ErrorIs(t, err, errInstanceSettingsDB)
	})
}

// A redaction failure in the built entry is refused before the audit insert and
// rolls the settings write back.
func TestInstanceSearchSettingsRepository_UpsertAudited_UnredactedEntryRollsBack(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(lockedInstanceSearchRead).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("INSERT INTO instance_search_settings").
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, int64(1)))
	mock.ExpectRollback()

	leaky := func(_, _ *models.InstanceSearchSettings) (*models.InstanceSettingsAuditEntry, error) {
		return &models.InstanceSettingsAuditEntry{
			Setting: models.InstanceSettingSearch,
			Action:  models.InstanceSettingsAuditActionUpsert,
			After:   []byte(`{"secret":"x"}`),
		}, nil
	}
	err := NewInstanceSearchSettingsRepository(db).
		UpsertAudited(context.Background(), instanceSearchSettingsFixture(), leaky)

	assert.ErrorIs(t, err, repositories.ErrInstanceSettingsAuditUnredacted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSearchSettingsRepository_DeleteAudited(t *testing.T) {
	t.Run("a stored row is deleted and audited", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		now := time.Now().UTC()
		mock.ExpectBegin()
		mock.ExpectQuery(lockedInstanceSearchRead).
			WillReturnRows(sqlmock.NewRows(instanceSearchSettingsColumns).
				AddRow(true, 0.7, 0.1, 0.2, 30.0, 200, now, now, nil, int64(3)))
		mock.ExpectExec("DELETE FROM instance_search_settings").WillReturnResult(sqlmock.NewResult(0, 1))
		expectInstanceAuditInsert(mock)
		mock.ExpectCommit()

		audit := &testSearchAudit{}
		deleted, err := NewInstanceSearchSettingsRepository(db).DeleteAudited(context.Background(), audit.build)

		require.NoError(t, err)
		assert.True(t, deleted)
		require.NotNil(t, audit.before)
		assert.Equal(t, int64(3), audit.before.Version)
		assert.Nil(t, audit.after)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no row writes nothing and builds no entry", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(lockedInstanceSearchRead).WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		audit := &testSearchAudit{}
		deleted, err := NewInstanceSearchSettingsRepository(db).DeleteAudited(context.Background(), audit.build)

		require.NoError(t, err)
		assert.False(t, deleted)
		assert.Zero(t, audit.calls)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a delete failure rolls back", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		now := time.Now().UTC()
		mock.ExpectBegin()
		mock.ExpectQuery(lockedInstanceSearchRead).
			WillReturnRows(sqlmock.NewRows(instanceSearchSettingsColumns).
				AddRow(true, 0.7, 0.1, 0.2, 30.0, 200, now, now, nil, int64(3)))
		mock.ExpectExec("DELETE FROM instance_search_settings").WillReturnError(errInstanceSettingsDB)
		mock.ExpectRollback()

		deleted, err := NewInstanceSearchSettingsRepository(db).
			DeleteAudited(context.Background(), (&testSearchAudit{}).build)

		assert.ErrorIs(t, err, errInstanceSettingsDB)
		assert.False(t, deleted)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
