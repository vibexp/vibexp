package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

func newAdminRepoMock(t *testing.T) (*AdminRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	repo := &AdminRepository{db: &database.DB{DB: mockDB}}
	return repo, mock, mockDB
}

func TestAdminRepository_GetInstanceCounts(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewAdminRepository(&database.DB{DB: mockDB})

	rows := sqlmock.NewRows([]string{"users", "teams", "prompts", "artifacts", "memories"}).
		AddRow(42, 12, 340, 128, 512)
	mock.ExpectQuery(`SELECT`).WillReturnRows(rows)

	counts, err := repo.GetInstanceCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(42), counts.Users)
	assert.Equal(t, int64(12), counts.Teams)
	assert.Equal(t, int64(340), counts.Prompts)
	assert.Equal(t, int64(128), counts.Artifacts)
	assert.Equal(t, int64(512), counts.Memories)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetInstanceCounts_QueryError(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewAdminRepository(&database.DB{DB: mockDB})
	mock.ExpectQuery(`SELECT`).WillReturnError(errors.New("boom"))

	_, err = repo.GetInstanceCounts(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// defaultAdminUserFilters is an unfiltered first page — the shape every existing
// caller produced before #452 added filtering.
func defaultAdminUserFilters() repositories.AdminUserFilters {
	return repositories.AdminUserFilters{Page: 1, Limit: 20}
}

// defaultAdminTeamFilters is an unfiltered first page.
func defaultAdminTeamFilters() repositories.AdminTeamFilters {
	return repositories.AdminTeamFilters{Page: 1, Limit: 20}
}

func adminUserRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "email", "name", "idp_provider", "created_at", "team_count"}).
		AddRow("u1", "a@example.com", "A", "oidc", time.Now(), 2).
		AddRow("u2", "b@example.com", "B", nil, time.Now(), 0)
}

func adminTeamRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "slug", "is_personal", "created_at",
		"owner_id", "owner_email", "owner_name", "member_count",
	}).
		AddRow("t1", "Acme", "acme", false, time.Now(), "o1", "o@example.com", "Owner", 3).
		AddRow("t2", "Beta", "beta", true, time.Now(), "o1", "o@example.com", "Owner", 0)
}

