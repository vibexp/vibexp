package services

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

const (
	idpTestClientID     = "resolver-client"
	idpTestClientSecret = "resolver-sentinel"
	idpTestCallbackURL  = "https://vibexp.example.com/api/v1/auth/callback"
)

// fakeIDPRepo serves a mutable provider list. List copies the rows so the
// resolver can never alias them. When gate is non-nil, List waits on it.
type fakeIDPRepo struct {
	repositories.InstanceAuthProviderRepository
	mu    sync.Mutex
	rows  []*models.InstanceAuthProvider
	err   error
	lists atomic.Int64
	gate  chan struct{}
}

func (f *fakeIDPRepo) List(context.Context) ([]*models.InstanceAuthProvider, error) {
	f.lists.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make([]*models.InstanceAuthProvider, len(f.rows))
	for i, r := range f.rows {
		c := *r
		out[i] = &c
	}
	return out, nil
}

func (f *fakeIDPRepo) set(rows ...*models.InstanceAuthProvider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = rows
}

// fakeIDPVersions is the shared auth settings version counter.
type fakeIDPVersions struct {
	v   atomic.Int64
	err error
}

func (f *fakeIDPVersions) Get(context.Context) (int64, error) { return f.v.Load(), f.err }
func (f *fakeIDPVersions) bump()                              { f.v.Add(1) }

// fakeClock is an injectable, manually advanced clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type idpResolverHarness struct {
	r        *identityProviderResolver
	repo     *fakeIDPRepo
	versions *fakeIDPVersions
	clock    *fakeClock
	enc      EncryptionServiceInterface
}

func newIDPResolverHarness(t *testing.T) *idpResolverHarness {
	t.Helper()
	enc, err := NewEncryptionService(strings.Repeat("k", 32))
	require.NoError(t, err)
	h := &idpResolverHarness{
		repo:     &fakeIDPRepo{},
		versions: &fakeIDPVersions{},
		clock:    &fakeClock{now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)},
		enc:      enc,
	}
	h.r = newIdentityProviderResolver(IdentityProviderResolverDeps{
		Providers:   h.repo,
		Versions:    h.versions,
		Enc:         enc,
		CallbackURL: idpTestCallbackURL,
		Logger:      slog.New(slog.DiscardHandler),
	})
	h.r.now = h.clock.Now
	h.r.discoveryTimeout = 2 * time.Second
	return h
}

// row builds an enabled provider row with an encrypted secret.
func (h *idpResolverHarness) row(t *testing.T, id, slug string, typ models.InstanceAuthProviderType, issuer string) *models.InstanceAuthProvider {
	t.Helper()
	ct, err := h.enc.Encrypt(idpTestClientSecret)
	require.NoError(t, err)
	r := &models.InstanceAuthProvider{
		ID: id, Type: typ, Slug: slug, DisplayName: "Label " + slug, Enabled: true,
		ClientID: idpTestClientID, ClientSecretEncrypted: &ct,
		UpdatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}
	if issuer != "" {
		r.IssuerURL = &issuer
	}
	return r
}

func newIDPTestIssuer(t *testing.T) *fakeoidc.Issuer {
	return fakeoidc.New(t, idpTestClientID, idpTestClientSecret, "sub-1", "user@example.com")
}

func slugs(reg *idp.Registry) []string {
	out := []string{}
	for _, n := range reg.Enabled() {
		out = append(out, string(n))
	}
	return out
}

func TestIDPResolver_CachesByVersion(t *testing.T) {
	h := newIDPResolverHarness(t)
	iss := newIDPTestIssuer(t)
	h.repo.set(h.row(t, "id-1", "corp-sso", models.InstanceAuthProviderOIDC, iss.URL))
	ctx := context.Background()

	reg, err := h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"corp-sso"}, slugs(reg))
	assert.EqualValues(t, 1, iss.Discoveries())

	for range 5 {
		again, err := h.r.Snapshot(ctx)
		require.NoError(t, err)
		assert.Same(t, reg, again, "an unchanged version serves the cached snapshot")
	}
	assert.EqualValues(t, 1, iss.Discoveries(), "no per-login discovery at an unchanged version")
	assert.EqualValues(t, 1, h.repo.lists.Load(), "an unchanged version does not even list the rows")
}

