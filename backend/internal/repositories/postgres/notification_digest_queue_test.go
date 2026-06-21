package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

func setupDigestQueueTest(t *testing.T) (*NotificationDigestQueueRepository, sqlmock.Sqlmock, func()) {
	t.Helper()

	mockDB, dbMock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewNotificationDigestQueueRepository(db).(*NotificationDigestQueueRepository)

	return repo, dbMock, func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}
}

func TestNotificationDigestQueueRepository_Enqueue_Success(t *testing.T) {
	repo, dbMock, cleanup := setupDigestQueueTest(t)
	defer cleanup()

	ctx := context.Background()
	scheduledFor := time.Date(2024, 1, 16, 9, 0, 0, 0, time.UTC)

	dbMock.ExpectExec(`INSERT INTO notification_digest_queue`).
		WithArgs("user-1", "notif-1", scheduledFor).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Enqueue(ctx, "user-1", "notif-1", scheduledFor)
	require.NoError(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestNotificationDigestQueueRepository_Enqueue_Error(t *testing.T) {
	repo, dbMock, cleanup := setupDigestQueueTest(t)
	defer cleanup()

	ctx := context.Background()
	scheduledFor := time.Date(2024, 1, 16, 9, 0, 0, 0, time.UTC)

	dbMock.ExpectExec(`INSERT INTO notification_digest_queue`).
		WithArgs("user-1", "notif-1", scheduledFor).
		WillReturnError(errors.New("db error"))

	err := repo.Enqueue(ctx, "user-1", "notif-1", scheduledFor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enqueue notification for digest")
}

func TestNotificationDigestQueueRepository_FetchPending_Success(t *testing.T) {
	repo, dbMock, cleanup := setupDigestQueueTest(t)
	defer cleanup()

	ctx := context.Background()
	before := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	scheduledFor := before.Add(-1 * time.Hour)
	created := before.Add(-2 * time.Hour)

	rows := sqlmock.NewRows([]string{"id", "user_id", "notification_id", "scheduled_for", "sent_at", "created_at"}).
		AddRow("row-1", "user-1", "notif-1", scheduledFor, nil, created).
		AddRow("row-2", "user-1", "notif-2", scheduledFor, nil, created)

	dbMock.ExpectQuery(`SELECT id, user_id, notification_id, scheduled_for, sent_at, created_at`).
		WithArgs(before).
		WillReturnRows(rows)

	result, err := repo.FetchPending(ctx, before)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "row-1", result[0].ID)
	assert.Equal(t, "user-1", result[0].UserID)
	assert.Equal(t, "notif-1", result[0].NotificationID)
	assert.Nil(t, result[0].SentAt)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestNotificationDigestQueueRepository_FetchPending_Empty(t *testing.T) {
	repo, dbMock, cleanup := setupDigestQueueTest(t)
	defer cleanup()

	ctx := context.Background()
	before := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{"id", "user_id", "notification_id", "scheduled_for", "sent_at", "created_at"})

	dbMock.ExpectQuery(`SELECT id, user_id, notification_id, scheduled_for, sent_at, created_at`).
		WithArgs(before).
		WillReturnRows(rows)

	result, err := repo.FetchPending(ctx, before)
	require.NoError(t, err)
	assert.Empty(t, result)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestNotificationDigestQueueRepository_FetchPending_Error(t *testing.T) {
	repo, dbMock, cleanup := setupDigestQueueTest(t)
	defer cleanup()

	ctx := context.Background()
	before := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	dbMock.ExpectQuery(`SELECT id, user_id, notification_id, scheduled_for, sent_at, created_at`).
		WithArgs(before).
		WillReturnError(errors.New("db error"))

	_, err := repo.FetchPending(ctx, before)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch pending digest queue rows")
}

func TestNotificationDigestQueueRepository_MarkSent_Success(t *testing.T) {
	repo, dbMock, cleanup := setupDigestQueueTest(t)
	defer cleanup()

	ctx := context.Background()
	sentAt := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	dbMock.ExpectExec(`UPDATE notification_digest_queue SET sent_at`).
		WithArgs(sentAt, "row-1", "row-2").
		WillReturnResult(sqlmock.NewResult(0, 2))

	err := repo.MarkSent(ctx, []string{"row-1", "row-2"}, sentAt)
	require.NoError(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestNotificationDigestQueueRepository_MarkSent_EmptyIDs(t *testing.T) {
	repo, _, cleanup := setupDigestQueueTest(t)
	defer cleanup()

	ctx := context.Background()
	sentAt := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	// No DB calls should be made for empty ID slice
	err := repo.MarkSent(ctx, []string{}, sentAt)
	require.NoError(t, err)
}

func TestNotificationDigestQueueRepository_MarkSent_Error(t *testing.T) {
	repo, dbMock, cleanup := setupDigestQueueTest(t)
	defer cleanup()

	ctx := context.Background()
	sentAt := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	dbMock.ExpectExec(`UPDATE notification_digest_queue SET sent_at`).
		WithArgs(sentAt, "row-1").
		WillReturnError(errors.New("db error"))

	err := repo.MarkSent(ctx, []string{"row-1"}, sentAt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mark digest queue rows sent")
}
