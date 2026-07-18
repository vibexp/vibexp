package services

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Fixtures shared by the Accept/Reject/Revoke/Get tests below. Tokens are plain
// strings: the service treats them as opaque lookup keys.
const (
	invActionToken  = "invitation-token-abc"
	invActionInvID  = "inv-100"
	invActionTeamID = "team-100"
	invActionUserID = "user-100"
	invActionEmail  = "invitee@example.com"
)

// invitationActionMocks bundles every mock the invitation-action tests touch.
type invitationActionMocks struct {
	invitationRepo *mocks.MockTeamInvitationRepository
	teamRepo       *mocks.MockTeamRepository
	teamMemberRepo *mocks.MockTeamMemberRepository
	userRepo       *mocks.MockUserRepository
}

// newInvitationActionMocks wires a TeamInvitationService to fresh mocks. The
// authz dependency is the real AuthorizationService fed by the member-repo mock,
// so tests stub the caller's membership row to choose an authz outcome — they do
// not re-test the permission matrix itself (internal/authz owns that).
func newInvitationActionMocks() (*TeamInvitationService, *invitationActionMocks) {
	m := &invitationActionMocks{
		invitationRepo: &mocks.MockTeamInvitationRepository{},
		teamRepo:       &mocks.MockTeamRepository{},
		teamMemberRepo: &mocks.MockTeamMemberRepository{},
		userRepo:       &mocks.MockUserRepository{},
	}
	svc := NewTeamInvitationService(TeamInvitationServiceDeps{
		InvitationRepo: m.invitationRepo,
		TeamRepo:       m.teamRepo,
		TeamMemberRepo: m.teamMemberRepo,
		UserRepo:       m.userRepo,
		EmailService:   &MockEmailService{},
		Authz:          NewAuthorizationService(m.teamMemberRepo, nil),
		Cfg:            &config.Config{},
		Logger:         nil,
	})
	return svc, m
}

// pendingInvitationFixture returns a pending, unexpired invitation carrying an
// admin role — deliberately not the default member role, so the tests can prove
// the role is copied from the invitation rather than defaulted.
func pendingInvitationFixture() *models.TeamInvitation {
	return &models.TeamInvitation{
		ID:           invActionInvID,
		TeamID:       invActionTeamID,
		InviterID:    "user-inviter",
		InviteeEmail: invActionEmail,
		Role:         models.TeamMemberRoleAdmin,
		Token:        invActionToken,
		Status:       models.InvitationStatusPending,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		CreatedAt:    time.Now().Add(-time.Hour),
		UpdatedAt:    time.Now().Add(-time.Hour),
	}
}

// acceptInvitationCase drives one AcceptInvitation scenario. All cases share
// the same wiring; the flags say how far execution must get.
type acceptInvitationCase struct {
	name              string
	mutate            func(*models.TeamInvitation)
	userEmail         string
	alreadyMember     bool
	createErr         error
	updateErr         error
	wantErrContains   string
	wantMemberCreated bool
	wantStatusUpdated bool
}

func acceptInvitationCases() []acceptInvitationCase {
	return []acceptInvitationCase{
		{
			name:              "success with exact email match",
			userEmail:         invActionEmail,
			wantMemberCreated: true,
			wantStatusUpdated: true,
		},
		{
			// The comparison is strings.EqualFold: a case-different email is the
			// SAME mailbox and must be accepted, not rejected.
			name:              "success with case-different email",
			userEmail:         "INVITEE@Example.COM",
			wantMemberCreated: true,
			wantStatusUpdated: true,
		},
		{
			name:            "wrong email is rejected",
			userEmail:       "someone-else@example.com",
			wantErrContains: "different email address",
		},
		{
			name:            "already accepted invitation cannot be accepted again",
			mutate:          func(i *models.TeamInvitation) { i.Status = models.InvitationStatusAccepted },
			userEmail:       invActionEmail,
			wantErrContains: "invitation is not pending",
		},
		{
			name:            "revoked invitation cannot be accepted",
			mutate:          func(i *models.TeamInvitation) { i.Status = models.InvitationStatusRevoked },
			userEmail:       invActionEmail,
			wantErrContains: "invitation is not pending",
		},
		{
			name:            "expired invitation cannot be accepted",
			mutate:          func(i *models.TeamInvitation) { i.ExpiresAt = time.Now().Add(-time.Minute) },
			userEmail:       invActionEmail,
			wantErrContains: "invitation has expired",
		},
		{
			name:            "existing team member cannot accept again",
			userEmail:       invActionEmail,
			alreadyMember:   true,
			wantErrContains: "already a member of this team",
		},
		{
			name:              "member creation failure aborts the accept",
			userEmail:         invActionEmail,
			createErr:         stderrors.New("insert failed"),
			wantErrContains:   "failed to create team member",
			wantMemberCreated: true,
		},
		{
			// Pinned semantics: a failed status update AFTER the member row was
			// created PROPAGATES — the caller sees an error even though the
			// membership already exists (there is no rollback in the service).
			name:              "status update failure after member creation propagates",
			userEmail:         invActionEmail,
			updateErr:         stderrors.New("update failed"),
			wantErrContains:   "failed to update invitation status",
			wantMemberCreated: true,
			wantStatusUpdated: true,
		},
	}
}

