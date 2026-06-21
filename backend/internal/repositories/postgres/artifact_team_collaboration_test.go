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

// TestArtifactRepository_TeamMember_CanListOtherMembersArtifacts verifies that team members can list
// artifacts created by other team members
func TestArtifactRepository_TeamMember_CanListOtherMembersArtifacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	// Scenario:
	// - Alice creates team "Engineering"
	// - Alice creates artifact "design-doc"
	// - Bob joins team as member
	// - Bob lists artifacts
	// - Expected: Bob sees Alice's "design-doc" in the list

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceArtifactID := "artifact-alice"

	filters := repositories.ArtifactFilters{
		TeamID: teamID,
		Page:   1,
		Limit:  20,
	}

	// Mock count query - no JOINs needed with EXISTS subqueries.
	// squirrel binds the team/user pair individually per EXISTS clause:
	// team_id, then (team, user, team, user). LIMIT/OFFSET are literals.
	countQuery := `SELECT COUNT\(\*\) FROM artifacts a WHERE`
	mock.ExpectQuery(countQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID, "archived").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Mock list query - no DISTINCT or JOINs needed with EXISTS subqueries
	listQuery := `SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id, a\.title, ` +
		`a\.description, a\.status, a\.type, a\.metadata, a\.created_at, a\.updated_at\s+FROM artifacts a\s+WHERE`
	mock.ExpectQuery(listQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID, "archived").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"status", "type", "metadata", "created_at", "updated_at",
		}).AddRow(
			aliceArtifactID, "project-123", "design-doc", aliceUserID, teamID,
			"System Design Document", "Architecture design",
			"active", "work_reports", []byte("{}"), now, now,
		))

	// Bob lists artifacts
	artifacts, total, err := repo.List(ctx, bobUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, aliceArtifactID, artifacts[0].ID)
	assert.Equal(t, aliceUserID, artifacts[0].UserID) // Created by Alice
	assert.Equal(t, "System Design Document", artifacts[0].Title)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_TeamMember_CanGetOtherMembersArtifacts verifies that team members can get
// artifacts created by other team members
func TestArtifactRepository_TeamMember_CanGetOtherMembersArtifacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	// Scenario:
	// - Alice creates artifact in team
	// - Bob (team member) gets artifact by ID
	// - Expected: Bob successfully retrieves Alice's artifact

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceArtifactID := "artifact-alice"

	getQuery := `SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id, a\.title`
	mock.ExpectQuery(getQuery).
		WithArgs(aliceArtifactID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"content", "status", "type", "metadata", "created_at", "updated_at", "version",
		}).AddRow(
			aliceArtifactID, "project-123", "design-doc", aliceUserID, teamID,
			"System Design Document", "Architecture design", "Content here",
			"active", "work_reports", []byte("{}"), now, now, 1,
		))

	// Bob gets Alice's artifact
	artifact, err := repo.GetByID(ctx, bobUserID, teamID, aliceArtifactID)

	assert.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, aliceArtifactID, artifact.ID)
	assert.Equal(t, aliceUserID, artifact.UserID)
	assert.Equal(t, "System Design Document", artifact.Title)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_TeamMember_CanUpdateOtherMembersArtifacts verifies that team members can
