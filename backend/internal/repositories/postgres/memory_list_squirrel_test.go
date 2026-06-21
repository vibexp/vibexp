package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// memoryListColumns mirrors the 8 columns scanned by List (no version column on
// the list path).
var memoryListColumns = []string{
	"id", "user_id", "team_id", "project_id", "text", "metadata", "created_at", "updated_at",
}

// setupMemoryListTest builds a MemoryRepository backed by a sqlmock connection.
func setupMemoryListTest(t *testing.T) (*MemoryRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewMemoryRepository(&database.DB{DB: mockDB}).(*MemoryRepository)
	return repo, mock, mockDB
}

// memoryListBaseArgs is the base argument set squirrel binds for every List
// query, regardless of optional filters: team_id, then the team/user pair
// repeated per EXISTS clause.
func memoryListBaseArgs() []driver.Value {
	return []driver.Value{"team-123", "team-123", "user-123", "team-123", "user-123"}
}

//nolint:funlen // table-driven test with multiple filter scenarios
func TestMemoryRepository_List_SquirrelMigration(t *testing.T) {
	repo, mock, mockDB := setupMemoryListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	projectID := "project-x"
	metaKey := "env"
	metaValue := "prod"

	oneRow := func() *sqlmock.Rows {
		return sqlmock.NewRows(memoryListColumns).AddRow(
			"memory-1", "user-123", "team-123", "project-123", "remember this",
			[]byte(`{"env":"prod"}`), now, now,
		)
	}

	tests := []struct {
		name        string
		filters     repositories.MemoryFilters
		setupMock   func()
		expectTotal int
		expectCount int
	}{
		{
			name: "valid SortBy text asc orders ascending by text",
			filters: repositories.MemoryFilters{
				TeamID: "team-123", SortBy: "text", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories m .* ORDER BY m\.text ASC LIMIT 10 OFFSET 0`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "invalid SortBy falls back to default updated_at DESC ordering",
			filters: repositories.MemoryFilters{
				TeamID: "team-123", SortBy: "; DROP TABLE memories; --", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// The injection attempt must never reach the query; default ordering applies.
				mock.ExpectQuery(`FROM memories m .* ORDER BY m\.updated_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "Search binds a single %term% via ILIKE",
			filters: repositories.MemoryFilters{
				TeamID: "team-123", Search: "alpha", Page: 1, Limit: 10,
			},
			setupMock: func() {
				args := append(memoryListBaseArgs(), "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m .* AND m\.text ILIKE \$6`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories m .* AND m\.text ILIKE \$6`).
					WithArgs(args...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ProjectID filter binds project_id equality",
			filters: repositories.MemoryFilters{
				TeamID: "team-123", ProjectID: &projectID, Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m .* AND m\.project_id = \$6`).
					WithArgs(append(memoryListBaseArgs(), "project-x")...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories m .* AND m\.project_id = \$6`).
					WithArgs(append(memoryListBaseArgs(), "project-x")...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "MetadataKey and MetadataValue bind two args via ->> operator",
			filters: repositories.MemoryFilters{
				TeamID: "team-123", MetadataKey: &metaKey, MetadataValue: &metaValue, Page: 1, Limit: 10,
			},
			setupMock: func() {
				args := append(memoryListBaseArgs(), "env", "prod")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m .* AND m\.metadata ->> \$6 = \$7`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories m .* AND m\.metadata ->> \$6 = \$7`).
					WithArgs(args...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "non-positive page and limit clamp to LIMIT 0 OFFSET 0",
			filters: repositories.MemoryFilters{
				TeamID: "team-123", Page: 0, Limit: -5,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
				mock.ExpectQuery(`FROM memories m .* ORDER BY m\.updated_at DESC LIMIT 0 OFFSET 0`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(memoryListColumns))
			},
			expectTotal: 3,
			expectCount: 0,
		},
		{
			name: "pagination computes offset from page and limit",
			filters: repositories.MemoryFilters{
				TeamID: "team-123", Page: 3, Limit: 5,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
				// offset = (3-1)*5 = 10
				mock.ExpectQuery(`FROM memories m .* ORDER BY m\.updated_at DESC LIMIT 5 OFFSET 10`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 20,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			memories, total, err := repo.List(ctx, "user-123", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, memories, "List must return a non-nil empty slice, never nil")
			assert.Len(t, memories, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestMemoryRepository_List_RequiresTeamID verifies the required-TeamID guard
// short-circuits before any query is issued.
func TestMemoryRepository_List_RequiresTeamID(t *testing.T) {
	repo, mock, mockDB := setupMemoryListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	memories, total, err := repo.List(context.Background(), "user-123", repositories.MemoryFilters{Page: 1, Limit: 10})

	require.Error(t, err)
	assert.EqualError(t, err, "TeamID is required but was empty")
	assert.Nil(t, memories)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_List_ExplicitProjection pins the full 8-column projection
// for the default path. A `.+` matcher would not catch column drift, so the
// projection is asserted verbatim.
func TestMemoryRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupMemoryListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.MemoryFilters{TeamID: "team-123", Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
		WithArgs(memoryListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT m\.id, m\.user_id, m\.team_id, m\.project_id, ` +
			`m\.text, m\.metadata, m\.created_at, m\.updated_at ` +
			`FROM memories m WHERE`,
	).
		WithArgs(memoryListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows(memoryListColumns).AddRow(
			"memory-1", "user-123", "team-123", "project-123", "remember this",
			[]byte(`{"env":"prod"}`), now, now,
		))

	memories, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, memories, 1)
	assert.Equal(t, "memory-1", memories[0].ID)
	assert.Equal(t, "prod", memories[0].Metadata["env"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven error-path test with multiple scenarios
func TestMemoryRepository_List_ErrorPaths(t *testing.T) {
	now := time.Now()
	filters := repositories.MemoryFilters{TeamID: "team-123", Page: 1, Limit: 10}

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count memories",
		},
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list memories",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// One column instead of the eight the scan expects forces a scan error.
				mock.ExpectQuery(`FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("memory-1"))
			},
			wantErr: "failed to scan memory",
		},
		{
			name: "invalid metadata JSON propagates unmarshal error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(memoryListColumns).AddRow(
						"memory-1", "user-123", "team-123", "project-123", "remember this",
						[]byte(`{not valid json`), now, now,
					))
			},
			wantErr: "failed to unmarshal metadata",
		},
		{
			name: "row iteration error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				rows := sqlmock.NewRows(memoryListColumns).AddRow(
					"memory-1", "user-123", "team-123", "project-123", "remember this",
					[]byte(`{"env":"prod"}`), now, now,
				).RowError(0, sql.ErrConnDone)
				mock.ExpectQuery(`FROM memories m`).
					WithArgs(memoryListBaseArgs()...).
					WillReturnRows(rows)
			},
			wantErr: "failed to iterate memories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupMemoryListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			memories, total, err := repo.List(context.Background(), "user-123", filters)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, memories)
			assert.Zero(t, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
