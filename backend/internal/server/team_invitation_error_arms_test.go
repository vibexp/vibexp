package server

// Error-arm coverage for team_invitation_handlers.go (issue #363, coverage
// epic #358): every failure branch of accept / reject / get-by-token / revoke /
// list gets a spec-validated response assertion, driven through the same real
// TeamInvitationService + mocked-repository wiring the happy-path conformance
// tests use (team_invitation_conformance_test.go). The #251 token-decode
// regression tests live in team_invitation_token_test.go and are not repeated
// here — this file only adds the one handler-level proof that a malformed
// percent-escape maps to a 400 response.

import (
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Fixture identifiers shared by the error-arm tests. The token matches
// pendingInvitation() (team_invitation_conformance_test.go) so the same
// fixture drives every arm.
const (
	errArmsToken  = "tok-abc-123"
	errArmsUserID = "user-1"
)

// inviteeUser is the user the fixture invitation was addressed to.
func inviteeUser() *models.User {
	return &models.User{ID: errArmsUserID, Email: "invitee@example.com", Name: "Invitee"}
}

// TestHandleAcceptInvitation_ErrorArms drives every failure branch of
// handleAcceptInvitationError through the real service so the string-matched
// service errors and their HTTP mappings cannot drift apart silently.
func TestHandleAcceptInvitation_ErrorArms(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(m *invConfMocks)
		wantStatus     int
		wantBodyPhrase string
	}{
		{
			name: "unknown token → 404",
			setup: func(m *invConfMocks) {
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).
					Return((*models.TeamInvitation)(nil), repositories.ErrTeamInvitationNotFound).Once()
			},
			wantStatus:     http.StatusNotFound,
			wantBodyPhrase: "Invalid invitation token",
		},
		{
			name: "wrong email → 403",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
				m.userRepo.On("GetByID", mock.Anything, errArmsUserID).
					Return(&models.User{ID: errArmsUserID, Email: "someone-else@example.com"}, nil).Once()
			},
			wantStatus:     http.StatusForbidden,
			wantBodyPhrase: "different email address",
		},
		{
			name: "already accepted (double accept) → 400",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				inv.Status = models.InvitationStatusAccepted
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
				m.userRepo.On("GetByID", mock.Anything, errArmsUserID).Return(inviteeUser(), nil).Once()
			},
			wantStatus:     http.StatusBadRequest,
			wantBodyPhrase: "not pending",
		},
		{
			name: "expired → 410",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				inv.ExpiresAt = time.Now().Add(-time.Hour)
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
				m.userRepo.On("GetByID", mock.Anything, errArmsUserID).Return(inviteeUser(), nil).Once()
			},
			wantStatus:     http.StatusGone,
			wantBodyPhrase: "expired",
		},
		{
			name: "already a member → 409",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
				m.userRepo.On("GetByID", mock.Anything, errArmsUserID).Return(inviteeUser(), nil).Once()
				m.memberRepo.On("GetByTeamAndUser", mock.Anything, confTeamID, errArmsUserID).
					Return(&models.TeamMember{TeamID: confTeamID, UserID: errArmsUserID}, nil).Once()
			},
			wantStatus:     http.StatusConflict,
			wantBodyPhrase: "already a member",
		},
		{
			name: "member write failure → 500",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
				m.userRepo.On("GetByID", mock.Anything, errArmsUserID).Return(inviteeUser(), nil).Once()
				m.memberRepo.On("GetByTeamAndUser", mock.Anything, confTeamID, errArmsUserID).
					Return((*models.TeamMember)(nil), repositories.ErrUserNotFound).Once()
				m.memberRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.TeamMember")).
					Return(stderrors.New("db write timeout")).Once()
			},
			wantStatus:     http.StatusInternalServerError,
			wantBodyPhrase: "Failed to accept invitation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newInvConfMocks(t)
			tc.setup(m)
			srv := newInvConfServer(m)

			req := createAuthenticatedRequest(
				"POST", "/api/v1/invitations/"+errArmsToken+"/accept", "", errArmsUserID)
			req = addURLParams(req, map[string]string{"token": errArmsToken})
			rr := httptest.NewRecorder()

			srv.handleAcceptInvitation(rr, req)

			require.Equal(t, tc.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.wantBodyPhrase)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestHandleAcceptInvitation_MalformedTokenEncoding400 proves a handler maps
