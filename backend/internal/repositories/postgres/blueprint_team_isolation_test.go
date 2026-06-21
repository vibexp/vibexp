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
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// TestBlueprintRepository_GetByID_CrossTeamIsolation verifies that users cannot access blueprint from other teams
func TestBlueprintRepository_GetByID_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Spec library belongs to this team
	teamB := "team-bbb" // User tries to access from this team
	blueprintID := "spec-123"

	// Scenario: Spec library exists in Team A, user tries to access using Team B credentials
	// Simulate that the query returns no rows when accessing from different team
	// The query checks: WHERE id = $1 AND team_id = $2 AND (owner_id = $3 OR member_id = $3)
	mock.ExpectQuery("SELECT (.+) FROM blueprints s.*EXISTS.*teams.*").
		WithArgs(blueprintID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // Empty result

	// Try to access blueprint from Team A using Team B credentials - should fail
	_, err = repo.GetByID(ctx, userID, teamB, blueprintID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_Update_CrossTeamIsolation verifies that users cannot update blueprint from other teams
func TestBlueprintRepository_Update_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Spec library originally belongs to this team
	teamB := "team-bbb" // User tries to update from this team

	// Scenario: Spec library originally created in Team A, user tries to update from Team B
	subtype := "api"
	blueprint := &models.Blueprint{
		ID:          "spec-123",
		ProjectID:   "project-123",
		Slug:        "test-spec",
		UserID:      userID,
		TeamID:      "team-aaa", // Original team
		Title:       "Test Spec",
		Description: "Test description",
		Content:     "Test content",
		Status:      "active",
		Type:        "spec",
		Subtype:     &subtype,
		Metadata:    map[string]interface{}{},
		UpdatedAt:   now,
		Version:     1,
	}

	// Expect ownership validation query first - should fail since blueprint belongs to Team A
	ownershipQuery := "SELECT EXISTS\\(.*FROM blueprints s.*EXISTS.*teams.*"
	mock.ExpectQuery(ownershipQuery).
		WithArgs(blueprint.ID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false)) // Spec library not in Team B

	// Try to update blueprint using Team B credentials - should fail
	blueprint.TeamID = teamB
	err = repo.Update(ctx, blueprint)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_Delete_CrossTeamIsolation verifies that users cannot delete blueprint from other teams
func TestBlueprintRepository_Delete_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Spec library belongs to this team
	teamB := "team-bbb" // User tries to delete from this team
	blueprintID := "spec-123"

	// Scenario: Spec library exists in Team A, user tries to delete using Team B credentials
	// Simulate delete attempt with wrong team_id - returns 0 rows affected
	// The query checks: WHERE id = $1 AND team_id = $2 AND (owner/admin permissions)
	mock.ExpectExec("DELETE FROM blueprints.*EXISTS.*teams.*").
		WithArgs(blueprintID, teamB, userID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Try to delete blueprint from Team A using Team B credentials - should fail
	err = repo.Delete(ctx, userID, teamB, blueprintID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_GetByProjectIDAndSlug_CrossTeamIsolation verifies cross-team isolation for slug lookup
func TestBlueprintRepository_GetByProjectIDAndSlug_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Spec library belongs to this team
	teamB := "team-bbb" // User tries to access from this team
	projectID := "project-123"
	slug := "test-spec"

	// Scenario: Spec library exists in Team A, user tries to access by slug using Team B credentials
	// Simulate that the query returns no rows when accessing from different team
	// The query checks: WHERE project_id = $1 AND slug = $2 AND team_id = $3 AND (owner_id = $4 OR member_id = $4)
	mock.ExpectQuery("SELECT (.+) FROM blueprints s.*EXISTS.*teams.*").
		WithArgs(projectID, slug, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // Empty result

	// Try to access blueprint from Team A by slug using Team B credentials - should fail
	_, err = repo.GetByProjectIDAndSlug(ctx, userID, teamB, projectID, slug)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}
