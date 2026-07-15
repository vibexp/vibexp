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

// TestArtifactRepository_GetByID_CrossTeamIsolation verifies that users cannot access artifacts from other teams
func TestArtifactRepository_GetByID_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Artifact belongs to this team
	teamB := "team-bbb" // User tries to access from this team
	artifactID := "artifact-123"

	// Scenario: Artifact exists in Team A, user tries to access using Team B credentials
	// Simulate that the query returns no rows when accessing from different team
	// The query checks: WHERE id = $1 AND team_id = $2 AND (owner_id = $3 OR member_id = $3)
	mock.ExpectQuery("SELECT (.+) FROM artifacts a.*EXISTS.*teams.*").
		WithArgs(artifactID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // Empty result

	// Try to access artifact from Team A using Team B credentials - should fail
	_, err = repo.GetByID(ctx, userID, teamB, artifactID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_Update_CrossTeamIsolation verifies that users cannot update artifacts from other teams
func TestArtifactRepository_Update_CrossTeamIsolation(t *testing.T) {
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

	userID := "user-123"
	_ = "team-aaa"      // teamA: Artifact originally belongs to this team
	teamB := "team-bbb" // User tries to update from this team

	// Scenario: Artifact originally created in Team A, user tries to update from Team B
	artifact := &models.Artifact{
		ID:          "artifact-123",
		ProjectID:   "project-123",
		Slug:        "test-artifact",
		UserID:      userID,
		TeamID:      "team-aaa", // Original team
		Title:       "Test Artifact",
		Description: "Test description",
		Content:     "Test content",
		Status:      "active",
		Type:        "general",
		Metadata:    map[string]interface{}{},
		UpdatedAt:   now,
		Version:     1,
	}

	// Existence-in-team check — fails because the artifact belongs to Team A.
	// Tenancy stays in SQL (D3 moved only ROLE logic out), so cross-team
	// isolation is still enforced by the repository.
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM artifacts a.*").
		WithArgs(artifact.ID, teamB).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false)) // Artifact not in Team B

	// Try to update artifact using Team B credentials - should fail
	artifact.TeamID = teamB
	err = repo.Update(ctx, artifact)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_Delete_CrossTeamIsolation verifies that users cannot delete artifacts from other teams
func TestArtifactRepository_Delete_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Artifact belongs to this team
	teamB := "team-bbb" // User tries to delete from this team
	artifactID := "artifact-123"

	// Scenario: Artifact exists in Team A, user tries to delete using Team B credentials
	// Simulate delete attempt with wrong team_id - returns 0 rows affected
	// The query checks: WHERE id = $1 AND team_id = $2 AND (owner/admin permissions)
	mock.ExpectExec("DELETE FROM artifacts.*EXISTS.*teams.*").
		WithArgs(artifactID, teamB, userID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Try to delete artifact from Team A using Team B credentials - should fail
	err = repo.Delete(ctx, userID, teamB, artifactID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_GetByProjectIDAndSlug_CrossTeamIsolation verifies cross-team isolation for slug lookup
func TestArtifactRepository_GetByProjectIDAndSlug_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Artifact belongs to this team
	teamB := "team-bbb" // User tries to access from this team
	projectID := "project-123"
	slug := "test-artifact"

	// Scenario: Artifact exists in Team A, user tries to access by slug using Team B credentials
	// Simulate that the query returns no rows when accessing from different team
	// The query checks: WHERE project_id = $1 AND slug = $2 AND team_id = $3 AND (owner_id = $4 OR member_id = $4)
	mock.ExpectQuery("SELECT (.+) FROM artifacts a.*EXISTS.*teams.*").
		WithArgs(projectID, slug, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // Empty result

	// Try to access artifact from Team A by slug using Team B credentials - should fail
	_, err = repo.GetByProjectIDAndSlug(ctx, userID, teamB, projectID, slug)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}
