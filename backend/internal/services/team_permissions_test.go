package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The team `permissions` field (#224) is what clients gate their UI on, and it
// is computed here rather than stored. These tests assert the service actually
// populates it on every response that carries a team, per the caller's role —
// the handler tests mock this service out, so they cannot prove any of it.

// TestTeamService_GetTeam_PopulatesPermissionsForRole covers the read path for
// all three roles: `permissions` is exactly the caller's matrix row.
func TestTeamService_GetTeam_PopulatesPermissionsForRole(t *testing.T) {
	for _, role := range []models.TeamMemberRole{
		models.TeamMemberRoleOwner,
		models.TeamMemberRoleAdmin,
		models.TeamMemberRoleMember,
	} {
		t.Run(string(role), func(t *testing.T) {
			teamRepo := mocks.NewMockTeamRepository(t)
			memberRepo := mocks.NewMockTeamMemberRepository(t)
			userRepo := mocks.NewMockUserRepository(t)

			teamRepo.EXPECT().GetByID(mock.Anything, "team-456").
				Return(&models.Team{ID: "team-456", OwnerID: "user-123", Name: "Team"}, nil).Once()
			memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").
				Return(&models.TeamMember{TeamID: "team-456", UserID: "user-123", Role: role}, nil).Once()

			svc := createTestTeamService(teamRepo, memberRepo, userRepo)

			team, err := svc.GetTeam(context.Background(), "user-123", "team-456")

			require.NoError(t, err)
			assert.Equal(t, string(role), team.Role)
			assert.Equal(t, authz.RolePermissionStrings(role), []string(team.Permissions))
		})
	}
}

// TestTeamService_GetTeam_MemberDoesNotReceiveAdminPermissions is the negative
// half: it names the grants a member must never be handed, so a matrix-wiring
// mistake fails loudly here rather than silently unlocking a client's UI.
func TestTeamService_GetTeam_MemberDoesNotReceiveAdminPermissions(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)
	userRepo := mocks.NewMockUserRepository(t)

	teamRepo.EXPECT().GetByID(mock.Anything, "team-456").
		Return(&models.Team{ID: "team-456", Name: "Team"}, nil).Once()
	memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").
		Return(&models.TeamMember{TeamID: "team-456", UserID: "user-123", Role: models.TeamMemberRoleMember}, nil).Once()

	svc := createTestTeamService(teamRepo, memberRepo, userRepo)

	team, err := svc.GetTeam(context.Background(), "user-123", "team-456")

	require.NoError(t, err)
	for _, denied := range []authz.Permission{
		authz.TeamUpdate, authz.TeamDelete, authz.OwnershipTransfer,
		authz.MemberInvite, authz.MemberRemove, authz.MemberRoleUpdate,
		authz.ProjectCreate, authz.ProjectUpdate, authz.ProjectDelete,
		authz.ResourceDeleteAny, authz.FeedItemDeleteAny,
	} {
		assert.NotContains(t, []string(team.Permissions), denied.String(),
			"a member must not be granted %q", denied)
	}
}

// TestTeamService_ListTeams_PopulatesPermissionsPerTeamRole proves each list
// item carries the permissions of *its own* role — a user is commonly an owner
// of one team and a member of another, so a single set for the whole page
// would be wrong.
func TestTeamService_ListTeams_PopulatesPermissionsPerTeamRole(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)
	userRepo := mocks.NewMockUserRepository(t)

	teamRepo.EXPECT().ListByUserID(mock.Anything, "user-123", 20, 0).Return(
		[]models.Team{{ID: "team-1", Name: "Owned"}, {ID: "team-2", Name: "Joined"}}, 2, nil,
	).Once()

	memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-1", "user-123").
		Return(&models.TeamMember{TeamID: "team-1", UserID: "user-123", Role: models.TeamMemberRoleOwner}, nil).Once()
	memberRepo.EXPECT().GetByTeamID(mock.Anything, "team-1").
		Return([]models.TeamMember{{TeamID: "team-1", UserID: "user-123"}}, nil).Once()

	memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-2", "user-123").
		Return(&models.TeamMember{TeamID: "team-2", UserID: "user-123", Role: models.TeamMemberRoleMember}, nil).Once()
	memberRepo.EXPECT().GetByTeamID(mock.Anything, "team-2").
		Return([]models.TeamMember{{TeamID: "team-2", UserID: "user-123"}}, nil).Once()

	svc := createTestTeamService(teamRepo, memberRepo, userRepo)

	resp, err := svc.ListTeams(context.Background(), "user-123", 1, 20)

	require.NoError(t, err)
	require.Len(t, resp.Teams, 2)
	assert.Equal(t, authz.RolePermissionStrings(models.TeamMemberRoleOwner), []string(resp.Teams[0].Permissions))
	assert.Equal(t, authz.RolePermissionStrings(models.TeamMemberRoleMember), []string(resp.Teams[1].Permissions))
}

