// Tests for the user-authorization leg of the install flow (#463), driven
// against an httptest server via the baseURL / oauthBaseURL seams. No live
// network calls.
package implementations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v57/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// newUserAuthTestClient starts an httptest server and returns its mux plus a
// GitHubAppClient wired to it for both the API and the OAuth token endpoint.
// clientID/clientSecret empty models an instance with user auth unconfigured.
func newUserAuthTestClient(t *testing.T, clientID, clientSecret string) (*http.ServeMux, *GitHubAppClient) {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	key, keyPEM := githubAPITestKey()
	return mux, &GitHubAppClient{
		cfg: &config.GitHubAppConfig{
			AppID:         "12345",
			PrivateKey:    key,
			PrivateKeyPEM: keyPEM,
			ClientID:      clientID,
			ClientSecret:  clientSecret,
		},
		logger:       slog.New(slog.DiscardHandler),
		httpClient:   srv.Client(),
		clientCache:  make(map[int64]*github.Client),
		baseURL:      srv.URL,
		oauthBaseURL: srv.URL,
	}
}

func TestExchangeUserCode_Success(t *testing.T) {
	mux, client := newUserAuthTestClient(t, "Iv1.test", "secret")

	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "Iv1.test", r.PostForm.Get("client_id"))
		assert.Equal(t, "secret", r.PostForm.Get("client_secret"))
		assert.Equal(t, "the-code", r.PostForm.Get("code"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		writeBody(t, w, `{"access_token":"gho_user_token","token_type":"bearer"}`)
	})

	token, err := client.ExchangeUserCode(context.Background(), "the-code")

	require.NoError(t, err)
	assert.Equal(t, "gho_user_token", token)
}

// TestExchangeUserCode_NotConfigured verifies the client refuses before making
// any request when no OAuth credentials are set, so the caller can fail closed.
func TestExchangeUserCode_NotConfigured(t *testing.T) {
	tests := []struct {
		name                   string
		clientID, clientSecret string
	}{
		{"both empty", "", ""},
		{"id only", "Iv1.test", ""},
		{"secret only", "", "secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, client := newUserAuthTestClient(t, tt.clientID, tt.clientSecret)
			mux.HandleFunc("POST /login/oauth/access_token", func(http.ResponseWriter, *http.Request) {
				t.Error("token endpoint must not be called when user auth is unconfigured")
			})

			_, err := client.ExchangeUserCode(context.Background(), "the-code")

			assert.True(t, errors.Is(err, external.ErrGitHubUserAuthNotConfigured),
				"expected ErrGitHubUserAuthNotConfigured, got: %v", err)
		})
	}
}

// TestExchangeUserCode_Rejected covers GitHub's habit of reporting a bad code
// as HTTP 200 with an `error` field, which a status-code-only check would read
// as success.
func TestExchangeUserCode_Rejected(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"error payload", `{"error":"bad_verification_code","error_description":"expired"}`},
		{"empty token", `{"access_token":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, client := newUserAuthTestClient(t, "Iv1.test", "secret")
			mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				writeBody(t, w, tt.body)
			})

			token, err := client.ExchangeUserCode(context.Background(), "the-code")

			assert.True(t, errors.Is(err, external.ErrGitHubUserCodeInvalid),
				"expected ErrGitHubUserCodeInvalid, got: %v", err)
			assert.Empty(t, token)
		})
	}
}

func TestExchangeUserCode_UpstreamStatusError(t *testing.T) {
	mux, client := newUserAuthTestClient(t, "Iv1.test", "secret")
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := client.ExchangeUserCode(context.Background(), "the-code")

	require.Error(t, err)
	// A 5xx upstream is a transport failure, not a denial — it must not be
	// mistaken for "the code is invalid".
	assert.False(t, errors.Is(err, external.ErrGitHubUserCodeInvalid))
}

// writeUserInstallationsPage writes one page of GET /user/installations.
func writeUserInstallationsPage(t *testing.T, w http.ResponseWriter, ids ...int64) {
	t.Helper()
	body := `{"total_count":%d,"installations":[`
	for i, id := range ids {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"id":%d}`, id)
	}
	body += "]}"
	w.Header().Set("Content-Type", "application/json")
	writeBody(t, w, fmt.Sprintf(body, len(ids)))
}

func TestUserCanAccessInstallation(t *testing.T) {
	tests := []struct {
		name           string
		listed         []int64
		installationID int64
		want           bool
	}{
		{"listed", []int64{111, 222}, 222, true},
		{"not listed", []int64{111, 222}, 999, false},
		{"empty list", nil, 111, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, client := newUserAuthTestClient(t, "Iv1.test", "secret")
			mux.HandleFunc("GET /api/v3/user/installations", func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer gho_user_token", r.Header.Get("Authorization"),
					"the list must be fetched as the USER, not as the app")
				writeUserInstallationsPage(t, w, tt.listed...)
			})

			ok, err := client.UserCanAccessInstallation(
				context.Background(), "gho_user_token", tt.installationID)

			require.NoError(t, err)
			assert.Equal(t, tt.want, ok)
		})
	}
}

// TestUserCanAccessInstallation_Paginates verifies a match on a later page
// is still found — a user with many installations must not be denied wrongly.
func TestUserCanAccessInstallation_Paginates(t *testing.T) {
	mux, client := newUserAuthTestClient(t, "Iv1.test", "secret")

	mux.HandleFunc("GET /api/v3/user/installations", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			writeUserInstallationsPage(t, w, 777)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v3/user/installations?page=2>; rel="next"`, client.baseURL))
		writeUserInstallationsPage(t, w, 111)
	})

	ok, err := client.UserCanAccessInstallation(context.Background(), "gho_user_token", 777)

	require.NoError(t, err)
	assert.True(t, ok, "an installation on page 2 must still be found")
}

func TestUserCanAccessInstallation_UpstreamError(t *testing.T) {
	mux, client := newUserAuthTestClient(t, "Iv1.test", "secret")
	mux.HandleFunc("GET /api/v3/user/installations", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	ok, err := client.UserCanAccessInstallation(context.Background(), "gho_user_token", 111)

	require.Error(t, err)
	assert.False(t, ok, "an error must never read as authorized")
}

// TestStubGitHubAppClient_UserAuthFailsClosed pins the stub used when no GitHub
// App is configured: it must deny, never allow.
func TestStubGitHubAppClient_UserAuthFailsClosed(t *testing.T) {
	stub := &stubGitHubAppClient{}

	_, err := stub.ExchangeUserCode(context.Background(), "the-code")
	assert.True(t, errors.Is(err, external.ErrGitHubUserAuthNotConfigured),
		"expected ErrGitHubUserAuthNotConfigured, got: %v", err)

	ok, err := stub.UserCanAccessInstallation(context.Background(), "token", 111)
	assert.Error(t, err)
	assert.False(t, ok)
}
