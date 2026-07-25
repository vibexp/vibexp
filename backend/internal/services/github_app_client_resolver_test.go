package services

import (
	"bytes"
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// fakeResolvedClient records the credentials it was built from, so tests can
// prove WHICH App a resolved client belongs to without reaching GitHub.
type fakeResolvedClient struct {
	external.GitHubAppClient
	appID        string
	clientID     string
	clientSecret string
	privateKey   *rsa.PrivateKey
}

// newResolverForTest returns a resolver whose client construction is observable.
// built counts constructions, which is how the caching assertions distinguish a
// cache hit from a silent rebuild.
func newResolverForTest(
	t *testing.T, repo repositories.GitHubAppConfigRepository,
) (*githubAppClientResolver, *int) {
	t.Helper()

	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	built := 0
	r, _ := NewGitHubAppClientResolver(repo, enc, slog.New(slog.DiscardHandler)).(*githubAppClientResolver)
	r.build = func(cfg *config.GitHubAppConfig, _ *slog.Logger) external.GitHubAppClient {
		built++
		return &fakeResolvedClient{
			appID:        cfg.AppID,
			clientID:     cfg.ClientID,
			clientSecret: cfg.ClientSecret,
			privateKey:   cfg.PrivateKey,
		}
	}
	return r, &built
}

// storedAppConfig builds a persisted row: ciphertext, as the repository returns.
func storedAppConfig(
	t *testing.T, r *githubAppClientResolver, id, teamID, appID string, version int64,
) *models.GitHubAppConfig {
	t.Helper()

	pk, err := r.enc.Encrypt(testRSAPEM(t))
	require.NoError(t, err)
	cs, err := r.enc.Encrypt("client-secret-" + appID)
	require.NoError(t, err)
	ws, err := r.enc.Encrypt("webhook-secret-" + appID)
	require.NoError(t, err)

	return &models.GitHubAppConfig{
		ID: id, TeamID: teamID, AppID: appID, AppSlug: "slug-" + appID,
		ClientID: "Iv1." + appID, PrivateKeyEncrypted: pk,
		ClientSecretEncrypted: cs, WebhookSecretEncrypted: ws,
		WebhookToken: "tok-" + id, Version: version,
	}
}

// The whole point of the resolver: two teams with their own Apps must get
// clients bound to DIFFERENT credentials. A singleton could not do this, and a
// bug here would silently act on one team's GitHub org using another's App.
func TestResolver_DifferentTeamsGetDifferentClients(t *testing.T) {
	ctx := context.Background()
	repo := mocks.NewMockGitHubAppConfigRepository(t)
	r, _ := newResolverForTest(t, repo)

	repo.EXPECT().GetByTeamID(ctx, "team-a").
		Return(storedAppConfig(t, r, "cfg-a", "team-a", "1001", 1), nil)
	repo.EXPECT().GetByTeamID(ctx, "team-b").
		Return(storedAppConfig(t, r, "cfg-b", "team-b", "2002", 1), nil)

	a, err := r.ResolveForTeam(ctx, "team-a")
	require.NoError(t, err)
	b, err := r.ResolveForTeam(ctx, "team-b")
	require.NoError(t, err)

	assert.NotSame(t, a.Client, b.Client)
	assert.Equal(t, "cfg-a", a.AppConfigID)
	assert.Equal(t, "cfg-b", b.AppConfigID)

	fa, _ := a.Client.(*fakeResolvedClient)
	fb, _ := b.Client.(*fakeResolvedClient)
	// The App id is what ends up as the JWT `iss`, so distinct ids here are
	// exactly "the two teams authenticate as different GitHub Apps".
	assert.Equal(t, "1001", fa.appID)
	assert.Equal(t, "2002", fb.appID)
	assert.NotEqual(t, fa.clientSecret, fb.clientSecret)
}

// A team with no App gets a typed sentinel, which callers surface as 409 so the
// UI can say "configure your GitHub App first". This replaces the stub client,
// whose every method returned an opaque "GitHub App not configured".
func TestResolver_NoConfigIsTypedError(t *testing.T) {
	ctx := context.Background()
	repo := mocks.NewMockGitHubAppConfigRepository(t)
	r, built := newResolverForTest(t, repo)

	repo.EXPECT().GetByTeamID(ctx, "team-a").
		Return(nil, repositories.ErrGitHubAppConfigNotFound)

	_, err := r.ResolveForTeam(ctx, "team-a")
	assert.ErrorIs(t, err, ErrGitHubAppNotConfigured)
	assert.Zero(t, *built, "no client should be built for a team with no App")
}

// A transport failure must NOT be reported as "not configured" — that would
// tell the UI to offer setup for a team that already has an App.
func TestResolver_RepositoryFailureIsNotNotConfigured(t *testing.T) {
	ctx := context.Background()
	repo := mocks.NewMockGitHubAppConfigRepository(t)
	r, _ := newResolverForTest(t, repo)

	boom := errors.New("connection reset")
	repo.EXPECT().GetByTeamID(ctx, "team-a").Return(nil, boom)

	_, err := r.ResolveForTeam(ctx, "team-a")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrGitHubAppNotConfigured)
	assert.ErrorIs(t, err, boom)
}

