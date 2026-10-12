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
)

func setupResourceWriteEventRepoTest(t *testing.T) (*resourceWriteEventRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	return &resourceWriteEventRepository{db: &database.DB{DB: mockDB}}, dbMock
}

func TestNewResourceWriteEventRepository(t *testing.T) {
	t.Parallel()

	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	assert.NotNil(t, NewResourceWriteEventRepository(&database.DB{DB: mockDB}))
}

func TestResourceWriteEventRepository_Create_Success(t *testing.T) {
	t.Parallel()

	repo, dbMock := setupResourceWriteEventRepoTest(t)
	now := time.Now().UTC()
	event := &models.ResourceWriteEvent{
		TeamID:        "team-1",
		ProjectID:     ptr("project-1"),
		UserID:        ptr("user-1"),
		ResourceType:  "memory",
		ResourceID:    "res-1",
		Action:        models.WriteActionCreate,
		Source:        "mcp",
		AssistantName: ptr("claude-code"),
		APIKeyID:      ptr("key-1"),
	}

	dbMock.ExpectQuery(`INSERT INTO resource_write_events`).
		WithArgs("team-1", "project-1", "user-1", "memory", "res-1", "create", "mcp", "claude-code", "key-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("evt-1", now))

	require.NoError(t, repo.Create(context.Background(), event))

	assert.Equal(t, "evt-1", event.ID)
	assert.Equal(t, now, event.CreatedAt)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

// Absent attribution is bound as NULL, not as an empty string: the columns are
// uuid, and "" would fail the insert.
func TestResourceWriteEventRepository_Create_NullsOptionalColumns(t *testing.T) {
	t.Parallel()

	repo, dbMock := setupResourceWriteEventRepoTest(t)
	event := &models.ResourceWriteEvent{
		TeamID: "team-1", ResourceType: "prompt", ResourceID: "res-1",
		Action: models.WriteActionUpdate, Source: "web",
	}

	dbMock.ExpectQuery(`INSERT INTO resource_write_events`).
		WithArgs("team-1", nil, nil, "prompt", "res-1", "update", "web", nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("evt-1", time.Now()))

	require.NoError(t, repo.Create(context.Background(), event))
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestResourceWriteEventRepository_Create_Error(t *testing.T) {
	t.Parallel()

	repo, dbMock := setupResourceWriteEventRepoTest(t)
	boom := errors.New("insert failed")
	dbMock.ExpectQuery(`INSERT INTO resource_write_events`).WillReturnError(boom)

	err := repo.Create(context.Background(), &models.ResourceWriteEvent{TeamID: "team-1"})

	require.ErrorIs(t, err, boom)
}

func TestResourceWriteEventRepository_NilDB(t *testing.T) {
	t.Parallel()

	repo := &resourceWriteEventRepository{}
	ctx := context.Background()
	since := time.Now()

	require.Error(t, repo.Create(ctx, &models.ResourceWriteEvent{}))

	_, err := repo.GetTeamCreationCountsBySource(ctx, "team-1", since)
	require.Error(t, err)
	_, err = repo.GetProjectCreationCountsBySource(ctx, "team-1", "slug", since)
	require.Error(t, err)
	_, err = repo.GetTeamWriteCountsBySource(ctx, "team-1", since)
	require.Error(t, err)
	_, err = repo.GetProjectWriteCountsBySource(ctx, "team-1", "slug", since)
	require.Error(t, err)
}

// The memories branch needs its own placeholder bound to the same instant in
// UTC (see teamCreationBySourceQuery). This pins the binding: the third
// argument is the UTC form of the second, and the project query adds the slug.
func TestResourceWriteEventRepository_CreationCountsBySource_Bindings(t *testing.T) {
	t.Parallel()

	auckland, err := time.LoadLocation("Pacific/Auckland")
	require.NoError(t, err)
	since := time.Date(2026, 3, 2, 0, 0, 0, 0, auckland)
	rows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"resource_type", "source", "count"}).
			AddRow("memories", "mcp", 4).
			AddRow("prompts", "unknown", 2)
	}
	want := []models.ResourceCreationSourceCount{
		{ResourceType: "memories", Source: "mcp", Count: 4},
		{ResourceType: "prompts", Source: "unknown", Count: 2},
	}

	t.Run("team", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`FROM memories r`).WithArgs("team-1", since, since.UTC()).WillReturnRows(rows())

		got, err := repo.GetTeamCreationCountsBySource(context.Background(), "team-1", since)

		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.NoError(t, dbMock.ExpectationsWereMet())
	})

	t.Run("project", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`WITH target AS`).
			WithArgs("team-1", since, since.UTC(), "my-project").WillReturnRows(rows())

		got, err := repo.GetProjectCreationCountsBySource(context.Background(), "team-1", "my-project", since)

		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.NoError(t, dbMock.ExpectationsWereMet())
	})
}

