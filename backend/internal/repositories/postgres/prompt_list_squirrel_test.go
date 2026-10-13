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

// promptListColumns mirrors the 18 columns scanned by List: 14 prompt columns
// (no version column; is_shared is computed, not stored) followed by the four
// lock columns.
var promptListColumns = withLockColumns(
	"id", "name", "slug", "description", "body", "user_id", "team_id", "project_id",
	"status", "mcp_expose", "labels", "created_at", "updated_at", "is_shared",
)

// setupPromptListTest builds a PromptRepository backed by a sqlmock connection.
func setupPromptListTest(t *testing.T) (*PromptRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewPromptRepository(&database.DB{DB: mockDB}).(*PromptRepository)
	return repo, mock, mockDB
}

// promptListBaseArgs is the base argument set squirrel binds for every List
// query, regardless of optional filters: team_id, then the team/user pair
// repeated per EXISTS clause.
func promptListBaseArgs() []driver.Value {
	return []driver.Value{"team-123", "team-123", "user-123", "team-123", "user-123"}
}

//nolint:funlen // table-driven test with multiple filter scenarios
func TestPromptRepository_List_SquirrelMigration(t *testing.T) {
	repo, mock, mockDB := setupPromptListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	locked, unlocked := true, false
	isSharedTrue := true
	mcpExposeTrue := true
	projectID := "project-x"

	oneRow := func() *sqlmock.Rows {
		return sqlmock.NewRows(promptListColumns).AddRow(unlockedRow(
			"prompt-1", "Prompt 1", "prompt-1", "Desc 1", "Body 1", "user-123", "team-123",
			"project-123", "published", true, "{}", now, now, false,
		)...)
	}

	tests := []struct {
		name        string
		filters     repositories.PromptFilters
		setupMock   func()
		expectTotal int
		expectCount int
	}{
		{
			name: "valid SortBy name asc orders ascending by name",
			filters: repositories.PromptFilters{
				TeamID: "team-123", SortBy: "name", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM prompts p .* ORDER BY p\.name ASC LIMIT 10 OFFSET 0`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ExcludeStatuses renders a status denylist beside the mcp_expose filter",
			filters: repositories.PromptFilters{
				TeamID: "team-123", ExcludeStatuses: []string{"draft"}, MCPExpose: &mcpExposeTrue,
				Page: 1, Limit: 10,
			},
			setupMock: func() {
				args := append(promptListBaseArgs(), "draft", true)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p .* AND p\.status NOT IN \(\$6\) AND p\.mcp_expose = \$7`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM prompts p .* AND p\.status NOT IN \(\$6\) AND p\.mcp_expose = \$7`).
					WithArgs(args...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "invalid SortBy falls back to default updated_at DESC ordering",
			filters: repositories.PromptFilters{
				TeamID: "team-123", SortBy: "; DROP TABLE prompts; --", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// The injection attempt must never reach the query; default ordering applies.
				mock.ExpectQuery(`FROM prompts p .* ORDER BY p\.updated_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "IsShared true uses DISTINCT, COUNT DISTINCT, LEFT JOIN and ps.id IS NOT NULL",
			filters: repositories.PromptFilters{
				TeamID: "team-123", IsShared: &isSharedTrue, Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(
					`SELECT COUNT\(DISTINCT p\.id\) FROM prompts p ` +
						`LEFT JOIN prompt_shares ps ON p\.id = ps\.prompt_id .* ` +
						`AND ps\.id IS NOT NULL\)`,
				).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(
					`SELECT DISTINCT p\.id, .* CASE WHEN ps\.id IS NOT NULL THEN true ELSE false END as is_shared, ` +
						`p\.locked_by::text, p\.locked_at, p\.lock_comment, lock_u\.name, lw\.source, lw\.assistant_name ` +
						`FROM prompts p LEFT JOIN prompt_shares ps ON .* ` +
						`LEFT JOIN users lock_u ON lock_u\.id = p\.locked_by LEFT JOIN LATERAL \(.*\) lw ON true WHERE .* AND ps\.id IS NOT NULL\) ` +
						`ORDER BY p\.updated_at DESC LIMIT 10 OFFSET 0`,
				).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "Search binds the same %term% three times across name/description/body",
			filters: repositories.PromptFilters{
				TeamID: "team-123", Search: "alpha", Page: 1, Limit: 10,
			},
			setupMock: func() {
				args := append(promptListBaseArgs(), "%alpha%", "%alpha%", "%alpha%")
				mock.ExpectQuery(
					`SELECT COUNT\(\*\) FROM prompts p .* ` +
						`AND \(p\.name ILIKE \$6 OR p\.description ILIKE \$7 OR p\.body ILIKE \$8\)`,
				).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(
					`FROM prompts p .* ` +
						`AND \(p\.name ILIKE \$6 OR p\.description ILIKE \$7 OR p\.body ILIKE \$8\)`,
				).
					WithArgs(args...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "Labels filter binds a pq.Array via the && overlap operator",
			filters: repositories.PromptFilters{
				TeamID: "team-123", Labels: []string{"go", "sql"}, Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p .* AND p\.labels && \$6`).
					WithArgs(append(promptListBaseArgs(), sqlmock.AnyArg())...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM prompts p .* AND p\.labels && \$6`).
					WithArgs(append(promptListBaseArgs(), sqlmock.AnyArg())...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ProjectID filter binds project_id equality",
			filters: repositories.PromptFilters{
				TeamID: "team-123", ProjectID: &projectID, Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p .* AND p\.project_id = \$6`).
					WithArgs(append(promptListBaseArgs(), "project-x")...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM prompts p .* AND p\.project_id = \$6`).
					WithArgs(append(promptListBaseArgs(), "project-x")...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "non-positive page and limit clamp to LIMIT 0 OFFSET 0",
			filters: repositories.PromptFilters{
				TeamID: "team-123", Page: 0, Limit: -5,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
				mock.ExpectQuery(`FROM prompts p .* ORDER BY p\.updated_at DESC LIMIT 0 OFFSET 0`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(promptListColumns))
			},
			expectTotal: 3,
			expectCount: 0,
		},
		{
			name: "pagination computes offset from page and limit",
			filters: repositories.PromptFilters{
				TeamID: "team-123", Page: 3, Limit: 5,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
				// offset = (3-1)*5 = 10
				mock.ExpectQuery(`FROM prompts p .* ORDER BY p\.updated_at DESC LIMIT 5 OFFSET 10`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 20,
			expectCount: 1,
		},
		{
			name: "locked=true narrows count and page to locked rows, binding no extra arg",
			filters: repositories.PromptFilters{
				TeamID: "team-123", Locked: &locked, Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p WHERE .* AND p\.locked_by IS NOT NULL\)`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM prompts p LEFT JOIN users lock_u .* AND p\.locked_by IS NOT NULL\) ORDER BY`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "locked=false narrows count and page to unlocked rows",
			filters: repositories.PromptFilters{
				TeamID: "team-123", Locked: &unlocked, Page: 1, Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p WHERE .* AND p\.locked_by IS NULL\)`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM prompts p LEFT JOIN users lock_u .* AND p\.locked_by IS NULL\) ORDER BY`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(oneRow())
			},
			expectTotal: 1,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			prompts, total, err := repo.List(ctx, "user-123", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, prompts, "List must return a non-nil empty slice, never nil")
			assert.Len(t, prompts, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestPromptRepository_List_ExplicitProjection pins the full 18-column projection
// (the four lock columns last) and the locker join for the default
// (non-IsShared) path. A `.+` matcher would not catch column
// drift, so the projection is asserted verbatim.
func TestPromptRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupPromptListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.PromptFilters{TeamID: "team-123", Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
		WithArgs(promptListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT p\.id, p\.name, p\.slug, p\.description, p\.body, p\.user_id, p\.team_id, p\.project_id, ` +
			`p\.status, p\.mcp_expose, p\.labels, p\.created_at, p\.updated_at, ` +
			`EXISTS\(SELECT 1 FROM prompt_shares ps2 WHERE ps2\.prompt_id = p\.id AND ps2\.is_active = true ` +
			`AND \(ps2\.expires_at IS NULL OR ps2\.expires_at > NOW\(\)\)\) as is_shared, ` +
			`p\.locked_by::text, p\.locked_at, p\.lock_comment, lock_u\.name, lw\.source, lw\.assistant_name ` +
			`FROM prompts p LEFT JOIN users lock_u ON lock_u\.id = p\.locked_by LEFT JOIN LATERAL \(.*\) lw ON true WHERE`,
	).
		WithArgs(promptListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows(promptListColumns).AddRow(unlockedRow(
			"prompt-1", "Prompt 1", "prompt-1", "Desc 1", "Body 1", "user-123", "team-123",
			"project-123", "published", true, "{}", now, now, false,
		)...))

	prompts, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, prompts, 1)
	assert.Equal(t, "prompt-1", prompts[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven error-path test with multiple scenarios
func TestPromptRepository_List_ErrorPaths(t *testing.T) {
	now := time.Now()
	filters := repositories.PromptFilters{TeamID: "team-123", Page: 1, Limit: 10}

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count prompts",
		},
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list prompts",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// One fewer column than the scan expects forces a scan error.
				mock.ExpectQuery(`FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("prompt-1"))
			},
			wantErr: "failed to scan prompt",
		},
		{
			name: "row iteration error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				rows := sqlmock.NewRows(promptListColumns).AddRow(unlockedRow(
					"prompt-1", "Prompt 1", "prompt-1", "Desc 1", "Body 1", "user-123", "team-123",
					"project-123", "published", true, "{}", now, now, false,
				)...).RowError(0, sql.ErrConnDone)
				mock.ExpectQuery(`FROM prompts p`).
					WithArgs(promptListBaseArgs()...).
					WillReturnRows(rows)
			},
			wantErr: "failed to iterate prompts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupPromptListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			prompts, total, err := repo.List(context.Background(), "user-123", filters)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, prompts)
			assert.Zero(t, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
