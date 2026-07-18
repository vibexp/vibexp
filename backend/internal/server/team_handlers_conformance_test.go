package server

// Payload-conformance closure for the team CRUD/member ops (issue #363,
// coverage epic #358): clears the five remaining "TODO(#1714): uncovered"
// team entries from the payload-coverage ledger. The happy paths and most
// failure paths live in team_handlers_integration_test.go (now asserting
// specconformance); this file adds the missing member-list failure path and
// the authz-matrix 403s for every mutating team endpoint, driven through a
// REAL TeamService wired to a denying AuthorizationService mock (pattern:
// TestHandleListTeamInvitations_MemberForbidden) — so the handler mapping of
// services.ErrPermissionDenied is proven end-to-end without re-testing the
// matrix itself.

import (
	stderrors "errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
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

// Fixture identifiers for the authz-forbidden tests. The caller is a plain
// member, so every admin+/owner-gated write must be denied by the matrix.
const (
	authzTeamID       = "550e8400-e29b-41d4-a716-446655440777"
	authzMemberUserID = "member-user"
	authzOwnerUserID  = "owner-user"
)

// teamAuthzMocks bundles the repositories + authorizer behind a real
// TeamService instance.
type teamAuthzMocks struct {
	teamRepo   *repomocks.MockTeamRepository
	memberRepo *repomocks.MockTeamMemberRepository
	userRepo   *repomocks.MockUserRepository
	authz      *servicesmocks.MockAuthorizationServiceInterface
}

func newTeamAuthzMocks(t *testing.T) *teamAuthzMocks {
	return &teamAuthzMocks{
		teamRepo:   repomocks.NewMockTeamRepository(t),
		memberRepo: repomocks.NewMockTeamMemberRepository(t),
		userRepo:   repomocks.NewMockUserRepository(t),
		authz:      servicesmocks.NewMockAuthorizationServiceInterface(t),
	}
}

// teamAuthzContainer serves the real TeamService; everything else falls back
// to BaseMockContainer's nil defaults.
type teamAuthzContainer struct {
	BaseMockContainer
	teamSvc services.TeamServiceInterface
}

func (c *teamAuthzContainer) TeamService() services.TeamServiceInterface { return c.teamSvc }

// newTeamAuthzServer routes the three team-scoped mutating endpoints at a
// real TeamService whose authorization decisions come from the mocked
// AuthorizationService.
func newTeamAuthzServer(m *teamAuthzMocks) *Server {
	logger := slog.New(slog.DiscardHandler)
	svc := services.NewTeamService(m.teamRepo, m.memberRepo, m.userRepo, m.authz, logger, nil)

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: &teamAuthzContainer{teamSvc: svc},
		logger:    logger,
		config:    &config.Config{},
		router:    r,
	}
	r.Put("/api/v1/teams/{id}", srv.handleUpdateTeam)
	r.Delete("/api/v1/teams/{id}", srv.handleDeleteTeam)
	r.Delete("/api/v1/teams/{id}/members/{userId}", srv.handleRemoveTeamMember)
	return srv
}

// storedAuthzTeam is the team the repository hands back before authorization
// runs (authorizeTeam loads the team first, then asks the matrix).
func storedAuthzTeam() *models.Team {
	return &models.Team{
		ID:        authzTeamID,
		OwnerID:   authzOwnerUserID,
		Name:      "Acme",
		Slug:      "acme",
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now(),
	}
}

// TestTeamMutations_MemberForbiddenViaAuthzMatrix pins the 403 arm of each
// mutating team endpoint: the matrix denies, the handler maps
// services.ErrPermissionDenied to a 403 problem document, and no write ever
// reaches a repository.
func TestTeamMutations_MemberForbiddenViaAuthzMatrix(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		perm           authz.Permission
		wantBodyPhrase string
		// assertNoWrite verifies the guarded write was never attempted.
		assertNoWrite func(t *testing.T, m *teamAuthzMocks)
	}{
		{
			name:           "PUT /teams/{id}",
			method:         "PUT",
			path:           "/api/v1/teams/" + authzTeamID,
			body:           `{"name":"Renamed"}`,
			perm:           authz.TeamUpdate,
			wantBodyPhrase: "Only team owners and admins can update a team",
			assertNoWrite: func(t *testing.T, m *teamAuthzMocks) {
				m.teamRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
			},
		},
		{
			name:           "DELETE /teams/{id}",
			method:         "DELETE",
			path:           "/api/v1/teams/" + authzTeamID,
			perm:           authz.TeamDelete,
			wantBodyPhrase: "Only the team owner can delete a team",
			assertNoWrite: func(t *testing.T, m *teamAuthzMocks) {
				m.teamRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
			},
		},
		{
			name:           "DELETE /teams/{id}/members/{userId}",
			method:         "DELETE",
			path:           "/api/v1/teams/" + authzTeamID + "/members/some-other-member",
			perm:           authz.MemberRemove,
			wantBodyPhrase: "Only team owners and admins can remove members",
			assertNoWrite: func(t *testing.T, m *teamAuthzMocks) {
				m.memberRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeamAuthzMocks(t)
			m.teamRepo.On("GetByID", mock.Anything, authzTeamID).
				Return(storedAuthzTeam(), nil).Once()
			m.authz.On("Authorize", mock.Anything, authzMemberUserID, authzTeamID, tc.perm).
				Return(models.TeamMemberRole(""), services.ErrPermissionDenied).Once()

			srv := newTeamAuthzServer(m)
			req := createAuthenticatedRequest(tc.method, tc.path, tc.body, authzMemberUserID)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
			assert.Contains(t, w.Body.String(), tc.wantBodyPhrase)
			tc.assertNoWrite(t, m)
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

// TestHandleGetTeamMembers_NotFound covers the member-list failure path: an
// inaccessible (or unknown) team surfaces as a spec-conformant 404.
func TestHandleGetTeamMembers_NotFound(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("GetTeamMembers", mock.Anything, "user-outsider", "team-unknown", 1, 100).
		Return((*models.TeamMembersListResponse)(nil), stderrors.New("team not found"))

	srv := createTestTeamServer(mockContainer)
	req := createAuthenticatedRequest("GET", "/api/v1/teams/team-unknown/members", "", "user-outsider")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Team not found")
	specconformance.AssertConformsToSpec(t, req, w)

	mockContainer.teamService.AssertExpectations(t)
}
