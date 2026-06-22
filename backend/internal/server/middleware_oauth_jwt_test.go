package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/authkit"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/repositories"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

const (
	oauthTestSubject    = "user_workos_mobile"
	oauthTestInternalID = "vibexp-user-77"
	oauthTestKeyID      = "mw-test-key-1"
)

// oauthStubResolver implements authkit.UserResolver for middleware tests.
type oauthStubResolver struct {
	id  string
	err error
}

func (s oauthStubResolver) ResolveUserID(_ context.Context, _, _ string) (string, error) {
	return s.id, s.err
}

// oauthJWKSServer serves an ephemeral RSA JWKS at <baseURL>/oauth2/jwks and
// signs RS256 test tokens, mimicking AuthKit for the middleware tests.
type oauthJWKSServer struct {
	key    *rsa.PrivateKey
	server *httptest.Server
}

func newOAuthJWKSServer(t *testing.T) *oauthJWKSServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/jwks", func(w http.ResponseWriter, _ *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		jwks := map[string]any{
			"keys": []map[string]any{
				{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": oauthTestKeyID, "n": n, "e": e},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(jwks))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &oauthJWKSServer{key: key, server: srv}
}

func (j *oauthJWKSServer) issuer() string { return j.server.URL }

func (j *oauthJWKSServer) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = oauthTestKeyID
	signed, err := tok.SignedString(j.key)
	require.NoError(t, err)
	return signed
}

// validOAuthClaims mimics a plain AuthKit PKCE access token: no aud claim.
func validOAuthClaims(issuer string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": issuer,
		"sub": oauthTestSubject,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func newOAuthJWTTestServer(t *testing.T, cookiePassword string) *Server {
	t.Helper()
	cfg := &config.Config{WorkOSCookiePassword: cookiePassword}
	logger := slog.New(slog.DiscardHandler)
	return New("8080", nil, "test-api-key", cfg, logger)
}

// attachVerifier wires an authkit verifier with the given resolver directly
// onto the test server, bypassing config-driven construction (which would need
// a real user repository behind the container).
func attachVerifier(t *testing.T, srv *Server, j *oauthJWKSServer, resolver authkit.UserResolver) {
	t.Helper()
	v, err := authkit.New(context.Background(), j.issuer(), authkit.AllowAnyAudience(), resolver)
	require.NoError(t, err)
	srv.apiTokenVerifier = v
}

// captureNext returns a next-handler that records whether it ran and the auth
// context it saw.
func captureNext(called *bool, userID, authType *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		*userID, _ = r.Context().Value(contextkeys.UserID).(string)
		*authType, _ = r.Context().Value(contextkeys.AuthType).(string)
		w.WriteHeader(http.StatusOK)
	})
}

func doFlexibleAuth(srv *Server, req *http.Request) (*httptest.ResponseRecorder, bool, string, string) {
	var called bool
	var userID, authType string
	rr := httptest.NewRecorder()
	srv.flexibleAuthMiddleware(captureNext(&called, &userID, &authType)).ServeHTTP(rr, req)
	return rr, called, userID, authType
}

func TestFlexibleAuth_OAuthJWT_Valid(t *testing.T) {
	j := newOAuthJWKSServer(t)
	srv := newOAuthJWTTestServer(t, "")
	attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+j.sign(t, validOAuthClaims(j.issuer())))

	rr, called, userID, authType := doFlexibleAuth(srv, req)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, called)
	assert.Equal(t, oauthTestInternalID, userID, "context must carry the internal user ID, not the WorkOS sub")
	assert.Equal(t, "oauth", authType)
}

func TestFlexibleAuth_OAuthJWT_Rejections(t *testing.T) {
	j := newOAuthJWKSServer(t)

	tests := []struct {
		name    string
		mutate  func(jwt.MapClaims)
		resolve authkit.UserResolver
	}{
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
			oauthStubResolver{id: oauthTestInternalID}},
		{"wrong issuer", func(c jwt.MapClaims) { c["iss"] = "https://evil.example" },
			oauthStubResolver{id: oauthTestInternalID}},
		{"unknown subject", func(jwt.MapClaims) {},
			oauthStubResolver{err: repositories.ErrUserNotFound}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newOAuthJWTTestServer(t, "")
			attachVerifier(t, srv, j, tt.resolve)

			claims := validOAuthClaims(j.issuer())
			tt.mutate(claims)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			req.Header.Set("Authorization", "Bearer "+j.sign(t, claims))

			rr, called, _, _ := doFlexibleAuth(srv, req)
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
			assert.False(t, called, "handler must not run on auth failure")
		})
	}
}

