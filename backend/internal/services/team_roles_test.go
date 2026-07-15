package services

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	roleTestTeamID  = "team-456"
	roleTestOwnerID = "user-owner"
	roleTestCaller  = "user-caller"
	roleTestTarget  = "user-target"
)

// callerWithRole stubs the caller's membership lookup that AuthorizationService
// performs.
func callerWithRole(repo *mocks.MockTeamMemberRepository, role models.TeamMemberRole) {
	repo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestCaller).Return(&models.TeamMember{
		TeamID: roleTestTeamID,
		UserID: roleTestCaller,
		Role:   role,
	}, nil).Once()
}

func teamOwnedBy(repo *mocks.MockTeamRepository, ownerID string) {
	repo.EXPECT().GetByID(mock.Anything, roleTestTeamID).Return(&models.Team{
		ID:      roleTestTeamID,
		OwnerID: ownerID,
		Name:    "Team",
		Slug:    "team",
	}, nil).Once()
}

func TestTeamService_UpdateMemberRole_RolePolicy(t *testing.T) {
	tests := []struct {
		name       string
		callerRole models.TeamMemberRole
		newRole    models.TeamMemberRole
		target     string
		wantErr    error
	}{
		{
			name:       "owner promotes a member to admin",
			callerRole: models.TeamMemberRoleOwner,
			newRole:    models.TeamMemberRoleAdmin,
			target:     roleTestTarget,
		},
		{
			name:       "admin may also change roles",
			callerRole: models.TeamMemberRoleAdmin,
			newRole:    models.TeamMemberRoleMember,
			target:     roleTestTarget,
		},
		{
			name:       "member may not change roles",
			callerRole: models.TeamMemberRoleMember,
			newRole:    models.TeamMemberRoleAdmin,
			target:     roleTestTarget,
			wantErr:    ErrPermissionDenied,
		},
		{
			// The load-bearing guard: without it an admin could demote the
			// owner and take the team.
			name:       "admin may not change the owner's role",
			callerRole: models.TeamMemberRoleAdmin,
			newRole:    models.TeamMemberRoleMember,
			target:     roleTestOwnerID,
			wantErr:    ErrCannotChangeOwnerRole,
		},
		{
			name:       "owner may not change their own role",
			callerRole: models.TeamMemberRoleOwner,
			newRole:    models.TeamMemberRoleMember,
			target:     roleTestOwnerID,
			wantErr:    ErrCannotChangeOwnerRole,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			teamRepo := mocks.NewMockTeamRepository(t)
			memberRepo := mocks.NewMockTeamMemberRepository(t)
			userRepo := mocks.NewMockUserRepository(t)

			teamOwnedBy(teamRepo, roleTestOwnerID)
			callerWithRole(memberRepo, tc.callerRole)

			if tc.wantErr == nil {
				memberRepo.EXPECT().UpdateRole(mock.Anything, roleTestTeamID, tc.target, tc.newRole).
					Return(nil).Once()
				memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, tc.target).
					Return(&models.TeamMember{
						TeamID:    roleTestTeamID,
						UserID:    tc.target,
						Role:      tc.newRole,
						CreatedAt: time.Unix(0, 0).UTC(),
					}, nil).Once()
				userRepo.EXPECT().GetByID(mock.Anything, tc.target).Return(&models.User{
					ID:    tc.target,
					Email: "target@example.com",
					Name:  "Target",
				}, nil).Once()
			}

			svc := createTestTeamService(teamRepo, memberRepo, userRepo)
			detail, err := svc.UpdateMemberRole(context.Background(), roleTestCaller, roleTestTeamID, tc.target, tc.newRole)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, detail)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, detail)
			assert.Equal(t, string(tc.newRole), detail.Role)
			assert.Equal(t, tc.target, detail.UserID)
			assert.Equal(t, "target@example.com", detail.Email)
		})
	}
}

// TestTeamService_UpdateMemberRole_RejectsOwnerRole pins that owner can never be
// assigned here — it is checked before anything else, so no repo is touched.
func TestTeamService_UpdateMemberRole_RejectsOwnerRole(t *testing.T) {
	for _, role := range []models.TeamMemberRole{models.TeamMemberRoleOwner, "root", ""} {
		t.Run(string(role), func(t *testing.T) {
			svc := createTestTeamService(
				mocks.NewMockTeamRepository(t),
				mocks.NewMockTeamMemberRepository(t),
				mocks.NewMockUserRepository(t),
			)

			detail, err := svc.UpdateMemberRole(
				context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget, role,
			)

			assert.ErrorIs(t, err, ErrInvalidMemberRole)
			assert.Nil(t, detail)
		})
	}
}