func TestTeamInvitationService_AcceptInvitation(t *testing.T) {
	for _, tc := range acceptInvitationCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc, m := newInvitationActionMocks()
			invitation := pendingInvitationFixture()
			if tc.mutate != nil {
				tc.mutate(invitation)
			}

			m.invitationRepo.On("GetByToken", mock.Anything, invActionToken).Return(invitation, nil)
			m.userRepo.On("GetByID", mock.Anything, invActionUserID).
				Return(&models.User{ID: invActionUserID, Email: tc.userEmail}, nil)
			if tc.alreadyMember {
				m.teamMemberRepo.On("GetByTeamAndUser", mock.Anything, invActionTeamID, invActionUserID).
					Return(&models.TeamMember{TeamID: invActionTeamID, UserID: invActionUserID}, nil)
			} else {
				m.teamMemberRepo.On("GetByTeamAndUser", mock.Anything, invActionTeamID, invActionUserID).
					Return((*models.TeamMember)(nil), repositories.ErrTeamMemberNotFound)
			}
			// The role on the new member row must be the role from the
			// invitation (admin here), never a default.
			m.teamMemberRepo.On("Create", mock.Anything, mock.MatchedBy(func(member *models.TeamMember) bool {
				return member.TeamID == invActionTeamID &&
					member.UserID == invActionUserID &&
					member.Role == invitation.Role &&
					!member.CreatedAt.IsZero() &&
					!member.UpdatedAt.IsZero()
			})).Return(tc.createErr)
			m.invitationRepo.On("UpdateStatus", mock.Anything, invActionInvID, models.InvitationStatusAccepted).
				Return(tc.updateErr)

			teamID, err := svc.AcceptInvitation(context.Background(), invActionToken, invActionUserID)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				assert.Empty(t, teamID)
			} else {
				require.NoError(t, err)
				assert.Equal(t, invActionTeamID, teamID)
			}

			if tc.wantMemberCreated {
				m.teamMemberRepo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
			} else {
				m.teamMemberRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			}
			if tc.wantStatusUpdated {
				m.invitationRepo.AssertCalled(t, "UpdateStatus", mock.Anything, invActionInvID,
					models.InvitationStatusAccepted)
			} else {
				m.invitationRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
			}
		})
	}
}

