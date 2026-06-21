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

// TestMemoryRepository_GetByID_CrossTeamIsolation verifies that users cannot access memories from other teams
func TestMemoryRepository_GetByID_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewMemoryRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Memory belongs to this team
	teamB := "team-bbb" // User tries to access from this team
	memoryID := "memory-123"

	// Scenario: Memory exists in Team A, user tries to access using Team B credentials
	// Simulate that the query returns no rows when accessing from different team
	// The query checks: WHERE id = $1 AND team_id = $2 AND (owner_id = $3 OR member_id = $3)
	mock.ExpectQuery("SELECT (.+) FROM memories m.*EXISTS.*teams.*").
		WithArgs(memoryID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // Empty result

	// Try to access memory from Team A using Team B credentials - should fail
	_, err = repo.GetByID(ctx, userID, teamB, memoryID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_Update_CrossTeamIsolation verifies that users cannot update memories from other teams
func TestMemoryRepository_Update_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewMemoryRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Memory originally belongs to this team
	teamB := "team-bbb" // User tries to update from this team

	// Scenario: Memory originally created in Team A, user tries to update from Team B
	memory := &models.Memory{
		ID:        "memory-123",
		UserID:    userID,
		TeamID:    "team-aaa", // Original team
		Text:      "Test memory",
		Metadata:  map[string]interface{}{},
		UpdatedAt: now,
		Version:   1,
	}

	// Expect ownership validation query first - should fail since memory belongs to Team A
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM memories m.*EXISTS.*teams.*").
		WithArgs(memory.ID, teamB, userID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false)) // Memory not in Team B

	// Try to update memory using Team B credentials - should fail
	memory.TeamID = teamB
	err = repo.Update(ctx, memory)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_Delete_CrossTeamIsolation verifies that users cannot delete memories from other teams
func TestMemoryRepository_Delete_CrossTeamIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewMemoryRepository(&database.DB{DB: db})
	ctx := context.Background()

	userID := "user-123"
	_ = "team-aaa"      // teamA: Memory belongs to this team
	teamB := "team-bbb" // User tries to delete from this team
	memoryID := "memory-123"

	// Scenario: Memory exists in Team A, user tries to delete using Team B credentials
	// Simulate delete attempt with wrong team_id - returns 0 rows affected
	// The query checks: WHERE id = $1 AND team_id = $2 AND (owner/admin permissions)
	mock.ExpectExec("DELETE FROM memories.*EXISTS.*teams.*").
		WithArgs(memoryID, teamB, userID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Try to delete memory from Team A using Team B credentials - should fail
	err = repo.Delete(ctx, userID, teamB, memoryID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}
