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
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// These tests close the invitations slice of the progressive-conformance work
// (#255 / epic #122): every invitation op gets a spec-validated success-response
// assertion so hand-marshaled drift becomes a build failure. #249 already covered
// GET /teams/{id}/invitations; this file covers the remaining six.
//
// All six handlers reach a *concrete* *services.TeamInvitationService (not an
// interface), so each test constructs a real service wired to mocked repos +
// collaborators and serves it through a container that also exposes the handler's
// direct container reads (UserRepository/TeamRepository/TeamService).

const (
	confTeamID       = "550e8400-e29b-41d4-a716-446655440000"
	confInvitationID = "660e8400-e29b-41d4-a716-446655440111"
)

type invConfMocks struct {
	invRepo    *repomocks.MockTeamInvitationRepository
	teamRepo   *repomocks.MockTeamRepository
	userRepo   *repomocks.MockUserRepository
	memberRepo *repomocks.MockTeamMemberRepository
	email      *servicesmocks.MockEmailServiceInterface
	authz      *servicesmocks.MockAuthorizationServiceInterface
	teamSvc    *servicesmocks.MockTeamServiceInterface
}

func newInvConfMocks(t *testing.T) *invConfMocks {
	return &invConfMocks{
		invRepo:    repomocks.NewMockTeamInvitationRepository(t),
		teamRepo:   repomocks.NewMockTeamRepository(t),
		userRepo:   repomocks.NewMockUserRepository(t),
		memberRepo: repomocks.NewMockTeamMemberRepository(t),
		email:      servicesmocks.NewMockEmailServiceInterface(t),
		authz:      servicesmocks.NewMockAuthorizationServiceInterface(t),
		teamSvc:    servicesmocks.NewMockTeamServiceInterface(t),
	}
}

// invConfContainer embeds BaseMockContainer (nil defaults) and overrides only the
// accessors the invitation handlers touch.
type invConfContainer struct {
	BaseMockContainer
	invitationSvc *services.TeamInvitationService
	teamSvc       services.TeamServiceInterface
	userRepo      repositories.UserRepository
	teamRepo      repositories.TeamRepository
}

func (c *invConfContainer) TeamInvitationService() *services.TeamInvitationService {
	return c.invitationSvc
}
func (c *invConfContainer) TeamService() services.TeamServiceInterface  { return c.teamSvc }
func (c *invConfContainer) UserRepository() repositories.UserRepository { return c.userRepo }
func (c *invConfContainer) TeamRepository() repositories.TeamRepository { return c.teamRepo }

func newInvConfServer(m *invConfMocks) *Server {
	logger := slog.New(slog.DiscardHandler)
	svc := services.NewTeamInvitationService(
		m.invRepo, m.teamRepo, m.memberRepo, m.userRepo, m.email, m.authz,
		&config.Config{}, logger,
	)
	return &Server{
		port: "8080",
		container: &invConfContainer{
			invitationSvc: svc,
			teamSvc:       m.teamSvc,
			userRepo:      m.userRepo,
			teamRepo:      m.teamRepo,
		},
		logger: logger,
	}
}