func TestTeamInvitationService_AcceptInvitation_TokenLookupFails(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetByToken", mock.Anything, "missing-token").
		Return((*models.TeamInvitation)(nil), repositories.ErrTeamInvitationNotFound)

	teamID, err := svc.AcceptInvitation(context.Background(), "missing-token", invActionUserID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid invitation token")
	assert.Empty(t, teamID)
	m.userRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
	m.teamMemberRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestTeamInvitationService_AcceptInvitation_UserLookupFails(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetByToken", mock.Anything, invActionToken).Return(pendingInvitationFixture(), nil)
	m.userRepo.On("GetByID", mock.Anything, invActionUserID).
		Return((*models.User)(nil), stderrors.New("database unavailable"))

	teamID, err := svc.AcceptInvitation(context.Background(), invActionToken, invActionUserID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get user")
	assert.Empty(t, teamID)
	m.teamMemberRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	m.invitationRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
}

// rejectInvitationCase drives one RejectInvitation scenario.
type rejectInvitationCase struct {
	name              string
	mutate            func(*models.TeamInvitation)
	userEmail         string
	updateErr         error
	wantErrContains   string
	wantStatusUpdated bool
}

func rejectInvitationCases() []rejectInvitationCase {
	return []rejectInvitationCase{
		{
			name:              "success with exact email match",
			userEmail:         invActionEmail,
			wantStatusUpdated: true,
		},
		{
			name:              "success with case-different email",
			userEmail:         "Invitee@EXAMPLE.com",
			wantStatusUpdated: true,
		},
		{
			// Pinned semantics: RejectInvitation checks pending-ness but NOT
			// expiry, so an expired-but-still-pending invitation can be
			// rejected — declining a dead invitation is harmless.
			name:              "expired but pending invitation can still be rejected",
			mutate:            func(i *models.TeamInvitation) { i.ExpiresAt = time.Now().Add(-time.Hour) },
			userEmail:         invActionEmail,
			wantStatusUpdated: true,
		},
		{
			name:            "wrong email is rejected",
			userEmail:       "someone-else@example.com",
			wantErrContains: "not authorized to reject",
		},
		{
			name:            "already accepted invitation cannot be rejected",
			mutate:          func(i *models.TeamInvitation) { i.Status = models.InvitationStatusAccepted },
			userEmail:       invActionEmail,
			wantErrContains: "invitation is not pending",
		},
		{
			name:            "revoked invitation cannot be rejected",
			mutate:          func(i *models.TeamInvitation) { i.Status = models.InvitationStatusRevoked },
			userEmail:       invActionEmail,
			wantErrContains: "invitation is not pending",
		},
		{
			name:              "status update failure propagates",
			userEmail:         invActionEmail,
			updateErr:         stderrors.New("update failed"),
			wantErrContains:   "failed to update invitation status",
			wantStatusUpdated: true,
		},
	}
}

func TestTeamInvitationService_RejectInvitation(t *testing.T) {
	for _, tc := range rejectInvitationCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc, m := newInvitationActionMocks()
			invitation := pendingInvitationFixture()
			if tc.mutate != nil {
				tc.mutate(invitation)
			}

			m.invitationRepo.On("GetByToken", mock.Anything, invActionToken).Return(invitation, nil)
			m.userRepo.On("GetByID", mock.Anything, invActionUserID).
				Return(&models.User{ID: invActionUserID, Email: tc.userEmail}, nil)
			m.invitationRepo.On("UpdateStatus", mock.Anything, invActionInvID, models.InvitationStatusRejected).
				Return(tc.updateErr)

			err := svc.RejectInvitation(context.Background(), invActionToken, invActionUserID)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.wantStatusUpdated {
				m.invitationRepo.AssertCalled(t, "UpdateStatus", mock.Anything, invActionInvID,
					models.InvitationStatusRejected)
			} else {
				m.invitationRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
			}
		})
	}
}

