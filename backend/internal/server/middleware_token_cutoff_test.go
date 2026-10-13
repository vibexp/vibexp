package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/mcptoken"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Bearer-token revocation by issue time (#1449).
//
// An access token is a signed JWT that verifies until its own expiry, so
// revoking a user's API access sets users.api_tokens_valid_after and the
// authentication chokepoint refuses a token issued before it. These tests drive
// every bearer path end to end, because the cut-off is only real if each path
// hands the token's iat to the chokepoint and renders its error.

// cutoffServer is a test server whose user row carries the given cut-off (nil
// for "API access was never revoked").
func cutoffServer(t *testing.T, cutoff *time.Time) *Server {
	t.Helper()
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	users := repomocks.NewMockUserRepository(t)
	users.On("GetByID", mock.Anything, mock.Anything).
		Return(func(_ context.Context, id string) (*models.User, error) {
			return &models.User{ID: id, Status: models.UserStatusActive, APITokensValidAfter: cutoff}, nil
		}).Maybe()
	srv.container = containerWithUsers{BaseMockContainer: &BaseMockContainer{}, users: users}
	return srv
}

// cutoffCases are the issue times that matter around a cut-off. issuedAt nil
// means the token carries no iat claim.
type cutoffCase struct {
	name     string
	cutoff   *time.Time
	issuedAt *time.Time
	allowed  bool
}

func cutoffCases() []cutoffCase {
	now := time.Now().Truncate(time.Second)
	cutoff := now.Add(-time.Hour)
	before := cutoff.Add(-time.Minute)
	after := cutoff.Add(time.Minute)
	return []cutoffCase{
		{"never revoked: a token is judged on its own validity", nil, &before, true},
		{"never revoked: a token without iat still works", nil, nil, true},
		{"issued before the cut-off: refused", &cutoff, &before, false},
		{"issued after the cut-off: accepted", &cutoff, &after, true},
		{"revoked, and the token has no iat: refused", &cutoff, nil, false},
	}
}

func TestTokenCutoff_APIBearerPath(t *testing.T) {
	j := newOAuthJWKSServer(t)
	sign := func(t *testing.T, issuedAt *time.Time) string {
		claims := validOAuthClaims(j.issuer())
		if issuedAt != nil {
			claims["iat"] = issuedAt.Unix()
		}
		return j.sign(t, claims)
	}

	for _, tc := range cutoffCases() {
		t.Run("required: "+tc.name, func(t *testing.T) {
			srv := cutoffServer(t, tc.cutoff)
			attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			req.Header.Set("Authorization", "Bearer "+sign(t, tc.issuedAt))

			rr, called, userID, _ := doFlexibleAuth(srv, req)

			if tc.allowed {
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
				assert.True(t, called)
				assert.Equal(t, oauthTestInternalID, userID)
				return
			}
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
			assert.False(t, called, "the handler must not run with a revoked token")
			assert.Contains(t, rr.Body.String(), tokenRevokedAuthDetail)
		})

		t.Run("optional: "+tc.name, func(t *testing.T) {
			srv := cutoffServer(t, tc.cutoff)
			attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/some-optional", nil)
			req.Header.Set("Authorization", "Bearer "+sign(t, tc.issuedAt))

			rr, called, userID, _ := doOptionalAuth(srv, req)

			require.Equal(t, http.StatusOK, rr.Code, "an optional-auth path never rejects")
			assert.True(t, called)
			if tc.allowed {
				assert.Equal(t, oauthTestInternalID, userID)
			} else {
				assert.Empty(t, userID, "a revoked token proceeds anonymous")
			}
		})
	}
}

func TestTokenCutoff_MCPPath(t *testing.T) {
	for _, tc := range cutoffCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv := cutoffServer(t, tc.cutoff)
			verifier := func(context.Context, string, *http.Request) (*mcpauth.TokenInfo, error) {
				info := &mcpauth.TokenInfo{UserID: "internal-user-7", Expiration: time.Now().Add(time.Hour)}
				if tc.issuedAt != nil {
					// What mcptoken.Verifier records for a token with an iat.
					info.Extra = map[string]any{mcptoken.ExtraIssuedAt: *tc.issuedAt}
				}
				return info, nil
			}
			called := false
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
			req := httptest.NewRequest(http.MethodGet, "/mcp/v1/common", nil)
			req.Header.Set("Authorization", "Bearer some-token")
			rr := httptest.NewRecorder()

			mcpauth.RequireBearerToken(verifier, nil)(srv.mcpTokenContextMiddleware(next)).ServeHTTP(rr, req)

			assert.Equal(t, tc.allowed, called)
			if !tc.allowed {
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

// The cut-off is about bearer tokens only: an API key is a row (deleted by the
// revocation) and a cookie session has its own epoch.
func TestTokenCutoff_DoesNotApplyToOtherCredentials(t *testing.T) {
	cutoff := time.Now()
	srv := cutoffServer(t, &cutoff)
	ctx := context.Background()

	_, err := srv.authenticateUser(ctx, "user-1", "api_key", nil, nil, time.Time{})
	require.NoError(t, err, "an API key carries no issue time and is not judged by the cut-off")

	_, err = srv.authenticateUser(ctx, "user-1", authTypeCookie, nil, &sesslib.Session{UserID: "user-1"}, time.Time{})
	require.NoError(t, err, "a cookie session is ended by the session epoch, not by the cut-off")

	_, err = srv.authenticateUser(ctx, "user-1", authTypeOAuth, nil, nil, time.Time{})
	require.ErrorIs(t, err, errTokenRevoked)
}

// The same second counts as before: iat has one-second resolution, so a token
// that cannot be placed after the revocation is refused.
func TestTokenRevoked_Boundaries(t *testing.T) {
	cutoff := time.Date(2026, 10, 13, 12, 0, 0, 500_000_000, time.UTC)
	user := &models.User{APITokensValidAfter: &cutoff}

	assert.True(t, tokenRevoked(user, cutoff.Truncate(time.Second)), "issued in the cut-off's own second")
	assert.True(t, tokenRevoked(user, cutoff.Add(-time.Nanosecond)))
	assert.False(t, tokenRevoked(user, cutoff), "issued exactly at the cut-off is not before it")
	assert.False(t, tokenRevoked(user, cutoff.Truncate(time.Second).Add(time.Second)), "the next second")
	assert.False(t, tokenRevoked(&models.User{}, time.Time{}), "no cut-off, nothing is revoked")
}

// The cut-off reads the standard iat claim. This token is hand-built; that the
// embedded Authorization Server really mints iat, and what a refreshed token
// carries, is pinned in internal/auth/oauthserver/flow_test.go.
func TestTokenCutoff_ClaimNameIsIAT(t *testing.T) {
	j := newOAuthJWKSServer(t)
	cutoff := time.Now().Add(-time.Minute)
	srv := cutoffServer(t, &cutoff)
	attachVerifier(t, srv, j, oauthStubResolver{id: oauthTestInternalID})

	claims := jwt.MapClaims{
		"iss": j.issuer(), "sub": oauthTestSubject,
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+j.sign(t, claims))

	rr, called, _, _ := doFlexibleAuth(srv, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.True(t, called)
}
