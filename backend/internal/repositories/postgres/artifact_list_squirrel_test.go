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

// artifactListColumnsTest mirrors the 12 columns scanned by List and
// ListCrossTeam (content excluded from list operations).
var artifactListColumnsTest = []string{
	"id", "project_id", "slug", "user_id", "team_id", "title", "description",
	"status", "type", "metadata", "created_at", "updated_at",
}

// setupArtifactListTest builds an ArtifactRepository backed by a sqlmock
// connection.
func setupArtifactListTest(t *testing.T) (*ArtifactRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewArtifactRepository(&database.DB{DB: mockDB}).(*ArtifactRepository)
	return repo, mock, mockDB
}

// artifactListBaseArgs is the base argument set squirrel binds for every team-scoped
// List query: team_id, then the team/user pair repeated per EXISTS clause.
func artifactListBaseArgs() []driver.Value {
	return []driver.Value{"team-123", "team-123", "user-123", "team-123", "user-123"}
}

// artifactCrossTeamBaseArgs is the base argument set squirrel binds for every
// ListCrossTeam query: user_id, then user_id repeated for each EXISTS clause.
func artifactCrossTeamBaseArgs() []driver.Value {
	return []driver.Value{"user-123", "user-123", "user-123"}
}

// artifactListDefaultArgs and artifactCrossTeamDefaultArgs append the implicit
// "hide archived" bind that the default list path (no search, no explicit status
// filter) adds via `a.status <> 'archived'`.
func artifactListDefaultArgs() []driver.Value {
	return append(artifactListBaseArgs(), "archived")
}

func artifactCrossTeamDefaultArgs() []driver.Value {
	return append(artifactCrossTeamBaseArgs(), "archived")
}

func artifactListOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(artifactListColumnsTest).AddRow(
		"artifact-1", "project-1", "slug-1", "user-123", "team-123",
		"Title", "Description", "active", "general",
		[]byte(`{"env":"prod"}`), now, now,
	)
}

