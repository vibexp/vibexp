package implementations

// API-facing tests for GitHubAppClient: every test drives the real go-github /
// ghinstallation stack against an httptest.Server returning canned GitHub API
// responses via the baseURL test seam (issue #364). No live network calls.
//
// go-github's WithEnterpriseURLs appends "/api/v3/" to the base URL, so API
// handlers register under "/api/v3/...". ghinstallation's token minting uses
// the base URL verbatim, so token handlers register under
// "/app/installations/{id}/access_tokens".

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// githubAPITestKey lazily generates one RSA key shared by all API tests:
// generateJWT needs the parsed *rsa.PrivateKey and ghinstallation needs the
// PEM bytes, so both are derived from the same key.
var githubAPITestKey = sync.OnceValues(func() (*rsa.PrivateKey, []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("generating test RSA key: %v", err))
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return key, keyPEM
})

// newGitHubAPITestClient starts an httptest server and returns its mux plus a
// GitHubAppClient whose baseURL seam points at the server.
func newGitHubAPITestClient(t *testing.T) (*http.ServeMux, *GitHubAppClient) {
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
		},
		logger:      slog.New(slog.DiscardHandler),
		clientCache: make(map[int64]*github.Client),
		baseURL:     srv.URL,
	}
}

// writeBody writes a canned response body, keeping errcheck satisfied.
func writeBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	_, err := w.Write([]byte(body))
	assert.NoError(t, err)
}

