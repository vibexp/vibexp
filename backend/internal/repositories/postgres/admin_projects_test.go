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

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func defaultAdminProjectFilters() repositories.AdminProjectFilters {
	return repositories.AdminProjectFilters{Page: 1, Limit: 20}
}

// adminProjectLastResourceAt is the last_resource_created_at of
// adminProjectRows' p1.
var adminProjectLastResourceAt = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func adminProjectRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "slug", "created_at", "updated_at",
		"team_id", "team_name", "team_slug",
		"owner_id", "owner_email", "owner_name",
		"prompt_count", "memory_count", "artifact_count", "blueprint_count", "feed_item_count",
		"total_resource_count", "last_resource_created_at",
	}).
		AddRow("p1", "Platform", "platform", time.Now(), time.Now(),
			"t1", "Acme Engineering", "acme-engineering", "u1", "creator@example.com", "Creator",
			1, 2, 3, 4, 5, 15, adminProjectLastResourceAt).
		AddRow("p2", "Website", "website", time.Now(), time.Now(),
			"t1", "Acme Engineering", "acme-engineering", "u1", "creator@example.com", "Creator",
			0, 0, 0, 0, 0, 0, nil)
}

// adminProjectListFromRE matches the shared FROM of the admin project count and
// page queries: projects, the team and creator joins and the five 1:1 LEFT
// JOINed stats CTEs.
const adminProjectListFromRE = `FROM projects p JOIN teams t ON t.id = p.team_id JOIN users u ON u.id = p.user_id ` +
	`LEFT JOIN pr ON pr.project_id = p.id .*LEFT JOIN fi ON fi.project_id = p.id`