func TestFlexibleAuth_OAuthJWT_BadSignature(t *testing.T) {
	j := newOAuthJWKSServer(t)
	srv := newOAuthJWTTestServer(t, "")
	attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, validOAuthClaims(j.issuer()))
	tok.Header["kid"] = oauthTestKeyID
	signed, err := tok.SignedString(otherKey)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+signed)

	rr, called, _, _ := doFlexibleAuth(srv, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, called)
}

// TestFlexibleAuth_OAuthJWT_InfraError verifies an infrastructure failure
// during subject resolution surfaces as a 500, never a 401 — mirroring the MCP
// verifier's distinction.
func TestFlexibleAuth_OAuthJWT_InfraError(t *testing.T) {
	j := newOAuthJWKSServer(t)
	srv := newOAuthJWTTestServer(t, "")
	attachVerifier(t, srv, j, oauthStubResolver{err: errors.New("connection refused")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+j.sign(t, validOAuthClaims(j.issuer())))

	rr, called, _, _ := doFlexibleAuth(srv, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.False(t, called)
	assert.NotContains(t, rr.Body.String(), "connection refused",
		"raw infra detail must not leak to the client")
}

// TestFlexibleAuth_OAuthJWT_Unconfigured pins the pre-mobile behavior: with no
// verifier configured, non-API-key bearer tokens still get a 401.
func TestFlexibleAuth_OAuthJWT_Unconfigured(t *testing.T) {
	j := newOAuthJWKSServer(t)
	srv := newOAuthJWTTestServer(t, "")
	require.Nil(t, srv.apiTokenVerifier)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+j.sign(t, validOAuthClaims(j.issuer())))

	rr, called, _, _ := doFlexibleAuth(srv, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, called)
	assert.Contains(t, rr.Body.String(), "use session cookie authentication")
}

// TestFlexibleAuth_Precedence_APIKeyPrefixBeforeJWT verifies that within the
// Authorization header an API-key prefix match is tried before JWT
// verification: a bearer token carrying a known API-key prefix goes down the
// API-key path (and fails there) even when a JWT verifier is configured.
func TestFlexibleAuth_Precedence_APIKeyPrefixBeforeJWT(t *testing.T) {
	j := newOAuthJWKSServer(t)

	apiKeySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
	apiKeySvc.EXPECT().ValidateAPIKey(mock.Anything, mock.Anything).
		Return(nil, errors.New("unknown api key"))

	logger := slog.New(slog.DiscardHandler)
	srv := &Server{
		port:      "8080",
		container: &teamsAPIKeyContainer{apiKeySvc: apiKeySvc},
		logger:    logger,
		config:    &config.Config{},
		router:    chi.NewRouter(),
	}
	attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer vxk_"+j.sign(t, validOAuthClaims(j.issuer())))

	rr, called, _, _ := doFlexibleAuth(srv, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, called)
	assert.Contains(t, rr.Body.String(), "Invalid API key",
		"API-key prefix must route to the API-key path, not JWT verification")
}

// TestFlexibleAuth_Precedence_HeaderOverCookie verifies the Authorization
// header wins over the session cookie: with a valid JWT in the header and a
// garbage session cookie present, the request authenticates as oauth.
func TestFlexibleAuth_Precedence_HeaderOverCookie(t *testing.T) {
	j := newOAuthJWKSServer(t)
	srv := newOAuthJWTTestServer(t, testCookiePassword)
	require.NotNil(t, srv.sessionManager)
	attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+j.sign(t, validOAuthClaims(j.issuer())))
	req.AddCookie(&http.Cookie{Name: "vx_session", Value: "garbage"})

	rr, called, _, authType := doFlexibleAuth(srv, req)
	require.Equal(t, http.StatusOK, rr.Code,
		"header JWT must win; the garbage cookie alone would 401")
	assert.True(t, called)
	assert.Equal(t, "oauth", authType)
}

func doOptionalAuth(srv *Server, req *http.Request) (*httptest.ResponseRecorder, bool, string, string) {
	var called bool
	var userID, authType string
	rr := httptest.NewRecorder()
	srv.optionalAuthMiddleware(captureNext(&called, &userID, &authType)).ServeHTTP(rr, req)
	return rr, called, userID, authType
}

