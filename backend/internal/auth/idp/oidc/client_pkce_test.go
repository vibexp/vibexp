package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

const pkceRedirect = "https://app.example.com/api/v1/auth/callback"

// newPKCEClient builds a generic OIDC client over a fake issuer that enforces
// PKCE and echoes the nonce.
func newPKCEClient(t *testing.T) (*Client, *fakeoidc.Issuer) {
	t.Helper()
	iss := fakeoidc.New(t, testClientID, testClientSecret, "user-1", "user@example.com")
	c, err := New(context.Background(), Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL,
		ClientID: testClientID, ClientSecret: testClientSecret, RedirectURL: pkceRedirect,
	})
	require.NoError(t, err)
	return c, iss
}

func testFlow() idp.FlowParams {
	return idp.FlowParams{CodeVerifier: oauth2.GenerateVerifier(), Nonce: "nonce-" + oauth2.GenerateVerifier()}
}

func TestClient_AuthorizeURL_PKCEAndNonce_NoAccessType(t *testing.T) {
	c, _ := newPKCEClient(t)
	flow := testFlow()

	u, err := url.Parse(c.AuthorizeURL("state-1", "", "", flow))
	require.NoError(t, err)
	q := u.Query()

	sum := sha256.Sum256([]byte(flow.CodeVerifier))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), q.Get("code_challenge"))
	assert.Len(t, q.Get("code_challenge"), 43)
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, flow.Nonce, q.Get("nonce"))
	assert.False(t, q.Has("access_type"), "access_type is Google-only: %s", u)

	// No flow parameters: none of them is sent.
	q = mustQuery(t, c.AuthorizeURL("state-1", "", "", idp.FlowParams{}))
	for _, k := range []string{"code_challenge", "code_challenge_method", "nonce", "access_type"} {
		assert.False(t, q.Has(k), k)
	}
}

func TestClient_AccessTypeOfflineIsOptIn(t *testing.T) {
	iss := fakeoidc.New(t, testClientID, testClientSecret, "user-1", "user@example.com")
	c, err := New(context.Background(), Config{
		Name: idp.ProviderGoogle, IssuerURL: iss.URL, ClientID: testClientID, ClientSecret: testClientSecret,
		AccessTypeOffline: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "offline", mustQuery(t, c.AuthorizeURL("s", "", "", idp.FlowParams{})).Get("access_type"))
}

func TestClient_ExchangeCode_PKCEAndNonceRoundTrip(t *testing.T) {
	c, _ := newPKCEClient(t)
	flow := testFlow()
	code := fakeoidc.Authorize(t, c.AuthorizeURL("state-1", "", "", flow))

	tokens, claims, err := c.ExchangeCode(context.Background(), code, "", flow)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
	assert.NotEmpty(t, tokens.IDToken)
}

func TestClient_ExchangeCode_RejectsWrongOrMissingVerifier(t *testing.T) {
	for name, verifier := range map[string]string{"wrong": oauth2.GenerateVerifier(), "missing": ""} {
		t.Run(name, func(t *testing.T) {
			c, _ := newPKCEClient(t)
			flow := testFlow()
			code := fakeoidc.Authorize(t, c.AuthorizeURL("state-1", "", "", flow))

			_, _, err := c.ExchangeCode(context.Background(), code, "",
				idp.FlowParams{CodeVerifier: verifier, Nonce: flow.Nonce})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid_grant")
			assert.NotErrorIs(t, err, idp.ErrNonceMismatch)
		})
	}
}

func TestClient_ExchangeCode_RejectsNonceMismatch(t *testing.T) {
	for name, override := range map[string]string{"different": "someone-elses-nonce", "missing": ""} {
		t.Run(name, func(t *testing.T) {
			c, iss := newPKCEClient(t)
			flow := testFlow()
			code := fakeoidc.Authorize(t, c.AuthorizeURL("state-1", "", "", flow))
			iss.SetNonceOverride(override)

			tokens, claims, err := c.ExchangeCode(context.Background(), code, "", flow)
			require.ErrorIs(t, err, idp.ErrNonceMismatch)
			assert.Nil(t, tokens)
			assert.Nil(t, claims)
		})
	}
}

// A flow without a nonce checks none, so an ID token carrying one is accepted.
func TestClient_ExchangeCode_NoNonceRequestedChecksNone(t *testing.T) {
	c, iss := newPKCEClient(t)
	flow := idp.FlowParams{CodeVerifier: oauth2.GenerateVerifier()}
	code := fakeoidc.Authorize(t, c.AuthorizeURL("state-1", "", "", flow))
	iss.SetNonceOverride("unrequested")

	_, claims, err := c.ExchangeCode(context.Background(), code, "", flow)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u.Query()
}