// TestAdminRepository_ListUsers is the no-filter regression case: the unfiltered
// call must still count every user, page newest-first, and bind only LIMIT/OFFSET.
func TestAdminRepository_ListUsers(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users u$`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`ORDER BY u.created_at DESC, u.id LIMIT 20 OFFSET 0`).
		WillReturnRows(adminUserRows())

	users, total, err := repo.ListUsers(context.Background(), defaultAdminUserFilters())
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, users, 2)
	assert.Equal(t, int64(2), users[0].TeamCount)
	require.NotNil(t, users[0].IDPProvider)
	assert.Nil(t, users[1].IDPProvider)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAdminRepository_ListUsers_Filters covers each user filter individually and
// all of them combined, asserting the count and page queries receive the SAME
// bound arguments — the invariant that keeps the envelope from diverging.
func TestAdminRepository_ListUsers_Filters(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	search := "alice"
	idp := "google"

	tests := []struct {
		name     string
		filters  repositories.AdminUserFilters
		wantSQL  string
		wantArgs []driver.Value
	}{
		{
			name:     "search matches email or name",
			filters:  repositories.AdminUserFilters{Search: &search, Page: 1, Limit: 20},
			wantSQL:  `\(u.email ILIKE \$1 OR u.name ILIKE \$2\)`,
			wantArgs: []driver.Value{"%alice%", "%alice%"},
		},
		{
			name:     "idp_provider is an exact match",
			filters:  repositories.AdminUserFilters{IDPProvider: &idp, Page: 1, Limit: 20},
			wantSQL:  `u.idp_provider = \$1`,
			wantArgs: []driver.Value{"google"},
		},
		{
			name:     "created_from is inclusive",
			filters:  repositories.AdminUserFilters{CreatedFrom: &from, Page: 1, Limit: 20},
			wantSQL:  `u.created_at >= \$1`,
			wantArgs: []driver.Value{from},
		},
		{
			name:     "created_to is inclusive",
			filters:  repositories.AdminUserFilters{CreatedTo: &to, Page: 1, Limit: 20},
			wantSQL:  `u.created_at <= \$1`,
			wantArgs: []driver.Value{to},
		},
		{
			name: "all filters combine with AND",
			filters: repositories.AdminUserFilters{
				Search: &search, IDPProvider: &idp, CreatedFrom: &from, CreatedTo: &to,
				Page: 1, Limit: 20,
			},
			wantSQL: `\(u.email ILIKE \$1 OR u.name ILIKE \$2\) AND u.idp_provider = \$3 ` +
				`AND u.created_at >= \$4 AND u.created_at <= \$5`,
			wantArgs: []driver.Value{"%alice%", "%alice%", "google", from, to},
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

			// The count query and the page query must carry identical WHERE args.
			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users u WHERE \(` + tc.wantSQL + `\)`).
				WithArgs(tc.wantArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery(`FROM users u LEFT JOIN team_members tm .* WHERE \(` + tc.wantSQL + `\)`).
				WithArgs(tc.wantArgs...).
				WillReturnRows(adminUserRows())

			_, total, err := repo.ListUsers(context.Background(), tc.filters)
			require.NoError(t, err)
			assert.Equal(t, 1, total)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestAdminRepository_ListUsers_Sorting asserts the ORDER BY allowlist: every
// accepted enum maps to a fixed column, and an injection-shaped or unknown
// sort_by falls back to the default instead of reaching the query text.
func TestAdminRepository_ListUsers_Sorting(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		wantOrder string
	}{
		{"default", "", "", "ORDER BY u.created_at DESC, u.id"},
		{"email asc", "email", "asc", "ORDER BY u.email ASC, u.id"},
		{"name desc", "name", "desc", "ORDER BY u.name DESC, u.id"},
		{"created_at asc", "created_at", "asc", "ORDER BY u.created_at ASC, u.id"},
		{"team_count uses the aggregate", "team_count", "desc", "ORDER BY COUNT(tm.team_id) DESC, u.id"},
		{"unknown sort_by falls back", "totally_unknown", "asc", "ORDER BY u.created_at ASC, u.id"},
		{
			name:      "injection-shaped sort_by never reaches SQL",
			sortBy:    "u.id; DROP TABLE users--",
			sortOrder: "asc",
			wantOrder: "ORDER BY u.created_at ASC, u.id",
		},
		{"unknown sort_order defaults to DESC", "email", "sideways", "ORDER BY u.email DESC, u.id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filters := repositories.AdminUserFilters{
				SortBy: tc.sortBy, SortOrder: tc.sortOrder, Page: 1, Limit: 20,
			}
			assert.Equal(t, strings.TrimPrefix(tc.wantOrder, "ORDER BY "), buildAdminUserOrderBy(filters))
			assert.NotContains(t, buildAdminUserOrderBy(filters), "DROP TABLE")
		})
	}
}

// TestAdminRepository_ListUsers_Paging checks the OFFSET math for a later page.
func TestAdminRepository_ListUsers_Paging(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users u$`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(45))
	mock.ExpectQuery(`LIMIT 20 OFFSET 40`).WillReturnRows(adminUserRows())

	_, total, err := repo.ListUsers(context.Background(), repositories.AdminUserFilters{Page: 3, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, 45, total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetUserDetail_Found(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM users WHERE id = \$1`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "idp_provider", "created_at"}).
			AddRow("u1", "a@example.com", "A", "oidc", time.Now()))
	mock.ExpectQuery(`FROM team_members tm`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "role"}).
			AddRow("t1", "Acme", "owner"))

	detail, err := repo.GetUserDetail(context.Background(), "u1")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "u1", detail.ID)
	require.Len(t, detail.Memberships, 1)
	assert.Equal(t, "owner", detail.Memberships[0].Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetUserDetail_NotFound(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM users WHERE id = \$1`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	detail, err := repo.GetUserDetail(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, detail)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_ListUsers_CountError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).WillReturnError(errors.New("boom"))

	_, _, err := repo.ListUsers(context.Background(), defaultAdminUserFilters())
	require.Error(t, err)
}

func TestAdminRepository_ListUsers_QueryError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`LEFT JOIN team_members`).WillReturnError(errors.New("boom"))

	_, _, err := repo.ListUsers(context.Background(), defaultAdminUserFilters())
	require.Error(t, err)
}

func TestAdminRepository_GetUserDetail_QueryError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM users WHERE id = \$1`).WithArgs("u1").WillReturnError(errors.New("boom"))

	_, err := repo.GetUserDetail(context.Background(), "u1")
	require.Error(t, err)
}

