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

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Statement-level suite for InstanceSiteInfoRepository: the error paths and
// argument plumbing. What the database does with the statements (the
// singleton, the CHECKs, ON CONFLICT behaviour) is pinned by the integration
// suite.

var instanceSiteInfoColumns = []string{
	"company_name", "company_email", "website", "support_url",
	"terms_url", "privacy_url", "logo_url", "logo_dark_url",
	"created_at", "updated_at", "updated_by", "version",
}

const (
	instanceSiteInfoLock   = "LOCK TABLE instance_site_info IN SHARE ROW EXCLUSIVE MODE"
	instanceSiteInfoRead   = "SELECT (.+) FROM instance_site_info"
	instanceSiteInfoWrite  = "INSERT INTO instance_site_info (.+) ON CONFLICT \\(id\\)\\s+DO UPDATE"
	instanceSiteInfoDelete = "DELETE FROM instance_site_info"
)

func siteInfoPtr(s string) *string { return &s }

// instanceSiteInfoFixture sets a name, an email and one URL, leaving the rest
// not configured, so the NULL plumbing is exercised on every write.
func instanceSiteInfoFixture() *models.InstanceSiteInfo {
	return &models.InstanceSiteInfo{
		CompanyName:  siteInfoPtr("Acme Inc."),
		CompanyEmail: siteInfoPtr("hello@acme.test"),
		Website:      siteInfoPtr("https://acme.test"),
	}
}

// storedInstanceSiteInfoRow is a stored row at the given version.
func storedInstanceSiteInfoRow(version int64) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows(instanceSiteInfoColumns).
		AddRow("Old Co", nil, nil, nil, nil, nil, "https://old.test/logo.png", nil, now, now, nil, version)
}

func expectInstanceSiteInfoBegin(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(instanceSiteInfoLock).WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectInstanceSiteInfoUpsert(mock sqlmock.Sqlmock, version int64) {
	now := time.Now().UTC()
	mock.ExpectQuery(instanceSiteInfoWrite).
		WithArgs("Acme Inc.", "hello@acme.test", "https://acme.test", nil, nil, nil, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, version))
}

// testSiteInfoAudit builds a minimal, valid entry and records what it was given.
type testSiteInfoAudit struct {
	calls         int
	before, after *models.InstanceSiteInfo
	err           error
}

func (a *testSiteInfoAudit) build(before, after *models.InstanceSiteInfo) (*models.InstanceSettingsAuditEntry, error) {
	a.calls++
	a.before, a.after = before, after
	if a.err != nil {
		return nil, a.err
	}
	return &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingSiteInfo,
		Action:  models.InstanceSettingsAuditActionUpsert,
		After:   []byte(`{"company_name":"Acme Inc."}`),
	}, nil
}

func TestInstanceSiteInfoRepository_Get(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	now := time.Now().UTC()
	editor := "11111111-1111-1111-1111-111111111111"

	mock.ExpectQuery(instanceSiteInfoRead).
		WillReturnRows(sqlmock.NewRows(instanceSiteInfoColumns).
			AddRow("Acme Inc.", "hello@acme.test", nil, "https://acme.test/help", nil, nil,
				"https://acme.test/logo.png", nil, now, now, editor, int64(4)))

	got, err := NewInstanceSiteInfoRepository(db).Get(context.Background())

	require.NoError(t, err)
	require.NotNil(t, got.CompanyName)
	assert.Equal(t, "Acme Inc.", *got.CompanyName)
	require.NotNil(t, got.SupportURL)
	assert.Equal(t, "https://acme.test/help", *got.SupportURL)
	assert.Nil(t, got.Website, "a NULL column scans to nil")
	assert.Nil(t, got.LogoDarkURL)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, editor, *got.UpdatedBy)
	assert.Equal(t, int64(4), got.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSiteInfoRepository_Get_Errors(t *testing.T) {
	t.Run("no row maps to the sentinel", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(sql.ErrNoRows)

		got, err := NewInstanceSiteInfoRepository(db).Get(context.Background())

		assert.Nil(t, got)
		assert.ErrorIs(t, err, repositories.ErrInstanceSiteInfoNotFound)
	})

	t.Run("a database failure is not reported as not-found", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(errInstanceSettingsDB)

		_, err := NewInstanceSiteInfoRepository(db).Get(context.Background())

		assert.ErrorIs(t, err, errInstanceSettingsDB)
		assert.NotErrorIs(t, err, repositories.ErrInstanceSiteInfoNotFound)
	})
}