func TestIDPResolver_VersionBumpWithUnchangedRowDoesNotRediscover(t *testing.T) {
	h := newIDPResolverHarness(t)
	iss := newIDPTestIssuer(t)
	h.repo.set(h.row(t, "id-1", "corp-sso", models.InstanceAuthProviderOIDC, iss.URL))
	ctx := context.Background()

	_, err := h.r.Snapshot(ctx)
	require.NoError(t, err)

	// An allowlist write bumps the same version and touches no provider row.
	h.versions.bump()
	reg, err := h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"corp-sso"}, slugs(reg))
	assert.EqualValues(t, 2, h.repo.lists.Load(), "the bump re-reads the rows")
	assert.EqualValues(t, 1, iss.Discoveries(), "an unchanged row is reused, not rediscovered")
}

func TestIDPResolver_ChangedRowIsRebuiltOnNextResolve(t *testing.T) {
	h := newIDPResolverHarness(t)
	iss := newIDPTestIssuer(t)
	row := h.row(t, "id-1", "corp-sso", models.InstanceAuthProviderOIDC, iss.URL)
	h.repo.set(row)
	ctx := context.Background()

	_, err := h.r.Snapshot(ctx)
	require.NoError(t, err)

	edited := *row
	edited.DisplayName = "Corporate SSO"
	edited.UpdatedAt = row.UpdatedAt.Add(time.Minute)
	h.repo.set(&edited)
	h.versions.bump()

	reg, err := h.r.Snapshot(ctx)
	require.NoError(t, err)
	e, ok := reg.Entry("corp-sso")
	require.True(t, ok)
	assert.Equal(t, "Corporate SSO", e.DisplayName, "the edit applies with no restart")
	assert.EqualValues(t, 2, iss.Discoveries(), "only the changed row is rediscovered, once")
}

func TestIDPResolver_AddAndDisableTakeEffectOnNextResolve(t *testing.T) {
	h := newIDPResolverHarness(t)
	iss := newIDPTestIssuer(t)
	oidcRow := h.row(t, "id-1", "corp-sso", models.InstanceAuthProviderOIDC, iss.URL)
	h.repo.set(oidcRow)
	ctx := context.Background()

	reg, err := h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"corp-sso"}, slugs(reg))

	// Add a GitHub provider ahead of it in sort order (the repository returns
	// rows ordered by sort_order).
	gh := h.row(t, "id-2", "github", models.InstanceAuthProviderGitHub, "")
	h.repo.set(gh, oidcRow)
	h.versions.bump()
	reg, err = h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"github", "corp-sso"}, slugs(reg), "the new provider appears, in sort order")
	e, _ := reg.Entry("github")
	assert.Equal(t, idp.ProviderGitHub, e.Type)
	assert.Contains(t, e.Provider.AuthorizeURL("s", "", ""), "redirect_uri="+url.QueryEscape(idpTestCallbackURL),
		"providers are built with the derived callback URL")

	// Disable the OIDC one.
	disabled := *oidcRow
	disabled.Enabled = false
	h.repo.set(gh, &disabled)
	h.versions.bump()
	reg, err = h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"github"}, slugs(reg))
	_, has := h.r.Health()["corp-sso"]
	assert.False(t, has, "a disabled provider has no health entry")

	p, ok, err := h.r.Provider(ctx, "corp-sso")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, p)
}