// update artifacts created by other team members
func TestArtifactRepository_TeamMember_CanUpdateOtherMembersArtifacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	// Scenario:
	// - Alice creates artifact in team
	// - Bob (team member) updates the artifact
	// - Expected: Update succeeds

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceArtifactID := "artifact-alice"

	artifact := &models.Artifact{
		ID:          aliceArtifactID,
		ProjectID:   "project-123",
		Slug:        "design-doc",
		UserID:      bobUserID, // Bob is updating
		TeamID:      teamID,
		Title:       "System Design - Updated by Bob",
		Description: "Updated description",
		Content:     "Updated content",
		Status:      "active",
		Type:        "work_reports",
		Metadata:    map[string]interface{}{},
		UpdatedAt:   now,
		Version:     1,
	}

	// Mock ownership validation
	ownershipQuery := `SELECT EXISTS\(`
	mock.ExpectQuery(ownershipQuery).
		WithArgs(aliceArtifactID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Mock update query
	updateQuery := `UPDATE artifacts`
	mock.ExpectQuery(updateQuery).
		WithArgs(
			aliceArtifactID, "project-123", "design-doc",
			"System Design - Updated by Bob", "Updated description", "Updated content",
			"active", "work_reports", sqlmock.AnyArg(), teamID, now, teamID, 1, bobUserID,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	// Bob updates Alice's artifact
	err = repo.Update(ctx, artifact)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), artifact.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_RegularMember_CannotDeleteOtherMembersArtifacts verifies that regular members
// cannot delete artifacts created by other team members
func TestArtifactRepository_RegularMember_CannotDeleteOtherMembersArtifacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates artifact in team
	// - Bob (regular member) tries to delete
	// - Expected: Delete fails

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceArtifactID := "artifact-alice"

	deleteQuery := `DELETE FROM artifacts`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceArtifactID, teamID, bobUserID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Bob tries to delete Alice's artifact
	err = repo.Delete(ctx, bobUserID, teamID, aliceArtifactID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_TeamAdmin_CanDeleteOtherMembersArtifacts verifies that team admins can delete
// artifacts created by other team members
func TestArtifactRepository_TeamAdmin_CanDeleteOtherMembersArtifacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates artifact in team
	// - Carol (team admin) deletes the artifact
	// - Expected: Delete succeeds

	teamID := "team-123"
	carolUserID := "user-carol" // Carol is admin
	aliceArtifactID := "artifact-alice"

	deleteQuery := `DELETE FROM artifacts`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceArtifactID, teamID, carolUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Carol (admin) deletes Alice's artifact
	err = repo.Delete(ctx, carolUserID, teamID, aliceArtifactID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_TeamOwner_CanDeleteAnyTeamArtifact verifies that team owners can delete
// any artifact in the team
func TestArtifactRepository_TeamOwner_CanDeleteAnyTeamArtifact(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Bob creates artifact in team
	// - Alice (team owner) deletes Bob's artifact
	// - Expected: Delete succeeds

	teamID := "team-123"
	aliceUserID := "user-alice" // Alice is team owner
	bobArtifactID := "artifact-bob"

	deleteQuery := `DELETE FROM artifacts`
	mock.ExpectExec(deleteQuery).
		WithArgs(bobArtifactID, teamID, aliceUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Alice (owner) deletes Bob's artifact
	err = repo.Delete(ctx, aliceUserID, teamID, bobArtifactID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_ListCrossTeam_ReturnsArtifactsFromMultipleTeams verifies that ListCrossTeam
// returns artifacts from multiple teams owned by the user.
func TestArtifactRepository_ListCrossTeam_ReturnsArtifactsFromMultipleTeams(t *testing.T) {
	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	// Scenario:
	// - Alice owns team-A and team-B
	// - Artifact-1 belongs to team-A, Artifact-2 belongs to team-B
	// - ListCrossTeam should return both artifacts (user_id ownership, no team filter)

	aliceUserID := "user-alice"
	teamAID := "team-a"
	teamBID := "team-b"
	artifactAID := "artifact-team-a"
	artifactBID := "artifact-team-b"

	filters := repositories.ArtifactFilters{
		Page:  1,
		Limit: 20,
	}

	// COUNT query must include a.user_id = $1 to prove user-ownership scoping.
	// squirrel binds userID individually for each EXISTS clause (3×);
	// LIMIT/OFFSET are literals.
	countQuery := `SELECT COUNT\(\*\) FROM artifacts a WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(countQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, "archived").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// LIST query must include a.user_id = $1 to prove creator-ownership scoping
	listQuery := `SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id,` +
		` a\.title, a\.description, a\.status, a\.type, a\.metadata,` +
		` a\.created_at, a\.updated_at\s+FROM artifacts a\s+WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(listQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, "archived").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"status", "type", "metadata", "created_at", "updated_at",
		}).
			AddRow(artifactAID, "project-1", "slug-a", aliceUserID, teamAID,
				"Artifact A", "Desc A", "active", "general", []byte("{}"), now, now).
			AddRow(artifactBID, "project-2", "slug-b", aliceUserID, teamBID,
				"Artifact B", "Desc B", "active", "work_reports", []byte("{}"), now, now))

	artifacts, total, err := repo.ListCrossTeam(ctx, aliceUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, artifacts, 2)
	// Verify artifacts from both teams are returned
	teamIDs := map[string]bool{artifacts[0].TeamID: true, artifacts[1].TeamID: true}
	assert.True(t, teamIDs[teamAID], "expected artifact from team-a")
	assert.True(t, teamIDs[teamBID], "expected artifact from team-b")
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

// TestArtifactRepository_ListCrossTeam_ExcludesOtherUsersArtifacts verifies that ListCrossTeam
// does not return artifacts belonging to other users.
func TestArtifactRepository_ListCrossTeam_ExcludesOtherUsersArtifacts(t *testing.T) {
	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	// Scenario:
	// - Alice queries ListCrossTeam
	// - DB returns only Alice's artifacts (Bob's are excluded by user_id predicate)
	// - Expected: only Alice's artifact is returned

	aliceUserID := "user-alice"
	aliceArtifactID := "artifact-alice"

	filters := repositories.ArtifactFilters{
		Page:  1,
		Limit: 20,
	}

	// COUNT must scope to a.user_id = $1, proving Bob's artifacts are excluded at the SQL level.
	// squirrel binds userID individually for each EXISTS clause (3×); LIMIT/OFFSET are literals.
	countQuery := `SELECT COUNT\(\*\) FROM artifacts a WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(countQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, "archived").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	listQuery := `SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id, a\.title, ` +
		`a\.description, a\.status, a\.type, a\.metadata,` +
		` a\.created_at, a\.updated_at\s+FROM artifacts a\s+WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(listQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, "archived").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"status", "type", "metadata", "created_at", "updated_at",
		}).AddRow(
			aliceArtifactID, "project-1", "slug-alice", aliceUserID, "team-1",
			"Alice's Artifact", "", "active", "general", []byte("{}"), now, now,
		))

	artifacts, total, err := repo.ListCrossTeam(ctx, aliceUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, aliceArtifactID, artifacts[0].ID)
	assert.Equal(t, aliceUserID, artifacts[0].UserID)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

// TestArtifactRepository_ListCrossTeam_HonoursProjectFilter verifies that ListCrossTeam applies
// the ProjectID filter correctly.
func TestArtifactRepository_ListCrossTeam_HonoursProjectFilter(t *testing.T) {
	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	aliceUserID := "user-alice"
	projectID := "specific-project"
	artifactID := "artifact-in-project"

	projectIDStr := projectID
	filters := repositories.ArtifactFilters{
		ProjectID: &projectIDStr,
		Page:      1,
		Limit:     20,
	}

	// squirrel binds userID 3× (one per EXISTS clause), then the project filter,
	// then the implicit "hide archived" default-status bind; LIMIT/OFFSET are literals.
	countQuery := `SELECT COUNT\(\*\) FROM artifacts a WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(countQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, projectID, "archived").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	listQuery := `SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id, a\.title, ` +
		`a\.description, a\.status, a\.type, a\.metadata,` +
		` a\.created_at, a\.updated_at\s+FROM artifacts a\s+WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(listQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, projectID, "archived").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"status", "type", "metadata", "created_at", "updated_at",
		}).AddRow(
			artifactID, projectID, "slug-1", aliceUserID, "team-1",
			"Project Artifact", "", "active", "general", []byte("{}"), now, now,
		))

	artifacts, total, err := repo.ListCrossTeam(ctx, aliceUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, projectID, artifacts[0].ProjectID)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

