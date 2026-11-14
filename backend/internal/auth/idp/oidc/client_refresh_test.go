package oidc

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// newRefreshClient builds a generic OIDC client over a fake issuer whose
// discovery document lists scopes.
func newRefreshClient(t *testing.T, scopes []string) (*Client, *fakeoidc.Issuer) {
	t.Helper()
	iss := fakeoidc.New(t, testClientID, testClientSecret, "user-1", "user@example.com")
	iss.SetScopesSupported(scopes)
	c, err := New(context.Background(), Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL,
		ClientID: testClientID, ClientSecret: testClientSecret, RedirectURL: pkceRedirect,
	})
	require.NoError(t, err)
	return c, iss
}

func TestNew_OfflineAccessFollowsDiscovery(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   string
	}{
		{"listed", []string{"openid", "email", "profile", "offline_access"}, "openid email profile offline_access"},
		{"not listed", []string{"openid", "email", "profile"}, "openid email profile"},
		{"no scopes_supported", nil, "openid email profile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newRefreshClient(t, tt.scopes)
			q := mustQuery(t, c.AuthorizeURL("state", "", "", idp.FlowParams{}))
			assert.Equal(t, tt.want, q.Get("scope"))
		})
	}
}

func TestNew_OfflineAccessNeverAddedToExplicitScopesOrAccessTypeOffline(t *testing.T) {
	iss := fakeoidc.New(t, testClientID, testClientSecret, "user-1", "user@example.com")
	iss.SetScopesSupported([]string{"openid", "offline_access"})

	explicit, err := New(context.Background(), Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL, ClientID: testClientID, ClientSecret: testClientSecret,
		Scopes: []string{"openid", "email"},
	})
	require.NoError(t, err)
	assert.Equal(t, "openid email", mustQuery(t, explicit.AuthorizeURL("s", "", "", idp.FlowParams{})).Get("scope"))

	offline, err := New(context.Background(), Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL, ClientID: testClientID, ClientSecret: testClientSecret,
		AccessTypeOffline: true,
	})
	require.NoError(t, err)
	q := mustQuery(t, offline.AuthorizeURL("s", "", "", idp.FlowParams{}))
	assert.Equal(t, "openid email profile", q.Get("scope"))
	assert.Equal(t, "offline", q.Get("access_type"))
}

func TestClient_Refresh_VerifiesTheIDTokenAndReturnsItsClaims(t *testing.T) {
	c, iss := newRefreshClient(t, nil)
	rt := iss.IssueRefreshToken()

	tokens, claims, err := c.Refresh(context.Background(), rt)

	require.NoError(t, err)
	require.NotNil(t, claims)
	assert.Equal(t, "user-1", claims.Subject)
	assert.Equal(t, "user@example.com", claims.Email)
	assert.Equal(t, "user-1", claims.Raw["sub"])
	assert.NotEmpty(t, tokens.IDToken)
	assert.NotEqual(t, rt, tokens.RefreshToken, "the issuer rotated the refresh token")
	assert.Equal(t, int64(1), iss.RefreshCalls())
}

func TestClient_Refresh_RejectsAnUnverifiableIDToken(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeoidc.Issuer)
	}{
		{"bad signature", func(i *fakeoidc.Issuer) { i.SetForgeIDTokens(true) }},
		{"wrong audience", func(i *fakeoidc.Issuer) { i.SetClaim("aud", "another-client") }},
		{"wrong issuer", func(i *fakeoidc.Issuer) { i.SetClaim("iss", "https://evil.example.com") }},
		{"expired", func(i *fakeoidc.Issuer) { i.SetClaim("exp", 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, iss := newRefreshClient(t, nil)
			tt.setup(iss)

			tokens, claims, err := c.Refresh(context.Background(), iss.IssueRefreshToken())

			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "oidc: verify id token"), err.Error())
			assert.Nil(t, tokens)
			assert.Nil(t, claims)
		})
	}
}

func TestClient_Refresh_ChecksNoNonce(t *testing.T) {
	c, iss := newRefreshClient(t, nil)
	iss.SetClaim("nonce", "a-nonce-from-some-sign-in")

	_, claims, err := c.Refresh(context.Background(), iss.IssueRefreshToken())

	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
}

func TestClient_Refresh_FallsBackToUserInfoWithoutAnIDToken(t *testing.T) {
	c, iss := newRefreshClient(t, nil)
	iss.SetRefreshIDToken(false)

	tokens, claims, err := c.Refresh(context.Background(), iss.IssueRefreshToken())

	require.NoError(t, err)
	assert.Empty(t, tokens.IDToken)
	require.NotNil(t, claims)
	assert.Equal(t, "user-1", claims.Subject)
	assert.Equal(t, "user@example.com", claims.Email)
}

func TestClient_Refresh_ReusedTokenIsInvalidGrant(t *testing.T) {
	c, iss := newRefreshClient(t, nil)
	rt := iss.IssueRefreshToken()
	_, _, err := c.Refresh(context.Background(), rt)
	require.NoError(t, err)

	_, _, err = c.Refresh(context.Background(), rt)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_grant")
}

func TestClient_Refresh_IssuerOutageIsProviderUnavailable(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*fakeoidc.Issuer)
		unavailable bool
	}{
		{"keys endpoint 503", func(i *fakeoidc.Issuer) { i.SetJWKSStatus(http.StatusServiceUnavailable) }, true},
		{"userinfo 503", func(i *fakeoidc.Issuer) {
			i.SetRefreshIDToken(false)
			i.SetUserInfoStatus(http.StatusServiceUnavailable)
		}, true},
		{"userinfo 429", func(i *fakeoidc.Issuer) {
			i.SetRefreshIDToken(false)
			i.SetUserInfoStatus(http.StatusTooManyRequests)
		}, true},
		{"userinfo 401 is a verdict on the token", func(i *fakeoidc.Issuer) {
			i.SetRefreshIDToken(false)
			i.SetUserInfoStatus(http.StatusUnauthorized)
		}, false},
		{"bad signature is a verdict on the token", func(i *fakeoidc.Issuer) { i.SetForgeIDTokens(true) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, iss := newRefreshClient(t, nil)
			tt.setup(iss)

			_, _, err := c.Refresh(context.Background(), iss.IssueRefreshToken())

			require.Error(t, err)
			assert.Equal(t, tt.unavailable, errors.Is(err, idp.ErrProviderUnavailable), err.Error())
		})
	}
}
