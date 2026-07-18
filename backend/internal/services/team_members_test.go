package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// This file covers the member-management branches of TeamService that
// team_roles_test.go / team_test.go leave untested: repository-error arms and
// the GetTeamMembers pagination/detail-fetch semantics. Role-policy outcomes
// (who may do what) live in team_roles_test.go and the matrix itself in
// internal/authz — they are deliberately not re-tested here.

func TestTeamService_UpdateMemberRole_RepoErrors(t *testing.T) {
	t.Run("role update database failure is wrapped", func(t *testing.T) {
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)

		teamOwnedBy(teamRepo, roleTestOwnerID)
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)
		memberRepo.EXPECT().UpdateRole(mock.Anything, roleTestTeamID, roleTestTarget, models.TeamMemberRoleAdmin).
			Return(stderrors.New("connection reset")).Once()

		svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
		detail, err := svc.UpdateMemberRole(
			context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget, models.TeamMemberRoleAdmin,
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update team member role")
		// A database failure must never masquerade as "member not found".
		assert.NotErrorIs(t, err, repositories.ErrTeamMemberNotFound)
		assert.Nil(t, detail)
	})

	t.Run("reload of updated member fails", func(t *testing.T) {
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)

		teamOwnedBy(teamRepo, roleTestOwnerID)
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)
		memberRepo.EXPECT().UpdateRole(mock.Anything, roleTestTeamID, roleTestTarget, models.TeamMemberRoleAdmin).
			Return(nil).Once()
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestTarget).
			Return(nil, stderrors.New("connection reset")).Once()

		svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
		detail, err := svc.UpdateMemberRole(
			context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget, models.TeamMemberRoleAdmin,
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load updated team member")
		assert.Nil(t, detail)
	})

	t.Run("load of updated member's user fails", func(t *testing.T) {
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)
		userRepo := mocks.NewMockUserRepository(t)

		teamOwnedBy(teamRepo, roleTestOwnerID)
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)
		memberRepo.EXPECT().UpdateRole(mock.Anything, roleTestTeamID, roleTestTarget, models.TeamMemberRoleAdmin).
			Return(nil).Once()
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestTarget).
			Return(&models.TeamMember{
				TeamID: roleTestTeamID,
				UserID: roleTestTarget,
				Role:   models.TeamMemberRoleAdmin,
			}, nil).Once()
		userRepo.EXPECT().GetByID(mock.Anything, roleTestTarget).
			Return(nil, stderrors.New("connection reset")).Once()

		svc := createTestTeamService(teamRepo, memberRepo, userRepo)
		detail, err := svc.UpdateMemberRole(
			context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget, models.TeamMemberRoleAdmin,
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load updated team member's user")
		assert.Nil(t, detail)
	})
}