// TestArtifactRepository_ListCrossTeam_HonoursStatusAndTypeFilter verifies that ListCrossTeam
// applies status and type filters correctly.
func TestArtifactRepository_ListCrossTeam_HonoursStatusAndTypeFilter(t *testing.T) {
	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	aliceUserID := "user-alice"
	artifactID := "artifact-work-report"

	statusStr := "active"
	typeStr := "work_reports"
	filters := repositories.ArtifactFilters{
		Status: &statusStr,
		Type:   &typeStr,
		Page:   1,
		Limit:  20,
	}

	// squirrel binds userID 3× (one per EXISTS clause), then type and the explicit
	// status filter (type is appended before the status block); LIMIT/OFFSET are literals.
	countQuery := `SELECT COUNT\(\*\) FROM artifacts a WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(countQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, typeStr, statusStr).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	listQuery := `SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id, a\.title, ` +
		`a\.description, a\.status, a\.type, a\.metadata,` +
		` a\.created_at, a\.updated_at\s+FROM artifacts a\s+WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(listQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, typeStr, statusStr).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"status", "type", "metadata", "created_at", "updated_at",
		}).AddRow(
			artifactID, "project-1", "slug-work", aliceUserID, "team-1",
			"Work Report", "", "active", "work_reports", []byte("{}"), now, now,
		))

	artifacts, total, err := repo.ListCrossTeam(ctx, aliceUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "active", artifacts[0].Status)
	assert.Equal(t, "work_reports", artifacts[0].Type)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

// TestArtifactRepository_ListCrossTeam_HonoursSearchFilter verifies that ListCrossTeam
// applies the search filter correctly.
func TestArtifactRepository_ListCrossTeam_HonoursSearchFilter(t *testing.T) {
	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	aliceUserID := "user-alice"
	artifactID := "artifact-search-result"

	filters := repositories.ArtifactFilters{
		Search: "design",
		Page:   1,
		Limit:  20,
	}

	// squirrel binds userID 3× (one per EXISTS clause), then the search-forced
	// active status, then the search term 3× (title/description/content ILIKE);
	// LIMIT/OFFSET are literals.
	countQuery := `SELECT COUNT\(\*\) FROM artifacts a WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(countQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, "active", "%design%", "%design%", "%design%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	listQuery := `SELECT a\.id, a\.project_id, a\.slug, a\.user_id, a\.team_id, a\.title, ` +
		`a\.description, a\.status, a\.type, a\.metadata,` +
		` a\.created_at, a\.updated_at\s+FROM artifacts a\s+WHERE.*a\.user_id = \$1`
	mockDB.ExpectQuery(listQuery).
		WithArgs(aliceUserID, aliceUserID, aliceUserID, "active", "%design%", "%design%", "%design%").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"status", "type", "metadata", "created_at", "updated_at",
		}).AddRow(
			artifactID, "project-1", "slug-design", aliceUserID, "team-1",
			"System Design Document", "", "active", "general", []byte("{}"), now, now,
		))

	artifacts, total, err := repo.ListCrossTeam(ctx, aliceUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, artifactID, artifacts[0].ID)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}
