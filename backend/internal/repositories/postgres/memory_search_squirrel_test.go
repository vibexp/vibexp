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

	"github.com/vibexp/vibexp/internal/repositories"
)

// searchBaseArgs is the argument set squirrel binds for every SearchByMetadata
// query regardless of optional filters: user_id, then the metadata key/value.
// It must return a fresh slice on every call — test cases append to the result,
// and a shared backing array would let cases corrupt each other.
func searchBaseArgs() []driver.Value {
	return []driver.Value{"user-123", "env", "prod"}
}

// searchDefaultArgs adds the trailing status-visibility argument bound by the
// default path (no keyword search, no explicit status filter): archived memories
// are excluded via `status <> 'archived'`.
func searchDefaultArgs() []driver.Value {
	return append(searchBaseArgs(), "archived")
}

//nolint:funlen // table-driven test with multiple filter scenarios
func TestMemoryRepository_SearchByMetadata_Squirrel(t *testing.T) {
	repo, mock, mockDB := setupMemoryListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	projectID := "project-x"
	draftStatus := "draft"

	oneRow := func() *sqlmock.Rows {
		return sqlmock.NewRows(memoryListColumns).AddRow(
			"memory-1", "user-123", "team-123", "project-123", "remember this",
			"active", []byte(`{"env":"prod"}`), now, now,
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
			name:    "base metadata filter binds user_id, key/value and hides archived",
			filters: repositories.MemoryFilters{Page: 1, Limit: 10},
			setupMock: func() {
				mock.ExpectQuery(
					`SELECT COUNT\(\*\) FROM memories WHERE ` +
						`\(user_id = \$1 AND metadata ->> \$2 = \$3 AND status <> \$4\)`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories WHERE .* ORDER BY created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "explicit status filter selects that status",
			filters: repositories.MemoryFilters{Status: &draftStatus, Page: 1, Limit: 10},
			setupMock: func() {
				args := append(searchBaseArgs(), "draft")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE .* AND status = \$4`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories WHERE .* AND status = \$4`).
					WithArgs(args...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "ProjectID filter binds project_id equality",
			filters: repositories.MemoryFilters{ProjectID: &projectID, Page: 1, Limit: 10},
			setupMock: func() {
				args := append(searchBaseArgs(), "project-x", "archived")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE .* AND project_id = \$4 AND status <> \$5`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories WHERE .* AND project_id = \$4 AND status <> \$5`).
					WithArgs(args...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "Search binds a single %term% via ILIKE and forces active status",
			filters: repositories.MemoryFilters{Search: "alpha", Page: 1, Limit: 10},
			setupMock: func() {
				args := append(searchBaseArgs(), "%alpha%", "active")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE .* AND text ILIKE \$4 AND status = \$5`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories WHERE .* AND text ILIKE \$4 AND status = \$5`).
					WithArgs(args...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "ProjectID and Search bind both extra args and force active",
			filters: repositories.MemoryFilters{ProjectID: &projectID, Search: "alpha", Page: 1, Limit: 10},
			setupMock: func() {
				args := append(searchBaseArgs(), "project-x", "%alpha%", "active")
				mock.ExpectQuery(
					`SELECT COUNT\(\*\) FROM memories WHERE .* AND project_id = \$4 AND text ILIKE \$5 AND status = \$6`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories WHERE .* AND project_id = \$4 AND text ILIKE \$5 AND status = \$6`).
					WithArgs(args...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "non-positive page and limit clamp to LIMIT 0 OFFSET 0",
			filters: repositories.MemoryFilters{Page: 0, Limit: -5},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
				mock.ExpectQuery(`FROM memories WHERE .* ORDER BY created_at DESC LIMIT 0 OFFSET 0`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows(memoryListColumns))
			},
			expectTotal: 3,
			expectCount: 0,
		},
		{
			name:    "pagination computes offset from page and limit",
			filters: repositories.MemoryFilters{Page: 3, Limit: 5},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
				// offset = (3-1)*5 = 10
				mock.ExpectQuery(`FROM memories WHERE .* ORDER BY created_at DESC LIMIT 5 OFFSET 10`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 20,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			memories, total, err := repo.SearchByMetadata(ctx, "user-123", "env", "prod", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, memories, "SearchByMetadata must return a non-nil empty slice, never nil")
			assert.Len(t, memories, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestMemoryRepository_SearchByMetadata_ExplicitProjection pins the full
// 9-column projection so a column drift fails the test.
func TestMemoryRepository_SearchByMetadata_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupMemoryListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE`).
		WithArgs(searchDefaultArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT id, user_id, team_id, project_id, text, status, metadata, created_at, updated_at ` +
			`FROM memories WHERE`,
	).
		WithArgs(searchDefaultArgs()...).
		WillReturnRows(sqlmock.NewRows(memoryListColumns).AddRow(
			"memory-1", "user-123", "team-123", "project-123", "remember this",
			"active", []byte(`{"env":"prod"}`), now, now,
		))

	memories, total, err := repo.SearchByMetadata(ctx, "user-123", "env", "prod",
		repositories.MemoryFilters{Page: 1, Limit: 10})
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, memories, 1)
	assert.Equal(t, "memory-1", memories[0].ID)
	assert.Equal(t, "active", memories[0].Status)
	assert.Equal(t, "prod", memories[0].Metadata["env"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven error-path test with multiple scenarios
func TestMemoryRepository_SearchByMetadata_ErrorPaths(t *testing.T) {
	now := time.Now()
	filters := repositories.MemoryFilters{Page: 1, Limit: 10}

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE`).
					WithArgs(searchDefaultArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count memories",
		},
		{
			name: "search query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories WHERE`).
					WithArgs(searchDefaultArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to search memories by metadata",
		},
		{
			name: "invalid metadata JSON propagates unmarshal error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM memories WHERE`).
					WithArgs(searchDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows(memoryListColumns).AddRow(
						"memory-1", "user-123", "team-123", "project-123", "remember this",
						"active", []byte(`{not valid json`), now, now,
					))
			},
			wantErr: "failed to unmarshal metadata",
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

			memories, total, err := repo.SearchByMetadata(context.Background(), "user-123", "env", "prod", filters)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, memories)
			assert.Zero(t, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