func TestIDPResolver_ConcurrentResolvesAfterBumpDiscoverOnce(t *testing.T) {
	h := newIDPResolverHarness(t)
	iss := newIDPTestIssuer(t)
	row := h.row(t, "id-1", "corp-sso", models.InstanceAuthProviderOIDC, iss.URL)
	h.repo.set(row)
	ctx := context.Background()
	_, err := h.r.Snapshot(ctx)
	require.NoError(t, err)

	edited := *row
	edited.UpdatedAt = row.UpdatedAt.Add(time.Minute)
	h.repo.set(&edited)
	h.versions.bump()

	// Hold the rebuild's List until every caller has started, so all of them
	// pile up behind the one in-flight rebuild.
	h.repo.gate = make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for range 50 {
		wg.Go(func() {
			reg, err := h.r.Snapshot(ctx)
			if err == nil && reg.Len() != 1 {
				err = errors.New("unexpected provider count")
			}
			errs <- err
		})
	}
	time.Sleep(100 * time.Millisecond)
	close(h.repo.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.EqualValues(t, 2, iss.Discoveries(), "50 concurrent resolves after a bump rediscover the changed row once")
	assert.EqualValues(t, 2, h.repo.lists.Load(), "one rebuild served every concurrent caller")
}

func TestIDPResolver_HangingIssuerFailsWithinTimeoutAndOthersKeepWorking(t *testing.T) {
	h := newIDPResolverHarness(t)
	h.r.discoveryTimeout = 50 * time.Millisecond
	slow := newIDPTestIssuer(t)
	slow.SetHang(true)
	good := newIDPTestIssuer(t)
	h.repo.set(
		h.row(t, "id-1", "slow-sso", models.InstanceAuthProviderOIDC, slow.URL),
		h.row(t, "id-2", "good-sso", models.InstanceAuthProviderOIDC, good.URL),
	)

	start := time.Now()
	reg, err := h.r.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 2*time.Second, "a hanging issuer is bounded by the discovery timeout")
	assert.Equal(t, []string{"good-sso"}, slugs(reg), "the unhealthy provider is excluded, the other keeps working")

	health := h.r.Health()
	require.Contains(t, health, "slow-sso")
	assert.False(t, health["slow-sso"].Healthy)
	assert.NotEmpty(t, health["slow-sso"].LastError)
	assert.Equal(t, h.clock.Now(), health["slow-sso"].CheckedAt)
	assert.True(t, health["good-sso"].Healthy)
	assert.Empty(t, health["good-sso"].LastError)
}

func TestIDPResolver_UnhealthyProviderIsRetriedAfterBackoff(t *testing.T) {
	h := newIDPResolverHarness(t)
	h.r.discoveryTimeout = 50 * time.Millisecond
	iss := newIDPTestIssuer(t)
	iss.SetHang(true)
	h.repo.set(h.row(t, "id-1", "corp-sso", models.InstanceAuthProviderOIDC, iss.URL))
	ctx := context.Background()

	reg, err := h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, reg.Len())
	assert.EqualValues(t, 1, iss.Discoveries())

	iss.SetHang(false)
	h.clock.advance(defaultIDPRetryBackoff - time.Second)
	reg, err = h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, reg.Len(), "within the backoff the failure is not retried")
	assert.EqualValues(t, 1, iss.Discoveries())

	h.clock.advance(2 * time.Second)
	reg, err = h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"corp-sso"}, slugs(reg), "after the backoff it is retried at the same version")
	assert.EqualValues(t, 2, iss.Discoveries())
	assert.True(t, h.r.Health()["corp-sso"].Healthy)

	_, err = h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, iss.Discoveries(), "a recovered provider is cached again")
}

func TestIDPResolver_RowsThatCannotBuildAreUnhealthy(t *testing.T) {
	h := newIDPResolverHarness(t)
	noSecret := h.row(t, "id-1", "no-secret", models.InstanceAuthProviderGitHub, "")
	noSecret.ClientSecretEncrypted = nil
	garbled := h.row(t, "id-2", "garbled", models.InstanceAuthProviderGitHub, "")
	bad := "not-ciphertext"
	garbled.ClientSecretEncrypted = &bad
	unknown := h.row(t, "id-3", "mystery", models.InstanceAuthProviderType("saml"), "")
	h.repo.set(noSecret, garbled, unknown)

	reg, err := h.r.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, reg.Len())
	health := h.r.Health()
	assert.Contains(t, health["no-secret"].LastError, "no client secret")
	assert.Contains(t, health["garbled"].LastError, "decrypt")
	assert.Contains(t, health["mystery"].LastError, "unknown identity provider type")
	for slug, hl := range health {
		assert.NotContains(t, hl.LastError, idpTestClientSecret, "health for %s must not leak the secret", slug)
	}
}

