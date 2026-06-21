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
	"github.com/vibexp/vibexp/internal/models"
)

func setupResourceAccessRepoTest(t *testing.T) (*resourceAccessRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := &resourceAccessRepository{db: db}

	return repo, mock, mockDB
}

func ptr(s string) *string { return &s }

func TestNewResourceAccessRepository(t *testing.T) {
	t.Parallel()

	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewResourceAccessRepository(&database.DB{DB: mockDB})

	assert.NotNil(t, repo)
}

func TestResourceAccessRepository_Create_Success(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now().UTC()
	event := &models.ResourceAccessEvent{
		TeamID:       "team-123",
		UserID:       ptr("user-456"),
		ResourceType: "prompt",
		ResourceID:   "resource-789",
		Source:       "web",
		APIKeyID:     ptr("key-1"),
		UserAgent:    ptr("agent/1.0"),
		SourceIP:     ptr("10.0.0.1"),
	}

	dbMock.ExpectQuery(`INSERT INTO resource_access_events`).
		WithArgs(
			event.TeamID,
			event.UserID,
			event.ResourceType,
			event.ResourceID,
			event.Source,
			event.APIKeyID,
			event.UserAgent,
			event.SourceIP,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("generated-id", now))

	err := repo.Create(context.Background(), event)

	assert.NoError(t, err)
	assert.Equal(t, "generated-id", event.ID)
	assert.Equal(t, now, event.CreatedAt)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_Create_DatabaseError(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	event := &models.ResourceAccessEvent{
		TeamID:       "team-123",
		ResourceType: "prompt",
		ResourceID:   "resource-789",
		Source:       "web",
	}

	dbMock.ExpectQuery(`INSERT INTO resource_access_events`).
		WillReturnError(sql.ErrConnDone)

	err := repo.Create(context.Background(), event)

	assert.Error(t, err)
	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_Create_NilDB(t *testing.T) {
	t.Parallel()

	repo := &resourceAccessRepository{db: nil}

	err := repo.Create(context.Background(), &models.ResourceAccessEvent{})

	assert.Error(t, err)
	assert.Equal(t, fmt.Errorf("database connection is nil"), err)
}

func TestResourceAccessRepository_GetMetricsByResource_GroupedBySource(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	since := time.Now().UTC().AddDate(0, 0, -7)

	rows := sqlmock.NewRows([]string{"date", "source", "count"}).
		AddRow("2026-05-28", "web", 5).
		AddRow("2026-05-28", "cli", 2).
		AddRow("2026-05-29", "web", 9)

	dbMock.ExpectQuery(`SELECT TO_CHAR\(DATE\(created_at\), 'YYYY-MM-DD'\) AS date, source, COUNT\(\*\) AS count`).
		WithArgs("team-123", "prompt", "resource-789", since).
		WillReturnRows(rows)

	got, err := repo.GetMetricsByResource(context.Background(), "team-123", "prompt", "resource-789", since)

	assert.NoError(t, err)
	assert.Equal(t, []models.DailyAccessCount{
		{Date: "2026-05-28", Source: "web", Count: 5},
		{Date: "2026-05-28", Source: "cli", Count: 2},
		{Date: "2026-05-29", Source: "web", Count: 9},
	}, got)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_GetMetricsByResource_Empty(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	since := time.Now().UTC().AddDate(0, 0, -7)

	dbMock.ExpectQuery(`SELECT TO_CHAR\(DATE\(created_at\), 'YYYY-MM-DD'\) AS date, source, COUNT\(\*\) AS count`).
		WithArgs("team-123", "prompt", "resource-789", since).
		WillReturnRows(sqlmock.NewRows([]string{"date", "source", "count"}))

	got, err := repo.GetMetricsByResource(context.Background(), "team-123", "prompt", "resource-789", since)

	assert.NoError(t, err)
	assert.Equal(t, []models.DailyAccessCount{}, got)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_GetMetricsByResource_ScanError(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	since := time.Now().UTC().AddDate(0, 0, -7)

	// "count" returns a non-numeric value, forcing a Scan error into the int field.
	rows := sqlmock.NewRows([]string{"date", "source", "count"}).
		AddRow("2026-05-28", "web", "not-a-number")

	dbMock.ExpectQuery(`SELECT TO_CHAR\(DATE\(created_at\), 'YYYY-MM-DD'\) AS date, source, COUNT\(\*\) AS count`).
		WithArgs("team-123", "prompt", "resource-789", since).
		WillReturnRows(rows)

	got, err := repo.GetMetricsByResource(context.Background(), "team-123", "prompt", "resource-789", since)

	assert.Error(t, err)
	assert.Nil(t, got)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_GetMetricsByResource_RowsErr(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	since := time.Now().UTC().AddDate(0, 0, -7)

	rows := sqlmock.NewRows([]string{"date", "source", "count"}).
		AddRow("2026-05-28", "web", 5).
		RowError(0, sql.ErrConnDone)

	dbMock.ExpectQuery(`SELECT TO_CHAR\(DATE\(created_at\), 'YYYY-MM-DD'\) AS date, source, COUNT\(\*\) AS count`).
		WithArgs("team-123", "prompt", "resource-789", since).
		WillReturnRows(rows)

	got, err := repo.GetMetricsByResource(context.Background(), "team-123", "prompt", "resource-789", since)

	assert.Error(t, err)
	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.Nil(t, got)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_GetMetricsByResource_QueryError(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	since := time.Now().UTC().AddDate(0, 0, -7)

	dbMock.ExpectQuery(`SELECT TO_CHAR\(DATE\(created_at\), 'YYYY-MM-DD'\) AS date, source, COUNT\(\*\) AS count`).
		WithArgs("team-123", "prompt", "resource-789", since).
		WillReturnError(sql.ErrConnDone)

	got, err := repo.GetMetricsByResource(context.Background(), "team-123", "prompt", "resource-789", since)

	assert.Error(t, err)
	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.Nil(t, got)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_GetMetricsByResource_NilDB(t *testing.T) {
	t.Parallel()

	repo := &resourceAccessRepository{db: nil}

	got, err := repo.GetMetricsByResource(context.Background(), "team-123", "prompt", "resource-789", time.Now())

	assert.Error(t, err)
	assert.Equal(t, fmt.Errorf("database connection is nil"), err)
	assert.Nil(t, got)
}

func TestResourceAccessRepository_DeleteOlderThan_DeletesRows(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(resourceAccessRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	dbMock.ExpectExec(`DELETE FROM resource_access_events WHERE id IN`).
		WithArgs(cutoff, resourceAccessRetentionBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 5))
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(resourceAccessRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_DeleteOlderThan_AdvisoryLockHeld(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(resourceAccessRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	count, err := repo.DeleteOlderThan(context.Background(), time.Now())

	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_DeleteOlderThan_DatabaseError(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(resourceAccessRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	dbMock.ExpectExec(`DELETE FROM resource_access_events WHERE id IN`).
		WithArgs(cutoff, resourceAccessRetentionBatchSize).
		WillReturnError(sql.ErrConnDone)
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(resourceAccessRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.Error(t, err)
	assert.Equal(t, int64(0), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_DeleteOlderThan_MultiBatchPartialTotal(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupResourceAccessRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(resourceAccessRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	// First batch deletes a full batch, so the loop continues.
	dbMock.ExpectExec(`DELETE FROM resource_access_events WHERE id IN`).
		WithArgs(cutoff, resourceAccessRetentionBatchSize).
		WillReturnResult(sqlmock.NewResult(0, resourceAccessRetentionBatchSize))
	// Second batch errors; the partial total from the first batch must be returned.
	dbMock.ExpectExec(`DELETE FROM resource_access_events WHERE id IN`).
		WithArgs(cutoff, resourceAccessRetentionBatchSize).
		WillReturnError(sql.ErrConnDone)
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(resourceAccessRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.Error(t, err)
	assert.Equal(t, int64(resourceAccessRetentionBatchSize), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceAccessRepository_DeleteOlderThan_NilDB(t *testing.T) {
	t.Parallel()

	repo := &resourceAccessRepository{db: nil}

	count, err := repo.DeleteOlderThan(context.Background(), time.Now())

	assert.Error(t, err)
	assert.Equal(t, fmt.Errorf("database connection is nil"), err)
	assert.Equal(t, int64(0), count)
}