func TestTeamService_TransferOwnership_RepoErrors(t *testing.T) {
	t.Run("membership verification database failure is wrapped", func(t *testing.T) {
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)

		teamOwnedBy(teamRepo, roleTestOwnerID)
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestTarget).
			Return(nil, stderrors.New("connection reset")).Once()

		svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
		team, err := svc.TransferOwnership(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to verify new owner membership")
		assert.Nil(t, team)
		teamRepo.AssertNotCalled(t, "TransferOwnership", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	// The transactional repo call can itself report the sentinel not-found
	// errors; both pass through unwrapped so the handler can map them to 404.
	for _, sentinel := range []error{repositories.ErrTeamNotFound, repositories.ErrTeamMemberNotFound} {
		t.Run(fmt.Sprintf("repo sentinel %v passes through", sentinel), func(t *testing.T) {
			teamRepo := mocks.NewMockTeamRepository(t)
			memberRepo := mocks.NewMockTeamMemberRepository(t)

			teamOwnedBy(teamRepo, roleTestOwnerID)
			callerWithRole(memberRepo, models.TeamMemberRoleOwner)
			memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestTarget).
				Return(&models.TeamMember{TeamID: roleTestTeamID, UserID: roleTestTarget}, nil).Once()
			teamRepo.EXPECT().TransferOwnership(mock.Anything, roleTestTeamID, roleTestOwnerID, roleTestTarget).
				Return(sentinel).Once()

			svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
			team, err := svc.TransferOwnership(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

			assert.ErrorIs(t, err, sentinel)
			assert.Nil(t, team)
		})
	}

	t.Run("generic transfer failure is wrapped", func(t *testing.T) {
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)

		teamOwnedBy(teamRepo, roleTestOwnerID)
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestTarget).
			Return(&models.TeamMember{TeamID: roleTestTeamID, UserID: roleTestTarget}, nil).Once()
		teamRepo.EXPECT().TransferOwnership(mock.Anything, roleTestTeamID, roleTestOwnerID, roleTestTarget).
			Return(stderrors.New("transaction aborted")).Once()

		svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
		team, err := svc.TransferOwnership(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to transfer team ownership")
		assert.Nil(t, team)
	})
}

func TestTeamService_RemoveTeamMember_DeleteRepoError(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)

	teamOwnedBy(teamRepo, roleTestOwnerID)
	callerWithRole(memberRepo, models.TeamMemberRoleOwner)
	memberRepo.EXPECT().Delete(mock.Anything, roleTestTeamID, roleTestTarget).
		Return(stderrors.New("connection reset")).Once()

	svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
	err := svc.RemoveTeamMember(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove team member")
}

// TestTeamService_RemoveTeamMember_CommentCascade pins the semantics of the
// app-level comment cascade: it is best-effort, so a cascade failure is
// TOLERATED — the member removal already happened and still reports success.
func TestTeamService_RemoveTeamMember_CommentCascade(t *testing.T) {
	newSvc := func(t *testing.T, cascadeErr error) (*TeamService, *mocks.MockCommentRepository) {
		t.Helper()
		teamRepo := mocks.NewMockTeamRepository(t)
		memberRepo := mocks.NewMockTeamMemberRepository(t)
		commentRepo := mocks.NewMockCommentRepository(t)

		teamOwnedBy(teamRepo, roleTestOwnerID)
		callerWithRole(memberRepo, models.TeamMemberRoleOwner)
		memberRepo.EXPECT().Delete(mock.Anything, roleTestTeamID, roleTestTarget).Return(nil).Once()
		commentRepo.EXPECT().DeleteByUser(mock.Anything, roleTestTeamID, roleTestTarget).
			Return(int64(0), cascadeErr).Once()

		logger, _ := logtest.New()
		return NewTeamService(
			teamRepo, memberRepo, mocks.NewMockUserRepository(t),
			NewAuthorizationService(memberRepo, logger), logger, commentRepo,
		), commentRepo
	}

	t.Run("cascade deletes the departing member's comments", func(t *testing.T) {
		svc, commentRepo := newSvc(t, nil)

		err := svc.RemoveTeamMember(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

		require.NoError(t, err)
		commentRepo.AssertCalled(t, "DeleteByUser", mock.Anything, roleTestTeamID, roleTestTarget)
	})

	t.Run("cascade failure is tolerated after successful removal", func(t *testing.T) {
		svc, _ := newSvc(t, stderrors.New("comments table unavailable"))

		err := svc.RemoveTeamMember(context.Background(), roleTestCaller, roleTestTeamID, roleTestTarget)

		require.NoError(t, err, "comment-cascade failure must not fail the completed removal")
	})
}

// membersPageCase drives one GetTeamMembers pagination/detail scenario.
type membersPageCase struct {
	name          string
	callerIsOwner bool
	memberIDs     []string
	failUserID    string // this member's user lookup fails
	page          int
	pageSize      int
	wantIDs       []string
	wantTotal     int
}

func membersPageCases() []membersPageCase {
	return []membersPageCase{
		{
			name:          "owner gets full first page with invitation status",
			callerIsOwner: true,
			memberIDs:     []string{"m1", "m2", "m3"},
			page:          1, pageSize: 2,
			wantIDs:   []string{"m1", "m2"},
			wantTotal: 3,
		},
		{
			name:      "non-owner member gets page without invitation status",
			memberIDs: []string{"m1", "m2"},
			page:      1, pageSize: 10,
			wantIDs:   []string{"m1", "m2"},
			wantTotal: 2,
		},
		{
			name:      "partial last page",
			memberIDs: []string{"m1", "m2", "m3"},
			page:      2, pageSize: 2,
			wantIDs:   []string{"m3"},
			wantTotal: 3,
		},
		{
			name:      "page starting exactly at the end is empty",
			memberIDs: []string{"m1", "m2", "m3"},
			page:      2, pageSize: 3,
			wantIDs:   []string{},
			wantTotal: 3,
		},
		{
			name:      "page past the end is empty",
			memberIDs: []string{"m1", "m2", "m3"},
			page:      3, pageSize: 2,
			wantIDs:   []string{},
			wantTotal: 3,
		},
		{
			name:      "team with no member rows yields an empty page",
			memberIDs: []string{},
			page:      1, pageSize: 10,
			wantIDs:   []string{},
			wantTotal: 0,
		},
		{
			// Pinned semantics: a member whose user record cannot be loaded is
			// silently OMITTED — from the page AND from TotalCount, which counts
			// resolved details, not team_members rows.
			name:       "member with failing user lookup is omitted from page and total",
			memberIDs:  []string{"m1", "m2", "m3"},
			failUserID: "m2",
			page:       1, pageSize: 10,
			wantIDs:   []string{"m1", "m3"},
			wantTotal: 2,
		},
	}
}

func TestTeamService_GetTeamMembers_Pagination(t *testing.T) {
	for _, tc := range membersPageCases() {
		t.Run(tc.name, func(t *testing.T) {
			teamRepo := mocks.NewMockTeamRepository(t)
			memberRepo := mocks.NewMockTeamMemberRepository(t)
			userRepo := mocks.NewMockUserRepository(t)

			ownerID := roleTestOwnerID
			if tc.callerIsOwner {
				ownerID = roleTestCaller
			}
			teamOwnedBy(teamRepo, ownerID)
			callerWithRole(memberRepo, models.TeamMemberRoleMember)

			members := make([]models.TeamMember, 0, len(tc.memberIDs))
			for _, id := range tc.memberIDs {
				members = append(members, models.TeamMember{
					TeamID:    roleTestTeamID,
					UserID:    id,
					Role:      models.TeamMemberRoleMember,
					CreatedAt: time.Unix(0, 0).UTC(),
				})
				if id == tc.failUserID {
					userRepo.EXPECT().GetByID(mock.Anything, id).
						Return(nil, stderrors.New("connection reset")).Once()
				} else {
					userRepo.EXPECT().GetByID(mock.Anything, id).
						Return(&models.User{ID: id, Email: id + "@example.com", Name: "User " + id}, nil).Once()
				}
			}
			memberRepo.EXPECT().GetByTeamID(mock.Anything, roleTestTeamID).Return(members, nil).Once()

			svc := createTestTeamService(teamRepo, memberRepo, userRepo)
			resp, err := svc.GetTeamMembers(context.Background(), roleTestCaller, roleTestTeamID, tc.page, tc.pageSize)

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tc.wantTotal, resp.TotalCount)
			assert.Equal(t, tc.page, resp.Page)
			assert.Equal(t, tc.pageSize, resp.PageSize)

			gotIDs := make([]string, 0, len(resp.Members))
			for _, d := range resp.Members {
				gotIDs = append(gotIDs, d.UserID)
			}
			assert.Equal(t, tc.wantIDs, gotIDs)

			for _, d := range resp.Members {
				assert.Equal(t, d.UserID+"@example.com", d.Email)
				assert.Equal(t, "User "+d.UserID, d.Name)
				assert.Equal(t, string(models.TeamMemberRoleMember), d.Role)
				assert.Equal(t, "1970-01-01T00:00:00Z", d.JoinedAt)
				if tc.callerIsOwner {
					// Only the owner's view carries invitation status; every row
					// present in team_members reads "accepted".
					require.NotNil(t, d.InvitationStatus)
					assert.Equal(t, "accepted", *d.InvitationStatus)
				} else {
					assert.Nil(t, d.InvitationStatus)
				}
			}
		})
	}
}

func TestTeamService_GetTeamMembers_MembersRepoError(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)

	teamOwnedBy(teamRepo, roleTestOwnerID)
	callerWithRole(memberRepo, models.TeamMemberRoleMember)
	memberRepo.EXPECT().GetByTeamID(mock.Anything, roleTestTeamID).
		Return(nil, stderrors.New("connection reset")).Once()

	svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
	resp, err := svc.GetTeamMembers(context.Background(), roleTestCaller, roleTestTeamID, 1, 10)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get team members")
	assert.Nil(t, resp)
}

func TestTeamService_GetTeamMembers_CallerNotAMember(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	memberRepo := mocks.NewMockTeamMemberRepository(t)

	teamOwnedBy(teamRepo, roleTestOwnerID)
	memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, roleTestTeamID, roleTestCaller).
		Return(nil, repositories.ErrTeamMemberNotFound).Once()

	svc := createTestTeamService(teamRepo, memberRepo, mocks.NewMockUserRepository(t))
	resp, err := svc.GetTeamMembers(context.Background(), roleTestCaller, roleTestTeamID, 1, 10)

	require.Error(t, err)
	// GetTeam deliberately reports "team not found" for a non-member so the
	// team's existence is not leaked.
	assert.Contains(t, err.Error(), "team not found")
	assert.Nil(t, resp)
	memberRepo.AssertNotCalled(t, "GetByTeamID", mock.Anything, mock.Anything)
}
