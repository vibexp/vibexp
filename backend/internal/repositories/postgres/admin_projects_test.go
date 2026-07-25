package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

func defaultAdminProjectFilters() repositories.AdminProjectFilters {
	return repositories.AdminProjectFilters{Page: 1, Limit: 20}
}

func adminProjectRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "slug", "created_at", "updated_at",
		"team_id", "team_name", "team_slug",
		"owner_id", "owner_email", "owner_name",
	}).
		AddRow("p1", "Platform", "platform", time.Now(), time.Now(),
			"t1", "Acme Engineering", "acme-engineering", "u1", "creator@example.com", "Creator").
		AddRow("p2", "Website", "website", time.Now(), time.Now(),
			"t1", "Acme Engineering", "acme-engineering", "u1", "creator@example.com", "Creator")
}

// TestAdminRepository_ListProjects is the no-filter regression case and pins the
// scan order across the two joins.
func TestAdminRepository_ListProjects(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p JOIN teams t .* JOIN users u .*$`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`ORDER BY p.created_at DESC, p.id LIMIT 20 OFFSET 0`).
		WillReturnRows(adminProjectRows())

	projects, total, err := repo.ListProjects(context.Background(), defaultAdminProjectFilters())
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, projects, 2)
	assert.Equal(t, "Platform", projects[0].Name)
	assert.Equal(t, "platform", projects[0].Slug)
	assert.Equal(t, "Acme Engineering", projects[0].Team.Name)
	assert.Equal(t, "acme-engineering", projects[0].Team.Slug)
	assert.Equal(t, "creator@example.com", projects[0].Owner.Email)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAdminRepository_ListProjects_Filters covers each filter and all combined,
// asserting the count and page queries bind IDENTICAL args — the invariant that
// keeps the envelope from diverging from the rows.
func TestAdminRepository_ListProjects_Filters(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	search := "plat"
	teamID := "t1"

	tests := []struct {
		name     string
		filters  repositories.AdminProjectFilters
		wantSQL  string
		wantArgs []driver.Value
	}{
		{
			name:     "search matches name or slug",
			filters:  repositories.AdminProjectFilters{Search: &search, Page: 1, Limit: 20},
			wantSQL:  `\(p.name ILIKE \$1 OR p.slug ILIKE \$2\)`,
			wantArgs: []driver.Value{"%plat%", "%plat%"},
		},
		{
			name:     "team_id is an exact match",
			filters:  repositories.AdminProjectFilters{TeamID: &teamID, Page: 1, Limit: 20},
			wantSQL:  `p.team_id = \$1`,
			wantArgs: []driver.Value{"t1"},
		},
		{
			name:     "created_from is inclusive",
			filters:  repositories.AdminProjectFilters{CreatedFrom: &from, Page: 1, Limit: 20},
			wantSQL:  `p.created_at >= \$1`,
			wantArgs: []driver.Value{from},
		},
		{
			name:     "created_to is inclusive",
			filters:  repositories.AdminProjectFilters{CreatedTo: &to, Page: 1, Limit: 20},
			wantSQL:  `p.created_at <= \$1`,
			wantArgs: []driver.Value{to},
		},
		{
			name: "all filters combine with AND",
			filters: repositories.AdminProjectFilters{
				Search: &search, TeamID: &teamID, CreatedFrom: &from, CreatedTo: &to,
				Page: 1, Limit: 20,
			},
			wantSQL: `\(p.name ILIKE \$1 OR p.slug ILIKE \$2\) AND p.team_id = \$3 ` +
				`AND p.created_at >= \$4 AND p.created_at <= \$5`,
			wantArgs: []driver.Value{"%plat%", "%plat%", "t1", from, to},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := newAdminRepoMock(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("failed to close mock DB: %v", closeErr)
				}
			}()

			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p .* WHERE \(` + tc.wantSQL + `\)`).
				WithArgs(tc.wantArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery(`p.id, p.name, p.slug.* WHERE \(` + tc.wantSQL + `\)`).
				WithArgs(tc.wantArgs...).
				WillReturnRows(adminProjectRows())

			_, total, err := repo.ListProjects(context.Background(), tc.filters)
			require.NoError(t, err)
			assert.Equal(t, 1, total)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestAdminRepository_ListProjects_Sorting asserts the ORDER BY allowlist.
func TestAdminRepository_ListProjects_Sorting(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      string
	}{
		{"default", "", "", "p.created_at DESC, p.id"},
		{"name asc", "name", "asc", "p.name ASC, p.id"},
		{"created_at asc", "created_at", "asc", "p.created_at ASC, p.id"},
		{"unknown sort_by falls back", "owner", "asc", "p.created_at ASC, p.id"},
		{
			name:      "injection-shaped sort_by never reaches SQL",
			sortBy:    "p.id; DROP TABLE projects--",
			sortOrder: "desc",
			want:      "p.created_at DESC, p.id",
		},
		{"unknown sort_order defaults to DESC", "name", "sideways", "p.name DESC, p.id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAdminProjectOrderBy(repositories.AdminProjectFilters{
				SortBy: tc.sortBy, SortOrder: tc.sortOrder, Page: 1, Limit: 20,
			})
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, "DROP TABLE")
		})
	}
}