// TestTeamService_ListTeams_RoleLookupFailureDegradesToMemberPermissions pins
// the pre-existing degrade-to-member fallback: when the role lookup fails the
// list still renders as member rather than erroring. The point here is that
// `permissions` degrades *with* `role` instead of contradicting it — the two
// are always one decision. Note this hands the client the member set on a
// transient lookup error, which is safe only because permissions is advisory:
// every write is still authorized server-side, so at worst the UI offers a
// button the server then refuses.
func TestTeamService_ListTeams_RoleLookupFailureDegradesToMemberPermissions(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)
	userRepo := mocks.NewMockUserRepository(t)

	teamRepo.EXPECT().ListByUserID(mock.Anything, "user-123", 20, 0).Return(
		[]models.Team{{ID: "team-1", Name: "Team"}}, 1, nil,
	).Once()
	memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-1", "user-123").
		Return(nil, fmt.Errorf("lookup exploded")).Once()
	memberRepo.EXPECT().GetByTeamID(mock.Anything, "team-1").
		Return([]models.TeamMember{{TeamID: "team-1", UserID: "user-123"}}, nil).Once()

	svc := createTestTeamService(teamRepo, memberRepo, userRepo)

	resp, err := svc.ListTeams(context.Background(), "user-123", 1, 20)

	require.NoError(t, err)
	require.Len(t, resp.Teams, 1)
	assert.Equal(t, string(models.TeamMemberRoleMember), resp.Teams[0].Role)
	assert.Equal(t, authz.RolePermissionStrings(models.TeamMemberRoleMember), []string(resp.Teams[0].Permissions))
}

// TestTeamService_CreateTeam_PopulatesOwnerPermissions covers the create
// response: the caller just became the owner, so it says so. An empty set here
// would read to a client as "you may do nothing in the team you just made".
func TestTeamService_CreateTeam_PopulatesOwnerPermissions(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)
	userRepo := mocks.NewMockUserRepository(t)

	teamRepo.EXPECT().GetByOwnerAndSlug(mock.Anything, "user-123", "new-team").Return(nil, fmt.Errorf("not found")).Once()
	teamRepo.EXPECT().Create(mock.Anything, mock.Anything).Run(func(_ context.Context, team *models.Team) {
		team.ID = "team-new"
	}).Return(nil).Once()
	memberRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(m *models.TeamMember) bool {
		return m.Role == models.TeamMemberRoleOwner
	})).Return(nil).Once()

	svc := createTestTeamService(teamRepo, memberRepo, userRepo)

	team, err := svc.CreateTeam(context.Background(), "user-123", &models.CreateTeamRequest{
		Name:        "New Team",
		Description: "desc",
	})

	require.NoError(t, err)
	assert.Equal(t, string(models.TeamMemberRoleOwner), team.Role)
	assert.Equal(t, authz.RolePermissionStrings(models.TeamMemberRoleOwner), []string(team.Permissions))
}

// TestTeamService_UpdateTeam_PopulatesPermissionsForRole covers the
// authorizeTeam path (shared by every write): an admin may update the team, and
// the echoed payload must carry the admin set — notably without team.delete.
func TestTeamService_UpdateTeam_PopulatesPermissionsForRole(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)
	userRepo := mocks.NewMockUserRepository(t)

	teamRepo.EXPECT().GetByID(mock.Anything, "team-456").
		Return(&models.Team{ID: "team-456", Name: "Old", Slug: "old"}, nil).Once()
	memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").
		Return(&models.TeamMember{TeamID: "team-456", UserID: "user-123", Role: models.TeamMemberRoleAdmin}, nil).Once()
	teamRepo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	svc := createTestTeamService(teamRepo, memberRepo, userRepo)

	// Description-only: renaming would pull in the slug-uniqueness path, which
	// has nothing to do with the permissions this test is about.
	newDescription := "Updated by an admin"
	team, err := svc.UpdateTeam(context.Background(), "user-123", "team-456",
		&models.UpdateTeamRequest{Description: &newDescription})

	require.NoError(t, err)
	assert.Equal(t, string(models.TeamMemberRoleAdmin), team.Role)
	assert.Equal(t, authz.RolePermissionStrings(models.TeamMemberRoleAdmin), []string(team.Permissions))
	assert.NotContains(t, []string(team.Permissions), authz.TeamDelete.String(),
		"team.delete stays owner-only (epic #220)")
}
