package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	teamRolesTestUserID = "user-caller"
	teamRolesTestTeamID = "3f1d9c2e-5b7a-4c1d-9e2f-0a1b2c3d4e5f"
	teamRolesTestTarget = "user-target"
)

// MockTeamRolesContainer overrides only the team service on the base container.
type MockTeamRolesContainer struct {
	BaseMockContainer
	teamService services.TeamServiceInterface
}

func (c *MockTeamRolesContainer) TeamService() services.TeamServiceInterface {
	return c.teamService
}

func createTestTeamRolesServer(svc services.TeamServiceInterface) *Server {
	r := chi.NewRouter()
	srv := &Server{
		container: &MockTeamRolesContainer{teamService: svc},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	srv.setupTeamRolesRoutes(r)
	return srv
}

// makeTeamRolesRequest injects the authenticated user the way the auth
// middleware would; the middleware itself is not mounted in these tests.
func makeTeamRolesRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, teamRolesTestUserID))
}

func rolePath() string {
	return "/api/v1/teams/" + teamRolesTestTeamID + "/members/" + teamRolesTestTarget + "/role"
}

func transferPath() string {
	return "/api/v1/teams/" + teamRolesTestTeamID + "/transfer-ownership"
}

func TestUpdateTeamMemberRole_Success(t *testing.T) {
	svc := mocks.NewMockTeamServiceInterface(t)
	svc.EXPECT().UpdateMemberRole(
		mock.Anything, teamRolesTestUserID, teamRolesTestTeamID, teamRolesTestTarget, models.TeamMemberRoleAdmin,
	).Return(&models.TeamMemberDetail{
		UserID:   teamRolesTestTarget,
		Email:    "target@example.com",
		Name:     "Target User",
		Role:     string(models.TeamMemberRoleAdmin),
		JoinedAt: time.Unix(0, 0).UTC().Format(time.RFC3339),
	}, nil).Once()

	srv := createTestTeamRolesServer(svc)
	req := makeTeamRolesRequest(http.MethodPatch, rolePath(), `{"role":"admin"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Contains(t, w.Body.String(), `"role":"admin"`)
}

func TestUpdateTeamMemberRole_ErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
	}{
		{"permission denied is 403", services.ErrPermissionDenied, http.StatusForbidden},
		{"changing the owner's role is 403", services.ErrCannotChangeOwnerRole, http.StatusForbidden},
		{"target not a member is 404", repositories.ErrTeamMemberNotFound, http.StatusNotFound},
		{"team not found is 404", services.ErrTeamNotFound, http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := mocks.NewMockTeamServiceInterface(t)
			svc.EXPECT().UpdateMemberRole(
				mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			).Return(nil, tc.serviceErr).Once()

			srv := createTestTeamRolesServer(svc)
			req := makeTeamRolesRequest(http.MethodPatch, rolePath(), `{"role":"admin"}`)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			// Errors must be RFC 9457 problem+json, per the spec.
			assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

// TestUpdateTeamMemberRole_RejectsOwnerRole pins that "owner" is rejected with a
// 400 — the privilege-escalation path this endpoint must never open.
//
// Note WHERE the rejection happens: oapi-codegen does NOT validate the spec's
// `enum: [member, admin]` at bind time, so "owner" arrives at the service
// untouched (this test drives the real generated stack and proves it). The enum
// is documentation; TeamService.UpdateMemberRole's ErrInvalidMemberRole guard is
// the enforcement. Deleting that guard on the assumption that the generated
// layer covers it would silently allow minting a second owner.
func TestUpdateTeamMemberRole_RejectsOwnerRole(t *testing.T) {
	svc := mocks.NewMockTeamServiceInterface(t)
	svc.EXPECT().UpdateMemberRole(
		mock.Anything, teamRolesTestUserID, teamRolesTestTeamID, teamRolesTestTarget, models.TeamMemberRole("owner"),
	).Return(nil, services.ErrInvalidMemberRole).Once()

	srv := createTestTeamRolesServer(svc)
	req := makeTeamRolesRequest(http.MethodPatch, rolePath(), `{"role":"owner"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
}

func TestUpdateTeamMemberRole_InvalidTeamUUID(t *testing.T) {
	svc := mocks.NewMockTeamServiceInterface(t)

	srv := createTestTeamRolesServer(svc)
	req := makeTeamRolesRequest(http.MethodPatch, "/api/v1/teams/not-a-uuid/members/u1/role", `{"role":"admin"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "team id must be a valid UUID")
}

func TestTransferTeamOwnership_Success(t *testing.T) {
	svc := mocks.NewMockTeamServiceInterface(t)
	svc.EXPECT().TransferOwnership(
		mock.Anything, teamRolesTestUserID, teamRolesTestTeamID, teamRolesTestTarget,
	).Return(&models.Team{
		ID:          teamRolesTestTeamID,
		OwnerID:     teamRolesTestTarget,
		Name:        "Team",
		Slug:        "team",
		Description: "desc",
		Role:        string(models.TeamMemberRoleAdmin),
		Permissions: authz.RolePermissionStrings(models.TeamMemberRoleAdmin),
		CreatedAt:   time.Unix(0, 0).UTC(),
		UpdatedAt:   time.Unix(0, 0).UTC(),
	}, nil).Once()

	srv := createTestTeamRolesServer(svc)
	req := makeTeamRolesRequest(http.MethodPost, transferPath(), `{"new_owner_id":"`+teamRolesTestTarget+`"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	// The caller is demoted, and the response says so.
	assert.Contains(t, w.Body.String(), `"owner_id":"`+teamRolesTestTarget+`"`)
	assert.Contains(t, w.Body.String(), `"role":"admin"`)
	// The demotion is reflected in permissions too, not just the role label:
	// the ex-owner loses team.delete and team.transfer (#224).
	assert.Contains(t, w.Body.String(), `"`+authz.TeamUpdate.String()+`"`)
	assert.NotContains(t, w.Body.String(), `"`+authz.TeamDelete.String()+`"`)
	assert.NotContains(t, w.Body.String(), `"`+authz.OwnershipTransfer.String()+`"`)
}

// TestTransferTeamOwnership_PermissionsNeverNull proves the generated Team
// type's required `permissions` array marshals as [] rather than null when the
// service leaves it unset. The generated type cannot use models.JSONArray, so
// this guarantee rests on toGenTeam's make(...,0) and nothing else enforces it.
func TestTransferTeamOwnership_PermissionsNeverNull(t *testing.T) {
	svc := mocks.NewMockTeamServiceInterface(t)
	svc.EXPECT().TransferOwnership(
		mock.Anything, teamRolesTestUserID, teamRolesTestTeamID, teamRolesTestTarget,
	).Return(&models.Team{
		ID:        teamRolesTestTeamID,
		OwnerID:   teamRolesTestTarget,
		Name:      "Team",
		Slug:      "team",
		CreatedAt: time.Unix(0, 0).UTC(),
		UpdatedAt: time.Unix(0, 0).UTC(),
	}, nil).Once()

	srv := createTestTeamRolesServer(svc)
	req := makeTeamRolesRequest(http.MethodPost, transferPath(), `{"new_owner_id":"`+teamRolesTestTarget+`"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Contains(t, w.Body.String(), `"permissions":[]`)
	assert.NotContains(t, w.Body.String(), `"permissions":null`)
}

func TestTransferTeamOwnership_ErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
	}{
		{"non-owner is 403", services.ErrPermissionDenied, http.StatusForbidden},
		{"personal workspace is 403", services.NewPersonalWorkspaceError(teamRolesTestTeamID), http.StatusForbidden},
		{"already the owner is 400", services.ErrAlreadyTeamOwner, http.StatusBadRequest},
		{"target not a member is 404", repositories.ErrTeamMemberNotFound, http.StatusNotFound},
		{"team vanished mid-transfer is 404", repositories.ErrTeamNotFound, http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := mocks.NewMockTeamServiceInterface(t)
			svc.EXPECT().TransferOwnership(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, tc.serviceErr).Once()

			srv := createTestTeamRolesServer(svc)
			req := makeTeamRolesRequest(http.MethodPost, transferPath(), `{"new_owner_id":"`+teamRolesTestTarget+`"}`)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

func TestTransferTeamOwnership_MissingNewOwner(t *testing.T) {
	svc := mocks.NewMockTeamServiceInterface(t)

	srv := createTestTeamRolesServer(svc)
	req := makeTeamRolesRequest(http.MethodPost, transferPath(), `{}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	svc.AssertNotCalled(t, "TransferOwnership", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestTeamRoles_Unauthenticated covers the context-missing path: the legacy
// handlers panic on this unchecked assertion; these must 401 cleanly.
func TestTeamRoles_Unauthenticated(t *testing.T) {
	srv := createTestTeamRolesServer(mocks.NewMockTeamServiceInterface(t))

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPatch, rolePath(), `{"role":"admin"}`},
		{http.MethodPost, transferPath(), `{"new_owner_id":"u"}`},
	} {
		t.Run(tc.method, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
		})
	}
}
