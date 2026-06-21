package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

func setupActivityRepoTest(t *testing.T) (*activityRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := &activityRepository{db: db}

	return repo, mock, mockDB
}

func TestActivityRepository_DeleteOlderThan_DeletesRows(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupActivityRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	// Expect advisory lock acquisition
	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	// First batch: 5 rows deleted (< batchSize → last batch)
	dbMock.ExpectExec(`DELETE FROM activities WHERE id IN`).
		WithArgs(cutoff, activityRetentionBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 5))
	// Advisory unlock
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestActivityRepository_DeleteOlderThan_NoRowsToDelete(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupActivityRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	dbMock.ExpectExec(`DELETE FROM activities WHERE id IN`).
		WithArgs(cutoff, activityRetentionBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 0))
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestActivityRepository_DeleteOlderThan_AdvisoryLockHeld(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupActivityRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	// Lock is already held by another instance — pg_try_advisory_lock returns false.
	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	count, err := repo.DeleteOlderThan(context.Background(), time.Now())

	assert.NoError(t, err) // Skipped cleanly — not an error
	assert.Equal(t, int64(0), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestActivityRepository_DeleteOlderThan_DatabaseError(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupActivityRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	dbMock.ExpectExec(`DELETE FROM activities WHERE id IN`).
		WithArgs(cutoff, activityRetentionBatchSize).
		WillReturnError(sql.ErrConnDone)
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.Error(t, err)
	assert.Equal(t, int64(0), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestActivityRepository_DeleteOlderThan_NilDB(t *testing.T) {
	t.Parallel()

	repo := &activityRepository{db: nil}

	count, err := repo.DeleteOlderThan(context.Background(), time.Now())

	assert.Error(t, err)
	assert.Equal(t, fmt.Errorf("database connection is nil"), err)
	assert.Equal(t, int64(0), count)
}