// grantInstallationToken registers a successful installation-token mint for
// installationID, which ghinstallation performs before any installation-authed
// API call.
func grantInstallationToken(t *testing.T, mux *http.ServeMux, installationID int64) {
	t.Helper()
	pattern := fmt.Sprintf("POST /app/installations/%d/access_tokens", installationID)
	mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		writeBody(t, w, fmt.Sprintf(`{"token":"ghs_test","expires_at":%q}`,
			time.Now().Add(time.Hour).Format(time.RFC3339)))
	})
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func TestGetInstallationToken(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantToken string
		wantErr   string
	}{
		{
			name:      "201 returns token and expiry",
			status:    http.StatusCreated,
			body:      `{"token":"ghs_minted","expires_at":"2026-03-01T12:00:00Z"}`,
			wantToken: "ghs_minted",
		},
		{
			name:    "404 installation not found",
			status:  http.StatusNotFound,
			body:    `{"message":"Not Found"}`,
			wantErr: "failed to create installation token",
		},
		{
			name:    "422 validation failed",
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"Validation Failed"}`,
			wantErr: "failed to create installation token",
		},
		{
			name:    "2xx other than 201 is rejected",
			status:  http.StatusOK,
			body:    `{"token":"ghs_minted","expires_at":"2026-03-01T12:00:00Z"}`,
			wantErr: "unexpected status code: 200",
		},
		{
			name:    "malformed body",
			status:  http.StatusCreated,
			body:    `{"token": not-json`,
			wantErr: "failed to create installation token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, c := newGitHubAPITestClient(t)
			mux.HandleFunc("POST /api/v3/app/installations/11/access_tokens",
				func(w http.ResponseWriter, r *http.Request) {
					assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "),
						"token mint must authenticate with the app JWT")
					w.WriteHeader(tt.status)
					writeBody(t, w, tt.body)
				})

			token, expiresAt, err := c.GetInstallationToken(context.Background(), 11)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
			assert.Equal(t, time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), expiresAt.UTC())
		})
	}
}

func TestGetInstallationRepositories_HappyPath(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	grantInstallationToken(t, mux, 21)

	var gotAuth string
	mux.HandleFunc("GET /api/v3/installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeBody(t, w, `{
			"total_count": 3,
			"repositories": [
				{"id": 101, "name": "vibexp", "full_name": "vibexp/vibexp", "private": true,
				 "description": "AI command center", "html_url": "https://github.com/vibexp/vibexp",
				 "owner": {"login": "vibexp", "type": "Organization"}},
				{"id": 102, "name": "docs", "full_name": "vibexp/docs", "private": false,
				 "html_url": "https://github.com/vibexp/docs",
				 "owner": {"login": "octocat", "type": "User"}},
				{"id": 103, "name": "orphan", "full_name": "x/orphan"}
			]
		}`)
	})

	repos, total, err := c.GetInstallationRepositories(context.Background(), 21, 1)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, repos, 2, "the repository with a nil owner must be skipped")

	first := repos[0]
	assert.Equal(t, int64(101), first.ID)
	assert.Equal(t, "vibexp", first.Name)
	assert.Equal(t, "vibexp/vibexp", first.FullName)
	require.NotNil(t, first.Description)
	assert.Equal(t, "AI command center", *first.Description)
	assert.True(t, first.Private)
	assert.Equal(t, "https://github.com/vibexp/vibexp", first.HTMLURL)
	assert.Equal(t, "vibexp", first.Owner.Login)
	assert.Equal(t, "Organization", first.Owner.Type)

	second := repos[1]
	assert.Equal(t, int64(102), second.ID)
	assert.Nil(t, second.Description)
	assert.False(t, second.Private)
	assert.Equal(t, "octocat", second.Owner.Login)
	assert.Equal(t, "User", second.Owner.Type)

	assert.Equal(t, "token ghs_test", gotAuth, "listing must use the minted installation token")
}

func TestGetInstallationRepositories_PaginationPassthrough(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	grantInstallationToken(t, mux, 22)

	var gotQuery url.Values
	mux.HandleFunc("GET /api/v3/installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		writeBody(t, w, `{"total_count": 42, "repositories": []}`)
	})

	repos, total, err := c.GetInstallationRepositories(context.Background(), 22, 5)
	require.NoError(t, err)
	assert.Empty(t, repos)
	assert.Equal(t, 42, total)
	assert.Equal(t, "5", gotQuery.Get("page"))
	assert.Equal(t, "100", gotQuery.Get("per_page"))
}

func TestGetInstallationRepositories_InstallationGone(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	// No token grant: the transport's token exchange itself 404s, which is
	// GitHub's definitive signal that the App installation was uninstalled.
	mux.HandleFunc("POST /app/installations/23/access_tokens",
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			writeBody(t, w, `{"message":"Not Found"}`)
		})

	_, _, err := c.GetInstallationRepositories(context.Background(), 23, 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, external.ErrGitHubInstallationGone)
}

func TestGetInstallationRepositories_PlainAPIErrorIsNotGone(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	grantInstallationToken(t, mux, 24)
	mux.HandleFunc("GET /api/v3/installation/repositories",
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeBody(t, w, `{"message":"boom"}`)
		})

	_, _, err := c.GetInstallationRepositories(context.Background(), 24, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list repositories")
	assert.NotErrorIs(t, err, external.ErrGitHubInstallationGone,
		"a plain API failure must not be classified as installation removal")
}

func TestGetRepository_HappyPath(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	grantInstallationToken(t, mux, 31)
	mux.HandleFunc("GET /api/v3/repositories/555", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `{"id": 555, "name": "cli", "full_name": "vibexp/cli", "private": true,
			"description": "VibeXP CLI", "html_url": "https://github.com/vibexp/cli",
			"owner": {"login": "vibexp", "type": "Organization"}}`)
	})

	repo, err := c.GetRepository(context.Background(), 31, 555)
	require.NoError(t, err)
	assert.Equal(t, int64(555), repo.ID)
	assert.Equal(t, "cli", repo.Name)
	assert.Equal(t, "vibexp/cli", repo.FullName)
	require.NotNil(t, repo.Description)
	assert.Equal(t, "VibeXP CLI", *repo.Description)
	assert.True(t, repo.Private)
	assert.Equal(t, "https://github.com/vibexp/cli", repo.HTMLURL)
	assert.Equal(t, "vibexp", repo.Owner.Login)
	assert.Equal(t, "Organization", repo.Owner.Type)
}

func TestGetRepository_Errors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "not found",
			status:  http.StatusNotFound,
			body:    `{"message":"Not Found"}`,
			wantErr: "failed to get repository",
		},
		{
			name:    "nil owner",
			status:  http.StatusOK,
			body:    `{"id": 556, "name": "noowner", "full_name": "x/noowner"}`,
			wantErr: "repository has no owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, c := newGitHubAPITestClient(t)
			grantInstallationToken(t, mux, 32)
			mux.HandleFunc("GET /api/v3/repositories/556", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				writeBody(t, w, tt.body)
			})

			_, err := c.GetRepository(context.Background(), 32, 556)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestGetInstallation_HappyPath(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	mux.HandleFunc("GET /api/v3/app/installations/41", func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "),
			"GetInstallation must authenticate with the app JWT")
		writeBody(t, w, `{
			"account": {"login": "vibexp", "type": "Organization"},
			"target_type": "Organization",
			"permissions": {"contents": "read", "metadata": "read",
				"pull_requests": "write", "issues": "write", "checks": "read"},
			"events": ["push", "pull_request"],
			"suspended_at": "2026-01-02T03:04:05Z"
		}`)
	})

	info, err := c.GetInstallation(context.Background(), 41)
	require.NoError(t, err)
	assert.Equal(t, "vibexp", info.AccountLogin)
	assert.Equal(t, "Organization", info.AccountType)
	assert.Equal(t, "Organization", info.TargetType)
	assert.Equal(t, map[string]string{
		"contents":      "read",
		"metadata":      "read",
		"pull_requests": "write",
		"issues":        "write",
	}, info.Permissions, "only the four mapped permissions are surfaced")
	assert.Equal(t, []string{"push", "pull_request"}, info.Events)
	require.NotNil(t, info.SuspendedAt)
	assert.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), info.SuspendedAt.UTC())
}

func TestGetInstallation_MinimalPayload(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	mux.HandleFunc("GET /api/v3/app/installations/42", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `{"account": {"login": "octocat", "type": "User"}, "target_type": "User"}`)
	})

	info, err := c.GetInstallation(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "octocat", info.AccountLogin)
	assert.Equal(t, "User", info.AccountType)
	assert.Equal(t, "User", info.TargetType)
	assert.Empty(t, info.Permissions)
	assert.Empty(t, info.Events)
	assert.Nil(t, info.SuspendedAt, "an installation without suspended_at is not suspended")
}

func TestGetInstallation_NotFound(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	mux.HandleFunc("GET /api/v3/app/installations/43", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeBody(t, w, `{"message":"Not Found"}`)
	})

	_, err := c.GetInstallation(context.Background(), 43)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get installation")
}

func TestGetFileContent(t *testing.T) {
	fileJSON := fmt.Sprintf(
		`{"type":"file","name":"readme.md","path":"docs/readme.md","sha":"blobsha123","encoding":"base64","content":%q}`,
		b64("hello vibexp\n"))

	tests := []struct {
		name        string
		status      int
		body        string
		wantContent string
		wantSHA     string
		wantErr     string
	}{
		{
			name:        "base64 file content is decoded and blob SHA captured",
			status:      http.StatusOK,
			body:        fileJSON,
			wantContent: "hello vibexp\n",
			wantSHA:     "blobsha123",
		},
		{
			name:    "not found",
			status:  http.StatusNotFound,
			body:    `{"message":"Not Found"}`,
			wantErr: "failed to get file content",
		},
		{
			name:    "path is a directory",
			status:  http.StatusOK,
			body:    `[{"type":"file","path":"docs/readme.md"}]`,
			wantErr: "file not found: docs/readme.md",
		},
		{
			name:    "invalid base64 content",
			status:  http.StatusOK,
			body:    `{"type":"file","path":"docs/readme.md","encoding":"base64","content":"%%%"}`,
			wantErr: "failed to decode file content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, c := newGitHubAPITestClient(t)
			grantInstallationToken(t, mux, 51)
			mux.HandleFunc("GET /api/v3/repos/vibexp/docs/contents/docs/readme.md",
				func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tt.status)
					writeBody(t, w, tt.body)
				})

			file, err := c.GetFileContent(context.Background(), 51, "vibexp", "docs", "docs/readme.md")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "docs/readme.md", file.Path)
			assert.Equal(t, tt.wantContent, file.Content)
			assert.Equal(t, tt.wantSHA, file.BlobSHA)
		})
	}
}

func TestGetBranchHeadSHA(t *testing.T) {
	t.Run("resolves head commit SHA", func(t *testing.T) {
		mux, c := newGitHubAPITestClient(t)
		grantInstallationToken(t, mux, 71)
		mux.HandleFunc("GET /api/v3/repos/o/r/git/ref/heads/main",
			func(w http.ResponseWriter, _ *http.Request) {
				writeBody(t, w, `{"ref":"refs/heads/main","object":{"type":"commit","sha":"commitsha789"}}`)
			})

		sha, err := c.GetBranchHeadSHA(context.Background(), 71, "o", "r", "main")
		require.NoError(t, err)
		assert.Equal(t, "commitsha789", sha)
	})

	t.Run("API error is surfaced", func(t *testing.T) {
		mux, c := newGitHubAPITestClient(t)
		grantInstallationToken(t, mux, 72)
		mux.HandleFunc("GET /api/v3/repos/o/r/git/ref/heads/main",
			func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				writeBody(t, w, `{"message":"Not Found"}`)
			})

		_, err := c.GetBranchHeadSHA(context.Background(), 72, "o", "r", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve branch head SHA")
	})
}

func TestGetDirectoryContentsRecursive_NestedDirectories(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	grantInstallationToken(t, mux, 61)

	mux.HandleFunc("GET /api/v3/repos/o/r/contents/.claude", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `[
			{"type":"file","name":"a.md","path":".claude/a.md"},
			{"type":"dir","name":"sub","path":".claude/sub"}
		]`)
	})
	mux.HandleFunc("GET /api/v3/repos/o/r/contents/.claude/a.md", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, fmt.Sprintf(
			`{"type":"file","path":".claude/a.md","sha":"sha-a","encoding":"base64","content":%q}`, b64("alpha")))
	})
	mux.HandleFunc("GET /api/v3/repos/o/r/contents/.claude/sub", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `[{"type":"file","name":"b.md","path":".claude/sub/b.md"}]`)
	})
	mux.HandleFunc("GET /api/v3/repos/o/r/contents/.claude/sub/b.md", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, fmt.Sprintf(
			`{"type":"file","path":".claude/sub/b.md","sha":"sha-b","encoding":"base64","content":%q}`, b64("beta")))
	})

	files, err := c.GetDirectoryContentsRecursive(context.Background(), 61, "o", "r", ".claude")
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, ".claude/a.md", files[0].Path)
	assert.Equal(t, "alpha", files[0].Content)
	assert.Equal(t, "sha-a", files[0].BlobSHA)
	assert.Equal(t, ".claude/sub/b.md", files[1].Path)
	assert.Equal(t, "beta", files[1].Content)
	assert.Equal(t, "sha-b", files[1].BlobSHA)
}

func TestGetDirectoryContentsRecursive_SkipsFailingEntries(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	grantInstallationToken(t, mux, 62)

	mux.HandleFunc("GET /api/v3/repos/o/r/contents/d", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `[
			{"type":"file","name":"broken.md","path":"d/broken.md"},
			{"type":"file","name":"bad64.md","path":"d/bad64.md"},
			{"type":"dir","name":"gone","path":"d/gone"},
			{"type":"file","name":"ok.md","path":"d/ok.md"}
		]`)
	})
	// Individual file fetch fails with a server error.
	mux.HandleFunc("GET /api/v3/repos/o/r/contents/d/broken.md",
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeBody(t, w, `{"message":"boom"}`)
		})
	// File fetch succeeds but the content cannot be decoded.
	mux.HandleFunc("GET /api/v3/repos/o/r/contents/d/bad64.md",
		func(w http.ResponseWriter, _ *http.Request) {
			writeBody(t, w, `{"type":"file","path":"d/bad64.md","encoding":"base64","content":"%%%"}`)
		})
	// Subdirectory listing fails with a server error.
	mux.HandleFunc("GET /api/v3/repos/o/r/contents/d/gone",
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeBody(t, w, `{"message":"boom"}`)
		})
	mux.HandleFunc("GET /api/v3/repos/o/r/contents/d/ok.md",
		func(w http.ResponseWriter, _ *http.Request) {
			writeBody(t, w, fmt.Sprintf(
				`{"type":"file","path":"d/ok.md","encoding":"base64","content":%q}`, b64("survivor")))
		})

	files, err := c.GetDirectoryContentsRecursive(context.Background(), 62, "o", "r", "d")
	require.NoError(t, err, "individual entry failures must not fail the whole walk")
	require.Len(t, files, 1)
	assert.Equal(t, "d/ok.md", files[0].Path)
	assert.Equal(t, "survivor", files[0].Content)
}

func TestGetDirectoryContentsRecursive_TopLevelListingError(t *testing.T) {
	mux, c := newGitHubAPITestClient(t)
	grantInstallationToken(t, mux, 63)
	mux.HandleFunc("GET /api/v3/repos/o/r/contents/missing",
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			writeBody(t, w, `{"message":"Not Found"}`)
		})

	_, err := c.GetDirectoryContentsRecursive(context.Background(), 63, "o", "r", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get directory contents for missing")
}

func TestEvictCachedClient(t *testing.T) {
	c := newTestGitHubAppClient(t)
	c.clientCache[7] = github.NewClient(nil)

	c.EvictCachedClient(7)

	_, ok := c.clientCache[7]
	assert.False(t, ok, "eviction must remove the cached client")

	assert.NotPanics(t, func() {
		(&stubGitHubAppClient{}).EvictCachedClient(7)
	}, "stub eviction is a no-op")
}

// TestFetchDirectoryRecursive_Caps exercises the maxFiles and depth guards
// directly: both short-circuit before any API call, so no server is needed.
func TestFetchDirectoryRecursive_Caps(t *testing.T) {
	c := newTestGitHubAppClient(t)

	t.Run("max files reached", func(t *testing.T) {
		files := []*external.GitHubFile{{Path: "kept.md", Content: "x"}}
		fetch := &directoryFetch{allFiles: &files, maxFiles: 1}
		require.NoError(t, c.fetchDirectoryRecursive(context.Background(), fetch, "dir", 5))
		assert.Len(t, files, 1, "collection must stop at the cap")
	})

	t.Run("depth exhausted", func(t *testing.T) {
		var files []*external.GitHubFile
		fetch := &directoryFetch{allFiles: &files, maxFiles: 500}
		require.NoError(t, c.fetchDirectoryRecursive(context.Background(), fetch, "dir", 0))
		assert.Empty(t, files, "no files may be collected below the depth limit")
	})
}

// TestBaseURLSeam_InvalidURLPropagates covers the withBaseURL error branch in
// each of its three call sites: an unparsable base URL must surface as an
// error rather than silently falling back to github.com.
func TestBaseURLSeam_InvalidURLPropagates(t *testing.T) {
	key, keyPEM := githubAPITestKey()
	c := &GitHubAppClient{
		cfg: &config.GitHubAppConfig{
			AppID:         "12345",
			PrivateKey:    key,
			PrivateKeyPEM: keyPEM,
		},
		logger:      slog.New(slog.DiscardHandler),
		clientCache: make(map[int64]*github.Client),
		baseURL:     "://not-a-url",
	}

	_, _, err := c.GetInstallationToken(context.Background(), 1)
	require.ErrorContains(t, err, "failed to apply GitHub base URL")

	_, err = c.GetInstallation(context.Background(), 1)
	require.ErrorContains(t, err, "failed to apply GitHub base URL")

	_, _, err = c.GetInstallationRepositories(context.Background(), 1, 1)
	require.ErrorContains(t, err, "failed to apply GitHub base URL")
}