// TestAdminRepository_ListProjects_Paging checks the OFFSET math.
func TestAdminRepository_ListProjects_Paging(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(45))
	mock.ExpectQuery(`LIMIT 20 OFFSET 40`).WillReturnRows(adminProjectRows())

	_, total, err := repo.ListProjects(context.Background(),
		repositories.AdminProjectFilters{Page: 3, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, 45, total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_ListProjects_Errors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(sqlmock.Sqlmock)
	}{
		{
			name: "count fails",
			setup: func(m sqlmock.Sqlmock) {
				m.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).WillReturnError(errors.New("boom"))
			},
		},
		{
			name: "page query fails",
			setup: func(m sqlmock.Sqlmock) {
				m.ExpectQuery(`SELECT COUNT\(\*\) FROM projects p`).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				m.ExpectQuery(`p.id, p.name, p.slug`).WillReturnError(errors.New("boom"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := newAdminRepoMock(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("failed to close mock DB: %v", closeErr)
				}
			}()

			tc.setup(mock)
			_, _, err := repo.ListProjects(context.Background(), defaultAdminProjectFilters())
			require.Error(t, err)
		})
	}
}

// TestAdminRepository_GetProjectDetail_Found pins the detail scan plus the
// resource-count query.
func TestAdminRepository_GetProjectDetail_Found(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM projects p .* WHERE p.id = \$1`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "slug", "description", "git_url", "homepage",
			"created_at", "updated_at",
			"team_id", "team_name", "team_slug",
			"owner_id", "owner_email", "owner_name",
		}).AddRow("p1", "Platform", "platform", "Core work", "https://git", "https://home",
			time.Now(), time.Now(), "t1", "Acme", "acme", "u1", "c@example.com", "Creator"))
	mock.ExpectQuery(`FROM blueprints WHERE project_id`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"prompts", "artifacts", "memories", "blueprints"}).
			AddRow(12, 4, 27, 3))

	detail, err := repo.GetProjectDetail(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "Platform", detail.Name)
	assert.Equal(t, "Core work", detail.Description)
	assert.Equal(t, "Acme", detail.Team.Name)
	assert.Equal(t, "c@example.com", detail.Owner.Email)
	// Distinct values per field, so a transposed scan fails.
	assert.Equal(t, int64(12), detail.ResourceCounts.Prompts)
	assert.Equal(t, int64(4), detail.ResourceCounts.Artifacts)
	assert.Equal(t, int64(27), detail.ResourceCounts.Memories)
	assert.Equal(t, int64(3), detail.ResourceCounts.Blueprints)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAdminRepository_GetProjectDetail_NotFound returns (nil, nil) so the handler
// 404s, and must NOT run the counts query.
func TestAdminRepository_GetProjectDetail_NotFound(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM projects p .* WHERE p.id = \$1`).
		WithArgs("missing").WillReturnError(sql.ErrNoRows)

	detail, err := repo.GetProjectDetail(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, detail)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetProjectDetail_Errors(t *testing.T) {
	t.Run("detail query fails", func(t *testing.T) {
		repo, mock, mockDB := newAdminRepoMock(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectQuery(`FROM projects p`).WillReturnError(errors.New("boom"))
		_, err := repo.GetProjectDetail(context.Background(), "p1")
		require.Error(t, err)
	})

	t.Run("counts query fails", func(t *testing.T) {
		repo, mock, mockDB := newAdminRepoMock(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectQuery(`FROM projects p .* WHERE p.id = \$1`).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "name", "slug", "description", "git_url", "homepage",
				"created_at", "updated_at",
				"team_id", "team_name", "team_slug",
				"owner_id", "owner_email", "owner_name",
			}).AddRow("p1", "P", "p", "", "", "", time.Now(), time.Now(),
				"t1", "T", "t", "u1", "e@example.com", "N"))
		mock.ExpectQuery(`FROM blueprints WHERE project_id`).WillReturnError(errors.New("boom"))

		_, err := repo.GetProjectDetail(context.Background(), "p1")
		require.Error(t, err)
	})
}

// TestAdminProjectResourceCountsQuery_CoversOnlyProjectScopedTables is the guard
// for this issue's key verification: the counts query must cover exactly the four
// tables that HAVE a project_id column, and must NOT reference agents or feeds,
// which are team-scoped. Counting zero for those would read as "this project has
// no agents" rather than "agents do not belong to projects".
func TestAdminProjectResourceCountsQuery_CoversOnlyProjectScopedTables(t *testing.T) {
	for _, table := range []string{"prompts", "artifacts", "memories", "blueprints"} {
		// Whitespace-tolerant: the query aligns its columns for readability.
		pattern := `FROM\s+` + table + `\s+WHERE project_id`
		matched, err := regexp.MatchString(pattern, adminProjectResourceCountsQuery)
		require.NoError(t, err)
		assert.True(t, matched, "%s is project-scoped and must be counted", table)
	}
	for _, table := range []string{"agents", "feeds"} {
		assert.NotContains(t, adminProjectResourceCountsQuery, "FROM "+table,
			"%s has no project_id column; counting it would invent a relationship", table)
	}
}

// TestAdminProjectListFrom_UsesOnlyManyToOneJoins guards against a future LEFT
// JOIN onto a resource table in the LIST query, which would multiply rows per
// project and silently corrupt both the page and the count.
func TestAdminProjectListFrom_UsesOnlyManyToOneJoins(t *testing.T) {
	query, _, err := adminProjectListFrom(psql.Select("COUNT(*)")).ToSql()
	require.NoError(t, err)

	assert.Contains(t, query, "JOIN teams t ON t.id = p.team_id")
	assert.Contains(t, query, "JOIN users u ON u.id = p.user_id")
	assert.NotContains(t, query, "LEFT JOIN",
		"a LEFT JOIN onto a resource table would fan out rows per project")
	for _, table := range []string{"prompts", "artifacts", "memories", "blueprints"} {
		assert.NotContains(t, query, table,
			"resource counts belong on the detail endpoint, not the list join")
	}
}
