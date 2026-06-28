package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
)

func TestNew_Validation(t *testing.T) {
	_, err := New(Config{ClientSecret: "test-client-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client ID is required")

	_, err = New(Config{ClientID: "test-client-id"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client secret is required")
}

func TestName(t *testing.T) {
	p, err := New(Config{ClientID: "id", ClientSecret: "secret"})
	require.NoError(t, err)
	assert.Equal(t, idp.ProviderGitHub, p.Name())
}

func TestAuthorizeURL(t *testing.T) {
	p, err := New(Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://app.example.com/api/v1/auth/callback",
	})
	require.NoError(t, err)

	raw := p.AuthorizeURL("state-xyz", "", "")
	u, err := url.Parse(raw)
	require.NoError(t, err)

	assert.Equal(t, "github.com", u.Host)
	assert.Equal(t, "/login/oauth/authorize", u.Path)
	q := u.Query()
	assert.Equal(t, "test-client-id", q.Get("client_id"))
	assert.Equal(t, "https://app.example.com/api/v1/auth/callback", q.Get("redirect_uri"))
	assert.Equal(t, "read:user user:email", q.Get("scope"))
	assert.Equal(t, "state-xyz", q.Get("state"))

	// redirectURI argument overrides the configured default.
	raw = p.AuthorizeURL("s", "https://other.example.com/cb", "")
	u, err = url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "https://other.example.com/cb", u.Query().Get("redirect_uri"))
}

func TestRefresh_Unsupported(t *testing.T) {
	p, err := New(Config{ClientID: "id", ClientSecret: "secret"})
	require.NoError(t, err)

	_, err = p.Refresh(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

// fakeGitHub stands in for GitHub's token + REST endpoints.
type fakeGitHub struct {
	tokenStatus int
	tokenBody   string
	userBody    string
	emailsBody  string
}

func newFakeGitHubServer(t *testing.T, fg fakeGitHub) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		status := fg.tokenStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, err := w.Write([]byte(fg.tokenBody))
		require.NoError(t, err)
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(fg.userBody))
		require.NoError(t, err)
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(fg.emailsBody))
		require.NoError(t, err)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// withEndpoints points the package-level endpoint vars at the test server and
// restores them when the test finishes.
func withEndpoints(t *testing.T, base string) {
	t.Helper()
	origAuth, origExchange, origAPI := authorizeURL, exchangeURL, apiBaseURL
	authorizeURL = base + "/login/oauth/authorize"
	exchangeURL = base + "/login/oauth/access_token"
	apiBaseURL = base
	t.Cleanup(func() {
		authorizeURL, exchangeURL, apiBaseURL = origAuth, origExchange, origAPI
	})
}

func TestExchangeCode_PrimaryVerifiedEmail(t *testing.T) {
	srv := newFakeGitHubServer(t, fakeGitHub{
		tokenBody:  `{"access_token":"test-access-token","token_type":"bearer","scope":"read:user,user:email"}`,
		userBody:   `{"id":12345,"login":"octocat","name":"The Octocat","avatar_url":"https://avatars.example/oct.png","email":null}`,
		emailsBody: `[{"email":"secondary@example.com","primary":false,"verified":true},{"email":"octo@example.com","primary":true,"verified":true}]`,
	})
	withEndpoints(t, srv.URL)

	p, err := New(Config{ClientID: "id", ClientSecret: "secret", RedirectURL: srv.URL + "/cb"})
	require.NoError(t, err)

	tokens, claims, err := p.ExchangeCode(context.Background(), "the-code", "")
	require.NoError(t, err)

	assert.Equal(t, "test-access-token", tokens.AccessToken)
	assert.True(t, tokens.ExpiresAt.IsZero(), "GitHub tokens are long-lived; ExpiresAt must be zero")
	assert.Equal(t, "12345", claims.Subject)
	assert.Equal(t, "octo@example.com", claims.Email)
	assert.True(t, claims.EmailVerified)
	assert.Equal(t, "The Octocat", claims.Name)
	assert.Equal(t, "https://avatars.example/oct.png", claims.Picture)
}

func TestExchangeCode_NameFallsBackToLogin(t *testing.T) {
	srv := newFakeGitHubServer(t, fakeGitHub{
		tokenBody:  `{"access_token":"test-access-token"}`,
		userBody:   `{"id":7,"login":"ghost","name":"","avatar_url":""}`,
		emailsBody: `[{"email":"ghost@example.com","primary":true,"verified":true}]`,
	})
	withEndpoints(t, srv.URL)

	p, err := New(Config{ClientID: "id", ClientSecret: "secret"})
	require.NoError(t, err)

	_, claims, err := p.ExchangeCode(context.Background(), "code", "")
	require.NoError(t, err)
	assert.Equal(t, "ghost", claims.Name)
}

func TestExchangeCode_FirstVerifiedWhenNoPrimary(t *testing.T) {
	srv := newFakeGitHubServer(t, fakeGitHub{
		tokenBody:  `{"access_token":"test-access-token"}`,
		userBody:   `{"id":9,"login":"dev","name":"Dev"}`,
		emailsBody: `[{"email":"unverified@example.com","primary":true,"verified":false},{"email":"verified@example.com","primary":false,"verified":true}]`,
	})
	withEndpoints(t, srv.URL)

	p, err := New(Config{ClientID: "id", ClientSecret: "secret"})
	require.NoError(t, err)

	_, claims, err := p.ExchangeCode(context.Background(), "code", "")
	require.NoError(t, err)
	assert.Equal(t, "verified@example.com", claims.Email)
	assert.True(t, claims.EmailVerified)
}

func TestExchangeCode_TokenError(t *testing.T) {
	srv := newFakeGitHubServer(t, fakeGitHub{
		tokenBody: `{"error":"bad_verification_code","error_description":"The code passed is incorrect or expired."}`,
	})
	withEndpoints(t, srv.URL)

	p, err := New(Config{ClientID: "id", ClientSecret: "secret"})
	require.NoError(t, err)

	_, _, err = p.ExchangeCode(context.Background(), "bad-code", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad_verification_code")
}
