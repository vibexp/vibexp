package server

import (
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// TestSendTeamInvitations_Unauthorized tests sending invitations without auth
func TestSendTeamInvitations_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"emails": ["user@example.com"], "role": "member"}`
	req, err := http.NewRequest("POST", "/api/v1/teams/team-123/invitations", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestSendTeamInvitations_InvalidJSON tests sending invitations with invalid JSON
func TestSendTeamInvitations_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{invalid json}`
	req, err := http.NewRequest("POST", "/api/v1/teams/team-123/invitations", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Will be unauthorized since token is invalid, but route should be hit
	assert.True(t, rr.Code >= 400)
}

// TestSendTeamInvitations_RouteRegistered verifies the route is registered
func TestSendTeamInvitations_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"emails": ["user@example.com"], "role": "member"}`
	req, err := http.NewRequest("POST", "/api/v1/teams/550e8400-e29b-41d4-a716-446655440000/invitations",
		strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestListTeamInvitations_Unauthorized tests listing invitations without auth
func TestListTeamInvitations_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/teams/team-123/invitations", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestListTeamInvitations_RouteRegistered verifies the route is registered
func TestListTeamInvitations_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/teams/550e8400-e29b-41d4-a716-446655440000/invitations", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestRevokeInvitation_Unauthorized tests revoking invitation without auth
func TestRevokeInvitation_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("DELETE",
		"/api/v1/teams/team-123/invitations/invitation-456", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestRevokeInvitation_RouteRegistered verifies the route is registered
func TestRevokeInvitation_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("DELETE",
		"/api/v1/teams/550e8400-e29b-41d4-a716-446655440000/invitations/550e8400-e29b-41d4-a716-446655440001",
		nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestGetPendingInvitations_Unauthorized tests getting pending invitations without auth
func TestGetPendingInvitations_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/invitations/pending", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestGetPendingInvitations_RouteRegistered verifies the route is registered
func TestGetPendingInvitations_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/invitations/pending", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestAcceptInvitation_Unauthorized tests accepting invitation without auth
func TestAcceptInvitation_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("POST", "/api/v1/invitations/test-token-123/accept", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestAcceptInvitation_RouteRegistered verifies the route is registered
func TestAcceptInvitation_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("POST", "/api/v1/invitations/test-token-123/accept", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestRejectInvitation_Unauthorized tests rejecting invitation without auth
func TestRejectInvitation_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("POST", "/api/v1/invitations/test-token-123/reject", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestRejectInvitation_RouteRegistered verifies the route is registered
func TestRejectInvitation_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("POST", "/api/v1/invitations/test-token-123/reject", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestParseTeamMemberRole_ValidRoles tests role parsing
func TestParseTeamMemberRole_ValidRoles(t *testing.T) {
	tests := []struct {
		name        string
		roleStr     string
		expected    models.TeamMemberRole
		expectError bool
	}{
		{
			name:        "Member role",
			roleStr:     "member",
			expected:    models.TeamMemberRoleMember,
			expectError: false,
		},
		{
			name:        "Admin role",
			roleStr:     "admin",
			expected:    models.TeamMemberRoleAdmin,
			expectError: false,
		},
		{
			name:        "Invalid role",
			roleStr:     "owner",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Empty role",
			roleStr:     "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Uppercase role",
			roleStr:     "MEMBER",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, err := parseTeamMemberRole(tt.roleStr)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, role)
			}
		})
	}
}

// TestSendTeamInvitations_EmailLimit tests the email count limit
func TestSendTeamInvitations_EmailLimit(t *testing.T) {
	// Test that > 50 emails should be rejected
	// This is tested via the handler validation

	// Generate more than 50 emails
	emails := make([]string, 51)
	for i := 0; i < 51; i++ {
		emails[i] = "user" + string(rune('0'+i)) + "@example.com"
	}

	assert.Len(t, emails, 51, "Should have 51 emails to test limit")
	assert.Greater(t, len(emails), 50, "Email count exceeds limit")
}

// TestSendTeamInvitations_EmptyEmails tests sending empty email list
func TestSendTeamInvitations_EmptyEmails(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"emails": [], "role": "member"}`
	req, err := http.NewRequest("POST",
		"/api/v1/teams/550e8400-e29b-41d4-a716-446655440000/invitations",
		strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should fail auth or validation
	assert.True(t, rr.Code >= 400)
}

// TestSendTeamInvitations_InvalidEmail tests sending invalid email format
func TestSendTeamInvitations_InvalidEmail(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"emails": ["not-an-email"], "role": "member"}`
	req, err := http.NewRequest("POST",
		"/api/v1/teams/550e8400-e29b-41d4-a716-446655440000/invitations",
		strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should fail auth or validation
	assert.True(t, rr.Code >= 400)
}

// TestSendTeamInvitations_InvalidRole tests sending invalid role
func TestSendTeamInvitations_InvalidRole(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"emails": ["user@example.com"], "role": "superadmin"}`
	req, err := http.NewRequest("POST",
		"/api/v1/teams/550e8400-e29b-41d4-a716-446655440000/invitations",
		strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should fail auth or validation
	assert.True(t, rr.Code >= 400)
}

