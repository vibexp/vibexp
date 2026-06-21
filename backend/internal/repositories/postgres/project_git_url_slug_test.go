package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// listGitURLToSlugQueryRegex matches the SELECT issued by ListGitURLToSlugByTeam.
// We anchor on the distinctive `git_url <> ”` filter and the EXISTS subqueries so
// the regex stays robust to whitespace/formatting changes in project.go.
const listGitURLToSlugQueryRegex = `SELECT p\.git_url, p\.slug.*FROM projects p.*` +
	`p\.team_id = \$1.*p\.git_url <> ''.*EXISTS.*teams.*owner_id = \$2.*` +
	`EXISTS.*team_members.*user_id = \$2`

// TestProjectRepository_ListGitURLToSlugByTeam_MatchedSingleProject verifies the
// happy path: a single project in the team with a non-empty git_url is returned.
func TestProjectRepository_ListGitURLToSlugByTeam_MatchedSingleProject(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	teamID := "team-123"
	userID := "user-123"

	rows := sqlmock.NewRows([]string{"git_url", "slug"}).
		AddRow("https://github.com/owner/repo", "owner-repo")

	mock.ExpectQuery(listGitURLToSlugQueryRegex).
		WithArgs(teamID, userID).
		WillReturnRows(rows)

	result, err := repo.ListGitURLToSlugByTeam(ctx, teamID, userID)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"https://github.com/owner/repo": "owner-repo",
	}, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_ListGitURLToSlugByTeam_Unmatched verifies that no rows
// returns an empty (non-nil) map.
func TestProjectRepository_ListGitURLToSlugByTeam_Unmatched(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	teamID := "team-empty"
	userID := "user-123"

	mock.ExpectQuery(listGitURLToSlugQueryRegex).
		WithArgs(teamID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"git_url", "slug"}))

	result, err := repo.ListGitURLToSlugByTeam(ctx, teamID, userID)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_ListGitURLToSlugByTeam_MultipleProjects verifies that
// every matching project is included in the returned map.
func TestProjectRepository_ListGitURLToSlugByTeam_MultipleProjects(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	teamID := "team-mixed"
	userID := "user-123"

	rows := sqlmock.NewRows([]string{"git_url", "slug"}).
		AddRow("https://github.com/owner/repo-one", "owner-repo-one").
		AddRow("https://github.com/owner/repo-two", "owner-repo-two").
		AddRow("https://github.com/other/repo-three", "other-repo-three")

	mock.ExpectQuery(listGitURLToSlugQueryRegex).
		WithArgs(teamID, userID).
		WillReturnRows(rows)

	result, err := repo.ListGitURLToSlugByTeam(ctx, teamID, userID)

	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "owner-repo-one", result["https://github.com/owner/repo-one"])
	assert.Equal(t, "owner-repo-two", result["https://github.com/owner/repo-two"])
	assert.Equal(t, "other-repo-three", result["https://github.com/other/repo-three"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_ListGitURLToSlugByTeam_TeamIsolation verifies that the
// query is scoped to the requested team. Projects from another team are filtered
// out by the `p.team_id = $1` predicate enforced server-side; here we simulate a
// caller asking about Team B and confirm only Team B's row is returned.
func TestProjectRepository_ListGitURLToSlugByTeam_TeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	teamB := "team-bbb"
	userID := "user-123"

	// Caller asks about Team B; the SQL filter ensures Team A rows never reach
	// the result set even though the user owns projects in both teams.
	rows := sqlmock.NewRows([]string{"git_url", "slug"}).
		AddRow("https://github.com/owner/team-b-repo", "team-b-repo")

	mock.ExpectQuery(listGitURLToSlugQueryRegex).
		WithArgs(teamB, userID).
		WillReturnRows(rows)

	result, err := repo.ListGitURLToSlugByTeam(ctx, teamB, userID)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "team-b-repo", result["https://github.com/owner/team-b-repo"])
	assert.NotContains(t, result, "https://github.com/owner/team-a-repo")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_ListGitURLToSlugByTeam_MemberAccess verifies that a
// non-owner team member can read the map. The EXISTS branch on team_members
// satisfies the access check, and that's verified by sqlmock matching the
// query with the member user ID.
func TestProjectRepository_ListGitURLToSlugByTeam_MemberAccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	teamID := "team-shared"
	memberUserID := "user-member"

	rows := sqlmock.NewRows([]string{"git_url", "slug"}).
		AddRow("https://github.com/owner/shared-repo", "shared-repo")

	mock.ExpectQuery(listGitURLToSlugQueryRegex).
		WithArgs(teamID, memberUserID).
		WillReturnRows(rows)

	result, err := repo.ListGitURLToSlugByTeam(ctx, teamID, memberUserID)

	require.NoError(t, err)
	assert.Equal(t, "shared-repo", result["https://github.com/owner/shared-repo"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_ListGitURLToSlugByTeam_FiltersEmptyGitURL verifies that
// the SQL contains the `p.git_url <> ”` predicate, which the regex match
// asserts. (sqlmock validates the query text but does not execute it, so the
// row-level filter is enforced upstream by the predicate itself.)
func TestProjectRepository_ListGitURLToSlugByTeam_FiltersEmptyGitURL(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	teamID := "team-with-blanks"
	userID := "user-123"

	// The DB returns only rows with a non-empty git_url because of the
	// `git_url <> ''` predicate. We model that here by returning a single row.
	rows := sqlmock.NewRows([]string{"git_url", "slug"}).
		AddRow("https://github.com/owner/has-url", "has-url")

	mock.ExpectQuery(listGitURLToSlugQueryRegex).
		WithArgs(teamID, userID).
		WillReturnRows(rows)

	result, err := repo.ListGitURLToSlugByTeam(ctx, teamID, userID)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Contains(t, result, "https://github.com/owner/has-url")
	assert.NotContains(t, result, "")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_ListGitURLToSlugByTeam_QueryError verifies that a
// database error is wrapped with a descriptive message.
func TestProjectRepository_ListGitURLToSlugByTeam_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	teamID := "team-error"
	userID := "user-123"

	mock.ExpectQuery(listGitURLToSlugQueryRegex).
		WithArgs(teamID, userID).
		WillReturnError(sql.ErrConnDone)

	result, err := repo.ListGitURLToSlugByTeam(ctx, teamID, userID)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to list git_url->slug for team")
	assert.True(t, errors.Is(err, sql.ErrConnDone))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_ListGitURLToSlugByTeam_ScanError verifies that a row
// scan failure (e.g. column type mismatch) is wrapped with a descriptive
// message.
func TestProjectRepository_ListGitURLToSlugByTeam_ScanError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	teamID := "team-scan-err"
	userID := "user-123"

	// Returning a row with a NULL git_url forces Scan to fail because the
	// destination is a string, not sql.NullString.
	rows := sqlmock.NewRows([]string{"git_url", "slug"}).AddRow(nil, "slug")

	mock.ExpectQuery(listGitURLToSlugQueryRegex).
		WithArgs(teamID, userID).
		WillReturnRows(rows)

	result, err := repo.ListGitURLToSlugByTeam(ctx, teamID, userID)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to scan git_url->slug row")
	assert.NoError(t, mock.ExpectationsWereMet())
}