func TestAdminRepository_GetUserDetail_MembershipError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM users WHERE id = \$1`).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "idp_provider", "created_at"}).
			AddRow("u1", "a@example.com", "A", nil, time.Now()))
	mock.ExpectQuery(`FROM team_members tm`).WithArgs("u1").WillReturnError(errors.New("boom"))

	_, err := repo.GetUserDetail(context.Background(), "u1")
	require.Error(t, err)
}

// TestAdminRepository_ListTeams is the no-filter regression case, and also pins
// the two additive payload fields (slug, is_personal) to their scan positions.
func TestAdminRepository_ListTeams(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams t JOIN users u ON u.id = t.owner_id$`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`ORDER BY t.created_at DESC, t.id LIMIT 20 OFFSET 0`).
		WillReturnRows(adminTeamRows())

	teams, total, err := repo.ListTeams(context.Background(), defaultAdminTeamFilters())
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, teams, 2)
	assert.Equal(t, "Owner", teams[0].Owner.Name)
	assert.Equal(t, int64(3), teams[0].MemberCount)
	assert.Equal(t, "acme", teams[0].Slug)
	assert.False(t, teams[0].IsPersonal)
	assert.True(t, teams[1].IsPersonal)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAdminRepository_ListTeams_Filters covers each team filter individually and
