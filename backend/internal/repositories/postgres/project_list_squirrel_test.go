package postgres_test

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
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// projectListColumns mirrors the 11 columns scanned by List (including version,
// unlike the memory/artifact list paths).
var projectListColumns = []string{
	"id", "user_id", "team_id", "name", "slug", "description",
	"git_url", "homepage", "created_at", "updated_at", "version",
}

// setupProjectListTest builds a ProjectRepository backed by a sqlmock connection.
func setupProjectListTest(t *testing.T) (repositories.ProjectRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := postgres.NewProjectRepository(&database.DB{DB: mockDB})
	return repo, mock, mockDB
}

// projectListBaseArgs is the base argument set squirrel binds for every List
// query, regardless of optional filters: team_id, then the team/user pair
// repeated per EXISTS clause.
func projectListBaseArgs() []driver.Value {
	return []driver.Value{"team-123", "team-123", "user-123", "team-123", "user-123"}
}

func projectOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(projectListColumns).AddRow(
		"project-1", "user-123", "team-123", "Mobile App", "mobile-app",
		"an app", "https://github.com/org/app", "https://app.example.com",
		now, now, 1,
	)
}

//nolint:funlen // table-driven test with multiple filter and ordering scenarios
func TestProjectRepository_List_SquirrelMigration(t *testing.T) {
	repo, mock, mockDB := setupProjectListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name        string
		filters     repositories.ProjectListFilters
		setupMock   func()
		expectTotal int
		expectCount int
	}{
		{
			name: "default ordering is created_at DESC",
			filters: repositories.ProjectListFilters{
				TeamID: "team-123", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM projects p .* ORDER BY p\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(projectOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "valid SortBy name asc orders ascending by name",
			filters: repositories.ProjectListFilters{
				TeamID: "team-123", SortBy: "name", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM projects p .* ORDER BY p\.name ASC LIMIT 10 OFFSET 0`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(projectOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "valid SortBy slug desc orders descending by slug",
			filters: repositories.ProjectListFilters{
				TeamID: "team-123", SortBy: "slug", SortOrder: "desc", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM projects p .* ORDER BY p\.slug DESC LIMIT 10 OFFSET 0`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(projectOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "valid SortBy updated_at asc orders ascending by updated_at",
			filters: repositories.ProjectListFilters{
				TeamID: "team-123", SortBy: "updated_at", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM projects p .* ORDER BY p\.updated_at ASC LIMIT 10 OFFSET 0`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(projectOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "invalid SortBy falls back to default created_at DESC ordering",
			filters: repositories.ProjectListFilters{
				TeamID: "team-123", SortBy: "; DROP TABLE projects; --", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// The injection attempt must never reach the query; default ordering applies.
				mock.ExpectQuery(`FROM projects p .* ORDER BY p\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(projectOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "Search binds the same %term% three times across name, description, slug",
			filters: repositories.ProjectListFilters{
				TeamID: "team-123", Search: "alpha", Page: 1, Limit: 10,
			},
			setupMock: func() {
				args := append(projectListBaseArgs(), "%alpha%", "%alpha%", "%alpha%")
				mock.ExpectQuery(
					`SELECT COUNT\(\*\) FROM projects p .* ` +
						`AND \(p\.name ILIKE \$6 OR p\.description ILIKE \$7 OR p\.slug ILIKE \$8\)`,
				).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(
					`FROM projects p .* ` +
						`AND \(p\.name ILIKE \$6 OR p\.description ILIKE \$7 OR p\.slug ILIKE \$8\)`,
				).
					WithArgs(args...).
					WillReturnRows(projectOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "non-positive page and limit clamp to LIMIT 0 OFFSET 0",
			filters: repositories.ProjectListFilters{
				TeamID: "team-123", Page: 0, Limit: -5,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
				mock.ExpectQuery(`FROM projects p .* ORDER BY p\.created_at DESC LIMIT 0 OFFSET 0`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(projectListColumns))
			},
			expectTotal: 3,
			expectCount: 0,
		},
		{
			name: "pagination computes offset from page and limit",
			filters: repositories.ProjectListFilters{
				TeamID: "team-123", Page: 3, Limit: 5,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
				// offset = (3-1)*5 = 10
				mock.ExpectQuery(`FROM projects p .* ORDER BY p\.created_at DESC LIMIT 5 OFFSET 10`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(projectOneRow(now))
			},
			expectTotal: 20,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			projects, total, err := repo.List(ctx, "user-123", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, projects, "List must return a non-nil empty slice, never nil")
			assert.Len(t, projects, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestProjectRepository_List_RequiresTeamID verifies the required-TeamID guard
// short-circuits before any query is issued.
func TestProjectRepository_List_RequiresTeamID(t *testing.T) {
	repo, mock, mockDB := setupProjectListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	filters := repositories.ProjectListFilters{Page: 1, Limit: 10}
	projects, total, err := repo.List(context.Background(), "user-123", filters)

	require.Error(t, err)
	assert.EqualError(t, err, "TeamID is required but was empty")
	assert.Nil(t, projects)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_List_ExplicitProjection pins the full 11-column
// projection for the default path. A `.+` matcher would not catch column drift,
// so the projection is asserted verbatim.
func TestProjectRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupProjectListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.ProjectListFilters{TeamID: "team-123", Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
		WithArgs(projectListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT p\.id, p\.user_id, p\.team_id, p\.name, p\.slug, p\.description, ` +
			`p\.git_url, p\.homepage, p\.created_at, p\.updated_at, p\.version ` +
			`FROM projects p WHERE`,
	).
		WithArgs(projectListBaseArgs()...).
		WillReturnRows(projectOneRow(now))

	projects, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, projects, 1)
	assert.Equal(t, "project-1", projects[0].ID)
	assert.Equal(t, "mobile-app", projects[0].Slug)
	assert.Equal(t, int64(1), projects[0].Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven error-path test with multiple scenarios
func TestProjectRepository_List_ErrorPaths(t *testing.T) {
	filters := repositories.ProjectListFilters{TeamID: "team-123", Page: 1, Limit: 10}

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count projects",
		},
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list projects",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// One column instead of the eleven the scan expects forces a scan error.
				mock.ExpectQuery(`FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("project-1"))
			},
			wantErr: "failed to scan project",
		},
		{
			name: "row iteration error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				now := time.Now()
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				rows := projectOneRow(now).RowError(0, sql.ErrConnDone)
				mock.ExpectQuery(`FROM projects p`).
					WithArgs(projectListBaseArgs()...).
					WillReturnRows(rows)
			},
			wantErr: "failed to iterate projects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupProjectListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			projects, total, err := repo.List(context.Background(), "user-123", filters)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, projects)
			assert.Zero(t, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
