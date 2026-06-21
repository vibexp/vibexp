package postgres_test

import (
	"context"
	"database/sql/driver"
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

// TestMemoryRepository_TeamMember_CanListOtherMembersMemories verifies that team members can list
// memories created by other team members
func TestMemoryRepository_TeamMember_CanListOtherMembersMemories(t *testing.T) {
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

	// Scenario:
	// - Alice creates team "DataScience"
	// - Alice creates memory "ml-notes"
	// - Bob joins team as member
	// - Bob lists memories
	// - Expected: Bob sees Alice's "ml-notes" in the list

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceMemoryID := "memory-alice"

	filters := repositories.MemoryFilters{
		TeamID: teamID,
		Page:   1,
		Limit:  20,
	}

	// squirrel binds the EXISTS pair individually: team_id, then team/user per EXISTS clause.
	memoryListArgs := []driver.Value{teamID, teamID, bobUserID, teamID, bobUserID}

	// Mock count query - no JOINs needed with EXISTS subqueries
	countQuery := `SELECT COUNT\(\*\) FROM memories m WHERE`
	mock.ExpectQuery(countQuery).
		WithArgs(memoryListArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Mock list query - no DISTINCT or JOINs needed with EXISTS subqueries;
	// LIMIT/OFFSET are inlined literals, so they are no longer bound args.
	listQuery := `SELECT m\.id, m\.user_id, m\.team_id, m\.project_id, m\.text, m\.metadata, m\.created_at, ` +
		`m\.updated_at FROM memories m WHERE .* LIMIT 20 OFFSET 0`
	mock.ExpectQuery(listQuery).
		WithArgs(memoryListArgs...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "project_id", "text", "metadata", "created_at", "updated_at",
		}).AddRow(
			aliceMemoryID, aliceUserID, teamID, "project-123", "Machine learning best practices",
			[]byte(`{"project": "ml-research"}`), now, now,
		))

	// Bob lists memories
	memories, total, err := repo.List(ctx, bobUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, memories, 1)
	assert.Equal(t, aliceMemoryID, memories[0].ID)
	assert.Equal(t, aliceUserID, memories[0].UserID) // Created by Alice
	assert.Equal(t, "Machine learning best practices", memories[0].Text)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_TeamMember_CanGetOtherMembersMemories verifies that team members can get
// memories created by other team members
func TestMemoryRepository_TeamMember_CanGetOtherMembersMemories(t *testing.T) {
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

	// Scenario:
	// - Alice creates memory in team
	// - Bob (team member) gets memory by ID
	// - Expected: Bob successfully retrieves Alice's memory

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceMemoryID := "memory-alice"

	getQuery := `SELECT m\.id, m\.user_id, m\.team_id, m\.project_id, m\.text, ` +
		`m\.metadata, m\.created_at, m\.updated_at, m\.version`
	mock.ExpectQuery(getQuery).
		WithArgs(aliceMemoryID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "project_id", "text", "metadata", "created_at", "updated_at", "version",
		}).AddRow(
			aliceMemoryID, aliceUserID, teamID, "project-123", "Machine learning best practices",
			[]byte(`{"project": "ml-research"}`), now, now, 1,
		))

	// Bob gets Alice's memory
	memory, err := repo.GetByID(ctx, bobUserID, teamID, aliceMemoryID)

	assert.NoError(t, err)
	require.NotNil(t, memory)
	assert.Equal(t, aliceMemoryID, memory.ID)
	assert.Equal(t, aliceUserID, memory.UserID)
	assert.Equal(t, "Machine learning best practices", memory.Text)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_TeamMember_CanUpdateOtherMembersMemories verifies that team members can update
// memories created by other team members
func TestMemoryRepository_TeamMember_CanUpdateOtherMembersMemories(t *testing.T) {
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

	// Scenario:
	// - Alice creates memory in team
	// - Bob (team member) updates the memory
	// - Expected: Update succeeds

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceMemoryID := "memory-alice"

	memory := &models.Memory{
		ID:        aliceMemoryID,
		UserID:    bobUserID, // Bob is updating
		TeamID:    teamID,
		Text:      "Machine learning best practices - Updated by Bob",
		Metadata:  map[string]interface{}{"project": "ml-research", "updated_by": "bob"},
		UpdatedAt: now,
		Version:   1,
	}

	// Mock ownership validation
	ownershipQuery := `SELECT EXISTS\(`
	mock.ExpectQuery(ownershipQuery).
		WithArgs(aliceMemoryID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Mock update query (args: id, text, metadata, project_id, team_id, updated_at, team_id, version, user_id)
	updateQuery := `UPDATE memories`
	mock.ExpectQuery(updateQuery).
		WithArgs(
			aliceMemoryID, "Machine learning best practices - Updated by Bob",
			sqlmock.AnyArg(), "", teamID, now, teamID, 1, bobUserID,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	// Bob updates Alice's memory
	err = repo.Update(ctx, memory)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), memory.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_RegularMember_CannotDeleteOtherMembersMemories verifies that regular members
// cannot delete memories created by other team members
func TestMemoryRepository_RegularMember_CannotDeleteOtherMembersMemories(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewMemoryRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates memory in team
	// - Bob (regular member) tries to delete
	// - Expected: Delete fails

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceMemoryID := "memory-alice"

	deleteQuery := `DELETE FROM memories`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceMemoryID, teamID, bobUserID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Bob tries to delete Alice's memory
	err = repo.Delete(ctx, bobUserID, teamID, aliceMemoryID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_TeamAdmin_CanDeleteOtherMembersMemories verifies that team admins can delete
// memories created by other team members
func TestMemoryRepository_TeamAdmin_CanDeleteOtherMembersMemories(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewMemoryRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates memory in team
	// - Carol (team admin) deletes the memory
	// - Expected: Delete succeeds

	teamID := "team-123"
	carolUserID := "user-carol" // Carol is admin
	aliceMemoryID := "memory-alice"

	deleteQuery := `DELETE FROM memories`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceMemoryID, teamID, carolUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Carol (admin) deletes Alice's memory
	err = repo.Delete(ctx, carolUserID, teamID, aliceMemoryID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_TeamOwner_CanDeleteAnyTeamMemory verifies that team owners can delete any memory in the team
func TestMemoryRepository_TeamOwner_CanDeleteAnyTeamMemory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewMemoryRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Bob creates memory in team
	// - Alice (team owner) deletes Bob's memory
	// - Expected: Delete succeeds

	teamID := "team-123"
	aliceUserID := "user-alice" // Alice is team owner
	bobMemoryID := "memory-bob"

	deleteQuery := `DELETE FROM memories`
	mock.ExpectExec(deleteQuery).
		WithArgs(bobMemoryID, teamID, aliceUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Alice (owner) deletes Bob's memory
	err = repo.Delete(ctx, aliceUserID, teamID, bobMemoryID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