func TestResolver_Caching(t *testing.T) {
	ctx := context.Background()

	t.Run("same team and version reuses the client", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		r, built := newResolverForTest(t, repo)
		repo.EXPECT().GetByTeamID(ctx, "team-a").
			Return(storedAppConfig(t, r, "cfg-a", "team-a", "1001", 1), nil).Twice()

		first, err := r.ResolveForTeam(ctx, "team-a")
		require.NoError(t, err)
		second, err := r.ResolveForTeam(ctx, "team-a")
		require.NoError(t, err)

		assert.Same(t, first.Client, second.Client)
		assert.Equal(t, 1, *built, "the second resolve must be a cache hit")
	})

	// A credential edit bumps version, so the cache invalidates itself with
	// nothing to remember at the write site — that is why version is in the key.
	t.Run("a version bump rebuilds with the new credentials", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		r, built := newResolverForTest(t, repo)

		repo.EXPECT().GetByTeamID(ctx, "team-a").
			Return(storedAppConfig(t, r, "cfg-a", "team-a", "1001", 1), nil).Once()
		first, err := r.ResolveForTeam(ctx, "team-a")
		require.NoError(t, err)

		repo.EXPECT().GetByTeamID(ctx, "team-a").
			Return(storedAppConfig(t, r, "cfg-a", "team-a", "9999", 2), nil).Once()
		second, err := r.ResolveForTeam(ctx, "team-a")
		require.NoError(t, err)

		assert.NotSame(t, first.Client, second.Client)
		assert.Equal(t, 2, *built)
		rebuilt, _ := second.Client.(*fakeResolvedClient)
		assert.Equal(t, "9999", rebuilt.appID, "the rebuild must use the NEW credentials")
	})

	t.Run("evict forces a rebuild", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		r, built := newResolverForTest(t, repo)
		repo.EXPECT().GetByTeamID(ctx, "team-a").
			Return(storedAppConfig(t, r, "cfg-a", "team-a", "1001", 1), nil).Twice()

		_, err := r.ResolveForTeam(ctx, "team-a")
		require.NoError(t, err)
		r.Evict("cfg-a")
		_, err = r.ResolveForTeam(ctx, "team-a")
		require.NoError(t, err)

		assert.Equal(t, 2, *built, "an evicted entry must be rebuilt")
	})

	t.Run("evicting an unknown id is a no-op", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		r, _ := newResolverForTest(t, repo)
		assert.NotPanics(t, func() { r.Evict("never-cached") })
	})
}