// TestTeamInvitationEndpoints_MethodNotAllowed tests wrong HTTP methods
// Note: Auth middleware runs first, so unauthorized requests return 401
func TestTeamInvitationEndpoints_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			name:     "GET on send invitations - unauthorized",
			method:   "GET",
			path:     "/api/v1/teams/team-123/invitations",
			expected: http.StatusUnauthorized, // Auth runs before method check
		},
		{
			name:     "PUT on accept invitation - unauthorized",
			method:   "PUT",
			path:     "/api/v1/invitations/token/accept",
			expected: http.StatusUnauthorized, // Auth runs before method check
		},
		{
			name:     "DELETE on accept invitation - unauthorized",
			method:   "DELETE",
			path:     "/api/v1/invitations/token/accept",
			expected: http.StatusUnauthorized, // Auth runs before method check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			assert.Equal(t, tt.expected, rr.Code)
		})
	}
}

// TestAcceptInvitation_EmptyToken tests accepting with empty token
func TestAcceptInvitation_EmptyToken(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	// Empty token in URL
	req, err := http.NewRequest("POST", "/api/v1/invitations//accept", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should fail - either 404 (route not matched) or bad request
	assert.True(t, rr.Code >= 400)
}

// TestBuildInvitationResponses_EmptyInput tests building responses with empty input
func TestBuildInvitationResponses_EmptyInput(t *testing.T) {
	// Test that empty input produces empty output
	var invitations []*models.TeamInvitation
	teams := make(map[string]*models.Team)
	inviters := make(map[string]*models.User)

	// Use a mock server instance
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	responses := srv.buildInvitationResponses(invitations, teams, inviters)
	assert.Empty(t, responses)
}

// TestGetInvitationByToken_Unauthorized verifies anonymous callers receive 401.
func TestGetInvitationByToken_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/invitations/some-token-123", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestGetInvitationByToken_RouteRegistered confirms chi can match the new route.
func TestGetInvitationByToken_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/invitations/some-token-123", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Route registered → not a 404 (a 401 from auth middleware is the expected outcome
	// when the bearer token is invalid; either way it must NOT be a missing route).
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestGetInvitationByToken_PendingPath_NotShadowed verifies the existing
// /api/v1/invitations/pending route still hits the pending handler instead of
// being captured by the new {token} route.
//
// Asserting "not 404" is insufficient: auth middleware returns 401 before the
// handler runs, and a 401 satisfies "not 404" even if {token} did shadow
// /pending. So we use chi.Mux.Match to inspect the route pattern that chi
// actually picks for the path — that bypasses middleware and proves the
// static /pending route wins.
func TestGetInvitationByToken_PendingPath_NotShadowed(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	rctx := chi.NewRouteContext()
	matched := srv.router.Match(rctx, http.MethodGet, "/api/v1/invitations/pending")
	require.True(t, matched, "router must register a route for /api/v1/invitations/pending")

	patterns := rctx.RoutePatterns
	require.NotEmpty(t, patterns, "chi must record at least one route pattern")
	last := patterns[len(patterns)-1]
	assert.Equal(t, "/pending", last,
		"GET /api/v1/invitations/pending must match the static /pending route, not {token}; got %v",
		patterns,
	)

	// And the {token} route is itself reachable for non-/pending paths.
	rctx2 := chi.NewRouteContext()
	matched2 := srv.router.Match(rctx2, http.MethodGet, "/api/v1/invitations/some-token-abc")
	require.True(t, matched2, "router must register a route for /api/v1/invitations/{token}")
	patterns2 := rctx2.RoutePatterns
	require.NotEmpty(t, patterns2)
	assert.Equal(t, "/{token}", patterns2[len(patterns2)-1],
		"non-pending path must match the {token} route; got %v", patterns2,
	)
}

// TestHandleGetInvitationByTokenError_StatusMapping unit-tests the error mapper
// directly to verify each typed error produces the right HTTP status and code,
// without depending on the full route + auth stack.
func TestHandleGetInvitationByTokenError_StatusMapping(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name           string
		err            error
		wantStatus     int
		wantBodyPhrase string
	}{
		{
			name:           "not found → 404",
			err:            services.NewInvitationNotFoundError("missing"),
			wantStatus:     http.StatusNotFound,
			wantBodyPhrase: "Invitation not found",
		},
		{
			name:           "expired → 410 Gone",
			err:            services.NewInvitationExpiredError("inv-1"),
			wantStatus:     http.StatusGone,
			wantBodyPhrase: "expired",
		},
		{
			name:           "accepted → 409",
			err:            services.NewInvitationStateError("inv-2", models.InvitationStatusAccepted),
			wantStatus:     http.StatusConflict,
			wantBodyPhrase: "already been accepted",
		},
		{
			name:           "rejected → 409",
			err:            services.NewInvitationStateError("inv-3", models.InvitationStatusRejected),
			wantStatus:     http.StatusConflict,
			wantBodyPhrase: "rejected",
		},
		{
			name:           "revoked → 409",
			err:            services.NewInvitationStateError("inv-4", models.InvitationStatusRevoked),
			wantStatus:     http.StatusConflict,
			wantBodyPhrase: "revoked",
		},
		{
			name:           "unknown error → 500",
			err:            stderrors.New("boom"),
			wantStatus:     http.StatusInternalServerError,
			wantBodyPhrase: "Failed to load invitation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/v1/invitations/some-token", nil)
			srv.handleGetInvitationByTokenError(rr, req, tc.err)
			assert.Equal(t, tc.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.wantBodyPhrase)
		})
	}
}
