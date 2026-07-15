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

	"github.com/vibexp/vibexp/internal/auth/oauthserver"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/feature_flags"
)

func newMCPOAuthTestServer(t *testing.T, issuer string) *Server {
	t.Helper()
	return newMCPOAuthTestServerWithResource(t, issuer, "https://connect.vibexp.io/mcp/v1/common")
}

func newMCPOAuthTestServerWithResource(t *testing.T, issuer, resourceURI string) *Server {
	t.Helper()
	cfg := &config.Config{
		MCP: config.MCPConfig{
			OAuthIssuer: issuer,
			ResourceURI: resourceURI,
		},
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
	assert.Equal(t, []string{oauthserver.ScopeMCP}, meta.ScopesSupported)
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
	assert.Contains(t, challenge, `scope="`+oauthserver.ScopeMCP+`"`,
		"the challenge must advertise the MCP scope so clients know what to request")
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
	challenge := rr.Header().Get("WWW-Authenticate")
	assert.Contains(t, challenge, "resource_metadata=")
	assert.Contains(t, challenge, `scope="`+oauthserver.ScopeMCP+`"`)
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

// TestUserResolverAdapter resolves a token subject to the internal user ID and
// surfaces both the not-found and error paths.
func TestUserResolverAdapter(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves internal id", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByIDPSubject(ctx, "oidc", "sub-1").
			Return(&models.User{ID: "internal-9"}, nil)

		id, err := userResolverAdapter{users: repo}.ResolveUserID(ctx, "oidc", "sub-1")
		require.NoError(t, err)
		assert.Equal(t, "internal-9", id)
	})

	t.Run("nil user yields empty id", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByIDPSubject(ctx, "oidc", "sub-2").Return(nil, nil)

		id, err := userResolverAdapter{users: repo}.ResolveUserID(ctx, "oidc", "sub-2")
		require.NoError(t, err)
		assert.Empty(t, id)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByIDPSubject(ctx, "oidc", "sub-3").
			Return(nil, assert.AnError)

		_, err := userResolverAdapter{users: repo}.ResolveUserID(ctx, "oidc", "sub-3")
		require.Error(t, err)
	})
}

// TestConsentAccessPolicyAdapter covers the attach-time allowlist re-check (#217):
// the adapter resolves the consenting user's email and applies the same evaluator
// the login path uses, and fails closed on anything it cannot vouch for.
func TestConsentAccessPolicyAdapter(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	newAdapter := func(repo repositories.UserRepository, domains, emails []string) consentAccessPolicyAdapter {
		return consentAccessPolicyAdapter{
			users:     repo,
			allowlist: feature_flags.NewUserSignInAllowlistFlag(logger, domains, emails),
		}
	}

	t.Run("allows an email on the allowlist", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByID(ctx, "user-1").
			Return(&models.User{ID: "user-1", Email: "dev@example.com"}, nil)

		allowed, err := newAdapter(repo, []string{"example.com"}, nil).AllowUser(ctx, "user-1")
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("denies an email removed from the allowlist", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByID(ctx, "user-2").
			Return(&models.User{ID: "user-2", Email: "gone@evil.example"}, nil)

		allowed, err := newAdapter(repo, []string{"example.com"}, nil).AllowUser(ctx, "user-2")
		require.NoError(t, err)
		assert.False(t, allowed, "a user no longer on the allowlist must not mint MCP tokens")
	})

	t.Run("allows everyone when no allowlist is configured", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByID(ctx, "user-3").
			Return(&models.User{ID: "user-3", Email: "anyone@anywhere.test"}, nil)

		allowed, err := newAdapter(repo, nil, nil).AllowUser(ctx, "user-3")
		require.NoError(t, err)
		assert.True(t, allowed, "an unconfigured allowlist is open access")
	})

	t.Run("denies a user that no longer exists", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByID(ctx, "ghost").
			Return(nil, repositories.ErrUserNotFound)

		allowed, err := newAdapter(repo, []string{"example.com"}, nil).AllowUser(ctx, "ghost")
		require.NoError(t, err, "a deleted user is a denial, not an internal error")
		assert.False(t, allowed)
	})

	t.Run("propagates an unexpected lookup error", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByID(ctx, "user-4").Return(nil, assert.AnError)

		allowed, err := newAdapter(repo, []string{"example.com"}, nil).AllowUser(ctx, "user-4")
		require.Error(t, err, "an undecidable policy must surface, so the caller fails closed")
		assert.False(t, allowed)
	})
}

