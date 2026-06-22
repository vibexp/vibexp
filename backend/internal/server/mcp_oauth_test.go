package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newMCPOAuthTestServer(t *testing.T, issuer string) *Server {
	t.Helper()
	return newMCPOAuthTestServerWithResource(t, issuer, "https://connect.vibexp.io/mcp/v1/common")
}

func newMCPOAuthTestServerWithResource(t *testing.T, issuer, resourceURI string) *Server {
	t.Helper()
	cfg := &config.Config{
		MCPOAuthIssuer: issuer,
		MCPResourceURI: resourceURI,
	}
	logger := slog.New(slog.DiscardHandler)
	return New("8080", nil, "test-api-key", cfg, logger)
}

// TestProtectedResourceMetadata_Public verifies the RFC 9728 metadata document
// is publicly reachable (no auth), returns the expected JSON, and carries the
// CORS header the SDK handler sets.
func TestProtectedResourceMetadata_Public(t *testing.T) {
	srv := newMCPOAuthTestServer(t, "https://issuer.example")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp/v1/common", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	var meta oauthex.ProtectedResourceMetadata
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &meta))
	assert.Equal(t, "https://connect.vibexp.io/mcp/v1/common", meta.Resource)
	assert.Equal(t, []string{"https://issuer.example"}, meta.AuthorizationServers)
	assert.Equal(t, []string{"header"}, meta.BearerMethodsSupported)
}

// TestProtectedResourceMetadata_CORSPreflight verifies the SDK handler answers
// the CORS preflight without auth.
func TestProtectedResourceMetadata_CORSPreflight(t *testing.T) {
	srv := newMCPOAuthTestServer(t, "https://issuer.example")

	req := httptest.NewRequest(http.MethodOptions, "/.well-known/oauth-protected-resource/mcp/v1/common", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
}

// TestAuthorizationServerMetadata_Redirect verifies the legacy AS-metadata probe
// redirects to the configured AuthKit issuer.
func TestAuthorizationServerMetadata_Redirect(t *testing.T) {
	srv := newMCPOAuthTestServer(t, "https://issuer.example")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t,
		"https://issuer.example/.well-known/oauth-authorization-server",
		rr.Header().Get("Location"))
}

// TestAuthorizationServerMetadata_NotConfigured verifies the redirect endpoint
// 404s when no issuer is configured.
func TestAuthorizationServerMetadata_NotConfigured(t *testing.T) {
	srv := newMCPOAuthTestServer(t, "")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestMCPEndpoint_MissingTokenChallenge verifies that an unauthenticated request
// to the MCP endpoint returns 401 with a WWW-Authenticate header that points at
// the protected-resource metadata document (RFC 9728 discovery).
func TestMCPEndpoint_MissingTokenChallenge(t *testing.T) {
	srv := newMCPOAuthTestServer(t, "https://issuer.example")

	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/common", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	challenge := rr.Header().Get("WWW-Authenticate")
	assert.Contains(t, challenge, "Bearer")
	assert.Contains(t, challenge,
		`resource_metadata="https://connect.vibexp.io/.well-known/oauth-protected-resource/mcp/v1/common"`)
}

// TestMCPEndpoint_InvalidTokenChallenge verifies that an invalid bearer token
// also returns 401 with the discovery challenge.
func TestMCPEndpoint_InvalidTokenChallenge(t *testing.T) {
	srv := newMCPOAuthTestServer(t, "https://issuer.example")

	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/common", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Header().Get("WWW-Authenticate"), "resource_metadata=")
}

// TestDeriveMCPMetadata covers the path/URL derivation and the neutral fallback
// (bare well-known prefix, empty URL) for empty or unparseable input.
func TestDeriveMCPMetadata(t *testing.T) {
	tests := []struct {
		name        string
		resourceURI string
		wantPath    string
		wantURL     string
	}{
		{
			"prod",
			"https://connect.vibexp.io/mcp/v1/common",
			"/.well-known/oauth-protected-resource/mcp/v1/common",
			"https://connect.vibexp.io/.well-known/oauth-protected-resource/mcp/v1/common",
		},
		{
			"staging",
			"https://connect-staging.vibexp.io/mcp/v1/common",
			"/.well-known/oauth-protected-resource/mcp/v1/common",
			"https://connect-staging.vibexp.io/.well-known/oauth-protected-resource/mcp/v1/common",
		},
		{
			"empty falls back to neutral prefix",
			"",
			"/.well-known/oauth-protected-resource",
			"",
		},
		{
			"missing scheme falls back to neutral prefix",
			"connect.example.com/mcp/v1/common",
			"/.well-known/oauth-protected-resource",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotURL := deriveMCPMetadata(tt.resourceURI)
			assert.Equal(t, tt.wantPath, gotPath)
			assert.Equal(t, tt.wantURL, gotURL)
		})
	}
}