// an invitationTokenParam decode failure to a 400 response. The route context
// is built by hand because net/http rejects a malformed percent-escape while
// parsing the request URI, so such a request never reaches a handler through
// the real router (see team_invitation_token_test.go for the helper-level
// tests).
func TestHandleAcceptInvitation_MalformedTokenEncoding400(t *testing.T) {
	m := newInvConfMocks(t)
	srv := newInvConfServer(m)

	req := createAuthenticatedRequest(
		"POST", "/api/v1/invitations/placeholder/accept", "", errArmsUserID)
	req = addURLParams(req, map[string]string{"token": "bad%ZZtoken"}) // %ZZ is not a valid escape
	rr := httptest.NewRecorder()

	srv.handleAcceptInvitation(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), invitationMsgInvalidTokenEncoding)
	// No service call must have been attempted with the undecodable token —
	// the strict mocks would fail on any unexpected repository call.
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestHandleRejectInvitation_ErrorArms covers the reject handler's error
// mapping: 404 invalid token, 403 not the invitee, 400 not pending, 500
// repository failure.
func TestHandleRejectInvitation_ErrorArms(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(m *invConfMocks)
		wantStatus     int
		wantBodyPhrase string
	}{
		{
			name: "unknown token → 404",
			setup: func(m *invConfMocks) {
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).
					Return((*models.TeamInvitation)(nil), repositories.ErrTeamInvitationNotFound).Once()
			},
			wantStatus:     http.StatusNotFound,
			wantBodyPhrase: "Invalid invitation token",
		},
		{
			name: "wrong email → 403",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
				m.userRepo.On("GetByID", mock.Anything, errArmsUserID).
					Return(&models.User{ID: errArmsUserID, Email: "someone-else@example.com"}, nil).Once()
			},
			wantStatus:     http.StatusForbidden,
			wantBodyPhrase: "not authorized to reject",
		},
		{
			name: "not pending → 400",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				inv.Status = models.InvitationStatusRevoked
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
				m.userRepo.On("GetByID", mock.Anything, errArmsUserID).Return(inviteeUser(), nil).Once()
			},
			wantStatus:     http.StatusBadRequest,
			wantBodyPhrase: "not pending",
		},
		{
			name: "status write failure → 500",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
				m.userRepo.On("GetByID", mock.Anything, errArmsUserID).Return(inviteeUser(), nil).Once()
				m.invRepo.On("UpdateStatus", mock.Anything, confInvitationID, models.InvitationStatusRejected).
					Return(stderrors.New("db write timeout")).Once()
			},
			wantStatus:     http.StatusInternalServerError,
			wantBodyPhrase: "Failed to reject invitation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newInvConfMocks(t)
			tc.setup(m)
			srv := newInvConfServer(m)

			req := createAuthenticatedRequest(
				"POST", "/api/v1/invitations/"+errArmsToken+"/reject", "", errArmsUserID)
			req = addURLParams(req, map[string]string{"token": errArmsToken})
			rr := httptest.NewRecorder()

			srv.handleRejectInvitation(rr, req)

			require.Equal(t, tc.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.wantBodyPhrase)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestHandleGetInvitationByToken_ErrorArms drives the typed-error mapping