// TestMCPUserResolverAdapter verifies the MCP resolver treats the token subject
// as an internal user ID (the embedded AS mints sub = user ID) and resolves it
// via GetByID — distinct from the IdP-subject adapter the web/API path uses.
func TestMCPUserResolverAdapter(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves internal id by user id", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByID(ctx, "internal-9").Return(&models.User{ID: "internal-9"}, nil)

		id, err := mcpUserResolverAdapter{users: repo}.ResolveUserID(ctx, "", "internal-9")
		require.NoError(t, err)
		assert.Equal(t, "internal-9", id)
	})

	t.Run("propagates not-found (authkit maps it to an auth failure)", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByID(ctx, "ghost").Return(nil, repositories.ErrUserNotFound)

		_, err := mcpUserResolverAdapter{users: repo}.ResolveUserID(ctx, "", "ghost")
		require.ErrorIs(t, err, repositories.ErrUserNotFound)
	})

	t.Run("nil user yields empty id", func(t *testing.T) {
		repo := repomocks.NewMockUserRepository(t)
		repo.EXPECT().GetByID(ctx, "x").Return(nil, nil)

		id, err := mcpUserResolverAdapter{users: repo}.ResolveUserID(ctx, "", "x")
		require.NoError(t, err)
		assert.Empty(t, id)
	})
}

// TestAdvertiseChallengeScope verifies the challenge-scope wrapper annotates the
// WWW-Authenticate Bearer challenge on 401/403 without enforcing the scope (it
// never blocks a request or touches success responses), and leaves non-Bearer
// challenges alone.
func TestAdvertiseChallengeScope(t *testing.T) {
	chain := func(next http.Handler) http.Handler { return advertiseChallengeScope("mcp")(next) }
	req := func() *http.Request { return httptest.NewRequest(http.MethodGet, "/mcp/v1/common", nil) }

	t.Run("appends scope to a 401 Bearer challenge", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://x/.well-known"`)
			w.WriteHeader(http.StatusUnauthorized)
		})
		rr := httptest.NewRecorder()
		chain(next).ServeHTTP(rr, req())

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Equal(t, `Bearer resource_metadata="https://x/.well-known", scope="mcp"`,
			rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("annotates a 403 Bearer challenge", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusForbidden)
		})
		rr := httptest.NewRecorder()
		chain(next).ServeHTTP(rr, req())
		assert.Equal(t, `Bearer, scope="mcp"`, rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("does not block or annotate a 200 response", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("ok"))
			require.NoError(t, err)
		})
		rr := httptest.NewRecorder()
		chain(next).ServeHTTP(rr, req())

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "ok", rr.Body.String())
		assert.Empty(t, rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("preserves Flush for the streaming success path", func(t *testing.T) {
		// The MCP streamable handler flushes via http.NewResponseController(w); the
		// wrapper must Unwrap to the underlying writer or SSE silently stops working.
		var flushErr error
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			flushErr = http.NewResponseController(w).Flush()
		})
		rr := httptest.NewRecorder()
		chain(next).ServeHTTP(rr, req())
		require.NoError(t, flushErr, "Flush must reach the underlying writer through the wrapper")
		assert.True(t, rr.Flushed)
	})

	t.Run("leaves a non-Bearer challenge untouched", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="x"`)
			w.WriteHeader(http.StatusUnauthorized)
		})
		rr := httptest.NewRecorder()
		chain(next).ServeHTTP(rr, req())
		assert.Equal(t, `Basic realm="x"`, rr.Header().Get("WWW-Authenticate"))
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
