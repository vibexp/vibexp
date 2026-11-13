package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// pkceHarness is the resolver harness with one OIDC provider ("corp-sso") over
// a fake issuer that enforces PKCE and echoes the nonce.
func pkceHarness(t *testing.T) (*resolverAuthHarness, *fakeoidc.Issuer) {
	t.Helper()
	h := newResolverAuthHarness(t)
	iss := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "ada@example.com")
	h.providers.replace(h.oidcRow(t, "id-a", "corp-sso", iss.URL, 0))
	return h, iss
}

// loginQuery runs GET /auth/login for corp-sso and returns the authorization
// URL's query and the state cookie.
func loginQuery(t *testing.T, h *resolverAuthHarness) (url.Values, *http.Cookie) {
	t.Helper()
	w := httptest.NewRecorder()
	h.srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login?provider=corp-sso", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp LoginResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	u, err := url.Parse(resp.URL)
	require.NoError(t, err)
	for _, c := range w.Result().Cookies() {
		if c.Name == stateCookieName {
			return u.Query(), c
		}
	}
	t.Fatal("no state cookie")
	return nil, nil
}

func TestLogin_SendsPKCEAndNonceBoundToTheStateCookie(t *testing.T) {
	h, _ := pkceHarness(t)
	q, cookie := loginQuery(t, h)

	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Len(t, q.Get("code_challenge"), 43)
	nonce, err := base64.RawURLEncoding.DecodeString(q.Get("nonce"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(nonce), 16, "the nonce carries at least 128 bits")
	assert.False(t, q.Has("access_type"), "a generic OIDC provider gets no access_type")

	// The cookie carries the verifier behind the challenge and the same nonce.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	bound, err := h.srv.validateStateCookie(req, q.Get("state"))
	require.NoError(t, err)
	assert.Equal(t, "corp-sso", bound.provider)
	assert.Equal(t, oauth2.S256ChallengeFromVerifier(bound.flow.CodeVerifier), q.Get("code_challenge"))
	assert.Equal(t, q.Get("nonce"), bound.flow.Nonce)

	// Every login gets a fresh verifier and nonce.
	q2, _ := loginQuery(t, h)
	assert.NotEqual(t, q.Get("code_challenge"), q2.Get("code_challenge"))
	assert.NotEqual(t, q.Get("nonce"), q2.Get("nonce"))
}

func TestCallback_PKCEAndNonceRoundTripSignsIn(t *testing.T) {
	h, _ := pkceHarness(t)
	state, cookie := h.login(t, "corp-sso")

	w := h.callback(t, state, cookie)

	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	require.NotNil(t, sessionCookie(w))
}

func TestCallback_NonceMismatchIsRefusedAndAudited(t *testing.T) {
	for name, override := range map[string]string{"different nonce": "replayed-nonce", "missing nonce": ""} {
		t.Run(name, func(t *testing.T) {
			h, iss := pkceHarness(t)
			rec := withAudit(h.srv)
			state, cookie := h.login(t, "corp-sso")
			iss.SetNonceOverride(override)

			w := h.callback(t, state, cookie)

			assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
			assert.Nil(t, sessionCookie(w), "no session is issued")
			assert.Empty(t, h.users.users, "no user is created")
			got := rec.only(t)
			assert.Equal(t, models.AuditOpAuthLoginFailed, got.event.Operation)
			assert.Equal(t, callbackFailureNonceMismatch, got.event.Metadata[auditMetaReason])
			assert.Equal(t, "corp-sso", got.event.Metadata[auditMetaProvider])
		})
	}
}

// A state cookie whose verifier or nonce was altered fails the HMAC check like
// a tampered state, and so does a correctly signed cookie of the pre-PKCE
// "<state>:<provider>" shape.
func TestCallback_TamperedOrLegacyStateCookieIsRejected(t *testing.T) {
	h, _ := pkceHarness(t)
	state, cookie := h.login(t, "corp-sso")
	payload, sig, _ := strings.Cut(cookie.Value, ".")
	fields := strings.Split(payload, ":")
	require.Len(t, fields, stateCookieFields)

	tamper := func(i int) string {
		f := append([]string(nil), fields...)
		f[i] = oauth2.GenerateVerifier()
		return strings.Join(f, ":") + "." + sig
	}
	legacy := func() string {
		p := state + ":corp-sso"
		mac := hmac.New(sha256.New, h.srv.stateMACKey())
		_, _ = mac.Write([]byte(p))
		return p + "." + hex.EncodeToString(mac.Sum(nil))
	}
	for name, value := range map[string]string{
		"tampered verifier": tamper(2),
		"tampered nonce":    tamper(3),
		"legacy two-field":  legacy(),
	} {
		t.Run(name, func(t *testing.T) {
			w := h.callback(t, state, &http.Cookie{Name: stateCookieName, Value: value})

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Contains(t, w.Body.String(), "Invalid or expired state parameter")
			assert.Nil(t, sessionCookie(w))
		})
	}
}