// (InvitationExpiredError / InvitationStateError / InvitationNotFoundError)
// through the handler itself — the direct-mapper unit test in
// team_invitation_handlers_test.go does not exercise the handler branch or
// validate the wire shape.
func TestHandleGetInvitationByToken_ErrorArms(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(m *invConfMocks)
		wantStatus     int
		wantBodyPhrase string
	}{
		{
			name: "expired → 410",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				inv.ExpiresAt = time.Now().Add(-time.Hour)
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
			},
			wantStatus:     http.StatusGone,
			wantBodyPhrase: "expired",
		},
		{
			name: "already accepted → 409",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				inv.Status = models.InvitationStatusAccepted
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
			},
			wantStatus:     http.StatusConflict,
			wantBodyPhrase: "already been accepted",
		},
		{
			name: "revoked → 409",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				inv.Status = models.InvitationStatusRevoked
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).Return(&inv, nil).Once()
			},
			wantStatus:     http.StatusConflict,
			wantBodyPhrase: "revoked",
		},
		{
			name: "unknown token → 404",
			setup: func(m *invConfMocks) {
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).
					Return((*models.TeamInvitation)(nil), repositories.ErrTeamInvitationNotFound).Once()
			},
			wantStatus:     http.StatusNotFound,
			wantBodyPhrase: "Invitation not found",
		},
		{
			name: "repository failure → 500",
			setup: func(m *invConfMocks) {
				m.invRepo.On("GetByToken", mock.Anything, errArmsToken).
					Return((*models.TeamInvitation)(nil), stderrors.New("connection reset")).Once()
			},
			wantStatus:     http.StatusInternalServerError,
			wantBodyPhrase: "Failed to load invitation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newInvConfMocks(t)
			tc.setup(m)
			srv := newInvConfServer(m)

			req := createAuthenticatedRequest(
				"GET", "/api/v1/invitations/"+errArmsToken, "", errArmsUserID)
			req = addURLParams(req, map[string]string{"token": errArmsToken})
			rr := httptest.NewRecorder()

			srv.handleGetInvitationByToken(rr, req)

			require.Equal(t, tc.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.wantBodyPhrase)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestHandleRevokeInvitation_ErrorArms covers the revoke handler's error
// mapping: 403 without member.invite, 404 unknown invitation, 500 repository
// failure.
func TestHandleRevokeInvitation_ErrorArms(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(m *invConfMocks)
		wantStatus     int
		wantBodyPhrase string
	}{
		{
			name: "permission denied → 403",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				m.invRepo.On("GetByID", mock.Anything, confInvitationID).Return(&inv, nil).Once()
				m.authz.On("Can", mock.Anything, errArmsUserID, confTeamID, authz.MemberInvite).
					Return(services.ErrPermissionDenied).Once()
			},
			wantStatus:     http.StatusForbidden,
			wantBodyPhrase: "permission to revoke",
		},
		{
			name: "unknown invitation → 404",
			setup: func(m *invConfMocks) {
				m.invRepo.On("GetByID", mock.Anything, confInvitationID).
					Return((*models.TeamInvitation)(nil), repositories.ErrTeamInvitationNotFound).Once()
			},
			wantStatus:     http.StatusNotFound,
			wantBodyPhrase: "Invitation not found",
		},
		{
			name: "status write failure → 500",
			setup: func(m *invConfMocks) {
				inv := pendingInvitation()
				m.invRepo.On("GetByID", mock.Anything, confInvitationID).Return(&inv, nil).Once()
				m.authz.On("Can", mock.Anything, errArmsUserID, confTeamID, authz.MemberInvite).
					Return(nil).Once()
				m.invRepo.On("UpdateStatus", mock.Anything, confInvitationID, models.InvitationStatusRevoked).
					Return(stderrors.New("db write timeout")).Once()
			},
			wantStatus:     http.StatusInternalServerError,
			wantBodyPhrase: "Failed to revoke invitation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newInvConfMocks(t)
			tc.setup(m)
			srv := newInvConfServer(m)

			req := createAuthenticatedRequest("DELETE",
				"/api/v1/teams/"+confTeamID+"/invitations/"+confInvitationID, "", errArmsUserID)
			req = addURLParams(req, map[string]string{"id": confTeamID, "invitationId": confInvitationID})
			rr := httptest.NewRecorder()

			srv.handleRevokeInvitation(rr, req)

			require.Equal(t, tc.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.wantBodyPhrase)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestHandleListTeamInvitations_ServiceError covers the 500 branch of
// handleListTeamInvitations: an authorized caller whose repository read fails
// receives a problem document, not a token leak.
func TestHandleListTeamInvitations_ServiceError(t *testing.T) {
	m := newInvConfMocks(t)
	m.authz.On("Can", mock.Anything, errArmsUserID, confTeamID, authz.MemberInvite).
		Return(nil).Once()
	m.invRepo.On("GetByTeamID", mock.Anything, confTeamID).
		Return(([]models.TeamInvitation)(nil), stderrors.New("connection reset")).Once()

	srv := newInvConfServer(m)
	req := createAuthenticatedRequest(
		"GET", "/api/v1/teams/"+confTeamID+"/invitations", "", errArmsUserID)
	req = addURLParams(req, map[string]string{"id": confTeamID})
	rr := httptest.NewRecorder()

	srv.handleListTeamInvitations(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Failed to list invitations")
	specconformance.AssertConformsToSpec(t, req, rr)
}