func TestTeamService_UpdateMemberRole_TargetNotAMember(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)

	teamOwnedBy(teamRepo, roleTestOwnerID)
	callerWithRole(memberRepo, models.TeamMemberRoleOwner)
	memberRepo.EXPECT().UpdateRole(mock.Anything, roleTestTeamID, roleTestTarget, models.TeamMemberRoleAdmin).
		Return(repositories.ErrTeamMemberNotFound).Once()

	svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
	detail, err := svc.UpdateMemberRole(
		context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget, models.TeamMemberRoleAdmin,
	)

	assert.ErrorIs(t, err, repositories.ErrTeamMemberNotFound)
	assert.Nil(t, detail)
}

func TestTeamService_TransferOwnership_RolePolicy(t *testing.T) {
	tests := []struct {
		name       string
		callerRole models.TeamMemberRole
		wantErr    error
	}{
		{name: "owner may transfer", callerRole: models.TeamMemberRoleOwner},
		{
			// Admin has MemberRoleUpdate but deliberately not OwnershipTransfer.
			name:       "admin may not transfer",
			callerRole: models.TeamMemberRoleAdmin,
			wantErr:    ErrPermissionDenied,
		},
		{name: "member may not transfer", callerRole: models.TeamMemberRoleMember, wantErr: ErrPermissionDenied},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			teamRepo := mocks.NewMockTeamRepository(t)
			memberRepo := mocks.NewMockTeamMemberRepository(t)

			teamOwnedBy(teamRepo, roleTestOwnerID)
			callerWithRole(memberRepo, tc.callerRole)

			if tc.wantErr == nil {
				memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestTarget).
					Return(&models.TeamMember{TeamID: roleTestTeamID, UserID: roleTestTarget}, nil).Once()
				teamRepo.EXPECT().TransferOwnership(mock.Anything, roleTestTeamID, roleTestOwnerID, roleTestTarget).
					Return(nil).Once()
			}

			svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
			team, err := svc.TransferOwnership(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, team)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, team)
			// The response must reflect the post-transfer world, not the stale read.
			assert.Equal(t, roleTestTarget, team.OwnerID)
			assert.Equal(t, string(models.TeamMemberRoleAdmin), team.Role)
		})
	}
}

func TestTeamService_TransferOwnership_Rejections(t *testing.T) {
	t.Run("personal workspace cannot be transferred", func(t *testing.T) {
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)

		teamRepo.EXPECT().GetByID(mock.Anything, roleTestTeamID).Return(&models.Team{
			ID:         roleTestTeamID,
			OwnerID:    roleTestCaller,
			IsPersonal: true,
		}, nil).Once()
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)

		svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
		team, err := svc.TransferOwnership(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

		var personal *PersonalWorkspaceError
		assert.True(t, stderrors.As(err, &personal), "want PersonalWorkspaceError, got %v", err)
		assert.Nil(t, team)
	})

	t.Run("transfer to the current owner is rejected", func(t *testing.T) {
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)

		teamOwnedBy(teamRepo, roleTestCaller)
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)

		svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
		team, err := svc.TransferOwnership(context.Background(), roleTestCaller, roleTestTeamID, roleTestCaller)

		assert.ErrorIs(t, err, ErrAlreadyTeamOwner)
		assert.Nil(t, team)
	})

	t.Run("target must already be a member", func(t *testing.T) {
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)

		teamOwnedBy(teamRepo, roleTestOwnerID)
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestTarget).
			Return(nil, repositories.ErrTeamMemberNotFound).Once()

		svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
		team, err := svc.TransferOwnership(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

		assert.ErrorIs(t, err, repositories.ErrTeamMemberNotFound)
		assert.Nil(t, team)
		// The transfer must not have been attempted.
		teamRepo.AssertNotCalled(t, "TransferOwnership", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

// TestTeamService_RemoveTeamMember_OwnerIsUntouchable pins that an admin cannot
// remove the owner even though member.remove is now Admin+.
func TestTeamService_RemoveTeamMember_OwnerIsUntouchable(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)

	teamOwnedBy(teamRepo, roleTestOwnerID)
	callerWithRole(memberRepo, models.TeamMemberRoleAdmin)

	svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
	err := svc.RemoveTeamMember(context.Background(), roleTestCaller, roleTestTeamID, roleTestOwnerID)

	assert.ErrorIs(t, err, ErrCannotRemoveTeamOwner)
	memberRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
}

// TestTeamService_RemoveTeamMember_AdminMayRemoveMember is the other half of the
// behavior change: admins gained member removal.
func TestTeamService_RemoveTeamMember_AdminMayRemoveMember(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)

	teamOwnedBy(teamRepo, roleTestOwnerID)
	callerWithRole(memberRepo, models.TeamMemberRoleAdmin)
	memberRepo.EXPECT().Delete(mock.Anything, roleTestTeamID, roleTestTarget).Return(nil).Once()

	svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
	err := svc.RemoveTeamMember(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

	assert.NoError(t, err)
}
