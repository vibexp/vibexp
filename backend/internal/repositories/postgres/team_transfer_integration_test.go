//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level tests for TeamRepository.TransferOwnership against real
// Postgres (#222, epic #220).
//
// A transaction is exactly the thing sqlmock cannot prove: these assert the
// end state of the rows, so a partially-applied transfer — the failure that
// would leave a team with no owner, or with owner_id disagreeing with
// team_members.role — fails here.

// seedTransferTeam creates a team owned by owner with member rows for owner
// (owner) and other (member), and returns (teamID, ownerID, otherID).
func seedTransferTeam(t *testing.T) (string, string, string) {
	t.Helper()
	ctx := context.Background()

	ownerID := insertTestUser(t)
	otherID := insertTestUser(t)
	teamID := uuid.New().String()

	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		teamID, ownerID, "Transfer Team", "transfer-team-"+teamID[:8])
	require.NoError(t, err)

	for _, m := range []struct {
		userID string
		role   models.TeamMemberRole
	}{
		{ownerID, models.TeamMemberRoleOwner},
		{otherID, models.TeamMemberRoleMember},
	} {
		_, err = integrationDB.ExecContext(ctx,
			"INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)",
			teamID, m.userID, m.role)
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	})

	return teamID, ownerID, otherID
}

func teamOwnerID(t *testing.T, teamID string) string {
	t.Helper()
	var ownerID string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT owner_id FROM teams WHERE id = $1", teamID).Scan(&ownerID))
	return ownerID
}

func memberRole(t *testing.T, teamID, userID string) string {
	t.Helper()
	var role string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT role FROM team_members WHERE team_id = $1 AND user_id = $2", teamID, userID).Scan(&role))
	return role
}

func TestIntegrationTeam_TransferOwnership_Success(t *testing.T) {
	resetIntegrationTables(t)
	teamID, ownerID, otherID := seedTransferTeam(t)
	repo := NewTeamRepository(integrationDB)

	err := repo.TransferOwnership(context.Background(), teamID, ownerID, otherID)
	require.NoError(t, err)

	// All three writes must have landed together.
	assert.Equal(t, otherID, teamOwnerID(t, teamID), "teams.owner_id must point at the new owner")
	assert.Equal(t, string(models.TeamMemberRoleOwner), memberRole(t, teamID, otherID), "new owner's role")
	assert.Equal(t, string(models.TeamMemberRoleAdmin), memberRole(t, teamID, ownerID), "previous owner is demoted to admin")
}

// TestIntegrationTeam_TransferOwnership_TargetNotAMemberRollsBack is the test
// the transaction exists for: the teams.owner_id UPDATE succeeds, then the
// promotion matches no row and the whole thing must roll back. Without the
// transaction the team would be left owned by a non-member with no owner role
// row anywhere.
func TestIntegrationTeam_TransferOwnership_TargetNotAMemberRollsBack(t *testing.T) {
	resetIntegrationTables(t)
	teamID, ownerID, _ := seedTransferTeam(t)
	strangerID := insertTestUser(t) // exists as a user, but is not a member
	repo := NewTeamRepository(integrationDB)

	err := repo.TransferOwnership(context.Background(), teamID, ownerID, strangerID)
	require.Error(t, err)
	assert.ErrorIs(t, err, repositories.ErrTeamMemberNotFound)

	// Nothing may have changed.
	assert.Equal(t, ownerID, teamOwnerID(t, teamID), "owner_id must be rolled back")
	assert.Equal(t, string(models.TeamMemberRoleOwner), memberRole(t, teamID, ownerID), "owner keeps their role")
}

// TestIntegrationTeam_TransferOwnership_StaleOwnerIsRejected covers the
// concurrency guard: a transfer issued against an owner who no longer owns the
// team (a second racing transfer, or a stale read) must lose cleanly rather
// than overwrite the winner.
func TestIntegrationTeam_TransferOwnership_StaleOwnerIsRejected(t *testing.T) {
	resetIntegrationTables(t)
	teamID, ownerID, otherID := seedTransferTeam(t)
	repo := NewTeamRepository(integrationDB)

	// First transfer wins.
	require.NoError(t, repo.TransferOwnership(context.Background(), teamID, ownerID, otherID))

	// The same call replayed is now stale: ownerID is no longer the owner.
	err := repo.TransferOwnership(context.Background(), teamID, ownerID, otherID)
	require.Error(t, err)
	assert.ErrorIs(t, err, repositories.ErrTeamNotFound)

	// The winner's state stands.
	assert.Equal(t, otherID, teamOwnerID(t, teamID))
	assert.Equal(t, string(models.TeamMemberRoleOwner), memberRole(t, teamID, otherID))
	assert.Equal(t, string(models.TeamMemberRoleAdmin), memberRole(t, teamID, ownerID))
}
