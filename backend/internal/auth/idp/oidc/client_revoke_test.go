package oidc

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// newRevokeClient builds a generic OIDC client over a fake issuer that
// advertises its revocation endpoint (when enabled) with authMethods.
func newRevokeClient(t *testing.T, enabled bool, authMethods []string) (*Client, *fakeoidc.Issuer) {
	t.Helper()
	iss := fakeoidc.New(t, testClientID, testClientSecret, "user-1", "user@example.com")
	iss.SetRevocationEndpoint(enabled)
	iss.SetRevocationAuthMethods(authMethods)
	c, err := New(context.Background(), Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL,
		ClientID: testClientID, ClientSecret: testClientSecret, RedirectURL: pkceRedirect,
	})
	require.NoError(t, err)
	return c, iss
}

func TestClient_ImplementsTokenRevoker(t *testing.T) {
	var p idp.IdentityProvider = &Client{}
	_, ok := p.(idp.TokenRevoker)
	assert.True(t, ok)
}

func TestRevokeRefreshToken_ClientAuthFollowsDiscovery(t *testing.T) {
	tests := []struct {
		name      string
		methods   []string
		wantBasic bool
	}{
		{"no auth methods listed", nil, true},
		{"basic listed", []string{"client_secret_basic"}, true},
		{"basic and post listed", []string{"client_secret_post", "client_secret_basic"}, true},
		{"only post listed", []string{"client_secret_post"}, false},
		{"neither listed", []string{"private_key_jwt"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, iss := newRevokeClient(t, true, tt.methods)
			token := iss.IssueRefreshToken()

			require.NoError(t, c.RevokeRefreshToken(context.Background(), token))

			got := iss.Revocations()
			require.Len(t, got, 1)
			assert.Equal(t, fakeoidc.Revocation{
				Token: token, TokenTypeHint: "refresh_token", BasicAuth: tt.wantBasic, Authenticated: true,
			}, got[0])
		})
	}
}

func TestRevokeRefreshToken_RevokesTheTokenAtTheIssuer(t *testing.T) {
	c, iss := newRevokeClient(t, true, nil)
	token := iss.IssueRefreshToken()

	require.NoError(t, c.RevokeRefreshToken(context.Background(), token))

	_, _, err := c.Refresh(context.Background(), token)
	require.Error(t, err, "a revoked refresh token no longer refreshes")
}

func TestRevokeRefreshToken_NoEndpointIsUnsupported(t *testing.T) {
	c, iss := newRevokeClient(t, false, nil)

	err := c.RevokeRefreshToken(context.Background(), iss.IssueRefreshToken())

	require.ErrorIs(t, err, idp.ErrRevocationUnsupported)
	assert.Empty(t, iss.Revocations(), "no request without an advertised endpoint")
}

func TestRevokeRefreshToken_EmptyTokenIsRefused(t *testing.T) {
	c, iss := newRevokeClient(t, true, nil)

	require.Error(t, c.RevokeRefreshToken(context.Background(), ""))
	assert.Empty(t, iss.Revocations())
}

func TestRevokeRefreshToken_NonOKAnswersAreErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, iss := newRevokeClient(t, true, nil)
			iss.SetRevocationStatus(status)

			err := c.RevokeRefreshToken(context.Background(), iss.IssueRefreshToken())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "status "+strconv.Itoa(status))
			assert.False(t, errors.Is(err, idp.ErrRevocationUnsupported))
		})
	}
}

func TestRevokeRefreshToken_UnknownTokenIsOK(t *testing.T) {
	// RFC 7009 §2.2: an invalid or already revoked token is answered 200.
	c, _ := newRevokeClient(t, true, nil)

	require.NoError(t, c.RevokeRefreshToken(context.Background(), "rt-never-issued"))
}

func TestRevokeRefreshToken_WrongClientSecretIsAnError(t *testing.T) {
	iss := fakeoidc.New(t, testClientID, testClientSecret, "user-1", "user@example.com")
	iss.SetRevocationEndpoint(true)
	c, err := New(context.Background(), Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL,
		ClientID: testClientID, ClientSecret: "not-the-secret", RedirectURL: pkceRedirect,
	})
	require.NoError(t, err)

	err = c.RevokeRefreshToken(context.Background(), iss.IssueRefreshToken())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestRevokeRefreshToken_UnreachableIssuerIsUnavailable(t *testing.T) {
	c, iss := newRevokeClient(t, true, nil)
	iss.SetRevocationHang(true)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := c.RevokeRefreshToken(ctx, iss.IssueRefreshToken())

	require.ErrorIs(t, err, idp.ErrProviderUnavailable)
}

func TestRevokeRefreshToken_UsesTheContextHTTPClient(t *testing.T) {
	c, iss := newRevokeClient(t, true, nil)
	var used bool
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, hc)

	require.NoError(t, c.RevokeRefreshToken(ctx, iss.IssueRefreshToken()))
	assert.True(t, used)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