// TestMCPMetadata_DerivedFromResourceURI verifies the protected-resource
// metadata route path and the WWW-Authenticate resource_metadata URL are both
// derived from MCP_RESOURCE_URI (host and path), so a non-prod environment
// advertises its own host rather than the hardcoded production one.
func TestMCPMetadata_DerivedFromResourceURI(t *testing.T) {
	tests := []struct {
		name         string
		resourceURI  string
		wantRoute    string
		wantMetadata string
	}{
		{
			name:         "production default",
			resourceURI:  "https://connect.vibexp.io/mcp/v1/common",
			wantRoute:    "/.well-known/oauth-protected-resource/mcp/v1/common",
			wantMetadata: "https://connect.vibexp.io/.well-known/oauth-protected-resource/mcp/v1/common",
		},
		{
			name:         "staging host",
			resourceURI:  "https://connect-staging.vibexp.io/mcp/v1/common",
			wantRoute:    "/.well-known/oauth-protected-resource/mcp/v1/common",
			wantMetadata: "https://connect-staging.vibexp.io/.well-known/oauth-protected-resource/mcp/v1/common",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newMCPOAuthTestServerWithResource(t, "https://issuer.example", tt.resourceURI)

			// (a) The metadata document is served at the derived route path.
			metaReq := httptest.NewRequest(http.MethodGet, tt.wantRoute, nil)
			metaRR := httptest.NewRecorder()
			srv.ServeHTTP(metaRR, metaReq)
			require.Equal(t, http.StatusOK, metaRR.Code, "metadata must be served at the derived path")

			// (b) The 401 challenge advertises the derived absolute metadata URL.
			mcpReq := httptest.NewRequest(http.MethodGet, "/mcp/v1/common", nil)
			mcpRR := httptest.NewRecorder()
			srv.ServeHTTP(mcpRR, mcpReq)
			require.Equal(t, http.StatusUnauthorized, mcpRR.Code)
			assert.Contains(t, mcpRR.Header().Get("WWW-Authenticate"),
				`resource_metadata="`+tt.wantMetadata+`"`)
		})
	}
}

// TestMCPTokenContextBridge_SetsUserID verifies that the full middleware chain
// (RequireBearerToken → mcpTokenContextMiddleware) copies the resolved internal
// user ID from the OAuth TokenInfo into contextkeys.UserID, so the unchanged MCP
// tool layer (getUserFromContext) keeps working. The SDK stores TokenInfo under
// an unexported key, so we drive the real RequireBearerToken middleware to set
// it rather than constructing the context value directly.
func TestMCPTokenContextBridge_SetsUserID(t *testing.T) {
	srv := newMCPOAuthTestServer(t, "")

	var gotUserID, gotAuthType string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(contextkeys.UserID).(string)
		gotAuthType, _ = r.Context().Value(contextkeys.AuthType).(string)
	})

	verifier := func(_ context.Context, _ string, _ *http.Request) (*mcpauth.TokenInfo, error) {
		return &mcpauth.TokenInfo{UserID: "internal-user-7", Expiration: time.Now().Add(time.Hour)}, nil
	}
	handler := mcpauth.RequireBearerToken(verifier, nil)(srv.mcpTokenContextMiddleware(next))

	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/common", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "internal-user-7", gotUserID)
	assert.Equal(t, "oauth", gotAuthType)
}

// TestUserResolverAdapter resolves a WorkOS subject to the internal user ID and
// surfaces both the not-found and error paths.
func TestUserResolverAdapter(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves internal id", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByIDPSubject(ctx, "workos", "sub-1").
			Return(&models.User{ID: "internal-9"}, nil)

		id, err := userResolverAdapter{users: repo}.ResolveUserID(ctx, "workos", "sub-1")
		require.NoError(t, err)
		assert.Equal(t, "internal-9", id)
	})

	t.Run("nil user yields empty id", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByIDPSubject(ctx, "workos", "sub-2").Return(nil, nil)

		id, err := userResolverAdapter{users: repo}.ResolveUserID(ctx, "workos", "sub-2")
		require.NoError(t, err)
		assert.Empty(t, id)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByIDPSubject(ctx, "workos", "sub-3").
			Return(nil, assert.AnError)

		_, err := userResolverAdapter{users: repo}.ResolveUserID(ctx, "workos", "sub-3")
		require.Error(t, err)
	})
}

// TestUnconfiguredMCPVerifier verifies the fallback verifier rejects every token
// with an error that unwraps to ErrInvalidToken (so the middleware emits 401).
func TestUnconfiguredMCPVerifier(t *testing.T) {
	_, err := unconfiguredMCPVerifier(context.Background(), "any-token",
		httptest.NewRequest(http.MethodGet, "/", nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, mcpauth.ErrInvalidToken)
}

// TestMCPTokenContextMiddleware_RejectsMissingTokenInfo verifies the bridge
// middleware returns 401 when no TokenInfo is present (defense in depth).
func TestMCPTokenContextMiddleware_RejectsMissingTokenInfo(t *testing.T) {
	srv := newMCPOAuthTestServer(t, "")

	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	handler := srv.mcpTokenContextMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/common", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, called, "next handler must not run without TokenInfo")
}
