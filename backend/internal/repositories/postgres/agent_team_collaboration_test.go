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

// TestAgentRepository_TeamMember_CanListOtherMembersAgents verifies that team members can list
// agents created by other team members
func TestAgentRepository_TeamMember_CanListOtherMembersAgents(t *testing.T) {
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

	// Scenario:
	// - Alice creates team "AI Research"
	// - Alice creates agent "code-reviewer"
	// - Bob joins team as member
	// - Bob lists agents
	// - Expected: Bob sees Alice's "code-reviewer" in the list

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceAgentID := "agent-alice"

	filters := repositories.AgentFilters{
		TeamID: teamID,
		Page:   1,
		Limit:  20,
	}

	// Mock count query - no JOINs needed with EXISTS subqueries. squirrel binds a
	// positional placeholder per argument, so the team-access predicate repeats
	// teamID/userID once per EXISTS branch: (team_id, team, user, team, user).
	countQuery := `SELECT COUNT\(\*\) FROM agents a WHERE`
	mock.ExpectQuery(countQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Mock list query - no DISTINCT or JOINs needed with EXISTS subqueries.
	// LIMIT/OFFSET are emitted as literals by squirrel, so they are not bound args.
	listQuery := `SELECT a\.id, a\.user_id, a\.team_id, a\.name, a\.description, a\.status, ` +
		`a\.card_url, a\.agent_card, a\.last_run, a\.last_synced_at, a\.total_runs, a\.success_rate, ` +
		`a\.created_at, a\.updated_at FROM agents a WHERE`
	mock.ExpectQuery(listQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "name", "description", "status",
			"card_url", "agent_card", "last_run", "last_synced_at",
			"total_runs", "success_rate", "created_at", "updated_at",
		}).AddRow(
			aliceAgentID, aliceUserID, teamID, "Code Reviewer", "Automated code review agent",
			"active", nil, nil, nil, nil,
			0, 0.0, now, now,
		))

	// Bob lists agents
	agents, total, err := repo.List(ctx, bobUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, agents, 1)
	assert.Equal(t, aliceAgentID, agents[0].ID)
	assert.Equal(t, aliceUserID, agents[0].UserID) // Created by Alice
	assert.Equal(t, "Code Reviewer", agents[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_TeamMember_CanGetOtherMembersAgents verifies that team members can get
// agents created by other team members
func TestAgentRepository_TeamMember_CanGetOtherMembersAgents(t *testing.T) {
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

	// Scenario:
	// - Alice creates agent in team
	// - Bob (team member) gets agent by ID
	// - Expected: Bob successfully retrieves Alice's agent

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceAgentID := "agent-alice"

	// Mock get query - no JOINs needed with EXISTS subqueries
	getQuery := `SELECT a\.id, a\.user_id, a\.team_id, a\.name, a\.description, a\.status, a\.card_url, ` +
		`a\.agent_card,\s+a\.credentials, a\.last_run, a\.last_synced_at, a\.total_runs, a\.success_rate, ` +
		`a\.created_at,\s+a\.updated_at, a\.version\s+FROM agents a\s+WHERE`
	mock.ExpectQuery(getQuery).
		WithArgs(aliceAgentID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "name", "description", "status",
			"card_url", "agent_card", "credentials", "last_run", "last_synced_at",
			"total_runs", "success_rate", "created_at", "updated_at", "version",
		}).AddRow(
			aliceAgentID, aliceUserID, teamID, "Code Reviewer", "Automated code review agent",
			"active", nil, nil, nil, nil, nil,
			0, 0.0, now, now, 1,
		))

	// Bob gets Alice's agent
	agent, err := repo.GetByID(ctx, bobUserID, teamID, aliceAgentID)

	assert.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, aliceAgentID, agent.ID)
	assert.Equal(t, aliceUserID, agent.UserID)
	assert.Equal(t, "Code Reviewer", agent.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_RegularMember_CannotUpdateOtherMembersAgents verifies that regular team members
// cannot update agents created by other team members (only owner/admin can update)
func TestAgentRepository_RegularMember_CannotUpdateOtherMembersAgents(t *testing.T) {
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

	// Scenario:
	// - Alice creates agent in team
	// - Bob (regular team member, not admin) tries to update the agent
	// - Expected: Update fails - only resource owner, team owner, or team admin can update

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceAgentID := "agent-alice"

	agent := &models.Agent{
		ID:          aliceAgentID,
		UserID:      bobUserID, // Bob is trying to update
		TeamID:      teamID,
		Name:        "Code Reviewer - Updated by Bob",
		Description: "Enhanced code review agent",
		Status:      "active",
		Config:      map[string]interface{}{},
		UpdatedAt:   now,
		Version:     1,
	}

	// Mock ownership/admin validation - should return false because Bob is not owner/admin
	ownershipQuery := `SELECT EXISTS\(`
	mock.ExpectQuery(ownershipQuery).
		WithArgs(aliceAgentID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	// Bob tries to update Alice's agent
	err = repo.Update(ctx, agent)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_TeamAdmin_CanUpdateOtherMembersAgents verifies that team admins can update
// agents created by other team members
func TestAgentRepository_TeamAdmin_CanUpdateOtherMembersAgents(t *testing.T) {
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

	// Scenario:
	// - Alice creates agent in team
	// - Carol (team admin) updates the agent
	// - Expected: Update succeeds

	teamID := "team-123"
	carolUserID := "user-carol" // Carol is team admin
	aliceAgentID := "agent-alice"

	agent := &models.Agent{
		ID:          aliceAgentID,
		UserID:      carolUserID, // Carol is updating
		TeamID:      teamID,
		Name:        "Code Reviewer - Updated by Carol",
		Description: "Enhanced code review agent by admin",
		Status:      "active",
		Config:      map[string]interface{}{},
		UpdatedAt:   now,
		Version:     1,
	}

	// Mock ownership/admin validation - should return true because Carol is admin
	ownershipQuery := `SELECT EXISTS\(`
	mock.ExpectQuery(ownershipQuery).
		WithArgs(aliceAgentID, teamID, carolUserID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Mock update query
	updateQuery := `UPDATE agents`
	mock.ExpectQuery(updateQuery).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	// Carol (admin) updates Alice's agent
	err = repo.Update(ctx, agent)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), agent.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_RegularMember_CannotDeleteOtherMembersAgents verifies that regular members
// cannot delete agents created by other team members
func TestAgentRepository_RegularMember_CannotDeleteOtherMembersAgents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewAgentRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates agent in team
	// - Bob (regular member) tries to delete
	// - Expected: Delete fails

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceAgentID := "agent-alice"

	deleteQuery := `DELETE FROM agents`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceAgentID, teamID, bobUserID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Bob tries to delete Alice's agent
	err = repo.Delete(ctx, bobUserID, teamID, aliceAgentID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_TeamAdmin_CanDeleteOtherMembersAgents verifies that team admins can delete
// agents created by other team members
func TestAgentRepository_TeamAdmin_CanDeleteOtherMembersAgents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewAgentRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates agent in team
	// - Carol (team admin) deletes the agent
	// - Expected: Delete succeeds

	teamID := "team-123"
	carolUserID := "user-carol" // Carol is admin
	aliceAgentID := "agent-alice"

	deleteQuery := `DELETE FROM agents`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceAgentID, teamID, carolUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Carol (admin) deletes Alice's agent
	err = repo.Delete(ctx, carolUserID, teamID, aliceAgentID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_TeamOwner_CanDeleteAnyTeamAgent verifies that team owners can delete any agent in the team
func TestAgentRepository_TeamOwner_CanDeleteAnyTeamAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewAgentRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Bob creates agent in team
	// - Alice (team owner) deletes Bob's agent
	// - Expected: Delete succeeds

	teamID := "team-123"
	aliceUserID := "user-alice" // Alice is team owner
	bobAgentID := "agent-bob"

	deleteQuery := `DELETE FROM agents`
	mock.ExpectExec(deleteQuery).
		WithArgs(bobAgentID, teamID, aliceUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Alice (owner) deletes Bob's agent
	err = repo.Delete(ctx, aliceUserID, teamID, bobAgentID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
