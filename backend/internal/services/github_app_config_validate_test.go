package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// newValidateService points the probe at a fake GitHub. apiBaseURL is not
// caller-supplied in production (the host is hardcoded), so overriding it here
// is a test seam, not a hole.
func newValidateService(
	t *testing.T, repo repositories.GitHubAppConfigRepository, baseURL string,
) *GitHubAppConfigService {
	t.Helper()
	svc := newAppConfigService(t, repo)
	svc.apiBaseURL = baseURL
	return svc
}

func fakeGitHub(t *testing.T, status int, body any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/app", r.URL.Path)
		// The probe must authenticate as the App, not as an installation.
		assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "),
			"probe must present the App JWT")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			require.NoError(t, json.NewEncoder(w).Encode(body))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func fullPermissions() map[string]string {
	return map[string]string{"contents": "read", "metadata": "read"}
}

func TestGitHubAppConfigService_Validate(t *testing.T) {
	ctx := context.Background()

	t.Run("valid credentials", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		server := fakeGitHub(t, http.StatusOK, map[string]any{
			"slug": "my-app", "permissions": fullPermissions(),
		})
		svc := newValidateService(t, repo, server.URL)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(storedConfig(t, svc), nil)

		resp, err := svc.ValidateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		require.NoError(t, err)
		assert.True(t, resp.IsValid)
		assert.Equal(t, "my-app", resp.AppSlug)
		assert.Equal(t, fullPermissions(), resp.Permissions)
		assert.Empty(t, resp.Details.ErrorDetails)
	})

	// Each of the five fixed categories. A failed probe is reported in the body,
	// never as a Go error — a wrong key is user-correctable, not a server fault.
	t.Run("error categories", func(t *testing.T) {
		cases := []struct {
			name     string
			status   int
			body     any
			expected string
		}{
			{"unauthorized", http.StatusUnauthorized, nil, githubAppErrInvalidCredentials},
			{"forbidden", http.StatusForbidden, nil, githubAppErrInvalidCredentials},
			{"app not found", http.StatusNotFound, nil, githubAppErrAppNotFound},
			{"unexpected status", http.StatusInternalServerError, nil, githubAppErrConnectionFailed},
			{
				"slug mismatch", http.StatusOK,
				map[string]any{"slug": "someone-elses-app", "permissions": fullPermissions()},
				githubAppErrSlugMismatch,
			},
			{
				"insufficient permissions", http.StatusOK,
				map[string]any{"slug": "my-app", "permissions": map[string]string{"metadata": "read"}},
				githubAppErrInsufficientPermission,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo := mocks.NewMockGitHubAppConfigRepository(t)
				server := fakeGitHub(t, tc.status, tc.body)
				svc := newValidateService(t, repo, server.URL)
				repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(storedConfig(t, svc), nil)

				resp, err := svc.ValidateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
				require.NoError(t, err, "a failed probe is a body, not an error")
				assert.False(t, resp.IsValid)
				assert.Equal(t, tc.expected, resp.Details.ErrorDetails)
				assertNoGitHubOracle(t, resp, server.URL)
			})
		}
	})

	// A slug typo would otherwise only surface as a 404 on an install URL built
	// from the wrong slug, long after the credentials were saved.
	t.Run("slug comparison is case-insensitive", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		server := fakeGitHub(t, http.StatusOK, map[string]any{
			"slug": "My-App", "permissions": fullPermissions(),
		})
		svc := newValidateService(t, repo, server.URL)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(storedConfig(t, svc), nil)

		resp, err := svc.ValidateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		require.NoError(t, err)
		assert.True(t, resp.IsValid, "GitHub's slug casing must not be treated as a mismatch")
	})

	t.Run("unreachable host is connection_failed", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		// A closed port: the probe must not report WHICH failure it hit.
		svc := newValidateService(t, repo, "http://127.0.0.1:1")
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(storedConfig(t, svc), nil)

		resp, err := svc.ValidateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		require.NoError(t, err)
		assert.False(t, resp.IsValid)
		assert.Equal(t, githubAppErrConnectionFailed, resp.Details.ErrorDetails)
		assertNoGitHubOracle(t, resp, "127.0.0.1:1")
	})

	t.Run("no config maps to the sentinel", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).
			Return(nil, repositories.ErrGitHubAppConfigNotFound)

		_, err := svc.ValidateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		assert.ErrorIs(t, err, ErrGitHubAppNotConfigured)
	})

	// A key that decrypts but is not a usable RSA key must still be a body-level
	// category, not a 500 and not a leak of the parse error.
	t.Run("unusable stored key is invalid_credentials", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)
		garbage, err := svc.encrypt("not a pem at all")
		require.NoError(t, err)
		stored.PrivateKeyEncrypted = garbage
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)

		resp, err := svc.ValidateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		require.NoError(t, err)
		assert.False(t, resp.IsValid)
		assert.Equal(t, githubAppErrInvalidCredentials, resp.Details.ErrorDetails)
	})
}

// assertNoGitHubOracle pins that the response is one of the fixed categories and
// leaks neither the target nor connection-level wording — the same discipline as
// assertNoOracle for the provider probes (#464). Without it, the response
// separates "closed port" from "open but unauthorized", which is a scanner.
func assertNoGitHubOracle(t *testing.T, resp *models.ValidateGitHubAppResponse, target string) {
	t.Helper()

	blob := resp.Message + " " + resp.Details.ErrorDetails
	assert.NotContains(t, blob, target, "response must not echo the target")
	for _, leak := range []string{"refused", "no such host", "dial tcp", "timeout", "lookup", "EOF"} {
		assert.NotContains(t, strings.ToLower(blob), leak,
			"response must not reveal the connection outcome")
	}
	assert.Contains(t, []string{
		githubAppErrInvalidCredentials,
		githubAppErrAppNotFound,
		githubAppErrSlugMismatch,
		githubAppErrInsufficientPermission,
		githubAppErrConnectionFailed,
	}, resp.Details.ErrorDetails, "error_details must be one of the fixed categories")
}