func TestInstanceSiteInfoRepository_UpsertAudited(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	expectInstanceSiteInfoBegin(mock)
	mock.ExpectQuery(instanceSiteInfoRead).WillReturnRows(storedInstanceSiteInfoRow(1))
	expectInstanceSiteInfoUpsert(mock, 2)
	expectInstanceAuditInsert(mock)
	mock.ExpectCommit()

	audit := &testSiteInfoAudit{}
	s := instanceSiteInfoFixture()
	require.NoError(t, NewInstanceSiteInfoRepository(db).UpsertAudited(context.Background(), s, nil, audit.build))

	assert.Equal(t, 1, audit.calls)
	require.NotNil(t, audit.before, "the current row is handed over as before")
	require.NotNil(t, audit.before.CompanyName)
	assert.Equal(t, "Old Co", *audit.before.CompanyName)
	assert.Same(t, s, audit.after, "after is the row as written")
	assert.Equal(t, int64(2), s.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSiteInfoRepository_UpsertAudited_NoRowGivesNilBefore(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	expectInstanceSiteInfoBegin(mock)
	mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(sql.ErrNoRows)
	expectInstanceSiteInfoUpsert(mock, 1)
	expectInstanceAuditInsert(mock)
	mock.ExpectCommit()

	audit := &testSiteInfoAudit{}
	require.NoError(t, NewInstanceSiteInfoRepository(db).
		UpsertAudited(context.Background(), instanceSiteInfoFixture(), nil, audit.build))

	assert.Nil(t, audit.before)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Every failure inside the transaction rolls the whole change back: there is
// never a site info write without its audit entry.
func TestInstanceSiteInfoRepository_UpsertAudited_FailuresRollBack(t *testing.T) {
	errBuild := errors.New("snapshot failed")
	cases := []struct {
		name     string
		auditErr error
		setup    func(mock sqlmock.Sqlmock)
		wantErr  error
	}{
		{"current-row read fails", nil, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(errInstanceSettingsDB)
		}, errInstanceSettingsDB},
		{"upsert fails", nil, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(instanceSiteInfoWrite).WillReturnError(errInstanceSettingsDB)
		}, errInstanceSettingsDB},
		{"building the entry fails", errBuild, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(sql.ErrNoRows)
			expectInstanceSiteInfoUpsert(mock, 1)
		}, errBuild},
		{"audit insert fails", nil, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(sql.ErrNoRows)
			expectInstanceSiteInfoUpsert(mock, 1)
			mock.ExpectQuery("INSERT INTO instance_settings_audit").WillReturnError(errInstanceSettingsDB)
		}, errInstanceSettingsDB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newInstanceSettingsMock(t)
			expectInstanceSiteInfoBegin(mock)
			tc.setup(mock)
			mock.ExpectRollback()

			audit := &testSiteInfoAudit{err: tc.auditErr}
			err := NewInstanceSiteInfoRepository(db).
				UpsertAudited(context.Background(), instanceSiteInfoFixture(), nil, audit.build)

			assert.ErrorIs(t, err, tc.wantErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestInstanceSiteInfoRepository_UpsertAudited_BeginLockAndCommitErrors(t *testing.T) {
	t.Run("begin", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectBegin().WillReturnError(errInstanceSettingsDB)

		err := NewInstanceSiteInfoRepository(db).
			UpsertAudited(context.Background(), instanceSiteInfoFixture(), nil, (&testSiteInfoAudit{}).build)

		assert.ErrorIs(t, err, errInstanceSettingsDB)
	})

	t.Run("lock", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(instanceSiteInfoLock).WillReturnError(errInstanceSettingsDB)
		mock.ExpectRollback()

		err := NewInstanceSiteInfoRepository(db).
			UpsertAudited(context.Background(), instanceSiteInfoFixture(), nil, (&testSiteInfoAudit{}).build)

		assert.ErrorIs(t, err, errInstanceSettingsDB)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("commit", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		expectInstanceSiteInfoBegin(mock)
		mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(sql.ErrNoRows)
		expectInstanceSiteInfoUpsert(mock, 1)
		expectInstanceAuditInsert(mock)
		mock.ExpectCommit().WillReturnError(errInstanceSettingsDB)

		err := NewInstanceSiteInfoRepository(db).
			UpsertAudited(context.Background(), instanceSiteInfoFixture(), nil, (&testSiteInfoAudit{}).build)

		assert.ErrorIs(t, err, errInstanceSettingsDB)
	})
}

// A matching expected version is compared under the lock and the write goes
// ahead exactly as a last-write-wins upsert.
func TestInstanceSiteInfoRepository_UpsertAudited_MatchingVersionWrites(t *testing.T) {
	db, mock := newInstanceSettingsMock(t)
	expectInstanceSiteInfoBegin(mock)
	mock.ExpectQuery(instanceSiteInfoRead).WillReturnRows(storedInstanceSiteInfoRow(4))
	expectInstanceSiteInfoUpsert(mock, 5)
	expectInstanceAuditInsert(mock)
	mock.ExpectCommit()

	expected := int64(4)
	s := instanceSiteInfoFixture()
	require.NoError(t, NewInstanceSiteInfoRepository(db).
		UpsertAudited(context.Background(), s, &expected, (&testSiteInfoAudit{}).build))

	assert.Equal(t, int64(5), s.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A stale expected version, or one sent when nothing is stored, is a conflict:
// nothing is written, nothing is audited, and the transaction rolls back.
func TestInstanceSiteInfoRepository_UpsertAudited_VersionConflictRollsBack(t *testing.T) {
	cases := []struct {
		name string
		read func(mock sqlmock.Sqlmock)
	}{
		{"stale version", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(instanceSiteInfoRead).WillReturnRows(storedInstanceSiteInfoRow(5))
		}},
		{"nothing stored", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(sql.ErrNoRows)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newInstanceSettingsMock(t)
			expectInstanceSiteInfoBegin(mock)
			tc.read(mock)
			mock.ExpectRollback()

			expected := int64(4)
			audit := &testSiteInfoAudit{}
			err := NewInstanceSiteInfoRepository(db).
				UpsertAudited(context.Background(), instanceSiteInfoFixture(), &expected, audit.build)

			require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)
			assert.Zero(t, audit.calls)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestInstanceSiteInfoRepository_DeleteAudited(t *testing.T) {
	t.Run("a stored row is deleted and audited", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		expectInstanceSiteInfoBegin(mock)
		mock.ExpectQuery(instanceSiteInfoRead).WillReturnRows(storedInstanceSiteInfoRow(3))
		mock.ExpectExec(instanceSiteInfoDelete).WillReturnResult(sqlmock.NewResult(0, 1))
		expectInstanceAuditInsert(mock)
		mock.ExpectCommit()

		audit := &testSiteInfoAudit{}
		deleted, err := NewInstanceSiteInfoRepository(db).DeleteAudited(context.Background(), audit.build)

		require.NoError(t, err)
		assert.True(t, deleted)
		require.NotNil(t, audit.before)
		assert.Equal(t, int64(3), audit.before.Version)
		assert.Nil(t, audit.after)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no row writes nothing and builds no entry", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		expectInstanceSiteInfoBegin(mock)
		mock.ExpectQuery(instanceSiteInfoRead).WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		audit := &testSiteInfoAudit{}
		deleted, err := NewInstanceSiteInfoRepository(db).DeleteAudited(context.Background(), audit.build)

		require.NoError(t, err)
		assert.False(t, deleted)
		assert.Zero(t, audit.calls)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a delete failure rolls back", func(t *testing.T) {
		db, mock := newInstanceSettingsMock(t)
		expectInstanceSiteInfoBegin(mock)
		mock.ExpectQuery(instanceSiteInfoRead).WillReturnRows(storedInstanceSiteInfoRow(3))
		mock.ExpectExec(instanceSiteInfoDelete).WillReturnError(errInstanceSettingsDB)
		mock.ExpectRollback()

		deleted, err := NewInstanceSiteInfoRepository(db).
			DeleteAudited(context.Background(), (&testSiteInfoAudit{}).build)

		assert.ErrorIs(t, err, errInstanceSettingsDB)
		assert.False(t, deleted)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
