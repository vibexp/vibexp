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

// TestAgentRepository_GetByID_CrossTeamIsolation verifies that users cannot access agents from other teams
func TestAgentRepository_GetByID_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewAgentRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Agent belongs to this team
	teamB := "team-bbb" // User tries to access from this team
	agentID := "agent-123"

	// Scenario: Agent exists in Team A, user tries to access using Team B credentials
	// Simulate that the query returns no rows when accessing from different team
	// The query checks: WHERE id = $1 AND team_id = $2 AND (owner_id = $3 OR member_id = $3)
	mock.ExpectQuery("SELECT (.+) FROM agents a.*EXISTS.*teams.*").
		WithArgs(agentID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // Empty result

	// Try to access agent from Team A using Team B credentials - should fail
	_, err = repo.GetByID(ctx, userID, teamB, agentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_Update_CrossTeamIsolation verifies that users cannot update agents from other teams
func TestAgentRepository_Update_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewAgentRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Agent originally belongs to this team
	teamB := "team-bbb" // User tries to update from this team

	// Scenario: Agent originally created in Team A, user tries to update from Team B
	cardURL := "https://example.com/card"
	agent := &models.Agent{
		ID:          "agent-123",
		UserID:      userID,
		TeamID:      "team-aaa", // Original team
		Name:        "Test Agent",
		Description: "Test description",
		Status:      "active",
		CardURL:     &cardURL,
		TotalRuns:   0,
		SuccessRate: 0.0,
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}

	// Expect ownership validation query first - should fail since agent belongs to Team A
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM agents a.*EXISTS.*teams.*").
		WithArgs(agent.ID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false)) // Agent not in Team B

	// Try to update agent using Team B credentials - should fail
	agent.TeamID = teamB
	err = repo.Update(ctx, agent)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_Delete_CrossTeamIsolation verifies that users cannot delete agents from other teams
func TestAgentRepository_Delete_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewAgentRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Agent belongs to this team
	teamB := "team-bbb" // User tries to delete from this team
	agentID := "agent-123"

	// Scenario: Agent exists in Team A, user tries to delete using Team B credentials
	// Simulate delete attempt with wrong team_id - returns 0 rows affected
	// The query checks: WHERE id = $1 AND team_id = $2 AND (owner/admin permissions via EXISTS)
	mock.ExpectExec("DELETE FROM agents").
		WithArgs(agentID, teamB, userID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Try to delete agent from Team A using Team B credentials - should fail
	err = repo.Delete(ctx, userID, teamB, agentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}
