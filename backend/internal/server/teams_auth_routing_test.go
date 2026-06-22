package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// teamsAPIKeyContainer overrides only the API key service exercised by auth
// middleware for teams routes. All other methods return nil via BaseMockContainer.
type teamsAPIKeyContainer struct {
	BaseMockContainer
	apiKeySvc services.APIKeyServiceInterface
}

func (c *teamsAPIKeyContainer) APIKeyService() services.APIKeyServiceInterface {
	return c.apiKeySvc
}

func newTestServerForTeams(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	return New("8080", nil, "test-api-key", cfg, logger)
}

func assertEndpointRequiresAuth(t *testing.T, srv *Server, method, path string) {
	t.Helper()
	req, err := http.NewRequest(method, path, nil)
	assert.NoError(t, err)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code,
		"endpoint %s %s should return 401 when unauthenticated", method, path)
}

// TestTeamsEndpoints_RequireAuthentication verifies that teams endpoints return 401
// when accessed without any authentication credentials. This confirms the routes are
// protected by the flexibleAuthMiddleware which enforces authentication.
func TestTeamsEndpoints_RequireAuthentication(t *testing.T) {
	srv := newTestServerForTeams(t)

	assertEndpointRequiresAuth(t, srv, http.MethodGet, "/api/v1/teams")
	assertEndpointRequiresAuth(t, srv, http.MethodPost, "/api/v1/teams")
	assertEndpointRequiresAuth(t, srv, http.MethodGet, "/api/v1/teams/some-team-id")
	assertEndpointRequiresAuth(t, srv, http.MethodPut, "/api/v1/teams/some-team-id")
	assertEndpointRequiresAuth(t, srv, http.MethodDelete, "/api/v1/teams/some-team-id")
	assertEndpointRequiresAuth(t, srv, http.MethodGet, "/api/v1/teams/some-team-id/members")
	assertEndpointRequiresAuth(t, srv, http.MethodGet, "/api/v1/invitations/pending")
}

// TestTeamsEndpoints_InvalidTokenReturns401 verifies that teams endpoints return 401
// when accessed with a malformed/invalid authorization token.
func TestTeamsEndpoints_InvalidTokenReturns401(t *testing.T) {
	srv := newTestServerForTeams(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/teams"},
		{http.MethodPost, "/api/v1/teams"},
		{http.MethodGet, "/api/v1/teams/some-team-id"},
		{http.MethodPut, "/api/v1/teams/some-team-id"},
		{http.MethodDelete, "/api/v1/teams/some-team-id"},
	}

	for _, ep := range endpoints {
		req, err := http.NewRequest(ep.method, ep.path, nil)
		assert.NoError(t, err)
		req.Header.Set("Authorization", "Bearer invalid.jwt.token")
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code,
			"endpoint %s %s with invalid token should return 401", ep.method, ep.path)
	}
}

// TestTeamsEndpoints_AcceptsAPIKeyAuth verifies that teams endpoints accept CLI API key
// authentication — the core goal of this fix. A valid API key must pass flexibleAuthMiddleware
// and reach the handler (any non-401 response proves authentication succeeded).
func TestTeamsEndpoints_AcceptsAPIKeyAuth(t *testing.T) {
	const testUserID = "test-user-id-123"
	const cliAPIKey = "acli-test-api-key-for-cli-access" // #nosec G101 - test credential, not a real key

	mockAPIKeySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
	mockAPIKeySvc.EXPECT().
		ValidateAPIKey(mock.Anything, cliAPIKey).
		Return(&models.APIKey{UserID: testUserID}, nil)

	// Create server using the standard constructor (sets up full router + middleware chain),
	// then replace the container with our mock. Since tests are in package server,
	// we can access unexported struct fields directly.
	srv := newTestServerForTeams(t)
	srv.container = &teamsAPIKeyContainer{
		apiKeySvc: mockAPIKeySvc,
	}

	req, err := http.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	assert.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+cliAPIKey)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Auth must be accepted — response must NOT be 401 Unauthorized.
	// The handler may return another status (500, 402, etc.) due to nil downstream
	// services, but any non-401 proves the API key passed authentication successfully.
	assert.NotEqual(t, http.StatusUnauthorized, rr.Code,
		"teams endpoint must accept CLI API key auth (got %d, want anything but 401)", rr.Code)
}
