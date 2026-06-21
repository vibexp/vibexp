package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// TestProjectRepository_List_TeamFiltering verifies that List method filters by team_id
func TestProjectRepository_List_TeamFiltering(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	teamID := "team-123"

	filters := repositories.ProjectListFilters{
		TeamID: teamID,
		Page:   1,
		Limit:  20,
	}

	// Expect count query with team_id filter. The shared WHERE binds team_id
	// once for the equality plus the team/user pair per EXISTS clause.
	mock.ExpectQuery("SELECT COUNT.*FROM projects p.*EXISTS.*teams.*").
		WithArgs(teamID, teamID, userID, teamID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Expect list query with team_id filter
	mock.ExpectQuery("SELECT (.+) FROM projects p.*EXISTS.*teams.*").
		WithArgs(teamID, teamID, userID, teamID, userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "name", "slug", "description",
			"git_url", "homepage", "created_at", "updated_at", "version",
		}))

	// Execute list operation
	projects, total, err := repo.List(ctx, userID, filters)

	assert.NoError(t, err)
	assert.NotNil(t, projects)
	assert.Equal(t, 0, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_List_CrossTeamIsolation verifies that users cannot list projects from other teams
func TestProjectRepository_List_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	teamB := "team-bbb" // User tries to list from Team B

	filters := repositories.ProjectListFilters{
		TeamID: teamB,
		Page:   1,
		Limit:  20,
	}

	// Scenario: User has projects in Team A but tries to list from Team B
	// Should only return projects from Team B (none in this case)

	// Expect count query for Team B - returns 0
	mock.ExpectQuery("SELECT COUNT.*FROM projects p.*EXISTS.*teams.*").
		WithArgs(teamB, teamB, userID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Expect list query for Team B - returns empty
	mock.ExpectQuery("SELECT (.+) FROM projects p.*EXISTS.*teams.*").
		WithArgs(teamB, teamB, userID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "name", "slug", "description",
			"git_url", "homepage", "created_at", "updated_at", "version",
		}))

	// Execute list operation
	projects, total, err := repo.List(ctx, userID, filters)

	assert.NoError(t, err)
	assert.NotNil(t, projects)
	assert.Equal(t, 0, total)
	assert.Len(t, projects, 0) // Should return empty list, not projects from Team A
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_Update_CrossTeamMember_CannotSatisfyReCheck is a
// regression guard for issue #1718: the UPDATE statement's team_members
// access re-check must correlate to the project's team (team_id =
// projects.team_id), not self-compare (team_id = team_id, which is always
// true). The updateQuery regex below pins the corrected, qualified predicate,
// so a revert to the self-comparison fails to match the mock. The scenario
// simulates the SELECT->UPDATE race window: the gating ownership SELECT passes,
// but the UPDATE re-check matches zero rows because the actor is only a member
// of an unrelated team (not the project's team) and is not the project owner —
// so Update must surface ErrProjectNotFoundForRepo rather than mutate the row.
func TestProjectRepository_Update_CrossTeamMember_CannotSatisfyReCheck(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	projectID := "project-team-a"
	// Actor is a member of an unrelated team only (not the project's team) and
	// is not the project owner.
	crossTeamUserID := "user-outsider"

	project := &models.Project{
		ID:        projectID,
		UserID:    crossTeamUserID,
		TeamID:    "team-a",
		Name:      "Hijack attempt",
		Slug:      "hijack",
		UpdatedAt: now,
		Version:   1,
	}

	// The gating ownership SELECT passes (race window: access was true when
	// checked, leaving the UPDATE-side re-check as the last line of defense).
	mock.ExpectQuery(`SELECT EXISTS\(`).
		WithArgs(projectID, crossTeamUserID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Pin the corrected, row-correlated predicate. A regression to the
	// self-comparing `team_id = team_id` would not match this regex.
	updateQuery := `(?s)UPDATE projects.*EXISTS \(SELECT 1 FROM team_members ` +
		`WHERE team_id = projects\.team_id AND user_id = \$9\)`
	// No rows match the re-check for a cross-team member -> Scan sees ErrNoRows.
	mock.ExpectQuery(updateQuery).
		WithArgs(
			projectID, "Hijack attempt", "hijack",
			"", "", "",
			now, 1, crossTeamUserID,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}))

	err = repo.Update(ctx, project)

	require.Error(t, err)
	assert.ErrorIs(t, err, repositories.ErrProjectNotFoundForRepo)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_List_WithSearchAndTeamFilter verifies team filtering with search
func TestProjectRepository_List_WithSearchAndTeamFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	teamID := "team-123"
	searchTerm := "test"

	filters := repositories.ProjectListFilters{
		TeamID: teamID,
		Search: searchTerm,
		Page:   1,
		Limit:  20,
	}

	// Expect count query with team_id AND search filter. The search term binds at
	// $6 (after the five base args) and is repeated for each ILIKE column.
	countQuery := "SELECT COUNT.*FROM projects p.*EXISTS.*teams.*AND " +
		"\\(p.name ILIKE \\$6 OR p.description ILIKE \\$7 OR p.slug ILIKE \\$8\\)"
	mock.ExpectQuery(countQuery).
		WithArgs(teamID, teamID, userID, teamID, userID, "%test%", "%test%", "%test%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Expect list query with team_id AND search filter
	listQuery := "SELECT (.+) FROM projects p.*EXISTS.*teams.*AND " +
		"\\(p.name ILIKE \\$6 OR p.description ILIKE \\$7 OR p.slug ILIKE \\$8\\)"
	mock.ExpectQuery(listQuery).
		WithArgs(teamID, teamID, userID, teamID, userID, "%test%", "%test%", "%test%").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "name", "slug", "description",
			"git_url", "homepage", "created_at", "updated_at", "version",
		}))

	// Execute list operation
	projects, total, err := repo.List(ctx, userID, filters)

	assert.NoError(t, err)
	assert.NotNil(t, projects)
	assert.Equal(t, 0, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}
