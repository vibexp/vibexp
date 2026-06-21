package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// notificationScanColumns mirrors the 13 values scanNotificationRows reads.
var notificationScanColumns = []string{
	"id", "recipient_user_id", "team_id", "type", "category",
	"title", "body", "action_url", "entity_ref", "dedupe_key",
	"read_at", "dismissed_at", "created_at",
}

//nolint:funlen // table-driven test exercising default-limit, unread-only, and pagination shapes
func TestNotificationRepository_ListForUser_Squirrel(t *testing.T) {
	repo, dbMock, cleanup := setupNotificationRepoTest(t)
	defer cleanup()

	ctx := context.Background()
	createdAt := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	oneRow := func() *sqlmock.Rows {
		return sqlmock.NewRows(notificationScanColumns).
			AddRow("n1", "u1", "", "feed.item.created", "team",
				"Post title", "Post body", "https://example.com", nil, "",
				nil, nil, createdAt)
	}

	tests := []struct {
		name      string
		filters   repositories.NotificationListFilters
		setupMock func()
	}{
		{
			name:    "default limit applies and orders by created_at DESC",
			filters: repositories.NotificationListFilters{},
			setupMock: func() {
				// Default Limit of 20 is inlined by squirrel; Offset 0.
				dbMock.ExpectQuery(
					`FROM notifications WHERE recipient_user_id = \$1 ORDER BY created_at DESC LIMIT 20 OFFSET 0`).
					WithArgs("u1").
					WillReturnRows(oneRow())
			},
		},
		{
			name:    "unread-only appends read and dismissed predicate",
			filters: repositories.NotificationListFilters{UnreadOnly: true, Limit: 10},
			setupMock: func() {
				dbMock.ExpectQuery(
					`FROM notifications WHERE recipient_user_id = \$1 ` +
						`AND read_at IS NULL AND dismissed_at IS NULL ` +
						`ORDER BY created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs("u1").
					WillReturnRows(oneRow())
			},
		},
		{
			name:    "explicit limit and offset are inlined",
			filters: repositories.NotificationListFilters{Limit: 5, Offset: 15},
			setupMock: func() {
				dbMock.ExpectQuery(
					`FROM notifications WHERE recipient_user_id = \$1 ORDER BY created_at DESC LIMIT 5 OFFSET 15`).
					WithArgs("u1").
					WillReturnRows(oneRow())
			},
		},
		{
			name:    "negative offset clamps to zero",
			filters: repositories.NotificationListFilters{Limit: 5, Offset: -3},
			setupMock: func() {
				dbMock.ExpectQuery(
					`FROM notifications WHERE recipient_user_id = \$1 ORDER BY created_at DESC LIMIT 5 OFFSET 0`).
					WithArgs("u1").
					WillReturnRows(oneRow())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.ListForUser(ctx, "u1", tt.filters)
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, "n1", result[0].ID)
			assert.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}

func TestNotificationRepository_ListForUser_DBError(t *testing.T) {
	repo, dbMock, cleanup := setupNotificationRepoTest(t)
	defer cleanup()

	dbMock.ExpectQuery(`FROM notifications WHERE recipient_user_id = \$1`).
		WithArgs("u1").
		WillReturnError(errors.New("db error"))

	_, err := repo.ListForUser(context.Background(), "u1", repositories.NotificationListFilters{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list notifications for user")
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestNotificationRepository_ListForUser_Empty(t *testing.T) {
	repo, dbMock, cleanup := setupNotificationRepoTest(t)
	defer cleanup()

	dbMock.ExpectQuery(`FROM notifications WHERE recipient_user_id = \$1`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows(notificationScanColumns))

	result, err := repo.ListForUser(context.Background(), "u1", repositories.NotificationListFilters{})
	require.NoError(t, err)
	assert.Empty(t, result)
	assert.NotNil(t, result, "ListForUser must return a non-nil empty slice")
	assert.NoError(t, dbMock.ExpectationsWereMet())
}
