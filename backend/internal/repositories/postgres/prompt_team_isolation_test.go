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

// TestPromptRepository_GetByID_CrossTeamIsolation verifies that users cannot access prompts from other teams
func TestPromptRepository_GetByID_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Prompt belongs to this team
	teamB := "team-bbb" // User tries to access from this team
	promptID := "prompt-123"

	// Scenario: Prompt exists in Team A, user tries to access using Team B credentials
	// Simulate that the query returns no rows when accessing from different team
	// The query checks: WHERE id = $1 AND user_id = $2 AND team_id = $3
	mock.ExpectQuery("SELECT (.+) FROM prompts p.*EXISTS.*teams.*").
		WithArgs(promptID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // Empty result

	// Try to access prompt from Team A using Team B credentials - should fail
	_, err = repo.GetByID(ctx, userID, teamB, promptID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_Update_CrossTeamIsolation verifies that users cannot update prompts from other teams
func TestPromptRepository_Update_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Prompt originally belongs to this team
	teamB := "team-bbb" // User tries to update from this team

	// Scenario: Prompt originally created in Team A, user tries to update from Team B
	prompt := &models.Prompt{
		ID:          "prompt-123",
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "Test description",
		Body:        "Test body",
		UserID:      userID,
		TeamID:      "team-aaa", // Original team
		ProjectID:   "project-123",
		Status:      "published",
		MCPExpose:   false,
		UpdatedAt:   now,
		Version:     1,
	}

	// Expect ownership validation query first - should fail since prompt belongs to Team A
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM prompts p.*EXISTS.*teams.*").
		WithArgs(prompt.ID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false)) // Prompt not in Team B

	// Try to update prompt using Team B credentials - should fail
	prompt.TeamID = teamB
	err = repo.Update(ctx, prompt)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_Delete_CrossTeamIsolation verifies that users cannot delete prompts from other teams
func TestPromptRepository_Delete_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Prompt belongs to this team
	teamB := "team-bbb" // User tries to delete from this team
	promptID := "prompt-123"

	// Scenario: Prompt exists in Team A, user tries to delete using Team B credentials
	// Simulate delete attempt with wrong team_id - returns 0 rows affected
	// The query checks: WHERE id = $1 AND user_id = $2 AND team_id = $3
	mock.ExpectExec("DELETE FROM prompts").
		WithArgs(promptID, teamB, userID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Try to delete prompt from Team A using Team B credentials - should fail
	err = repo.Delete(ctx, userID, teamB, promptID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_GetBySlug_CrossTeamIsolation verifies that users cannot access prompts by slug from other teams
func TestPromptRepository_GetBySlug_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Prompt belongs to this team
	teamB := "team-bbb" // User tries to access from this team
	slug := "test-prompt"

	// Scenario: Prompt exists in Team A, user tries to access by slug using Team B credentials
	// Simulate that the query returns no rows when accessing from different team
	// The query checks: WHERE slug = $1 AND user_id = $2 AND team_id = $3
	mock.ExpectQuery("SELECT (.+) FROM prompts p.*EXISTS.*teams.*").
		WithArgs(slug, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // Empty result

	// Try to access prompt from Team A by slug using Team B credentials - should fail
	_, err = repo.GetBySlug(ctx, userID, teamB, slug)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}
