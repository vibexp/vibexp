package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const listInvitationsTeamID = "550e8400-e29b-41d4-a716-446655440000"

// mockInvitationListContainer serves a real TeamInvitationService wired to a
// mocked repository + authorizer, so handleListTeamInvitations runs its actual
// response-building path. Everything else falls back to BaseMockContainer's nil
// defaults.
type mockInvitationListContainer struct {
	BaseMockContainer
	invitationSvc *services.TeamInvitationService
}

func (m *mockInvitationListContainer) TeamInvitationService() *services.TeamInvitationService {
	return m.invitationSvc
}

func newInvitationListServer(svc *services.TeamInvitationService) *Server {
	return &Server{
		port:      "8080",
		container: &mockInvitationListContainer{invitationSvc: svc},
		logger:    slog.New(slog.DiscardHandler),
	}
}

// TestHandleListTeamInvitations_TokenPopulatedAndConforms covers #249: an
// admin/owner (holding member.invite) sees each pending invitation's token so
// the SPA can render a copyable accept link, and the response conforms to the
// spec (which is why the op's payload-coverage ledger entry is now gone).
func TestHandleListTeamInvitations_TokenPopulatedAndConforms(t *testing.T) {
	invRepo := repomocks.NewMockTeamInvitationRepository(t)
	authzMock := servicesmocks.NewMockAuthorizationServiceInterface(t)

	expiresAt := time.Now().Add(48 * time.Hour)
	createdAt := time.Now()
	invitations := []models.TeamInvitation{
		{
			ID: "inv-1", TeamID: listInvitationsTeamID, InviterID: "user-1",
			InviteeEmail: "a@example.com", Role: models.TeamMemberRoleMember,
			Token: "tok-abc-123", Status: models.InvitationStatusPending,
			ExpiresAt: expiresAt, CreatedAt: createdAt,
		},
		{
			ID: "inv-2", TeamID: listInvitationsTeamID, InviterID: "user-1",
			InviteeEmail: "b@example.com", Role: models.TeamMemberRoleAdmin,
			Token: "tok-def-456", Status: models.InvitationStatusPending,
			ExpiresAt: expiresAt, CreatedAt: createdAt,
		},
	}

	authzMock.On("Can", mock.Anything, "user-1", listInvitationsTeamID, authz.MemberInvite).
		Return(nil).Once()
	invRepo.On("GetByTeamID", mock.Anything, listInvitationsTeamID).
		Return(invitations, nil).Once()

	svc := services.NewTeamInvitationService(services.TeamInvitationServiceDeps{
		InvitationRepo: invRepo,
		TeamRepo:       nil,
		TeamMemberRepo: nil,
		UserRepo:       nil,
		EmailService:   nil,
		Authz:          authzMock,
		Cfg:            &config.Config{},
		Logger:         slog.New(slog.DiscardHandler),
	})
	srv := newInvitationListServer(svc)

	req := createAuthenticatedRequest(
		"GET", "/api/v1/teams/"+listInvitationsTeamID+"/invitations", "", "user-1",
	)
	req = addURLParams(req, map[string]string{"id": listInvitationsTeamID})
	rr := httptest.NewRecorder()

	srv.handleListTeamInvitations(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var got []models.InvitationResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "tok-abc-123", got[0].Token,
		"the list response must carry the persisted token so an admin can copy the accept link (#249)")
	assert.Equal(t, "tok-def-456", got[1].Token)

	specconformance.AssertConformsToSpec(t, req, rr)
	invRepo.AssertExpectations(t)
	authzMock.AssertExpectations(t)
}

// TestHandleListTeamInvitations_MemberForbidden pins the unchanged guard: a
// caller without member.invite gets 403 and no token leaks (#249 acceptance).
func TestHandleListTeamInvitations_MemberForbidden(t *testing.T) {
	invRepo := repomocks.NewMockTeamInvitationRepository(t)
	authzMock := servicesmocks.NewMockAuthorizationServiceInterface(t)

	authzMock.On("Can", mock.Anything, "member-user", listInvitationsTeamID, authz.MemberInvite).
		Return(services.ErrPermissionDenied).Once()

	svc := services.NewTeamInvitationService(services.TeamInvitationServiceDeps{
		InvitationRepo: invRepo,
		TeamRepo:       nil,
		TeamMemberRepo: nil,
		UserRepo:       nil,
		EmailService:   nil,
		Authz:          authzMock,
		Cfg:            &config.Config{},
		Logger:         slog.New(slog.DiscardHandler),
	})
	srv := newInvitationListServer(svc)

	req := createAuthenticatedRequest(
		"GET", "/api/v1/teams/"+listInvitationsTeamID+"/invitations", "", "member-user",
	)
	req = addURLParams(req, map[string]string{"id": listInvitationsTeamID})
	rr := httptest.NewRecorder()

	srv.handleListTeamInvitations(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	assert.NotContains(t, rr.Body.String(), "token",
		"a forbidden caller must never receive an invitation token")
	// GetByTeamID must never be reached once authorization fails.
	invRepo.AssertNotCalled(t, "GetByTeamID", mock.Anything, mock.Anything)
	authzMock.AssertExpectations(t)
}