func TestOptionalAuth_OAuthJWT(t *testing.T) {
	j := newOAuthJWKSServer(t)

	t.Run("valid JWT sets user context", func(t *testing.T) {
		srv := newOAuthJWTTestServer(t, "")
		attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/some-optional", nil)
		req.Header.Set("Authorization", "Bearer "+j.sign(t, validOAuthClaims(j.issuer())))

		rr, called, userID, authType := doOptionalAuth(srv, req)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, called)
		assert.Equal(t, oauthTestInternalID, userID)
		assert.Equal(t, "oauth", authType)
	})

	t.Run("invalid JWT falls through unauthenticated", func(t *testing.T) {
		srv := newOAuthJWTTestServer(t, "")
		attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})

		claims := validOAuthClaims(j.issuer())
		claims["exp"] = time.Now().Add(-time.Hour).Unix()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/some-optional", nil)
		req.Header.Set("Authorization", "Bearer "+j.sign(t, claims))

		rr, called, userID, _ := doOptionalAuth(srv, req)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, called, "optional auth must let the request proceed")
		assert.Empty(t, userID, "no user context on JWT failure")
	})

	t.Run("unconfigured verifier falls through unauthenticated", func(t *testing.T) {
		srv := newOAuthJWTTestServer(t, "")
		require.Nil(t, srv.apiTokenVerifier)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/some-optional", nil)
		req.Header.Set("Authorization", "Bearer "+j.sign(t, validOAuthClaims(j.issuer())))

		rr, called, userID, _ := doOptionalAuth(srv, req)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, called)
		assert.Empty(t, userID)
	})

}

// TestOptionalAuth_OAuthJWT_InfraError pins that an infrastructure failure
// during subject resolution never 500s the optional path — the request
// proceeds anonymous (and the failure is logged for operators).
func TestOptionalAuth_OAuthJWT_InfraError(t *testing.T) {
	j := newOAuthJWKSServer(t)
	srv := newOAuthJWTTestServer(t, "")
	attachVerifier(t, srv, j, oauthStubResolver{err: errors.New("connection refused")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/some-optional", nil)
	req.Header.Set("Authorization", "Bearer "+j.sign(t, validOAuthClaims(j.issuer())))

	rr, called, userID, _ := doOptionalAuth(srv, req)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, called)
	assert.Empty(t, userID)
}

// TestAPIAudiencePolicy pins the config-to-policy selection: only a list with
// at least one non-empty trimmed entry activates allow-list enforcement;
// otherwise any audience is accepted EXCEPT the MCP resource URI, so an MCP
// client's audience-bound token cannot double as a full API credential.
func TestAPIAudiencePolicy(t *testing.T) {
	const mcpResource = "https://connect.vibexp.io/mcp/v1/common"

	t.Run("no audiences: accepts absent aud", func(t *testing.T) {
		p := apiAudiencePolicy(nil, mcpResource)
		assert.NoError(t, p(nil))
	})

	t.Run("no audiences: rejects MCP-resource-bound token", func(t *testing.T) {
		p := apiAudiencePolicy(nil, mcpResource)
		assert.Error(t, p(jwt.ClaimStrings{mcpResource}))
	})

	t.Run("empty-string entries do not activate enforcement", func(t *testing.T) {
		p := apiAudiencePolicy([]string{"", "  "}, mcpResource)
		assert.NoError(t, p(nil), "an all-empty list must behave as unconfigured, not reject everything")
	})

	t.Run("configured list requires membership", func(t *testing.T) {
		p := apiAudiencePolicy([]string{"client_123"}, mcpResource)
		assert.NoError(t, p(jwt.ClaimStrings{"client_123"}))
		assert.Error(t, p(jwt.ClaimStrings{"client_456"}))
		assert.Error(t, p(nil))
	})

	t.Run("entries are whitespace-trimmed", func(t *testing.T) {
		p := apiAudiencePolicy([]string{" client_123 ", ""}, mcpResource)
		assert.NoError(t, p(jwt.ClaimStrings{"client_123"}),
			"API_OAUTH_AUDIENCES=\"a, b\" style input must match after trimming")
	})
}

// TestNewAPITokenVerifier_Activation pins that the verifier is nil (feature
// off) without an issuer and non-nil with one.
func TestNewAPITokenVerifier_Activation(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("no issuer: nil verifier", func(t *testing.T) {
		v := newAPITokenVerifier(&config.Config{}, &teamsAPIKeyContainer{}, logger)
		assert.Nil(t, v)
	})

	t.Run("issuer set: verifier constructed", func(t *testing.T) {
		cfg := &config.Config{APIOAuthIssuer: "https://issuer.example"}
		v := newAPITokenVerifier(cfg, &teamsAPIKeyContainer{}, logger)
		assert.NotNil(t, v)
	})
}