func TestTeamInvitationService_RejectInvitation_TokenLookupFails(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetByToken", mock.Anything, "missing-token").
		Return((*models.TeamInvitation)(nil), repositories.ErrTeamInvitationNotFound)

	err := svc.RejectInvitation(context.Background(), "missing-token", invActionUserID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid invitation token")
	m.invitationRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
}

func TestTeamInvitationService_RejectInvitation_UserLookupFails(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetByToken", mock.Anything, invActionToken).Return(pendingInvitationFixture(), nil)
	m.userRepo.On("GetByID", mock.Anything, invActionUserID).
		Return((*models.User)(nil), stderrors.New("database unavailable"))

	err := svc.RejectInvitation(context.Background(), invActionToken, invActionUserID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get user")
	m.invitationRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
}

// stubRevokerRole stubs the caller's membership row that the authz check
// resolves during RevokeInvitation / GetTeamInvitations.
func stubRevokerRole(m *invitationActionMocks, role models.TeamMemberRole) {
	m.teamMemberRepo.On("GetByTeamAndUser", mock.Anything, invActionTeamID, invActionUserID).
		Return(&models.TeamMember{TeamID: invActionTeamID, UserID: invActionUserID, Role: role}, nil)
}

func TestTeamInvitationService_RevokeInvitation_Success(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetByID", mock.Anything, invActionInvID).Return(pendingInvitationFixture(), nil)
	stubRevokerRole(m, models.TeamMemberRoleOwner)
	m.invitationRepo.On("UpdateStatus", mock.Anything, invActionInvID, models.InvitationStatusRevoked).
		Return(nil)

	err := svc.RevokeInvitation(context.Background(), invActionUserID, invActionInvID)

	require.NoError(t, err)
	m.invitationRepo.AssertCalled(t, "UpdateStatus", mock.Anything, invActionInvID, models.InvitationStatusRevoked)
}

func TestTeamInvitationService_RevokeInvitation_PermissionDenied(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetByID", mock.Anything, invActionInvID).Return(pendingInvitationFixture(), nil)
	// A plain member lacks authz.MemberInvite; the matrix decides, this test
	// only stubs the membership row that feeds it.
	stubRevokerRole(m, models.TeamMemberRoleMember)

	err := svc.RevokeInvitation(context.Background(), invActionUserID, invActionInvID)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPermissionDenied)
	m.invitationRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
}

func TestTeamInvitationService_RevokeInvitation_NotFound(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetByID", mock.Anything, "missing-inv").
		Return((*models.TeamInvitation)(nil), repositories.ErrTeamInvitationNotFound)

	err := svc.RevokeInvitation(context.Background(), invActionUserID, "missing-inv")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invitation not found")
	m.invitationRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
	m.teamMemberRepo.AssertNotCalled(t, "GetByTeamAndUser", mock.Anything, mock.Anything, mock.Anything)
}

func TestTeamInvitationService_RevokeInvitation_UpdateStatusFails(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetByID", mock.Anything, invActionInvID).Return(pendingInvitationFixture(), nil)
	stubRevokerRole(m, models.TeamMemberRoleAdmin)
	m.invitationRepo.On("UpdateStatus", mock.Anything, invActionInvID, models.InvitationStatusRevoked).
		Return(stderrors.New("update failed"))

	err := svc.RevokeInvitation(context.Background(), invActionUserID, invActionInvID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to revoke invitation")
}

func TestTeamInvitationService_GetTeamInvitations_Success(t *testing.T) {
	svc, m := newInvitationActionMocks()
	stubRevokerRole(m, models.TeamMemberRoleAdmin)
	stored := []models.TeamInvitation{
		{ID: "inv-1", TeamID: invActionTeamID, Status: models.InvitationStatusPending},
		{ID: "inv-2", TeamID: invActionTeamID, Status: models.InvitationStatusAccepted},
	}
	m.invitationRepo.On("GetByTeamID", mock.Anything, invActionTeamID).Return(stored, nil)

	invitations, err := svc.GetTeamInvitations(context.Background(), invActionUserID, invActionTeamID)

	require.NoError(t, err)
	// Returned as-is: no filtering by status — the team view shows all of them.
	assert.Equal(t, stored, invitations)
}

func TestTeamInvitationService_GetTeamInvitations_PermissionDenied(t *testing.T) {
	svc, m := newInvitationActionMocks()
	// A non-member resolves to a denial, and the invitation list must never be
	// fetched.
	m.teamMemberRepo.On("GetByTeamAndUser", mock.Anything, invActionTeamID, invActionUserID).
		Return((*models.TeamMember)(nil), repositories.ErrTeamMemberNotFound)

	invitations, err := svc.GetTeamInvitations(context.Background(), invActionUserID, invActionTeamID)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPermissionDenied)
	assert.Nil(t, invitations)
	m.invitationRepo.AssertNotCalled(t, "GetByTeamID", mock.Anything, mock.Anything)
}

func TestTeamInvitationService_GetTeamInvitations_RepoError(t *testing.T) {
	svc, m := newInvitationActionMocks()
	stubRevokerRole(m, models.TeamMemberRoleOwner)
	m.invitationRepo.On("GetByTeamID", mock.Anything, invActionTeamID).
		Return([]models.TeamInvitation(nil), stderrors.New("database unavailable"))

	invitations, err := svc.GetTeamInvitations(context.Background(), invActionUserID, invActionTeamID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get team invitations")
	assert.Nil(t, invitations)
}

func TestTeamInvitationService_GetPendingInvitations_Success(t *testing.T) {
	svc, m := newInvitationActionMocks()
	// Pinned semantics: the service does NO pending-filtering of its own — the
	// repository query (GetPendingByEmail) owns that contract. The service only
	// converts values to pointers, preserving order.
	stored := []models.TeamInvitation{
		{ID: "inv-1", TeamID: "team-a", InviteeEmail: invActionEmail, Status: models.InvitationStatusPending},
		{ID: "inv-2", TeamID: "team-b", InviteeEmail: invActionEmail, Status: models.InvitationStatusPending},
	}
	m.invitationRepo.On("GetPendingByEmail", mock.Anything, invActionEmail).Return(stored, nil)

	invitations, err := svc.GetPendingInvitations(context.Background(), invActionEmail)

	require.NoError(t, err)
	require.Len(t, invitations, 2)
	assert.Equal(t, "inv-1", invitations[0].ID)
	assert.Equal(t, "inv-2", invitations[1].ID)
	assert.Equal(t, stored[0], *invitations[0])
	assert.Equal(t, stored[1], *invitations[1])
}

func TestTeamInvitationService_GetPendingInvitations_Empty(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetPendingByEmail", mock.Anything, invActionEmail).
		Return([]models.TeamInvitation{}, nil)

	invitations, err := svc.GetPendingInvitations(context.Background(), invActionEmail)

	require.NoError(t, err)
	assert.Empty(t, invitations)
}

func TestTeamInvitationService_GetPendingInvitations_RepoError(t *testing.T) {
	svc, m := newInvitationActionMocks()
	m.invitationRepo.On("GetPendingByEmail", mock.Anything, invActionEmail).
		Return([]models.TeamInvitation(nil), stderrors.New("database unavailable"))

	invitations, err := svc.GetPendingInvitations(context.Background(), invActionEmail)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get pending invitations")
	assert.Nil(t, invitations)
}