func TestIDPResolver_NoEncryptionServiceIsUnhealthy(t *testing.T) {
	h := newIDPResolverHarness(t)
	h.repo.set(h.row(t, "id-1", "github", models.InstanceAuthProviderGitHub, ""))
	h.r.enc = nil

	reg, err := h.r.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, reg.Len())
	assert.Contains(t, h.r.Health()["github"].LastError, "encryption_key")
}

func TestIDPResolver_GoogleRowUsesSlugAndType(t *testing.T) {
	h := newIDPResolverHarness(t)
	iss := newIDPTestIssuer(t)
	h.r.googleIssuerURL = iss.URL
	h.repo.set(h.row(t, "id-1", "google", models.InstanceAuthProviderGoogle, ""))

	p, ok, err := h.r.Provider(context.Background(), "google")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, idp.ProviderGoogle, p.Name())
	reg, err := h.r.Snapshot(context.Background())
	require.NoError(t, err)
	e, _ := reg.Entry("google")
	assert.Equal(t, idp.ProviderGoogle, e.Type)
	assert.Equal(t, "Label google", e.DisplayName)
}

func TestIDPResolver_ReadErrors(t *testing.T) {
	t.Run("version read fails", func(t *testing.T) {
		h := newIDPResolverHarness(t)
		h.versions.err = errors.New("db down")
		_, err := h.r.Snapshot(context.Background())
		require.ErrorContains(t, err, "db down")
		_, _, err = h.r.Provider(context.Background(), "x")
		require.Error(t, err)
	})
	t.Run("list fails", func(t *testing.T) {
		h := newIDPResolverHarness(t)
		h.repo.err = errors.New("db down")
		_, err := h.r.Snapshot(context.Background())
		require.ErrorContains(t, err, "list auth providers")
	})
}

func TestIDPResolver_TestProviderDoesNotTouchTheCache(t *testing.T) {
	h := newIDPResolverHarness(t)
	iss := newIDPTestIssuer(t)
	h.repo.set(h.row(t, "id-1", "corp-sso", models.InstanceAuthProviderOIDC, iss.URL))
	ctx := context.Background()
	before, err := h.r.Snapshot(ctx)
	require.NoError(t, err)
	healthBefore := h.r.Health()

	other := newIDPTestIssuer(t)
	candidate := *h.row(t, "", "partner-sso", models.InstanceAuthProviderOIDC, other.URL)
	secret := "candidate-secret"
	require.NoError(t, h.r.TestProvider(ctx, candidate, &secret))
	assert.EqualValues(t, 1, other.Discoveries(), "the candidate's issuer was discovered")

	after, err := h.r.Snapshot(ctx)
	require.NoError(t, err)
	assert.Same(t, before, after, "TestProvider never replaces the cached snapshot")
	assert.Equal(t, healthBefore, h.r.Health(), "TestProvider never records health")
	assert.EqualValues(t, 1, h.repo.lists.Load())

	t.Run("unreachable issuer fails", func(t *testing.T) {
		h.r.discoveryTimeout = 50 * time.Millisecond
		dead := newIDPTestIssuer(t)
		dead.SetHang(true)
		c := *h.row(t, "", "", models.InstanceAuthProviderOIDC, dead.URL)
		require.Error(t, h.r.TestProvider(ctx, c, nil))
	})
}