// TestAdminRepository_ListProjects is the no-filter regression case and pins the
// scan order across the two joins.
func TestAdminRepository_ListProjects(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`^WITH .* SELECT COUNT\(\*\) ` + adminProjectListFromRE + `$`).
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
	// Distinct values per count, so a transposed scan fails.
	assert.Equal(t, models.AdminProjectResourceCounts{
		Prompts: 1, Memories: 2, Artifacts: 3, Blueprints: 4, FeedItems: 5, Total: 15,
	}, projects[0].ResourceCounts)
	require.NotNil(t, projects[0].LastResourceCreatedAt)
	assert.True(t, adminProjectLastResourceAt.Equal(*projects[0].LastResourceCreatedAt))
	assert.Equal(t, models.AdminProjectResourceCounts{}, projects[1].ResourceCounts)
	assert.Nil(t, projects[1].LastResourceCreatedAt)
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
	ownerEmail := "Creator@Example.com"
	one, five := int64(1), int64(5)
	rng := repositories.AdminCountRange{Min: &one, Max: &five}

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
			name:     "owner_email is a case-insensitive exact match on the creator",
			filters:  repositories.AdminProjectFilters{OwnerEmail: &ownerEmail, Page: 1, Limit: 20},
			wantSQL:  `lower\(u.email\) = lower\(\$1\)`,
			wantArgs: []driver.Value{ownerEmail},
		},
		{"prompt_count", repositories.AdminProjectFilters{PromptCount: rng, Page: 1, Limit: 20}, `COALESCE\(pr.n, 0\) >= \$1 AND COALESCE\(pr.n, 0\) <= \$2`, []driver.Value{one, five}},
		{"memory_count min only", repositories.AdminProjectFilters{MemoryCount: repositories.AdminCountRange{Min: &one}, Page: 1, Limit: 20}, `COALESCE\(me.n, 0\) >= \$1`, []driver.Value{one}},
		{"artifact_count max only", repositories.AdminProjectFilters{ArtifactCount: repositories.AdminCountRange{Max: &five}, Page: 1, Limit: 20}, `COALESCE\(ar.n, 0\) <= \$1`, []driver.Value{five}},
		{"blueprint_count", repositories.AdminProjectFilters{BlueprintCount: rng, Page: 1, Limit: 20}, `COALESCE\(bp.n, 0\) >= \$1 AND COALESCE\(bp.n, 0\) <= \$2`, []driver.Value{one, five}},
		{"feed_item_count", repositories.AdminProjectFilters{FeedItemCount: rng, Page: 1, Limit: 20}, `COALESCE\(fi.n, 0\) >= \$1 AND COALESCE\(fi.n, 0\) <= \$2`, []driver.Value{one, five}},
		{"total_resource_count", repositories.AdminProjectFilters{TotalResourceCount: repositories.AdminCountRange{Min: &five}, Page: 1, Limit: 20}, `\(COALESCE\(pr.n, 0\) \+ .*COALESCE\(fi.n, 0\)\) >= \$1`, []driver.Value{five}},
		{"last_resource_created range", repositories.AdminProjectFilters{LastResourceCreatedFrom: &from, LastResourceCreatedTo: &to, Page: 1, Limit: 20}, `GREATEST\(pr.last_at, .*fi.last_at\) >= \$1 AND GREATEST\(pr.last_at, .*fi.last_at\) <= \$2`, []driver.Value{from, to}},
		{
			name: "all filters combine with AND",
			filters: repositories.AdminProjectFilters{
				Search: &search, TeamID: &teamID, CreatedFrom: &from, CreatedTo: &to,
				OwnerEmail: &ownerEmail, PromptCount: repositories.AdminCountRange{Min: &one},
				LastResourceCreatedFrom: &from,
				Page:                    1, Limit: 20,
			},
			wantSQL: `\(p.name ILIKE \$1 OR p.slug ILIKE \$2\) AND p.team_id = \$3 ` +
				`AND p.created_at >= \$4 AND p.created_at <= \$5 AND lower\(u.email\) = lower\(\$6\) ` +
				`AND COALESCE\(pr.n, 0\) >= \$7 AND GREATEST\(.*\) >= \$8`,
			wantArgs: []driver.Value{"%plat%", "%plat%", "t1", from, to, ownerEmail, one, from},
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

			mock.ExpectQuery(`SELECT COUNT\(\*\) ` + adminProjectListFromRE + ` WHERE \(` + tc.wantSQL + `\)$`).
				WithArgs(tc.wantArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery(`AS last_resource_created_at ` + adminProjectListFromRE + ` WHERE \(` + tc.wantSQL + `\) ORDER BY`).
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
		{"prompt_count", "prompt_count", "desc", "COALESCE(pr.n, 0) DESC, p.id"},
		{"memory_count", "memory_count", "asc", "COALESCE(me.n, 0) ASC, p.id"},
		{"artifact_count", "artifact_count", "asc", "COALESCE(ar.n, 0) ASC, p.id"},
		{"blueprint_count", "blueprint_count", "asc", "COALESCE(bp.n, 0) ASC, p.id"},
		{"feed_item_count", "feed_item_count", "asc", "COALESCE(fi.n, 0) ASC, p.id"},
		{"total_resource_count", "total_resource_count", "desc", colProjectTotalResourceCount + " DESC, p.id"},
		{
			name: "last_resource_created_at desc keeps NULLs last", sortBy: "last_resource_created_at", sortOrder: "desc",
			want: colProjectLastResourceCreatedAt + " DESC NULLS LAST, p.id",
		},
		{
			name: "last_resource_created_at asc keeps NULLs last", sortBy: "last_resource_created_at", sortOrder: "asc",
			want: colProjectLastResourceCreatedAt + " ASC NULLS LAST, p.id",
		},
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
		WillReturnRows(sqlmock.NewRows([]string{"prompts", "artifacts", "memories", "blueprints", "feed_items"}).
			AddRow(12, 4, 27, 3, 9))

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
	assert.Equal(t, int64(9), detail.ResourceCounts.FeedItems)
	assert.Equal(t, int64(55), detail.ResourceCounts.Total, "total is the sum of the five")
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

// adminProjectScopedTables are the five tables with a direct project_id; the
// list aggregates and the detail counts must cover exactly these.
var adminProjectScopedTables = []string{"prompts", "artifacts", "memories", "blueprints", "feed_items"}

// adminProjectExcludedTables have no project_id: agents and feeds are
// team-scoped, comments and attachments reach a project only indirectly.
var adminProjectExcludedTables = []string{"agents", "feeds", "comments", "attachments"}

// TestAdminProjectResourceCountsQuery_CoversOnlyProjectScopedTables is the guard
// for the detail counts: exactly the five tables that HAVE a project_id column,
// never a team-scoped or indirectly-attached one. Counting zero for those would
// read as "this project has no agents" rather than "agents do not belong to
// projects".
func TestAdminProjectResourceCountsQuery_CoversOnlyProjectScopedTables(t *testing.T) {
	for _, table := range adminProjectScopedTables {
		// Whitespace-tolerant: the query aligns its columns for readability.
		pattern := `FROM\s+` + table + `\s+WHERE project_id`
		matched, err := regexp.MatchString(pattern, adminProjectResourceCountsQuery)
		require.NoError(t, err)
		assert.True(t, matched, "%s is project-scoped and must be counted", table)
	}
	for _, table := range adminProjectExcludedTables {
		assert.NotRegexp(t, `FROM\s+`+table+`\b`, adminProjectResourceCountsQuery,
			"%s has no project_id column; counting it would invent a relationship", table)
	}
}

// TestAdminProjectStatsCTE_CoversOnlyProjectScopedTables applies the same guard
// to the listing's aggregates, so list and detail count the same set.
func TestAdminProjectStatsCTE_CoversOnlyProjectScopedTables(t *testing.T) {
	for _, table := range adminProjectScopedTables {
		assert.Regexp(t, `FROM\s+`+table+`\s+(WHERE project_id IS NOT NULL )?GROUP BY project_id`,
			adminProjectStatsCTE, "%s is project-scoped and must be aggregated", table)
	}
	for _, table := range adminProjectExcludedTables {
		assert.NotRegexp(t, `FROM\s+`+table+`\b`, adminProjectStatsCTE,
			"%s has no project_id column; counting it would invent a relationship", table)
	}
}

// TestAdminProjectListFrom_JoinsAreOneToOne guards against a LEFT JOIN onto a
// raw resource table in the LIST query, which would multiply rows per project
// and silently corrupt both the page and the count: every LEFT JOIN must target
// a stats CTE, each of which is GROUP BY project_id and so unique on it.
func TestAdminProjectListFrom_JoinsAreOneToOne(t *testing.T) {
	query, _, err := adminProjectListFrom(psql.Select("COUNT(*)")).ToSql()
	require.NoError(t, err)

	assert.Contains(t, query, "JOIN teams t ON t.id = p.team_id")
	assert.Contains(t, query, "JOIN users u ON u.id = p.user_id")
	leftJoins := regexp.MustCompile(`LEFT JOIN (\w+) ON`).FindAllStringSubmatch(query, -1)
	joined := make([]string, 0, len(leftJoins))
	for _, m := range leftJoins {
		joined = append(joined, m[1])
	}
	assert.Equal(t, adminProjectStatsAliases, joined,
		"only the pre-aggregated CTEs may be LEFT JOINed; a raw resource table would fan out rows")
	for _, alias := range adminProjectStatsAliases {
		assert.Regexp(t, `(?s)\b`+alias+` AS \(SELECT project_id, COUNT\(\*\) AS n, .*?\sGROUP BY project_id\)`,
			adminProjectStatsCTE, "CTE %s must be unique on project_id", alias)
	}
}
