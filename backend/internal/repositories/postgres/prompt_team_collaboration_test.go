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

// TestPromptRepository_TeamMember_CanListOtherMembersPrompts verifies that team members can list
// prompts created by other team members
func TestPromptRepository_TeamMember_CanListOtherMembersPrompts(t *testing.T) {
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

	// Scenario:
	// - Alice (user-alice) creates team "Marketing" (team-123)
	// - Alice creates prompt "campaign-ideas" (prompt-alice)
	// - Bob (user-bob) is invited and joins team "Marketing"
	// - Bob lists prompts with team filter
	// - Expected: Bob sees Alice's "campaign-ideas" in the list

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	alicePromptID := "prompt-alice"

	filters := repositories.PromptFilters{
		TeamID: teamID,
		Page:   1,
		Limit:  20,
	}

	// Mock count query - should include Alice's prompt (no DISTINCT needed since EXISTS eliminates duplicates).
	// squirrel binds each EXISTS placeholder individually: (team, team, user, team, user).
	countQuery := `SELECT COUNT\(\*\) FROM prompts p`
	mock.ExpectQuery(countQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Mock list query - Bob should see Alice's prompt because they're both team members.
	// LIMIT/OFFSET are inlined literals by squirrel, so they are not bound args.
	listQuery := `SELECT p\.id, p\.name, p\.slug, p\.description, p\.body, p\.user_id, p\.team_id, p\.project_id`
	mock.ExpectQuery(listQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "slug", "description", "body", "user_id", "team_id",
			"project_id", "status", "mcp_expose", "labels", "created_at", "updated_at", "is_shared",
		}).AddRow(
			alicePromptID, "Campaign Ideas", "campaign-ideas", "Marketing campaign ideas",
			"Generate creative campaign ideas", aliceUserID, teamID, "project-123",
			"published", false, []byte("{}"), now, now, false,
		))

	// Bob lists prompts in the team
	prompts, total, err := repo.List(ctx, bobUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, prompts, 1)
	assert.Equal(t, alicePromptID, prompts[0].ID)
	assert.Equal(t, aliceUserID, prompts[0].UserID) // Prompt created by Alice
	assert.Equal(t, "Campaign Ideas", prompts[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_TeamMember_CanGetOtherMembersPrompts verifies that team members can get
// prompts created by other team members
func TestPromptRepository_TeamMember_CanGetOtherMembersPrompts(t *testing.T) {
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

	// Scenario:
	// - Alice creates prompt in team "Marketing"
	// - Bob (team member) gets prompt by ID
	// - Expected: Bob successfully retrieves Alice's prompt

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	alicePromptID := "prompt-alice"

	// Mock get query - Bob can access Alice's prompt because they're both in team-123
	getQuery := `SELECT p\.id, p\.name, p\.slug, p\.description, p\.body, p\.user_id, p\.team_id, p\.project_id`
	mock.ExpectQuery(getQuery).
		WithArgs(alicePromptID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "slug", "description", "body", "user_id", "team_id",
			"project_id", "status", "mcp_expose", "labels", "created_at", "updated_at", "version", "is_shared",
		}).AddRow(
			alicePromptID, "Campaign Ideas", "campaign-ideas", "Marketing campaign ideas",
			"Generate creative campaign ideas", aliceUserID, teamID, "project-123",
			"published", false, []byte("{}"), now, now, 1, false,
		))

	// Bob gets Alice's prompt
	prompt, err := repo.GetByID(ctx, bobUserID, teamID, alicePromptID)

	assert.NoError(t, err)
	require.NotNil(t, prompt)
	assert.Equal(t, alicePromptID, prompt.ID)
	assert.Equal(t, aliceUserID, prompt.UserID) // Prompt created by Alice
	assert.Equal(t, teamID, prompt.TeamID)
	assert.Equal(t, "Campaign Ideas", prompt.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_RegularMember_CanUpdateOtherMembersPrompts verifies the
// repository's post-#236 contract: it enforces TENANCY only, so it permits a
// regular member to update another member's prompt.
//
// This inverts the old assertion deliberately. Two epic #220 decisions land here:
// D1 makes update uniform (any member may update any resource), and D3 moves role
// logic out of SQL into PromptService. The role decision is asserted at the layer
// that now owns it — see internal/services/prompt_rbac_test.go.
func TestPromptRepository_RegularMember_CanUpdateOtherMembersPrompts(t *testing.T) {
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

	// Scenario:
	// - Alice creates a prompt in team "Marketing"
	// - Bob (regular team member) updates it
	// - Expected: the repository permits it; only tenancy is its concern

	teamID := "team-123"
	bobUserID := "user-bob"
	alicePromptID := "prompt-alice"

	prompt := &models.Prompt{
		ID:          alicePromptID,
		Name:        "Campaign Ideas - Updated by Bob",
		Slug:        "campaign-ideas",
		Description: "Updated description",
		Body:        "Updated body",
		UserID:      bobUserID,
		TeamID:      teamID,
		ProjectID:   "project-123",
		Status:      "published",
		MCPExpose:   false,
		Labels:      []string{},
		UpdatedAt:   now,
		Version:     1,
	}

	// Existence-in-team check: no user argument any more.
	mock.ExpectQuery(`SELECT EXISTS\(`).
		WithArgs(alicePromptID, teamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	mock.ExpectQuery(`UPDATE prompts`).
		WithArgs(
			alicePromptID, "Campaign Ideas - Updated by Bob", "campaign-ideas",
			"Updated description", "Updated body", "project-123", "published",
			false, sqlmock.AnyArg(), teamID, now, teamID, 1,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	err = repo.Update(ctx, prompt)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), prompt.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_TeamAdmin_CanUpdateOtherMembersPrompts verifies that team admins can update
// prompts created by other team members
func TestPromptRepository_TeamAdmin_CanUpdateOtherMembersPrompts(t *testing.T) {
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

	// Scenario:
	// - Alice creates prompt in team "Marketing"
	// - Carol (team admin) updates the prompt
	// - Expected: Update succeeds, prompt is modified

	teamID := "team-123"
	carolUserID := "user-carol" // Carol is team admin
	alicePromptID := "prompt-alice"

	prompt := &models.Prompt{
		ID:          alicePromptID,
		Name:        "Campaign Ideas - Updated by Carol",
		Slug:        "campaign-ideas",
		Description: "Updated by admin",
		Body:        "Updated body by admin",
		UserID:      carolUserID, // Carol is updating (as admin)
		TeamID:      teamID,
		ProjectID:   "project-123",
		Status:      "published",
		MCPExpose:   false,
		Labels:      []string{},
		UpdatedAt:   now,
		Version:     1,
	}

	// Existence-in-team check (tenancy only; the admin's role is checked in
	// PromptService, not here).
	ownershipQuery := `SELECT EXISTS\(`
	mock.ExpectQuery(ownershipQuery).
		WithArgs(alicePromptID, teamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Mock update query - should succeed
	updateQuery := `UPDATE prompts`
	mock.ExpectQuery(updateQuery).
		WithArgs(
			alicePromptID, "Campaign Ideas - Updated by Carol", "campaign-ideas",
			"Updated by admin", "Updated body by admin", "project-123", "published",
			false, sqlmock.AnyArg(), teamID, now, teamID, 1,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	// Carol (admin) updates Alice's prompt
	err = repo.Update(ctx, prompt)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), prompt.Version) // Version incremented
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_RegularMember_CannotDeleteOtherMembersPrompts verifies that regular members
// cannot delete prompts created by other team members
func TestPromptRepository_RegularMember_CannotDeleteOtherMembersPrompts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates prompt in team "Marketing"
	// - Bob (regular member, not admin) tries to delete
	// - Expected: Delete fails (not authorized)

	teamID := "team-123"
	bobUserID := "user-bob"
	alicePromptID := "prompt-alice"

	// Mock delete query - should return 0 rows affected because Bob is not:
	// 1. Resource owner (Alice is)
	// 2. Team owner
	// 3. Team admin
	deleteQuery := `DELETE FROM prompts`
	mock.ExpectExec(deleteQuery).
		WithArgs(alicePromptID, teamID, bobUserID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Bob tries to delete Alice's prompt
	err = repo.Delete(ctx, bobUserID, teamID, alicePromptID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_TeamAdmin_CanDeleteOtherMembersPrompts verifies that team admins can delete
// prompts created by other team members
func TestPromptRepository_TeamAdmin_CanDeleteOtherMembersPrompts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates prompt in team "Marketing"
	// - Carol is team admin
	// - Carol deletes Alice's prompt
	// - Expected: Delete succeeds

	teamID := "team-123"
	carolUserID := "user-carol" // Carol is admin
	alicePromptID := "prompt-alice"

	// Mock delete query - should return 1 row affected because Carol is team admin
	deleteQuery := `DELETE FROM prompts`
	mock.ExpectExec(deleteQuery).
		WithArgs(alicePromptID, teamID, carolUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Carol (admin) deletes Alice's prompt
	err = repo.Delete(ctx, carolUserID, teamID, alicePromptID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPromptRepository_TeamOwner_CanDeleteAnyTeamPrompt verifies that team owners can delete any prompt in the team
func TestPromptRepository_TeamOwner_CanDeleteAnyTeamPrompt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Bob creates prompt in team "Marketing"
	// - Alice is team owner
	// - Alice deletes Bob's prompt
	// - Expected: Delete succeeds

	teamID := "team-123"
	aliceUserID := "user-alice" // Alice is team owner
	bobPromptID := "prompt-bob"

	// Mock delete query - should return 1 row affected because Alice is team owner
	deleteQuery := `DELETE FROM prompts`
	mock.ExpectExec(deleteQuery).
		WithArgs(bobPromptID, teamID, aliceUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Alice (owner) deletes Bob's prompt
	err = repo.Delete(ctx, aliceUserID, teamID, bobPromptID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
