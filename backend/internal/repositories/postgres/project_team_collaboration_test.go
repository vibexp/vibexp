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

// TestProjectRepository_TeamMember_CanListOtherMembersProjects verifies that team members can list
// projects created by other team members
func TestProjectRepository_TeamMember_CanListOtherMembersProjects(t *testing.T) {
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

	// Scenario:
	// - Alice creates team "Development"
	// - Alice creates project "mobile-app"
	// - Bob joins team as member
	// - Bob lists projects
	// - Expected: Bob sees Alice's "mobile-app" in the list

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceProjectID := "project-alice"

	filters := repositories.ProjectListFilters{
		TeamID: teamID,
		Page:   1,
		Limit:  20,
	}

	// Mock count query - no DISTINCT needed since EXISTS subqueries eliminate duplicates
	countQuery := `SELECT COUNT\(\*\) FROM projects p`
	mock.ExpectQuery(countQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Mock list query - no DISTINCT needed since EXISTS subqueries eliminate duplicates
	listQuery := `SELECT p\.id, p\.user_id, p\.team_id, p\.name, p\.slug, p\.description`
	mock.ExpectQuery(listQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "name", "slug", "description",
			"git_url", "homepage", "created_at", "updated_at", "version",
		}).AddRow(
			aliceProjectID, aliceUserID, teamID, "Mobile App", "mobile-app",
			"iOS and Android application", "https://github.com/org/mobile-app", "https://app.example.com",
			now, now, 1,
		))

	// Bob lists projects
	projects, total, err := repo.List(ctx, bobUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, projects, 1)
	assert.Equal(t, aliceProjectID, projects[0].ID)
	assert.Equal(t, aliceUserID, projects[0].UserID) // Created by Alice
	assert.Equal(t, "Mobile App", projects[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_TeamMember_CanGetOtherMembersProjects verifies that team members can get
// projects created by other team members
func TestProjectRepository_TeamMember_CanGetOtherMembersProjects(t *testing.T) {
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

	// Scenario:
	// - Alice creates project in team
	// - Bob (team member) gets project by ID
	// - Expected: Bob successfully retrieves Alice's project

	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceProjectID := "project-alice"

	getQuery := `SELECT p\.id, p\.user_id, p\.team_id, p\.name, p\.slug, p\.description`
	mock.ExpectQuery(getQuery).
		WithArgs(aliceProjectID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "name", "slug", "description",
			"git_url", "homepage", "created_at", "updated_at", "version",
		}).AddRow(
			aliceProjectID, aliceUserID, "team-123", "Mobile App", "mobile-app",
			"iOS and Android application", "https://github.com/org/mobile-app", "https://app.example.com",
			now, now, 1,
		))

	// Bob gets Alice's project
	project, err := repo.GetByID(ctx, bobUserID, aliceProjectID)

	assert.NoError(t, err)
	require.NotNil(t, project)
	assert.Equal(t, aliceProjectID, project.ID)
	assert.Equal(t, aliceUserID, project.UserID)
	assert.Equal(t, "Mobile App", project.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_TeamMember_CanUpdateOtherMembersProjects verifies that team members can update
// projects created by other team members
func TestProjectRepository_TeamMember_CanUpdateOtherMembersProjects(t *testing.T) {
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

	// Scenario:
	// - Alice creates project in team
	// - Bob (team member) updates the project
	// - Expected: Update succeeds

	bobUserID := "user-bob"
	aliceProjectID := "project-alice"

	project := &models.Project{
		ID:          aliceProjectID,
		UserID:      bobUserID, // Bob is updating
		TeamID:      "team-123",
		Name:        "Mobile App - Updated by Bob",
		Slug:        "mobile-app",
		Description: "Updated description",
		GitURL:      "https://github.com/org/mobile-app",
		Homepage:    "https://app.example.com",
		UpdatedAt:   now,
		Version:     1,
	}

	// Mock ownership validation
	ownershipQuery := `SELECT EXISTS\(`
	mock.ExpectQuery(ownershipQuery).
		WithArgs(aliceProjectID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Mock update query
	updateQuery := `UPDATE projects`
	mock.ExpectQuery(updateQuery).
		WithArgs(
			aliceProjectID, "Mobile App - Updated by Bob", "mobile-app",
			"Updated description", "https://github.com/org/mobile-app", "https://app.example.com",
			now, 1, bobUserID,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	// Bob updates Alice's project
	err = repo.Update(ctx, project)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), project.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_RegularMember_CannotDeleteOtherMembersProjects verifies that regular members
// cannot delete projects created by other team members
func TestProjectRepository_RegularMember_CannotDeleteOtherMembersProjects(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates project in team
	// - Bob (regular member) tries to delete
	// - Expected: Delete fails

	bobUserID := "user-bob"
	aliceProjectSlug := "mobile-app"

	teamID := "team-123"

	deleteQuery := `DELETE FROM projects`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceProjectSlug, teamID, bobUserID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Bob tries to delete Alice's project
	err = repo.Delete(ctx, teamID, bobUserID, aliceProjectSlug)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_TeamAdmin_CanDeleteOtherMembersProjects verifies that team admins can delete
// projects created by other team members
func TestProjectRepository_TeamAdmin_CanDeleteOtherMembersProjects(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates project in team
	// - Carol (team admin) deletes the project
	// - Expected: Delete succeeds

	teamID := "team-123"
	carolUserID := "user-carol" // Carol is admin
	aliceProjectSlug := "mobile-app"

	deleteQuery := `DELETE FROM projects`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceProjectSlug, teamID, carolUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Carol (admin) deletes Alice's project
	err = repo.Delete(ctx, teamID, carolUserID, aliceProjectSlug)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_TeamOwner_CanDeleteAnyTeamProject verifies that team owners can delete any project in the team
func TestProjectRepository_TeamOwner_CanDeleteAnyTeamProject(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Bob creates project in team
	// - Alice (team owner) deletes Bob's project
	// - Expected: Delete succeeds

	teamID := "team-123"
	aliceUserID := "user-alice" // Alice is team owner
	bobProjectSlug := "backend-service"

	deleteQuery := `DELETE FROM projects`
	mock.ExpectExec(deleteQuery).
		WithArgs(bobProjectSlug, teamID, aliceUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Alice (owner) deletes Bob's project
	err = repo.Delete(ctx, teamID, aliceUserID, bobProjectSlug)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