//nolint:funlen // table-driven test with multiple filter scenarios for both list methods
func TestArtifactRepository_ListSquirrel(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	projectID := "project-x"

	tests := []struct {
		name        string
		filters     repositories.ArtifactFilters
		crossTeam   bool
		setupMock   func(mock sqlmock.Sqlmock)
		expectTotal int
		expectCount int
	}{
		{
			name:    "List default hides archived and uses default ordering",
			filters: repositories.ArtifactFilters{TeamID: "team-123", Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.status <> `).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.status <> .* ORDER BY a\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "List explicit status filter selects that status and keeps archived reachable",
			filters: repositories.ArtifactFilters{
				TeamID: "team-123", Status: ptr("draft"), Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(artifactListBaseArgs(), "draft")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.status = `).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.status = `).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "List search forces active status and adds ILIKE",
			filters: repositories.ArtifactFilters{
				TeamID: "team-123", Search: "alpha", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(artifactListBaseArgs(), "active", "%alpha%", "%alpha%", "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.status = ` +
					`.*a\.title ILIKE .* OR a\.description ILIKE .* OR a\.content ILIKE`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.status = .*a\.title ILIKE`).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "List search overrides an explicit status filter with active",
			filters: repositories.ArtifactFilters{
				TeamID: "team-123", Status: ptr("draft"), Search: "alpha", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				// 'draft' is intentionally absent: search forces active-only.
				args := append(artifactListBaseArgs(), "active", "%alpha%", "%alpha%", "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.status = .*ILIKE`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.status = .*ILIKE`).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "List project and type filters combine with search-forced active",
			filters: repositories.ArtifactFilters{
				TeamID: "team-123", ProjectID: &projectID, Type: ptr("general"),
				Search: "alpha", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(artifactListBaseArgs(),
					"project-x", "general", "active", "%alpha%", "%alpha%", "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.project_id = .*a\.type = ` +
					`.*a\.status = .*a\.title ILIKE .* OR a\.description ILIKE .* OR a\.content ILIKE`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.project_id = .*a\.type = ` +
					`.*a\.status = .*a\.title ILIKE`).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "List invalid SortBy falls back to created_at DESC",
			filters: repositories.ArtifactFilters{
				TeamID: "team-123", SortBy: "; DROP TABLE artifacts; --", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .* ORDER BY a\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "List valid SortBy updated_at asc orders ascending",
			filters: repositories.ArtifactFilters{
				TeamID: "team-123", SortBy: "updated_at", SortOrder: "asc", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .* ORDER BY a\.updated_at ASC LIMIT 10 OFFSET 0`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "List metadata single key binds via ->> operator",
			filters: repositories.ArtifactFilters{
				TeamID: "team-123", Metadata: map[string]string{"env": "prod"}, Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(artifactListDefaultArgs(), "env", "prod")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.metadata->>\$7 = \$8`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.metadata->>\$7 = \$8`).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "List metadata multi-key binds in sorted key order",
			filters: repositories.ArtifactFilters{
				TeamID: "team-123", Metadata: map[string]string{"zeta": "z", "alpha": "a"}, Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				// Sorted iteration guarantees alpha is bound before zeta regardless of
				// map ordering, so the parameter sequence is deterministic.
				args := append(artifactListDefaultArgs(), "alpha", "a", "zeta", "z")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.metadata->>\$7 = \$8 AND a\.metadata->>\$9 = \$10`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.metadata->>\$7 = \$8 AND a\.metadata->>\$9 = \$10`).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "List clamps non-positive page and limit to LIMIT 0 OFFSET 0",
			filters: repositories.ArtifactFilters{TeamID: "team-123", Page: 0, Limit: -5},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
				mock.ExpectQuery(`FROM artifacts a .* LIMIT 0 OFFSET 0`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows(artifactListColumnsTest))
			},
			expectTotal: 3,
			expectCount: 0,
		},
		{
			name:    "List computes offset from page and limit",
			filters: repositories.ArtifactFilters{TeamID: "team-123", Page: 3, Limit: 5},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
				// offset = (3-1)*5 = 10
				mock.ExpectQuery(`FROM artifacts a .* LIMIT 5 OFFSET 10`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 20,
			expectCount: 1,
		},
		{
			name:      "ListCrossTeam default hides archived and uses default ordering",
			filters:   repositories.ArtifactFilters{Page: 1, Limit: 10},
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.status <> `).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.status <> .* ORDER BY a\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ListCrossTeam project and type filters combine with search-forced active",
			filters: repositories.ArtifactFilters{
				ProjectID: &projectID, Type: ptr("general"),
				Search: "alpha", Page: 1, Limit: 10,
			},
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(artifactCrossTeamBaseArgs(),
					"project-x", "general", "active", "%alpha%", "%alpha%", "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.project_id = .*a\.type = ` +
					`.*a\.status = .*a\.title ILIKE .* OR a\.description ILIKE .* OR a\.content ILIKE`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.project_id = .*a\.type = ` +
					`.*a\.status = .*a\.title ILIKE`).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ListCrossTeam invalid SortBy falls back to created_at DESC",
			filters: repositories.ArtifactFilters{
				SortBy: "1; DELETE FROM artifacts", SortOrder: "asc", Page: 1, Limit: 10,
			},
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .* ORDER BY a\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ListCrossTeam valid SortBy updated_at asc orders ascending",
			filters: repositories.ArtifactFilters{
				SortBy: "updated_at", SortOrder: "asc", Page: 1, Limit: 10,
			},
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .* ORDER BY a\.updated_at ASC LIMIT 10 OFFSET 0`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ListCrossTeam metadata single key binds via ->> operator",
			filters: repositories.ArtifactFilters{
				Metadata: map[string]string{"env": "prod"}, Page: 1, Limit: 10,
			},
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(artifactCrossTeamDefaultArgs(), "env", "prod")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.metadata->>\$5 = \$6`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.metadata->>\$5 = \$6`).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ListCrossTeam metadata multi-key binds in sorted key order",
			filters: repositories.ArtifactFilters{
				Metadata: map[string]string{"zeta": "z", "alpha": "a"}, Page: 1, Limit: 10,
			},
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(artifactCrossTeamDefaultArgs(), "alpha", "a", "zeta", "z")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a .*a\.metadata->>\$5 = \$6 AND a\.metadata->>\$7 = \$8`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a .*a\.metadata->>\$5 = \$6 AND a\.metadata->>\$7 = \$8`).
					WithArgs(args...).
					WillReturnRows(artifactListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:      "ListCrossTeam clamps non-positive page and limit to LIMIT 0 OFFSET 0",
			filters:   repositories.ArtifactFilters{Page: 0, Limit: -5},
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
				mock.ExpectQuery(`FROM artifacts a .* LIMIT 0 OFFSET 0`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows(artifactListColumnsTest))
			},
			expectTotal: 2,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupArtifactListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			list := repo.List
			if tt.crossTeam {
				list = repo.ListCrossTeam
			}
			got, total, err := list(ctx, "user-123", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, got, "list must return a non-nil empty slice, never nil")
			assert.Len(t, got, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestArtifactRepository_List_ExplicitProjection pins the full 12-column
// projection (content excluded) for the default List path. A `.+` matcher would
// not catch column drift, so the projection is asserted verbatim.
func TestArtifactRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupArtifactListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.ArtifactFilters{TeamID: "team-123", Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
		WithArgs(artifactListDefaultArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id, ` +
			`a\.title, a\.description, a\.status, a\.type, a\.metadata, ` +
			`a\.created_at, a\.updated_at FROM artifacts a WHERE`,
	).
		WithArgs(artifactListDefaultArgs()...).
		WillReturnRows(artifactListOneRow(now))

	artifacts, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "artifact-1", artifacts[0].ID)
	assert.Equal(t, "prod", artifacts[0].Metadata["env"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_ListCrossTeam_ExplicitProjection pins the full
// 12-column projection for the default ListCrossTeam path.
func TestArtifactRepository_ListCrossTeam_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupArtifactListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.ArtifactFilters{Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
		WithArgs(artifactCrossTeamDefaultArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id, ` +
			`a\.title, a\.description, a\.status, a\.type, a\.metadata, ` +
			`a\.created_at, a\.updated_at FROM artifacts a WHERE`,
	).
		WithArgs(artifactCrossTeamDefaultArgs()...).
		WillReturnRows(artifactListOneRow(now))

	artifacts, total, err := repo.ListCrossTeam(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "artifact-1", artifacts[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_List_RequiresTeamID verifies the required-TeamID guard
// short-circuits before any query is issued.
func TestArtifactRepository_List_RequiresTeamID(t *testing.T) {
	repo, mock, mockDB := setupArtifactListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	artifacts, total, err := repo.List(context.Background(), "user-123", repositories.ArtifactFilters{Page: 1, Limit: 10})

	require.Error(t, err)
	assert.EqualError(t, err, "TeamID is required but was empty")
	assert.Nil(t, artifacts)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_ListSquirrel_InvalidMetadataKey verifies an invalid
// metadata key short-circuits before any query is issued, for both methods.
func TestArtifactRepository_ListSquirrel_InvalidMetadataKey(t *testing.T) {
	tests := []struct {
		name      string
		crossTeam bool
	}{
		{name: "List rejects invalid metadata key", crossTeam: false},
		{name: "ListCrossTeam rejects invalid metadata key", crossTeam: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupArtifactListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			filters := repositories.ArtifactFilters{
				TeamID:   "team-123",
				Metadata: map[string]string{"bad key'; DROP TABLE artifacts; --": "v"},
				Page:     1, Limit: 10,
			}

			var err error
			if tt.crossTeam {
				_, _, err = repo.ListCrossTeam(context.Background(), "user-123", filters)
			} else {
				_, _, err = repo.List(context.Background(), "user-123", filters)
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid metadata key")
			// No query must have been issued before the validation failure.
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven error-path test covering both list methods
func TestArtifactRepository_ListSquirrel_ErrorPaths(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		crossTeam bool
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "List unmarshal error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a`).
					WithArgs(artifactListDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows(artifactListColumnsTest).AddRow(
						"artifact-1", "project-1", "slug-1", "user-123", "team-123",
						"Title", "Description", "active", "general",
						[]byte(`{not valid json`), now, now,
					))
			},
			wantErr: "failed to unmarshal metadata",
		},
		{
			name:      "ListCrossTeam unmarshal error keeps cross-team suffix",
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows(artifactListColumnsTest).AddRow(
						"artifact-1", "project-1", "slug-1", "user-123", "team-123",
						"Title", "Description", "active", "general",
						[]byte(`{not valid json`), now, now,
					))
			},
			wantErr: "failed to unmarshal metadata (cross-team)",
		},
		{
			name:      "ListCrossTeam count error keeps cross-team suffix",
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count artifacts (cross-team)",
		},
		{
			name:      "ListCrossTeam list query error keeps cross-team suffix",
			crossTeam: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM artifacts a`).
					WithArgs(artifactCrossTeamDefaultArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list artifacts (cross-team)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupArtifactListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			var err error
			if tt.crossTeam {
				_, _, err = repo.ListCrossTeam(context.Background(), "user-123",
					repositories.ArtifactFilters{Page: 1, Limit: 10})
			} else {
				_, _, err = repo.List(context.Background(), "user-123",
					repositories.ArtifactFilters{TeamID: "team-123", Page: 1, Limit: 10})
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
