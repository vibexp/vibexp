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

// blueprintListColumnsTest mirrors the 13 columns scanned by List (content
// excluded from list operations).
var blueprintListColumnsTest = []string{
	"id", "project_id", "slug", "user_id", "team_id", "title", "description",
	"status", "type", "subtype", "metadata", "created_at", "updated_at",
}

// setupBlueprintListTest builds a BlueprintRepository backed by a sqlmock
// connection.
func setupBlueprintListTest(t *testing.T) (*BlueprintRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewBlueprintRepository(&database.DB{DB: mockDB}).(*BlueprintRepository)
	return repo, mock, mockDB
}

// blueprintListBaseArgs is the base argument set squirrel binds for every
// team-scoped List query: team_id, then the team/user pair repeated per EXISTS
// clause.
func blueprintListBaseArgs() []driver.Value {
	return []driver.Value{"team-123", "team-123", "user-123", "team-123", "user-123"}
}

func blueprintListOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(blueprintListColumnsTest).AddRow(
		"blueprint-1", "project-1", "slug-1", "user-123", "team-123",
		"Title", "Description", "active", "general", "openapi",
		[]byte(`{"env":"prod"}`), now, now,
	)
}

//nolint:funlen // table-driven test with multiple filter scenarios
func TestBlueprintRepository_ListSquirrel(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	projectID := "project-x"

	tests := []struct {
		name        string
		filters     repositories.BlueprintFilters
		setupMock   func(mock sqlmock.Sqlmock)
		expectTotal int
		expectCount int
	}{
		{
			name:    "baseline binds team-scoped base args and default ordering",
			filters: repositories.BlueprintFilters{TeamID: "team-123", Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s .* ORDER BY s\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(blueprintListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "combined filters bind in deterministic order incl subtype",
			filters: repositories.BlueprintFilters{
				TeamID: "team-123", ProjectID: &projectID,
				Status: ptr("active"), Type: ptr("general"), Subtype: ptr("openapi"),
				Search: "alpha", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(blueprintListBaseArgs(),
					"project-x", "active", "general", "openapi", "%alpha%", "%alpha%", "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s .*s\.project_id = .*s\.status = .*s\.type = ` +
					`.*s\.subtype = .*s\.title ILIKE .* OR s\.description ILIKE .* OR s\.content ILIKE`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s .*s\.project_id = .*s\.status = .*s\.type = ` +
					`.*s\.subtype = .*s\.title ILIKE .* OR s\.description ILIKE .* OR s\.content ILIKE`).
					WithArgs(args...).
					WillReturnRows(blueprintListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "subtype-only filter binds after base args",
			filters: repositories.BlueprintFilters{
				TeamID: "team-123", Subtype: ptr("openapi"), Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(blueprintListBaseArgs(), "openapi")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s .*s\.subtype = `).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s .*s\.subtype = `).
					WithArgs(args...).
					WillReturnRows(blueprintListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "invalid SortBy falls back to created_at DESC",
			filters: repositories.BlueprintFilters{
				TeamID: "team-123", SortBy: "; DROP TABLE blueprints; --", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s .* ORDER BY s\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(blueprintListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "valid SortBy updated_at asc orders ascending",
			filters: repositories.BlueprintFilters{
				TeamID: "team-123", SortBy: "updated_at", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s .* ORDER BY s\.updated_at ASC LIMIT 10 OFFSET 0`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(blueprintListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "metadata single key binds via ->> operator",
			filters: repositories.BlueprintFilters{
				TeamID: "team-123", Metadata: map[string]string{"env": "prod"}, Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(blueprintListBaseArgs(), "env", "prod")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s .*s\.metadata->>\$6 = \$7`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s .*s\.metadata->>\$6 = \$7`).
					WithArgs(args...).
					WillReturnRows(blueprintListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "metadata multi-key binds in sorted key order",
			filters: repositories.BlueprintFilters{
				TeamID: "team-123", Metadata: map[string]string{"zeta": "z", "alpha": "a"}, Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				// Sorted iteration guarantees alpha is bound before zeta regardless of
				// map ordering, so the parameter sequence is deterministic.
				args := append(blueprintListBaseArgs(), "alpha", "a", "zeta", "z")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s .*s\.metadata->>\$6 = \$7 AND s\.metadata->>\$8 = \$9`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s .*s\.metadata->>\$6 = \$7 AND s\.metadata->>\$8 = \$9`).
					WithArgs(args...).
					WillReturnRows(blueprintListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "clamps non-positive page and limit to LIMIT 0 OFFSET 0",
			filters: repositories.BlueprintFilters{TeamID: "team-123", Page: 0, Limit: -5},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
				mock.ExpectQuery(`FROM blueprints s .* LIMIT 0 OFFSET 0`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(blueprintListColumnsTest))
			},
			expectTotal: 3,
			expectCount: 0,
		},
		{
			name:    "computes offset from page and limit",
			filters: repositories.BlueprintFilters{TeamID: "team-123", Page: 3, Limit: 5},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
				// offset = (3-1)*5 = 10
				mock.ExpectQuery(`FROM blueprints s .* LIMIT 5 OFFSET 10`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(blueprintListOneRow(now))
			},
			expectTotal: 20,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupBlueprintListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			got, total, err := repo.List(ctx, "user-123", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, got, "list must return a non-nil empty slice, never nil")
			assert.Len(t, got, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestBlueprintRepository_List_ExplicitProjection pins the full 13-column
// projection (content excluded) for the List path. A `.+` matcher would not
// catch column drift, so the projection is asserted verbatim.
func TestBlueprintRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupBlueprintListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.BlueprintFilters{TeamID: "team-123", Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
		WithArgs(blueprintListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT s\.id, s\.project_id, s\.slug, s\.user_id, s\.team_id, ` +
			`s\.title, s\.description, s\.status, s\.type, s\.subtype, s\.metadata, ` +
			`s\.created_at, s\.updated_at FROM blueprints s WHERE`,
	).
		WithArgs(blueprintListBaseArgs()...).
		WillReturnRows(blueprintListOneRow(now))

	blueprints, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, blueprints, 1)
	assert.Equal(t, "blueprint-1", blueprints[0].ID)
	assert.Equal(t, "prod", blueprints[0].Metadata["env"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_List_RequiresTeamID verifies the required-TeamID guard
// short-circuits before any query is issued.
func TestBlueprintRepository_List_RequiresTeamID(t *testing.T) {
	repo, mock, mockDB := setupBlueprintListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	blueprints, total, err := repo.List(
		context.Background(), "user-123", repositories.BlueprintFilters{Page: 1, Limit: 10},
	)

	require.Error(t, err)
	assert.EqualError(t, err, "TeamID is required but was empty")
	assert.Nil(t, blueprints)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_List_InvalidMetadataKey verifies an invalid metadata
// key short-circuits before any query is issued.
func TestBlueprintRepository_List_InvalidMetadataKey(t *testing.T) {
	repo, mock, mockDB := setupBlueprintListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	filters := repositories.BlueprintFilters{
		TeamID:   "team-123",
		Metadata: map[string]string{"bad key'; DROP TABLE blueprints; --": "v"},
		Page:     1, Limit: 10,
	}

	_, _, err := repo.List(context.Background(), "user-123", filters)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid metadata key")
	// No query must have been issued before the validation failure.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_List_EmptyMetadataInitializesMap verifies the
// blueprint-specific quirk: an empty/NULL metadata column yields an initialized
// empty map rather than nil, without attempting to unmarshal.
func TestBlueprintRepository_List_EmptyMetadataInitializesMap(t *testing.T) {
	repo, mock, mockDB := setupBlueprintListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.BlueprintFilters{TeamID: "team-123", Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
		WithArgs(blueprintListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM blueprints s`).
		WithArgs(blueprintListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows(blueprintListColumnsTest).AddRow(
			"blueprint-1", "project-1", "slug-1", "user-123", "team-123",
			"Title", "Description", "active", "general", nil,
			nil, now, now, // NULL metadata
		))

	blueprints, total, err := repo.List(ctx, "user-123", filters)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, blueprints, 1)
	assert.NotNil(t, blueprints[0].Metadata, "empty metadata must initialize a non-nil map")
	assert.Empty(t, blueprints[0].Metadata)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven error-path test
func TestBlueprintRepository_ListSquirrel_ErrorPaths(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count blueprints",
		},
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list blueprint entries",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// Returning fewer columns than scanned triggers a scan error.
				mock.ExpectQuery(`FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("blueprint-1"))
			},
			wantErr: "failed to scan blueprint",
		},
		{
			name: "non-empty malformed metadata returns unmarshal error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(blueprintListColumnsTest).AddRow(
						"blueprint-1", "project-1", "slug-1", "user-123", "team-123",
						"Title", "Description", "active", "general", "openapi",
						[]byte(`{not valid json`), now, now,
					))
			},
			wantErr: "failed to unmarshal metadata",
		},
		{
			name: "iterate error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM blueprints s`).
					WithArgs(blueprintListBaseArgs()...).
					WillReturnRows(blueprintListOneRow(now).RowError(0, sql.ErrConnDone))
			},
			wantErr: "failed to iterate blueprint entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupBlueprintListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			_, _, err := repo.List(context.Background(), "user-123",
				repositories.BlueprintFilters{TeamID: "team-123", Page: 1, Limit: 10})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
