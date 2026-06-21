package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// newTestInvitationService wires up a TeamInvitationService backed by fresh mocks for
// GetInvitationByToken-focused tests. Returning all mocks lets each test stub only
// what it needs and assert expectations explicitly.
func newTestInvitationService() (
	*TeamInvitationService,
	*mocks.MockTeamInvitationRepository,
	*mocks.MockTeamRepository,
	*mocks.MockUserRepository,
) {
	mockInvitationRepo := &mocks.MockTeamInvitationRepository{}
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}
	mockEmailService := &MockEmailService{}

	svc := NewTeamInvitationService(
		mockInvitationRepo,
		mockTeamRepo,
		mockTeamMemberRepo,
		mockUserRepo,
		mockEmailService,
		&config.Config{},
		nil,
	)

	return svc, mockInvitationRepo, mockTeamRepo, mockUserRepo
}

func TestGetInvitationByToken_Success(t *testing.T) {
	svc, invRepo, teamRepo, userRepo := newTestInvitationService()
	ctx := context.Background()
	token := "valid-token"

	invitation := &models.TeamInvitation{
		ID:           "inv-1",
		TeamID:       "team-1",
		InviterID:    "user-1",
		InviteeEmail: "invitee@example.com",
		Role:         models.TeamMemberRoleMember,
		Token:        token,
		Status:       models.InvitationStatusPending,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		CreatedAt:    time.Now(),
	}
	invRepo.On("GetByToken", ctx, token).Return(invitation, nil)
	teamRepo.On("GetByID", ctx, "team-1").Return(&models.Team{ID: "team-1", Name: "Acme"}, nil)
	userRepo.On("GetByID", ctx, "user-1").Return(&models.User{
		ID: "user-1", Name: "Alice", Email: "alice@example.com",
	}, nil)

	details, err := svc.GetInvitationByToken(ctx, token)

	require.NoError(t, err)
	require.NotNil(t, details)
	assert.Equal(t, invitation, details.Invitation)
	assert.Equal(t, "Acme", details.TeamName)
	require.NotNil(t, details.InvitedBy)
	assert.Equal(t, "user-1", details.InvitedBy.ID)
	assert.Equal(t, "Alice", details.InvitedBy.Name)
	assert.Equal(t, "alice@example.com", details.InvitedBy.Email)

	invRepo.AssertExpectations(t)
	teamRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestGetInvitationByToken_NotFound(t *testing.T) {
	svc, invRepo, teamRepo, userRepo := newTestInvitationService()
	ctx := context.Background()
	token := "missing-token"

	invRepo.On("GetByToken", ctx, token).
		Return((*models.TeamInvitation)(nil), repositories.ErrTeamInvitationNotFound)

	details, err := svc.GetInvitationByToken(ctx, token)

	require.Error(t, err)
	assert.Nil(t, details)
	var notFound *InvitationNotFoundError
	require.True(t, stderrors.As(err, &notFound))
	assert.Equal(t, token, notFound.Token)

	invRepo.AssertExpectations(t)
	// Team and user lookups must NOT happen if the invitation is missing.
	teamRepo.AssertNotCalled(t, "GetByID")
	userRepo.AssertNotCalled(t, "GetByID")
}

func TestGetInvitationByToken_RepoErrorPropagated(t *testing.T) {
	svc, invRepo, _, _ := newTestInvitationService()
	ctx := context.Background()
	token := "any-token"

	invRepo.On("GetByToken", ctx, token).
		Return((*models.TeamInvitation)(nil), fmt.Errorf("database connection refused"))

	details, err := svc.GetInvitationByToken(ctx, token)

	require.Error(t, err)
	assert.Nil(t, details)
	// Should NOT be classified as not-found.
	var notFound *InvitationNotFoundError
	assert.False(t, stderrors.As(err, &notFound))
	assert.Contains(t, err.Error(), "failed to get invitation by token")

	invRepo.AssertExpectations(t)
}

func TestGetInvitationByToken_Expired(t *testing.T) {
	svc, invRepo, teamRepo, userRepo := newTestInvitationService()
	ctx := context.Background()
	token := "expired-token"

	invRepo.On("GetByToken", ctx, token).Return(&models.TeamInvitation{
		ID:        "inv-2",
		TeamID:    "team-2",
		InviterID: "user-2",
		Status:    models.InvitationStatusPending,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}, nil)

	details, err := svc.GetInvitationByToken(ctx, token)

	require.Error(t, err)
	assert.Nil(t, details)
	var expired *InvitationExpiredError
	require.True(t, stderrors.As(err, &expired))
	assert.Equal(t, "inv-2", expired.ID)

	invRepo.AssertExpectations(t)
	teamRepo.AssertNotCalled(t, "GetByID")
	userRepo.AssertNotCalled(t, "GetByID")
}

func TestGetInvitationByToken_Revoked(t *testing.T) {
	svc, invRepo, teamRepo, userRepo := newTestInvitationService()
	ctx := context.Background()
	token := "revoked-token"

	invRepo.On("GetByToken", ctx, token).Return(&models.TeamInvitation{
		ID:        "inv-3",
		Status:    models.InvitationStatusRevoked,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil)

	details, err := svc.GetInvitationByToken(ctx, token)

	require.Error(t, err)
	assert.Nil(t, details)
	var stateErr *InvitationStateError
	require.True(t, stderrors.As(err, &stateErr))
	assert.Equal(t, "inv-3", stateErr.ID)
	assert.Equal(t, models.InvitationStatusRevoked, stateErr.Status)

	invRepo.AssertExpectations(t)
	teamRepo.AssertNotCalled(t, "GetByID")
	userRepo.AssertNotCalled(t, "GetByID")
}

func TestGetInvitationByToken_Accepted(t *testing.T) {
	svc, invRepo, _, _ := newTestInvitationService()
	ctx := context.Background()
	token := "accepted-token"

	invRepo.On("GetByToken", ctx, token).Return(&models.TeamInvitation{
		ID:     "inv-4",
		Status: models.InvitationStatusAccepted,
	}, nil)

	details, err := svc.GetInvitationByToken(ctx, token)

	require.Error(t, err)
	assert.Nil(t, details)
	var stateErr *InvitationStateError
	require.True(t, stderrors.As(err, &stateErr))
	assert.Equal(t, models.InvitationStatusAccepted, stateErr.Status)

	invRepo.AssertExpectations(t)
}

func TestGetInvitationByToken_Rejected(t *testing.T) {
	svc, invRepo, _, _ := newTestInvitationService()
	ctx := context.Background()
	token := "rejected-token"

	invRepo.On("GetByToken", ctx, token).Return(&models.TeamInvitation{
		ID:     "inv-5",
		Status: models.InvitationStatusRejected,
	}, nil)

	details, err := svc.GetInvitationByToken(ctx, token)

	require.Error(t, err)
	assert.Nil(t, details)
	var stateErr *InvitationStateError
	require.True(t, stderrors.As(err, &stateErr))
	assert.Equal(t, models.InvitationStatusRejected, stateErr.Status)

	invRepo.AssertExpectations(t)
}

func TestGetInvitationByToken_TeamFetchFails(t *testing.T) {
	svc, invRepo, teamRepo, _ := newTestInvitationService()
	ctx := context.Background()
	token := "valid-token"

	invRepo.On("GetByToken", ctx, token).Return(&models.TeamInvitation{
		ID:        "inv-6",
		TeamID:    "team-6",
		InviterID: "user-6",
		Status:    models.InvitationStatusPending,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil)
	teamRepo.On("GetByID", ctx, "team-6").
		Return((*models.Team)(nil), fmt.Errorf("db error"))

	details, err := svc.GetInvitationByToken(ctx, token)

	require.Error(t, err)
	assert.Nil(t, details)
	assert.Contains(t, err.Error(), "failed to get team for invitation")
}

func TestGetInvitationByToken_InviterFetchFails(t *testing.T) {
	svc, invRepo, teamRepo, userRepo := newTestInvitationService()
	ctx := context.Background()
	token := "valid-token"

	invitation := &models.TeamInvitation{
		ID:           "inv-7",
		TeamID:       "team-7",
		InviterID:    "user-7",
		InviteeEmail: "invitee@example.com",
		Role:         models.TeamMemberRoleAdmin,
		Token:        token,
		Status:       models.InvitationStatusPending,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	invRepo.On("GetByToken", ctx, token).Return(invitation, nil)
	teamRepo.On("GetByID", ctx, "team-7").Return(&models.Team{ID: "team-7", Name: "Beta"}, nil)
	userRepo.On("GetByID", ctx, "user-7").
		Return((*models.User)(nil), fmt.Errorf("user lookup failed"))

	details, err := svc.GetInvitationByToken(ctx, token)

	require.NoError(t, err)
	require.NotNil(t, details)
	assert.Equal(t, "Beta", details.TeamName)
	assert.Nil(t, details.InvitedBy, "InvitedBy should be nil when inviter lookup fails (best-effort)")

	invRepo.AssertExpectations(t)
	teamRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestInvitationStateError_Message(t *testing.T) {
	tests := []struct {
		name   string
		status models.InvitationStatus
		want   string
	}{
		{"accepted", models.InvitationStatusAccepted, "invitation has already been accepted"},
		{"rejected", models.InvitationStatusRejected, "invitation has been rejected"},
		{"revoked", models.InvitationStatusRevoked, "invitation has been revoked"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := NewInvitationStateError("inv-x", tc.status)
			assert.Equal(t, tc.want, err.Error())
		})
	}
}
