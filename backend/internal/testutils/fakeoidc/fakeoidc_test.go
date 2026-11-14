package fakeoidc_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

const (
	clientID     = "client"
	clientSecret = "client-credential"
)

// authorizeURL is an authorization request for the test client against base.
func authorizeURL(base string) string {
	q := url.Values{"client_id": {clientID}, "redirect_uri": {"http://localhost/callback"}, "state": {"st"}}
	return base + "/authorize?" + q.Encode()
}

// tokenResponse is the part of a token endpoint answer the tests read.
type tokenResponse struct {
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
}

// postToken calls the token endpoint of base with form and decodes the answer.
func postToken(t *testing.T, base string, form url.Values) (int, tokenResponse) {
	t.Helper()
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/token",
		strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	var body tokenResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return resp.StatusCode, body
}

// signIn runs the authorization code flow against base and returns the tokens.
func signIn(t *testing.T, base string) tokenResponse {
	t.Helper()
	code := fakeoidc.Authorize(t, authorizeURL(base))
	status, tokens := postToken(t, base, url.Values{"grant_type": {"authorization_code"}, "code": {code}})
	require.Equal(t, http.StatusOK, status)
	return tokens
}

// refresh presents token to the token endpoint of base.
func refresh(t *testing.T, base, token string) (int, tokenResponse) {
	t.Helper()
	return postToken(t, base, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}})
}

// getJSON GETs target and decodes its JSON answer into out.
func getJSON(t *testing.T, target string, out any) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
}

// claimsOf decodes the payload of an ID token. The signature is checked by the
// identity-provider clients' own tests; here only the minted claims matter.
func claimsOf(t *testing.T, idToken string) map[string]any {
	t.Helper()
	parts := strings.Split(idToken, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims map[string]any
	require.NoError(t, json.Unmarshal(raw, &claims))
	return claims
}

func TestSetUser_BindsEachRefreshTokenToItsUser(t *testing.T) {
	iss := fakeoidc.New(t, clientID, clientSecret, "sub-a", "a@example.com")
	first := signIn(t, iss.URL)
	iss.SetUser("sub-b", "b@example.com")
	second := signIn(t, iss.URL)

	assert.Equal(t, "sub-a", claimsOf(t, first.IDToken)["sub"])
	assert.Equal(t, "sub-b", claimsOf(t, second.IDToken)["sub"])
	assert.Equal(t, "b@example.com", claimsOf(t, second.IDToken)["email"])
	assert.NotEqual(t, first.RefreshToken, second.RefreshToken, "every sign-in gets its own refresh token")

	status, refreshed := refresh(t, iss.URL, first.RefreshToken)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "sub-a", claimsOf(t, refreshed.IDToken)["sub"], "a refresh mints for the token's own user")
	assert.Equal(t, "a@example.com", claimsOf(t, refreshed.IDToken)["email"])
}

func TestSetClaims_ReplacesEveryAddedClaim(t *testing.T) {
	iss := fakeoidc.New(t, clientID, clientSecret, "sub", "u@example.com")
	iss.SetClaim("groups", []string{"old"})
	iss.SetClaim("dept", "eng")

	iss.SetClaims(map[string]any{"groups": []string{"vibexp-admins"}, "dropped": nil})

	claims := claimsOf(t, signIn(t, iss.URL).IDToken)
	assert.Equal(t, []any{"vibexp-admins"}, claims["groups"])
	assert.NotContains(t, claims, "dept")
	assert.NotContains(t, claims, "dropped")
}

func TestSetAuthorizeError_RefusesTheAuthorization(t *testing.T) {
	iss := fakeoidc.New(t, clientID, clientSecret, "sub", "u@example.com")
	iss.SetAuthorizeError("access_denied")

	var body map[string]string
	getJSON(t, authorizeURL(iss.URL), &body)
	assert.Equal(t, map[string]string{"error": "access_denied", "state": "st"}, body)

	iss.SetAuthorizeError("")
	assert.NotEmpty(t, fakeoidc.Authorize(t, authorizeURL(iss.URL)))
}

func TestRevokeRefreshTokens_AnswersInvalidGrantUntilReset(t *testing.T) {
	iss := fakeoidc.New(t, clientID, clientSecret, "sub", "u@example.com")
	tokens := signIn(t, iss.URL)

	iss.RevokeRefreshTokens()

	status, body := refresh(t, iss.URL, tokens.RefreshToken)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", body.Error)

	iss.Reset()
	status, _ = refresh(t, iss.URL, signIn(t, iss.URL).RefreshToken)
	assert.Equal(t, http.StatusOK, status)
}

func TestReset_RestoresTheInitialScript(t *testing.T) {
	iss := fakeoidc.New(t, clientID, clientSecret, "sub-initial", "initial@example.com")
	before := signIn(t, iss.URL)
	iss.SetUser("sub-other", "other@example.com")
	iss.SetClaim("groups", []string{"g"})
	iss.SetAuthorizeError("access_denied")
	iss.SetRefreshStatus(http.StatusServiceUnavailable)

	iss.Reset()

	tokens := signIn(t, iss.URL)
	claims := claimsOf(t, tokens.IDToken)
	assert.Equal(t, "sub-initial", claims["sub"])
	assert.NotContains(t, claims, "groups")
	status, _ := refresh(t, iss.URL, tokens.RefreshToken)
	assert.Equal(t, http.StatusOK, status, "the refresh status is restored")
	status, body := refresh(t, iss.URL, before.RefreshToken)
	assert.Equal(t, http.StatusBadRequest, status, "tokens issued before the reset are forgotten")
	assert.Equal(t, "invalid_grant", body.Error)
}

func TestNewHandler_ServesAtTheGivenIssuerURL(t *testing.T) {
	iss, handler, err := fakeoidc.NewHandler(fakeoidc.Options{
		Issuer: "http://issuer.invalid", ClientID: clientID, ClientSecret: clientSecret,
		Subject: "sub", Email: "u@example.com",
	})
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base := server.URL

	assert.Equal(t, "http://issuer.invalid", iss.URL)
	var doc map[string]any
	getJSON(t, base+"/.well-known/openid-configuration", &doc)
	assert.Equal(t, "http://issuer.invalid", doc["issuer"])
	assert.Equal(t, "http://issuer.invalid/authorize", doc["authorization_endpoint"])
	assert.Equal(t, "http://issuer.invalid", claimsOf(t, signIn(t, base).IDToken)["iss"])
}