func TestResourceWriteEventRepository_CreationCountsBySource_Empty(t *testing.T) {
	t.Parallel()

	repo, dbMock := setupResourceWriteEventRepoTest(t)
	dbMock.ExpectQuery(`FROM prompts r`).
		WillReturnRows(sqlmock.NewRows([]string{"resource_type", "source", "count"}))

	got, err := repo.GetTeamCreationCountsBySource(context.Background(), "team-1", time.Now())

	require.NoError(t, err)
	// An empty slice, not nil: the caller ranges over it either way, but the
	// repositories return non-nil by convention.
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestResourceWriteEventRepository_CreationCountsBySource_Errors(t *testing.T) {
	t.Parallel()

	boom := errors.New("query failed")

	t.Run("query", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`FROM prompts r`).WillReturnError(boom)

		_, err := repo.GetTeamCreationCountsBySource(context.Background(), "team-1", time.Now())

		require.ErrorIs(t, err, boom)
	})

	t.Run("scan", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`FROM prompts r`).WillReturnRows(
			sqlmock.NewRows([]string{"resource_type", "source", "count"}).AddRow("prompts", "web", "not-a-number"),
		)

		_, err := repo.GetTeamCreationCountsBySource(context.Background(), "team-1", time.Now())

		require.ErrorContains(t, err, "failed to scan resource creation count by source")
	})

	t.Run("iteration", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`FROM prompts r`).WillReturnRows(
			sqlmock.NewRows([]string{"resource_type", "source", "count"}).
				AddRow("prompts", "web", 1).RowError(0, boom),
		)

		_, err := repo.GetTeamCreationCountsBySource(context.Background(), "team-1", time.Now())

		require.ErrorIs(t, err, boom)
	})
}

func TestResourceWriteEventRepository_WriteCountsBySource(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"source", "count"}).AddRow("mcp", 48).AddRow("web", 31)
	}
	want := []models.ResourceWriteSourceCount{{Source: "mcp", Count: 48}, {Source: "web", Count: 31}}

	t.Run("team", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`FROM resource_write_events`).WithArgs("team-1", since).WillReturnRows(rows())

		got, err := repo.GetTeamWriteCountsBySource(context.Background(), "team-1", since)

		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.NoError(t, dbMock.ExpectationsWereMet())
	})

	t.Run("project", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`AND project_id = \(SELECT id FROM projects`).
			WithArgs("team-1", since, "my-project").WillReturnRows(rows())

		got, err := repo.GetProjectWriteCountsBySource(context.Background(), "team-1", "my-project", since)

		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.NoError(t, dbMock.ExpectationsWereMet())
	})
}

func TestResourceWriteEventRepository_WriteCountsBySource_Errors(t *testing.T) {
	t.Parallel()

	boom := errors.New("query failed")

	t.Run("query", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`FROM resource_write_events`).WillReturnError(boom)

		_, err := repo.GetTeamWriteCountsBySource(context.Background(), "team-1", time.Now())

		require.ErrorIs(t, err, boom)
	})

	t.Run("scan", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`FROM resource_write_events`).WillReturnRows(
			sqlmock.NewRows([]string{"source", "count"}).AddRow("web", "not-a-number"),
		)

		_, err := repo.GetTeamWriteCountsBySource(context.Background(), "team-1", time.Now())

		require.ErrorContains(t, err, "failed to scan resource write count by source")
	})

	t.Run("iteration", func(t *testing.T) {
		t.Parallel()
		repo, dbMock := setupResourceWriteEventRepoTest(t)
		dbMock.ExpectQuery(`FROM resource_write_events`).WillReturnRows(
			sqlmock.NewRows([]string{"source", "count"}).AddRow("web", 1).RowError(0, boom),
		)

		_, err := repo.GetTeamWriteCountsBySource(context.Background(), "team-1", time.Now())

		require.ErrorIs(t, err, boom)
	})
}
