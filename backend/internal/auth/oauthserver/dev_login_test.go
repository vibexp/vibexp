package oauthserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
)

// newDevLoginHarness builds an Authorization Server with NO identity providers
// configured and the dev-login bypass gated by devEnabled, mirroring a local
// zero-config setup (development env, DEV_LOGIN_ENABLED=true, no AUTH_PROVIDERS).
func newDevLoginHarness(t *testing.T, devEnabled bool) *testHarness {
	t.Helper()
	encKey := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	clients := newMemClientRepo()
	svc := NewService(
		Config{
			Issuer:              testIssuer,
			ResourceURI:         testResourceURI,
			AccessTokenTTL:      15 * time.Minute,
			RefreshTokenTTL:     24 * time.Hour,
			AuthCodeTTL:         10 * time.Minute,
			KeyRotationInterval: 720 * time.Hour,
			DevLoginEnabled:     devEnabled,
		},
		encKey,
		clients,
		newMemRequestRepo(), newMemRequestRepo(), newMemRequestRepo(), newMemRequestRepo(),
		newMemSigningKeyRepo(),
		newMemLoginSessionRepo(),
		idp.NewRegistry(), // no providers enabled
		&fakeProvisioner{user: &models.User{ID: "user-1"}},
		slog.New(slog.DiscardHandler),
	)
	require.NoError(t, svc.keys.EnsureActiveKey(context.Background()))

	return &testHarness{
		svc:    svc,
		server: httptest.NewServer(testMux(svc)),
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		clientID: registerTestClient(t, clients),
	}
}

func (h *testHarness) devAuthorizeQuery() url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {h.clientID},
		"redirect_uri":          {testRedirectURI},
		"state":                 {"state-abcdefgh"},
		"resource":              {testResourceURI},
		"code_challenge":        {pkceChallenge()},
		"code_challenge_method": {"S256"},
	}
}

// TestDevLoginAuthorizeFlow_IssuesTokenAsDevUser is the end-to-end proof of the
// zero-config local path: with no identity provider, /authorize takes the
// dev-login bypass (no upstream IdP redirect), goes straight to consent, and the
// issued token's subject is the provisioned dev user.
func TestDevLoginAuthorizeFlow_IssuesTokenAsDevUser(t *testing.T) {
	h := newDevLoginHarness(t, true)
	defer h.close()

	// /authorize redirects straight to the consent screen, not to an upstream IdP.
	consentLoc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+h.devAuthorizeQuery().Encode()))
	require.Contains(t, consentLoc, ConsentPath, "dev login must go to consent without an upstream IdP")
	loginID := queryParam(t, consentLoc, "login")
	require.NotEmpty(t, loginID)

	// Approve consent → authorization code → token.
	form := url.Values{
		"login":  {loginID},
		"csrf":   {h.svc.signConsent(loginID)},
		"action": {"approve"},
	}
	clientRedirect := h.requireRedirect(t, h.postForm(t, ConsentPath, form))
	code := queryParam(t, clientRedirect, "code")
	require.NotEmpty(t, code, "consent approval must issue an authorization code")

	res := h.exchangeCode(t, code, testVerifier)
	require.Equal(t, http.StatusOK, res.status)
	accessToken, _ := jsonBody(t, res)["access_token"].(string)
	require.NotEmpty(t, accessToken)

	claims := h.verifyJWTAgainstJWKS(t, accessToken)
	assert.Equal(t, "user-1", claims["sub"], "token must run as the provisioned dev user")
	assert.Contains(t, audienceStrings(claims["aud"]), testResourceURI)

	prov := h.svc.provisioner.(*fakeProvisioner)
	assert.Equal(t, 1, prov.devLoginCalls, "the dev-login leg must have provisioned the user")
	assert.Equal(t, devLoginEmail, prov.lastDevEmail)
}

// TestDevLoginAuthorize_DisabledWithoutFlag asserts the dev-login bypass is
// unreachable when DevLoginEnabled is false (the production posture): with no
// provider, /authorize fails with an OAuth error and never provisions a dev user.
func TestDevLoginAuthorize_DisabledWithoutFlag(t *testing.T) {
	h := newDevLoginHarness(t, false)
	defer h.close()

	loc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+h.devAuthorizeQuery().Encode()))
	assert.Contains(t, loc, "error=", "no provider + dev login off must reject the request")
	assert.NotContains(t, loc, ConsentPath, "dev login must not be offered when disabled")

	prov := h.svc.provisioner.(*fakeProvisioner)
	assert.Equal(t, 0, prov.devLoginCalls, "no dev user may be provisioned when dev login is off")
}

// TestDevLoginAuthorize_ProvisioningError asserts the dev-login leg fails closed:
// when provisioning the dev user errors, /authorize surfaces an OAuth server_error
// and never reaches consent (no authorization code is issued).
func TestDevLoginAuthorize_ProvisioningError(t *testing.T) {
	h := newDevLoginHarness(t, true)
	defer h.close()
	h.svc.provisioner.(*fakeProvisioner).devLoginErr = errors.New("boom")

	loc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+h.devAuthorizeQuery().Encode()))
	assert.Contains(t, loc, "error=server_error", "provisioning failure must surface server_error")
	assert.NotContains(t, loc, ConsentPath, "a failed dev login must not reach consent")
}
