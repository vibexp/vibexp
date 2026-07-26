package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// newSquirrelMockRepo wires a sqlmock-backed *database.DB usable by every
// repository under test in this file.
func newSquirrelMockRepo(t *testing.T) (*database.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	return &database.DB{DB: mockDB}, mock, mockDB
}

// closeMockDB closes the sqlmock connection, logging any error.
func closeMockDB(t *testing.T, mockDB *sql.DB) {
	t.Helper()
	if closeErr := mockDB.Close(); closeErr != nil {
		t.Logf("Failed to close mock DB: %v", closeErr)
	}
}

// ---------------------------------------------------------------------------
// activity.go — List (all-optional filters; empty-WHERE case)
// ---------------------------------------------------------------------------

var activityListColumns = []string{
	"id", "user_id", "activity_type", "entity_type", "entity_id", "session_id",
	"description", "metadata", "source_ip", "user_agent", "created_at",
}

func activityOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(activityListColumns).AddRow(
		"act-1", "user-123", "create", "prompt", "ent-1", "sess-1",
		"did a thing", "", "1.2.3.4", "agent", now,
	)
}

//nolint:funlen // table-driven test exercising every optional filter and the empty-WHERE case
func TestActivityRepository_List_SquirrelMigration(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		filters      repositories.ActivityFilters
		countMatcher string
		countArgs    []driver.Value
		listMatcher  string
		listArgs     []driver.Value
	}{
		{
			name:         "no filters emits no WHERE clause",
			filters:      repositories.ActivityFilters{Limit: 10, Offset: 0},
			countMatcher: `^SELECT COUNT\(\*\) FROM activities$`,
			countArgs:    nil,
			listMatcher:  `FROM activities ORDER BY created_at DESC LIMIT 10 OFFSET 0`,
			listArgs:     []driver.Value{},
		},
		{
			name:         "UserID filter binds user_id equality",
			filters:      repositories.ActivityFilters{UserID: strPtr("user-123"), Limit: 10},
			countMatcher: `FROM activities WHERE \(user_id = \$1\)`,
			countArgs:    []driver.Value{"user-123"},
			listMatcher:  `FROM activities WHERE \(user_id = \$1\)`,
			listArgs:     []driver.Value{"user-123"},
		},
		{
			name:         "ActivityType filter binds activity_type equality",
			filters:      repositories.ActivityFilters{ActivityType: strPtr("create"), Limit: 10},
			countArgs:    []driver.Value{"create"},
			countMatcher: `FROM activities WHERE \(activity_type = \$1\)`,
			listMatcher:  `FROM activities WHERE \(activity_type = \$1\)`,
			listArgs:     []driver.Value{"create"},
		},
		{
			name:         "Search filter binds one pattern across two ILIKE placeholders",
			filters:      repositories.ActivityFilters{Search: strPtr("hello"), Limit: 10},
			countMatcher: `WHERE \(\(description ILIKE \$1 OR activity_type ILIKE \$2\)\)`,
			countArgs:    []driver.Value{"%hello%", "%hello%"},
			listMatcher:  `WHERE \(\(description ILIKE \$1 OR activity_type ILIKE \$2\)\)`,
			listArgs:     []driver.Value{"%hello%", "%hello%"},
		},
		{
			name: "all filters combine in declared order",
			filters: repositories.ActivityFilters{
				UserID:       strPtr("user-123"),
				ActivityType: strPtr("create"),
				EntityType:   strPtr("prompt"),
				EntityID:     strPtr("ent-1"),
				SessionID:    strPtr("sess-1"),
				Search:       strPtr("hello"),
				DateFrom:     strPtr("2026-01-01"),
				DateTo:       strPtr("2026-12-31"),
				Limit:        10,
			},
			countMatcher: `WHERE \(user_id = \$1 AND activity_type = \$2 AND entity_type = \$3 AND ` +
				`entity_id = \$4 AND session_id = \$5 AND \(description ILIKE \$6 OR activity_type ILIKE \$7\) AND ` +
				`created_at >= \$8 AND created_at <= \$9\)`,
			countArgs: []driver.Value{
				"user-123", "create", "prompt", "ent-1", "sess-1", "%hello%", "%hello%", "2026-01-01", "2026-12-31",
			},
			listMatcher: `WHERE \(user_id = \$1 AND activity_type = \$2`,
			listArgs: []driver.Value{
				"user-123", "create", "prompt", "ent-1", "sess-1", "%hello%", "%hello%", "2026-01-01", "2026-12-31",
			},
		},
		{
			name:         "pagination passes Limit and Offset through verbatim",
			filters:      repositories.ActivityFilters{Limit: 5, Offset: 20},
			countMatcher: `^SELECT COUNT\(\*\) FROM activities$`,
			countArgs:    nil,
			listMatcher:  `FROM activities ORDER BY created_at DESC LIMIT 5 OFFSET 20`,
			listArgs:     []driver.Value{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, mockDB := newSquirrelMockRepo(t)
			defer closeMockDB(t, mockDB)
			repo := &activityRepository{db: db}

			mock.ExpectQuery(tt.countMatcher).WithArgs(tt.countArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery(tt.listMatcher).WithArgs(tt.listArgs...).
				WillReturnRows(activityOneRow(now))

			resp, err := repo.List(context.Background(), tt.filters)

			require.NoError(t, err)
			assert.NotNil(t, resp.Activities, "Activities must be a non-nil slice")
			assert.Len(t, resp.Activities, 1)
			assert.Equal(t, 1, resp.TotalCount)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestActivityRepository_List_ExplicitProjection pins the full 11-column
// projection so column drift is caught.
func TestActivityRepository_List_ExplicitProjection(t *testing.T) {
	db, mock, mockDB := newSquirrelMockRepo(t)
	defer closeMockDB(t, mockDB)
	repo := &activityRepository{db: db}

	mock.ExpectQuery(`^SELECT COUNT\(\*\) FROM activities$`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(
		`SELECT id, user_id, activity_type, entity_type, entity_id, session_id, ` +
			`description, metadata, source_ip, user_agent, created_at ` +
			`FROM activities ORDER BY created_at DESC`,
	).WillReturnRows(activityOneRow(time.Now()))

	resp, err := repo.List(context.Background(), repositories.ActivityFilters{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Activities, 1)
	assert.Equal(t, "act-1", resp.Activities[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityRepository_List_ErrorPaths(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT`).WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count activities",
		},
		{
			name: "list query error wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT`).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM activities ORDER BY`).WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to query activities",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, mockDB := newSquirrelMockRepo(t)
			defer closeMockDB(t, mockDB)
			repo := &activityRepository{db: db}

			tt.setupMock(mock)

			resp, err := repo.List(context.Background(), repositories.ActivityFilters{Limit: 10})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, resp)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------

var promptGalleryListColumns = []string{
	"id", "title", "description", "content", "category", "tags", "metadata", "created_at", "updated_at",
}

func promptGalleryOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(promptGalleryListColumns).AddRow(
		"p-1", "Title", "Desc", "Body", "Engineering", []byte(`["a"]`), []byte(`{}`), now, now,
	)
}

// TestPromptGalleryRepository_List_JSONBTagsOperator is the critical footgun
// guard: squirrel must emit the Postgres `?|` operator (not a `$N` placeholder)
// when the tags filter is set. The fragment is asserted verbatim.
func TestPromptGalleryRepository_List_JSONBTagsOperator(t *testing.T) {
	now := time.Now()
	db, mock, mockDB := newSquirrelMockRepo(t)
	defer closeMockDB(t, mockDB)
	repo := &PromptGalleryRepository{db: db}

	// The tags argument is bound as a single pq.Array placeholder; the `?|`
	// operator itself must NOT consume a placeholder.
	mock.ExpectQuery(`FROM prompt_gallery_templates WHERE \(tags \?\| \$1\)$`).
		WithArgs(pq.Array([]string{"x", "y"})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM prompt_gallery_templates WHERE \(tags \?\| \$1\) ORDER BY created_at DESC LIMIT 10 OFFSET 0`).
		WithArgs(pq.Array([]string{"x", "y"})).
		WillReturnRows(promptGalleryOneRow(now))

	prompts, total, err := repo.List(context.Background(),
		repositories.PromptGalleryFilters{Tags: []string{"x", "y"}, Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, prompts, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven test exercising filters, empty-WHERE, pagination
func TestPromptGalleryRepository_List_SquirrelMigration(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		filters      repositories.PromptGalleryFilters
		countMatcher string
		countArgs    []driver.Value
		listMatcher  string
		listArgs     []driver.Value
	}{
		{
			name:         "no filters emits no WHERE clause",
			filters:      repositories.PromptGalleryFilters{Page: 1, Limit: 10},
			countMatcher: `^SELECT COUNT\(\*\) FROM prompt_gallery_templates$`,
			listMatcher:  `FROM prompt_gallery_templates ORDER BY created_at DESC LIMIT 10 OFFSET 0`,
			listArgs:     []driver.Value{},
		},
		{
			name:         "search binds one pattern across two ILIKE placeholders",
			filters:      repositories.PromptGalleryFilters{Search: "go", Page: 1, Limit: 10},
			countMatcher: `WHERE \(\(title ILIKE \$1 OR description ILIKE \$2\)\)`,
			countArgs:    []driver.Value{"%go%", "%go%"},
			listMatcher:  `WHERE \(\(title ILIKE \$1 OR description ILIKE \$2\)\)`,
			listArgs:     []driver.Value{"%go%", "%go%"},
		},
		{
			name:         "category and search combine before pagination",
			filters:      repositories.PromptGalleryFilters{Category: "Eng", Search: "go", Page: 2, Limit: 5},
			countMatcher: `WHERE \(category = \$1 AND \(title ILIKE \$2 OR description ILIKE \$3\)\)`,
			countArgs:    []driver.Value{"Eng", "%go%", "%go%"},
			listMatcher:  `LIMIT 5 OFFSET 5`,
			listArgs:     []driver.Value{"Eng", "%go%", "%go%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, mockDB := newSquirrelMockRepo(t)
			defer closeMockDB(t, mockDB)
			repo := &PromptGalleryRepository{db: db}

			mock.ExpectQuery(tt.countMatcher).WithArgs(tt.countArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery(tt.listMatcher).WithArgs(tt.listArgs...).
				WillReturnRows(promptGalleryOneRow(now))

			prompts, total, err := repo.List(context.Background(), tt.filters)

			require.NoError(t, err)
			assert.Equal(t, 1, total)
			assert.Len(t, prompts, 1)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPromptGalleryRepository_List_ErrorPaths(t *testing.T) {
	db, mock, mockDB := newSquirrelMockRepo(t)
	defer closeMockDB(t, mockDB)
	repo := &PromptGalleryRepository{db: db}

	mock.ExpectQuery(`SELECT COUNT`).WillReturnError(sql.ErrConnDone)

	prompts, total, err := repo.List(context.Background(), repositories.PromptGalleryFilters{Page: 1, Limit: 10})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count prompts")
	assert.Nil(t, prompts)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}