// combined, asserting count and page queries bind identical WHERE args.
func TestAdminRepository_ListTeams_Filters(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	search := "acme"
	personal := true
	shared := false

	tests := []struct {
		name     string
		filters  repositories.AdminTeamFilters
		wantSQL  string
		wantArgs []driver.Value
	}{
		{
			name:     "search matches name, slug or owner email",
			filters:  repositories.AdminTeamFilters{Search: &search, Page: 1, Limit: 20},
			wantSQL:  `\(t.name ILIKE \$1 OR t.slug ILIKE \$2 OR u.email ILIKE \$3\)`,
			wantArgs: []driver.Value{"%acme%", "%acme%", "%acme%"},
		},
		{
			name:     "is_personal true narrows to personal workspaces",
			filters:  repositories.AdminTeamFilters{IsPersonal: &personal, Page: 1, Limit: 20},
			wantSQL:  `t.is_personal = \$1`,
			wantArgs: []driver.Value{true},
		},
		{
			name:     "is_personal false narrows to shared workspaces",
			filters:  repositories.AdminTeamFilters{IsPersonal: &shared, Page: 1, Limit: 20},
			wantSQL:  `t.is_personal = \$1`,
			wantArgs: []driver.Value{false},
		},
		{
			name:     "created_from is inclusive",
			filters:  repositories.AdminTeamFilters{CreatedFrom: &from, Page: 1, Limit: 20},
			wantSQL:  `t.created_at >= \$1`,
			wantArgs: []driver.Value{from},
		},
		{
			name:     "created_to is inclusive",
			filters:  repositories.AdminTeamFilters{CreatedTo: &to, Page: 1, Limit: 20},
			wantSQL:  `t.created_at <= \$1`,
			wantArgs: []driver.Value{to},
		},
		{
			name: "all filters combine with AND",
			filters: repositories.AdminTeamFilters{
				Search: &search, IsPersonal: &shared, CreatedFrom: &from, CreatedTo: &to,
				Page: 1, Limit: 20,
			},
			wantSQL: `\(t.name ILIKE \$1 OR t.slug ILIKE \$2 OR u.email ILIKE \$3\) ` +
				`AND t.is_personal = \$4 AND t.created_at >= \$5 AND t.created_at <= \$6`,
			wantArgs: []driver.Value{"%acme%", "%acme%", "%acme%", false, from, to},
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

			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams t JOIN users u .* WHERE \(` + tc.wantSQL + `\)`).
				WithArgs(tc.wantArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery(`member_count FROM teams t JOIN users u .* WHERE \(` + tc.wantSQL + `\)`).
				WithArgs(tc.wantArgs...).
				WillReturnRows(adminTeamRows())

			_, total, err := repo.ListTeams(context.Background(), tc.filters)
			require.NoError(t, err)
			assert.Equal(t, 1, total)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestAdminRepository_ListTeams_Sorting asserts the team ORDER BY allowlist.
func TestAdminRepository_ListTeams_Sorting(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      string
	}{
		{"default", "", "", "t.created_at DESC, t.id"},
		{"name asc", "name", "asc", "t.name ASC, t.id"},
		{"created_at asc", "created_at", "asc", "t.created_at ASC, t.id"},
		{"member_count uses the subquery alias", "member_count", "desc", "member_count DESC, t.id"},
		{"unknown sort_by falls back", "owner", "asc", "t.created_at ASC, t.id"},
		{
			name:      "injection-shaped sort_by never reaches SQL",
			sortBy:    "t.id; DROP TABLE teams--",
			sortOrder: "desc",
			want:      "t.created_at DESC, t.id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAdminTeamOrderBy(repositories.AdminTeamFilters{
				SortBy: tc.sortBy, SortOrder: tc.sortOrder, Page: 1, Limit: 20,
			})
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, "DROP TABLE")
		})
	}
}

func TestAdminRepository_ListTeams_CountError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams`).WillReturnError(errors.New("boom"))
	_, _, err := repo.ListTeams(context.Background(), defaultAdminTeamFilters())
	require.Error(t, err)
}

func TestAdminRepository_ListTeams_QueryError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`member_count FROM teams t`).WillReturnError(errors.New("boom"))
	_, _, err := repo.ListTeams(context.Background(), defaultAdminTeamFilters())
	require.Error(t, err)
}

func TestAdminRepository_GetTeamDetail_Found(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM teams t JOIN users u ON u.id = t.owner_id WHERE t.id = \$1`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "slug", "is_personal", "created_at", "owner_id", "owner_email", "owner_name",
		}).
			AddRow("t1", "Acme", "acme", false, time.Now(), "o1", "o@example.com", "Owner"))
	mock.ExpectQuery(`FROM team_members tm`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "role", "created_at"}).
			AddRow("u1", "m@example.com", "M", "member", time.Now()))

	detail, err := repo.GetTeamDetail(context.Background(), "t1")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "Acme", detail.Name)
	assert.Equal(t, "Owner", detail.Owner.Name)
	require.Len(t, detail.Members, 1)
	assert.Equal(t, "member", detail.Members[0].Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetTeamDetail_NotFound(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM teams t JOIN users u ON u.id = t.owner_id WHERE t.id = \$1`).
		WithArgs("missing").WillReturnError(sql.ErrNoRows)

	detail, err := repo.GetTeamDetail(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, detail)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetTeamDetail_MembersError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM teams t JOIN users u ON u.id = t.owner_id WHERE t.id = \$1`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "slug", "is_personal", "created_at", "owner_id", "owner_email", "owner_name",
		}).
			AddRow("t1", "Acme", "acme", false, time.Now(), "o1", "o@example.com", "Owner"))
	mock.ExpectQuery(`FROM team_members tm`).WithArgs("t1").WillReturnError(errors.New("boom"))

	_, err := repo.GetTeamDetail(context.Background(), "t1")
	require.Error(t, err)
}
