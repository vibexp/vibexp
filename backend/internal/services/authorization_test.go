package services

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	testAuthzUserID = "user-123"
	testAuthzTeamID = "team-456"
)

func createTestAuthorizationService(teamMemberRepo *mocks.MockTeamMemberRepository) *AuthorizationService {
	logger, _ := logtest.New()
	return NewAuthorizationService(teamMemberRepo, logger)
}

// memberWithRole stubs GetByTeamAndUser to report the caller as a team member
// holding the given role.
func memberWithRole(repo *mocks.MockTeamMemberRepository, role models.TeamMemberRole) {
	repo.EXPECT().
		GetByTeamAndUser(mock.Anything, testAuthzTeamID, testAuthzUserID).
		Return(&models.TeamMember{
			TeamID: testAuthzTeamID,
			UserID: testAuthzUserID,
			Role:   role,
		}, nil).
		Once()
}

func TestAuthorizationService_Can_AllowedAndDenied(t *testing.T) {
	tests := []struct {
		name    string
		role    models.TeamMemberRole
		perm    authz.Permission
		allowed bool
	}{
		{name: "owner may delete the team", role: models.TeamMemberRoleOwner, perm: authz.TeamDelete, allowed: true},
		{name: "admin may not delete the team", role: models.TeamMemberRoleAdmin, perm: authz.TeamDelete, allowed: false},
		{name: "admin may update the team", role: models.TeamMemberRoleAdmin, perm: authz.TeamUpdate, allowed: true},
		{name: "member may not update the team", role: models.TeamMemberRoleMember, perm: authz.TeamUpdate, allowed: false},
		{name: "member may create resources", role: models.TeamMemberRoleMember, perm: authz.ResourceCreate, allowed: true},
		{name: "member may not create projects", role: models.TeamMemberRoleMember, perm: authz.ProjectCreate, allowed: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.MockTeamMemberRepository{}
			memberWithRole(repo, tc.role)
			svc := createTestAuthorizationService(repo)

			err := svc.Can(context.Background(), testAuthzUserID, testAuthzTeamID, tc.perm)

			if tc.allowed {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrPermissionDenied)
				// The permission must be named, so a 403 is diagnosable.
				assert.Contains(t, err.Error(), tc.perm.String())
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestAuthorizationService_Can_NonMemberIsDenied(t *testing.T) {
	repo := &mocks.MockTeamMemberRepository{}
	repo.EXPECT().
		GetByTeamAndUser(mock.Anything, testAuthzTeamID, testAuthzUserID).
		Return(nil, repositories.ErrTeamMemberNotFound).
		Once()
	svc := createTestAuthorizationService(repo)

	err := svc.Can(context.Background(), testAuthzUserID, testAuthzTeamID, authz.ResourceCreate)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertExpectations(t)
}

// TestAuthorizationService_Can_DatabaseErrorIsNotADenial pins the contract that
// an unreachable database surfaces as an error the handler maps to a 500 —
// never as a 403, which would silently mislead the caller.
func TestAuthorizationService_Can_DatabaseErrorIsNotADenial(t *testing.T) {
	dbErr := stderrors.New("connection refused")
	repo := &mocks.MockTeamMemberRepository{}
	repo.EXPECT().
		GetByTeamAndUser(mock.Anything, testAuthzTeamID, testAuthzUserID).
		Return(nil, dbErr).
		Once()
	svc := createTestAuthorizationService(repo)

	err := svc.Can(context.Background(), testAuthzUserID, testAuthzTeamID, authz.ResourceCreate)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrPermissionDenied)
	assert.ErrorIs(t, err, dbErr)
	repo.AssertExpectations(t)
}

func TestAuthorizationService_CanActOnResource(t *testing.T) {
	const otherUserID = "user-999"

	tests := []struct {
		name            string
		role            models.TeamMemberRole
		resourceOwnerID string
		allowed         bool
	}{
		{
			name:            "member deleting own resource checks the own permission",
			role:            models.TeamMemberRoleMember,
			resourceOwnerID: testAuthzUserID,
			allowed:         true,
		},
		{
			name:            "member deleting another's resource checks the any permission",
			role:            models.TeamMemberRoleMember,
			resourceOwnerID: otherUserID,
			allowed:         false,
		},
		{
			name:            "admin deleting another's resource checks the any permission",
			role:            models.TeamMemberRoleAdmin,
			resourceOwnerID: otherUserID,
			allowed:         true,
		},
		{
			name:            "owner deleting another's resource checks the any permission",
			role:            models.TeamMemberRoleOwner,
			resourceOwnerID: otherUserID,
			allowed:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.MockTeamMemberRepository{}
			memberWithRole(repo, tc.role)
			svc := createTestAuthorizationService(repo)

			err := svc.CanActOnResource(
				context.Background(),
				testAuthzUserID, testAuthzTeamID, tc.resourceOwnerID,
				authz.ResourceDeleteOwn, authz.ResourceDeleteAny,
			)

			if tc.allowed {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrPermissionDenied)
			}
			repo.AssertExpectations(t)
		})
	}
}

// TestAuthorizationService_CanActOnResource_NonMemberIsDenied covers the
// own-branch for a caller who is not in the team at all: owning the resource
// must not smuggle in access.
func TestAuthorizationService_CanActOnResource_NonMemberIsDenied(t *testing.T) {
	repo := &mocks.MockTeamMemberRepository{}
	repo.EXPECT().
		GetByTeamAndUser(mock.Anything, testAuthzTeamID, testAuthzUserID).
		Return(nil, repositories.ErrTeamMemberNotFound).
		Once()
	svc := createTestAuthorizationService(repo)

	err := svc.CanActOnResource(
		context.Background(),
		testAuthzUserID, testAuthzTeamID, testAuthzUserID,
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny,
	)

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertExpectations(t)
}

// TestNewAuthorizationService_NilLoggerDefaults mirrors the constructor
// contract shared by the other services.
func TestNewAuthorizationService_NilLoggerDefaults(t *testing.T) {
	svc := NewAuthorizationService(&mocks.MockTeamMemberRepository{}, nil)

	require.NotNil(t, svc)
	assert.NotNil(t, svc.logger)
}
