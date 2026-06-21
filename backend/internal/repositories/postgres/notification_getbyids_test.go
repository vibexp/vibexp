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

func setupNotificationRepoTest(t *testing.T) (*NotificationRepository, sqlmock.Sqlmock, func()) {
	t.Helper()

	mockDB, dbMock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewNotificationRepository(db).(*NotificationRepository)

	return repo, dbMock, func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}
}

func TestNotificationRepository_GetByIDsForUser_Empty(t *testing.T) {
	repo, _, cleanup := setupNotificationRepoTest(t)
	defer cleanup()

	result, err := repo.GetByIDsForUser(context.Background(), "u1", []string{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNotificationRepository_GetByIDsForUser_Success(t *testing.T) {
	repo, dbMock, cleanup := setupNotificationRepoTest(t)
	defer cleanup()

	ctx := context.Background()
	createdAt := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	columns := []string{
		"id", "recipient_user_id", "team_id", "type", "category",
		"title", "body", "action_url", "entity_ref", "dedupe_key",
		"read_at", "dismissed_at", "created_at",
	}

	rows := sqlmock.NewRows(columns).
		AddRow("n1", "u1", "", "feed.item.created", "team",
			"Post title", "Post body", "https://example.com", nil, "",
			nil, nil, createdAt)

	dbMock.ExpectQuery(`SELECT id, recipient_user_id`).
		WithArgs("u1", "n1").
		WillReturnRows(rows)

	result, err := repo.GetByIDsForUser(ctx, "u1", []string{"n1"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "n1", result[0].ID)
	assert.Equal(t, "Post title", result[0].Title)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestNotificationRepository_GetByIDsForUser_MultipleIDs(t *testing.T) {
	repo, dbMock, cleanup := setupNotificationRepoTest(t)
	defer cleanup()

	ctx := context.Background()
	createdAt := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	columns := []string{
		"id", "recipient_user_id", "team_id", "type", "category",
		"title", "body", "action_url", "entity_ref", "dedupe_key",
		"read_at", "dismissed_at", "created_at",
	}

	rows := sqlmock.NewRows(columns).
		AddRow("n1", "u1", "", "feed.item.created", "team", "Title 1", "", "", nil, "", nil, nil, createdAt).
		AddRow("n2", "u1", "", "feed.reply.created", "team", "Title 2", "", "", nil, "", nil, nil, createdAt)

	dbMock.ExpectQuery(`SELECT id, recipient_user_id`).
		WithArgs("u1", "n1", "n2").
		WillReturnRows(rows)

	result, err := repo.GetByIDsForUser(ctx, "u1", []string{"n1", "n2"})
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestNotificationRepository_GetByIDsForUser_DBError(t *testing.T) {
	repo, dbMock, cleanup := setupNotificationRepoTest(t)
	defer cleanup()

	ctx := context.Background()

	dbMock.ExpectQuery(`SELECT id, recipient_user_id`).
		WithArgs("u1", "n1").
		WillReturnError(errors.New("db error"))

	_, err := repo.GetByIDsForUser(ctx, "u1", []string{"n1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get notifications by ids for user")
}