func TestIDPResolver_TestProviderGitHubCredentialCheck(t *testing.T) {
	var gotSecret atomic.Value
	answer := atomic.Value{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotSecret.Store(r.FormValue("client_secret"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer.Load().(string))) //nolint:errcheck // test server
	}))
	t.Cleanup(srv.Close)

	h := newIDPResolverHarness(t)
	h.r.githubExchangeURL = srv.URL
	stored := *h.row(t, "id-1", "github", models.InstanceAuthProviderGitHub, "")
	ctx := context.Background()

	answer.Store(`{"error":"bad_verification_code"}`)
	require.NoError(t, h.r.TestProvider(ctx, stored, nil), "valid credentials get bad_verification_code")
	assert.Equal(t, idpTestClientSecret, gotSecret.Load(), "a nil secret tests the stored one")

	answer.Store(`{"error":"incorrect_client_credentials"}`)
	candidate := "typed-secret"
	err := h.r.TestProvider(ctx, stored, &candidate)
	require.ErrorIs(t, err, ErrIdentityProviderCredentialsInvalid)
	assert.Equal(t, "typed-secret", gotSecret.Load(), "a candidate secret overrides the stored one")

	answer.Store(`{"error":"something_else"}`)
	require.ErrorContains(t, h.r.TestProvider(ctx, stored, nil), "unexpected credential check response")

	answer.Store(`not json`)
	require.ErrorContains(t, h.r.TestProvider(ctx, stored, nil), "unreadable body")

	noSecret := stored
	noSecret.ClientSecretEncrypted = nil
	require.ErrorContains(t, h.r.TestProvider(ctx, noSecret, nil), "no client secret")

	h.r.githubExchangeURL = "http://127.0.0.1:1"
	require.ErrorContains(t, h.r.TestProvider(ctx, stored, nil), "credential check")
}

func TestWarnIgnoredLegacyRedirectURIs(t *testing.T) {
	const derived = "https://vibexp.example.com/api/v1/auth/callback"
	auth := config.AuthConfig{
		LegacyProviders: []string{"oidc", "github", "google"},
		LegacyOIDC:      config.OIDCAuthConfig{RedirectURI: "https://api.example.com/api/v1/auth/callback"},
		LegacyGitHub:    config.GitHubAuthConfig{RedirectURI: derived},
		// Google is enabled but has no redirect_uri configured.
	}
	logger, rec := logtest.New()
	WarnIgnoredLegacyRedirectURIs(auth, derived, logger)

	var warned []string
	for _, e := range rec.AllEntries() {
		if e.Level == slog.LevelWarn {
			warned = append(warned, e.Data["provider"].(string))
			assert.Equal(t, derived, e.Data["callback_url"])
			assert.Equal(t, "https://api.example.com/api/v1/auth/callback", e.Data["configured_redirect_uri"])
		}
	}
	assert.Equal(t, []string{"oidc"}, warned, "only an enabled provider whose redirect_uri differs is reported")

	t.Run("the built-in default redirect_uri is not reported", func(t *testing.T) {
		logger, rec := logtest.New()
		WarnIgnoredLegacyRedirectURIs(config.AuthConfig{
			LegacyProvider: "oidc",
			LegacyOIDC:     config.OIDCAuthConfig{RedirectURI: "http://localhost:8080/api/v1/auth/callback"},
		}, derived, logger)
		assert.Empty(t, rec.AllEntries(), "config.docker.yaml's baked default is not an operator choice")
	})

	t.Run("a redirect_uri of a provider that is not enabled is not reported", func(t *testing.T) {
		logger, rec := logtest.New()
		WarnIgnoredLegacyRedirectURIs(config.AuthConfig{
			LegacyProvider: "github",
			LegacyOIDC:     config.OIDCAuthConfig{RedirectURI: "https://api.example.com/api/v1/auth/callback"},
		}, derived, logger)
		assert.Empty(t, rec.AllEntries())
	})
}

func TestIDPResolver_WaiterLeavesWhenItsRequestEnds(t *testing.T) {
	h := newIDPResolverHarness(t)
	iss := newIDPTestIssuer(t)
	h.repo.set(h.row(t, "id-1", "corp-sso", models.InstanceAuthProviderOIDC, iss.URL))
	h.repo.gate = make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := h.r.Snapshot(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second, "the caller stops waiting when its own request ends")

	// The rebuild itself was not cancelled: it completes for the next caller.
	close(h.repo.gate)
	reg, err := h.r.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"corp-sso"}, slugs(reg))
	assert.EqualValues(t, 1, h.repo.lists.Load(), "the detached rebuild served the later caller")
}