func pendingInvitation() models.TeamInvitation {
	return models.TeamInvitation{
		ID:           confInvitationID,
		TeamID:       confTeamID,
		InviterID:    "inviter-1",
		InviteeEmail: "invitee@example.com",
		Role:         models.TeamMemberRoleMember,
		Token:        "tok-abc-123",
		Status:       models.InvitationStatusPending,
		ExpiresAt:    time.Now().Add(48 * time.Hour),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// GET /api/v1/invitations/pending
func TestHandleGetPendingInvitations_Conformance(t *testing.T) {
	m := newInvConfMocks(t)
	inv := pendingInvitation()

	m.userRepo.On("GetByID", mock.Anything, "user-1").
		Return(&models.User{ID: "user-1", Email: "invitee@example.com", Name: "Invitee"}, nil).Once()
	m.invRepo.On("GetPendingByEmail", mock.Anything, "invitee@example.com").
		Return([]models.TeamInvitation{inv}, nil).Once()
	m.teamRepo.On("GetByID", mock.Anything, confTeamID).
		Return(&models.Team{ID: confTeamID, Name: "Acme"}, nil).Once()
	m.userRepo.On("GetByID", mock.Anything, "inviter-1").
		Return(&models.User{ID: "inviter-1", Email: "boss@example.com", Name: "Boss"}, nil).Once()

	srv := newInvConfServer(m)
	req := createAuthenticatedRequest("GET", "/api/v1/invitations/pending", "", "user-1")
	rr := httptest.NewRecorder()

	srv.handleGetPendingInvitations(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp models.PendingInvitationsListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Invitations, 1)
	assert.Equal(t, "Acme", resp.Invitations[0].TeamName)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// GET /api/v1/invitations/{token}
func TestHandleGetInvitationByToken_Conformance(t *testing.T) {
	m := newInvConfMocks(t)
	inv := pendingInvitation()

	m.invRepo.On("GetByToken", mock.Anything, "tok-abc-123").Return(&inv, nil).Once()
	m.teamRepo.On("GetByID", mock.Anything, confTeamID).
		Return(&models.Team{ID: confTeamID, Name: "Acme"}, nil).Once()
	m.userRepo.On("GetByID", mock.Anything, "inviter-1").
		Return(&models.User{ID: "inviter-1", Email: "boss@example.com", Name: "Boss"}, nil).Once()

	srv := newInvConfServer(m)
	req := createAuthenticatedRequest("GET", "/api/v1/invitations/tok-abc-123", "", "user-1")
	req = addURLParams(req, map[string]string{"token": "tok-abc-123"})
	rr := httptest.NewRecorder()

	srv.handleGetInvitationByToken(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp models.InvitationDetailsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "tok-abc-123", resp.Invitation.Token)
	assert.Equal(t, "Acme", resp.Invitation.TeamName)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// POST /api/v1/invitations/{token}/accept
func TestHandleAcceptInvitation_Conformance(t *testing.T) {
	m := newInvConfMocks(t)
	inv := pendingInvitation()

	m.invRepo.On("GetByToken", mock.Anything, "tok-abc-123").Return(&inv, nil).Once()
	m.userRepo.On("GetByID", mock.Anything, "user-1").
		Return(&models.User{ID: "user-1", Email: "invitee@example.com"}, nil).Once()
	// Not yet a member → GetByTeamAndUser returns an error (the success path).
	m.memberRepo.On("GetByTeamAndUser", mock.Anything, confTeamID, "user-1").
		Return((*models.TeamMember)(nil), repositories.ErrUserNotFound).Once()
	m.memberRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.TeamMember")).
		Return(nil).Once()
	m.invRepo.On("UpdateStatus", mock.Anything, confInvitationID, models.InvitationStatusAccepted).
		Return(nil).Once()
	m.teamSvc.On("GetTeam", mock.Anything, "user-1", confTeamID).
		Return(&models.Team{ID: confTeamID, Name: "Acme"}, nil).Once()

	srv := newInvConfServer(m)
	req := createAuthenticatedRequest("POST", "/api/v1/invitations/tok-abc-123/accept", "", "user-1")
	req = addURLParams(req, map[string]string{"token": "tok-abc-123"})
	rr := httptest.NewRecorder()

	srv.handleAcceptInvitation(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp models.AcceptInvitationResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, confTeamID, resp.TeamID)
	assert.Equal(t, "Acme", resp.TeamName)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// POST /api/v1/invitations/{token}/reject
func TestHandleRejectInvitation_Conformance(t *testing.T) {
	m := newInvConfMocks(t)
	inv := pendingInvitation()

	m.invRepo.On("GetByToken", mock.Anything, "tok-abc-123").Return(&inv, nil).Once()
	m.userRepo.On("GetByID", mock.Anything, "user-1").
		Return(&models.User{ID: "user-1", Email: "invitee@example.com"}, nil).Once()
	m.invRepo.On("UpdateStatus", mock.Anything, confInvitationID, models.InvitationStatusRejected).
		Return(nil).Once()

	srv := newInvConfServer(m)
	req := createAuthenticatedRequest("POST", "/api/v1/invitations/tok-abc-123/reject", "", "user-1")
	req = addURLParams(req, map[string]string{"token": "tok-abc-123"})
	rr := httptest.NewRecorder()

	srv.handleRejectInvitation(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// POST /api/v1/teams/{id}/invitations
func TestHandleSendTeamInvitations_Conformance(t *testing.T) {
	m := newInvConfMocks(t)

	m.authz.On("Can", mock.Anything, "user-1", confTeamID, authz.MemberInvite).Return(nil).Once()
	m.teamRepo.On("GetByID", mock.Anything, confTeamID).
		Return(&models.Team{ID: confTeamID, Name: "Acme", IsPersonal: false}, nil).Once()
	// Inviter display-name lookup.
	m.userRepo.On("GetByID", mock.Anything, "user-1").
		Return(&models.User{ID: "user-1", Name: "Boss", Email: "boss@example.com"}, nil).Once()
	// New invitee: not an existing user, no prior pending invitation.
	m.userRepo.On("GetByEmail", mock.Anything, "new@example.com").
		Return((*models.User)(nil), repositories.ErrUserNotFound).Once()
	m.invRepo.On("GetPendingByEmail", mock.Anything, "new@example.com").
		Return([]models.TeamInvitation{}, nil).Once()
	m.invRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.TeamInvitation")).
		Return(nil).Once()
	m.email.On("SendTeamInvitation", mock.AnythingOfType("*models.TeamInvitation"), "Acme", "Boss").
		Return(nil).Once()

	srv := newInvConfServer(m)
	body := `{"emails":["new@example.com"],"role":"member"}`
	req := createAuthenticatedRequest("POST", "/api/v1/teams/"+confTeamID+"/invitations", body, "user-1")
	req = addURLParams(req, map[string]string{"id": confTeamID})
	rr := httptest.NewRecorder()

	srv.handleSendTeamInvitations(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp []models.InvitationResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "new@example.com", resp[0].InviteeEmail)
	assert.Empty(t, resp[0].Token, "the create response must not carry the token")
	specconformance.AssertConformsToSpec(t, req, rr)
}

// DELETE /api/v1/teams/{id}/invitations/{invitationId}
func TestHandleRevokeInvitation_Conformance(t *testing.T) {
	m := newInvConfMocks(t)
	inv := pendingInvitation()

	m.invRepo.On("GetByID", mock.Anything, confInvitationID).Return(&inv, nil).Once()
	m.authz.On("Can", mock.Anything, "user-1", confTeamID, authz.MemberInvite).Return(nil).Once()
	m.invRepo.On("UpdateStatus", mock.Anything, confInvitationID, models.InvitationStatusRevoked).
		Return(nil).Once()

	srv := newInvConfServer(m)
	req := createAuthenticatedRequest("DELETE",
		"/api/v1/teams/"+confTeamID+"/invitations/"+confInvitationID, "", "user-1")
	req = addURLParams(req, map[string]string{"id": confTeamID, "invitationId": confInvitationID})
	rr := httptest.NewRecorder()

	srv.handleRevokeInvitation(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}