// The cache is keyed by App config and unbounded in team count, so on a large
// instance it would be a slow leak without a cap.
func TestResolver_CacheIsBounded(t *testing.T) {
	ctx := context.Background()
	repo := mocks.NewMockGitHubAppConfigRepository(t)
	r, _ := newResolverForTest(t, repo)

	overflow := githubAppClientCacheSize + 10
	for i := 0; i < overflow; i++ {
		team := fmt.Sprintf("team-%d", i)
		repo.EXPECT().GetByTeamID(ctx, team).
			Return(storedAppConfig(t, r, fmt.Sprintf("cfg-%d", i), team, fmt.Sprintf("%d", 1000+i), 1), nil).Once()
		_, err := r.ResolveForTeam(ctx, team)
		require.NoError(t, err)
	}

	r.mu.Lock()
	size, listLen := len(r.cache), r.lru.Len()
	_, oldestStillCached := r.cache["cfg-0"]
	_, newestCached := r.cache[fmt.Sprintf("cfg-%d", overflow-1)]
	r.mu.Unlock()

	assert.Equal(t, githubAppClientCacheSize, size, "the cache must not grow past its cap")
	assert.Equal(t, size, listLen, "map and LRU list must not drift apart")
	assert.False(t, oldestStillCached, "the least recently used entry must be evicted")
	assert.True(t, newestCached, "the most recent entry must survive")
}

// An unusable stored key must fail the resolve rather than yield a client that
// cannot sign — the failure belongs where the credentials are read.
func TestResolver_UnusablePrivateKeyFails(t *testing.T) {
	ctx := context.Background()
	repo := mocks.NewMockGitHubAppConfigRepository(t)
	r, built := newResolverForTest(t, repo)

	stored := storedAppConfig(t, r, "cfg-a", "team-a", "1001", 1)
	garbage, err := r.enc.Encrypt("not a pem")
	require.NoError(t, err)
	stored.PrivateKeyEncrypted = garbage
	repo.EXPECT().GetByTeamID(ctx, "team-a").Return(stored, nil)

	_, err = r.ResolveForTeam(ctx, "team-a")
	require.Error(t, err)
	assert.Zero(t, *built)
}

// Decrypted credentials must never reach the logs at any level. A leak here
// puts a team's GitHub private key in the operator's log store.
func TestResolver_NeverLogsDecryptedCredentials(t *testing.T) {
	ctx := context.Background()
	repo := mocks.NewMockGitHubAppConfigRepository(t)

	var logs bytes.Buffer
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	r, _ := NewGitHubAppClientResolver(repo, enc,
		slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	).(*githubAppClientResolver)

	privateKeyPEM := testRSAPEM(t)
	encPK, err := enc.Encrypt(privateKeyPEM)
	require.NoError(t, err)
	encCS, err := enc.Encrypt("super-secret-client-secret")
	require.NoError(t, err)
	encWS, err := enc.Encrypt("super-secret-webhook-secret")
	require.NoError(t, err)

	repo.EXPECT().GetByTeamID(ctx, "team-a").Return(&models.GitHubAppConfig{
		ID: "cfg-a", TeamID: "team-a", AppID: "1001", AppSlug: "slug",
		ClientID: "Iv1.x", PrivateKeyEncrypted: encPK,
		ClientSecretEncrypted: encCS, WebhookSecretEncrypted: encWS, Version: 1,
	}, nil)

	_, err = r.ResolveForTeam(ctx, "team-a")
	require.NoError(t, err)

	out := logs.String()
	for _, secret := range []string{
		"super-secret-client-secret",
		"super-secret-webhook-secret",
		"BEGIN RSA PRIVATE KEY",
	} {
		assert.NotContains(t, out, secret, "a decrypted credential reached the logs")
	}
	// A PEM body fragment would be just as bad as the header.
	assert.NotContains(t, out, strings.TrimSpace(strings.Split(privateKeyPEM, "\n")[1]))
}
