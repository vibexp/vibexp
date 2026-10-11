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
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

var resourceLockEventListColumns = []string{
	"id", "team_id", "resource_type", "resource_id", "action", "actor_user_id", "comment", "created_at", "name",
}

// The list statement pinned whole: tenancy-scoped, newest first with the id
// tiebreaker, actor name by LEFT JOIN.
const resourceLockEventListPattern = `FROM resource_lock_events e\s+LEFT JOIN users u ON u\.id = e\.actor_user_id` +
	`\s+WHERE e\.team_id = \$1 AND e\.resource_type = \$2 AND e\.resource_id = \$3` +
	`\s+ORDER BY e\.created_at DESC, e\.id DESC\s+LIMIT \$4$`

func setupResourceLockEventTest(t *testing.T) (*ResourceLockEventRepository, sqlmock.Sqlmock) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { closeMockDB(t, mockDB) })
	return NewResourceLockEventRepository(&database.DB{DB: mockDB}).(*ResourceLockEventRepository), mock
}

func TestResourceLockEventRepository_Append(t *testing.T) {
	ctx := context.Background()
	actor, comment := "user-1", "freeze for release"
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

	t.Run("inserts the event and populates id and created_at", func(t *testing.T) {
		repo, mock := setupResourceLockEventTest(t)
		mock.ExpectQuery(`INSERT INTO resource_lock_events\s+\(team_id, resource_type, resource_id, action, `+
			`actor_user_id, comment\)\s+VALUES \(\$1, \$2, \$3, \$4, \$5, \$6\)\s+RETURNING id, created_at`).
			WithArgs("team-1", models.RelationResourceTypePrompt, "res-1", models.LockActionLock, &actor, &comment).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("event-1", now))

		event := &models.ResourceLockEvent{
			TeamID: "team-1", ResourceType: models.RelationResourceTypePrompt, ResourceID: "res-1",
			Action: models.LockActionLock, ActorUserID: &actor, Comment: &comment,
		}
		require.NoError(t, repo.Append(ctx, event))
		assert.Equal(t, "event-1", event.ID)
		assert.Equal(t, now, event.CreatedAt)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an unlock event binds a NULL comment", func(t *testing.T) {
		repo, mock := setupResourceLockEventTest(t)
		mock.ExpectQuery(`INSERT INTO resource_lock_events`).
			WithArgs("team-1", models.RelationResourceTypeArtifact, "res-1", models.LockActionUnlock, &actor, nil).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("event-2", now))

		require.NoError(t, repo.Append(ctx, &models.ResourceLockEvent{
			TeamID: "team-1", ResourceType: models.RelationResourceTypeArtifact, ResourceID: "res-1",
			Action: models.LockActionUnlock, ActorUserID: &actor,
		}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// No query is expected in either case: sqlmock fails an unexpected one, so
	// these also prove an invalid event never reaches the database.
	t.Run("rejects a resource type that cannot be locked", func(t *testing.T) {
		repo, _ := setupResourceLockEventTest(t)
		err := repo.Append(ctx, &models.ResourceLockEvent{
			TeamID: "team-1", ResourceType: models.RelationResourceTypeMemory, ResourceID: "res-1",
			Action: models.LockActionLock,
		})
		assert.ErrorIs(t, err, repositories.ErrInvalidResourceLockEvent)
		assert.ErrorContains(t, err, `"memory"`)
	})

	t.Run("rejects an unknown action", func(t *testing.T) {
		repo, _ := setupResourceLockEventTest(t)
		err := repo.Append(ctx, &models.ResourceLockEvent{
			TeamID: "team-1", ResourceType: models.RelationResourceTypePrompt, ResourceID: "res-1",
			Action: "relock",
		})
		assert.ErrorIs(t, err, repositories.ErrInvalidResourceLockEvent)
		assert.ErrorContains(t, err, `"relock"`)
	})

	t.Run("a failed insert is wrapped", func(t *testing.T) {
		repo, mock := setupResourceLockEventTest(t)
		errDB := errors.New("connection reset")
		mock.ExpectQuery(`INSERT INTO resource_lock_events`).WillReturnError(errDB)

		err := repo.Append(ctx, &models.ResourceLockEvent{
			TeamID: "team-1", ResourceType: models.RelationResourceTypeBlueprint, ResourceID: "res-1",
			Action: models.LockActionLock,
		})
		assert.ErrorIs(t, err, errDB)
		assert.ErrorContains(t, err, "failed to append resource lock event")
	})
}

func TestResourceLockEventRepository_ListByResource(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	errDB := errors.New("connection reset")

	t.Run("scans events, resolving the actor name and tolerating a deleted actor", func(t *testing.T) {
		repo, mock := setupResourceLockEventTest(t)
		limit := 5
		mock.ExpectQuery(resourceLockEventListPattern).
			WithArgs("team-1", models.RelationResourceTypePrompt, "res-1", &limit).
			WillReturnRows(sqlmock.NewRows(resourceLockEventListColumns).
				AddRow("event-2", "team-1", "prompt", "res-1", "unlock", nil, nil, now, nil).
				AddRow("event-1", "team-1", "prompt", "res-1", "lock", "user-1", "freeze", now, "Ada"))

		events, err := repo.ListByResource(ctx, "team-1", models.RelationResourceTypePrompt, "res-1", 5)
		require.NoError(t, err)
		require.Len(t, events, 2)

		assert.Equal(t, "event-2", events[0].ID)
		assert.Nil(t, events[0].ActorUserID)
		assert.Empty(t, events[0].ActorName)
		assert.Nil(t, events[0].Comment)

		assert.Equal(t, "event-1", events[1].ID)
		require.NotNil(t, events[1].ActorUserID)
		assert.Equal(t, "user-1", *events[1].ActorUserID)
		assert.Equal(t, "Ada", events[1].ActorName)
		require.NotNil(t, events[1].Comment)
		assert.Equal(t, "freeze", *events[1].Comment)
		assert.Equal(t, now, events[1].CreatedAt)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	for _, limit := range []int{0, -3} {
		t.Run("a non-positive limit binds NULL and returns a non-nil empty slice", func(t *testing.T) {
			repo, mock := setupResourceLockEventTest(t)
			mock.ExpectQuery(resourceLockEventListPattern).
				WithArgs("team-1", models.RelationResourceTypeArtifact, "res-1", nil).
				WillReturnRows(sqlmock.NewRows(resourceLockEventListColumns))

			events, err := repo.ListByResource(ctx, "team-1", models.RelationResourceTypeArtifact, "res-1", limit)
			require.NoError(t, err)
			assert.NotNil(t, events)
			assert.Empty(t, events)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("a failed query is wrapped", func(t *testing.T) {
		repo, mock := setupResourceLockEventTest(t)
		mock.ExpectQuery(resourceLockEventListPattern).WillReturnError(errDB)

		_, err := repo.ListByResource(ctx, "team-1", models.RelationResourceTypePrompt, "res-1", 0)
		assert.ErrorIs(t, err, errDB)
		assert.ErrorContains(t, err, "failed to list resource lock events")
	})

	t.Run("a row that cannot be scanned is an error", func(t *testing.T) {
		repo, mock := setupResourceLockEventTest(t)
		mock.ExpectQuery(resourceLockEventListPattern).
			WillReturnRows(sqlmock.NewRows(resourceLockEventListColumns).
				AddRow("event-1", "team-1", "prompt", "res-1", "lock", "user-1", "freeze", "not-a-time", "Ada"))

		_, err := repo.ListByResource(ctx, "team-1", models.RelationResourceTypePrompt, "res-1", 0)
		assert.ErrorContains(t, err, "failed to scan resource lock event")
	})

	t.Run("an iteration error is wrapped", func(t *testing.T) {
		repo, mock := setupResourceLockEventTest(t)
		mock.ExpectQuery(resourceLockEventListPattern).
			WillReturnRows(sqlmock.NewRows(resourceLockEventListColumns).
				AddRow("event-1", "team-1", "prompt", "res-1", "lock", "user-1", "freeze", now, "Ada").
				RowError(0, errDB))

		_, err := repo.ListByResource(ctx, "team-1", models.RelationResourceTypePrompt, "res-1", 0)
		assert.ErrorIs(t, err, errDB)
		assert.ErrorContains(t, err, "failed to iterate resource lock events")
	})
}
