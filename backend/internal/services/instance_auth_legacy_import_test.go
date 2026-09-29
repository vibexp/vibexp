package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// legacyAuthSecret is a legacy client secret; no log line may contain it.
const legacyAuthSecret = "legacy-auth-sentinel-7d2a"

// legacyAuthIssuer is a valid OIDC issuer for the fixtures.
const legacyAuthIssuer = "https://sso.example.com/realms/corp"

func completeLegacyAuth() config.AuthConfig {
	return config.AuthConfig{
		LegacyProviders: []string{"oidc", "google"},
		LegacyGoogle:    config.GoogleAuthConfig{ClientID: "google-id", ClientSecret: legacyAuthSecret},
		LegacyOIDC: config.OIDCAuthConfig{
			IssuerURL: legacyAuthIssuer, ClientID: "oidc-id", ClientSecret: legacyAuthSecret,
		},
		LegacyAccessAllowlist: config.AccessAllowlistConfig{
			Domains: config.EnvStringSlice{" Example.com ", "example.com"},
			Emails:  config.EnvStringSlice{"Alice@Other.com"},
		},
	}
}

type legacyAuthFixture struct {
	providers *repomocks.MockInstanceAuthProviderRepository
	allowlist *repomocks.MockInstanceAuthAllowlistRepository
	enc       EncryptionServiceInterface
	logs      *logtest.Recorder
	logger    *slog.Logger
}

func newLegacyAuthFixture(t *testing.T) *legacyAuthFixture {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	logger, logs := logtest.New()
	return &legacyAuthFixture{
		providers: repomocks.NewMockInstanceAuthProviderRepository(t),
		allowlist: repomocks.NewMockInstanceAuthAllowlistRepository(t),
		enc:       enc,
		logs:      logs,
		logger:    logger,
	}
}

func (f *legacyAuthFixture) run(auth config.AuthConfig) LegacyAuthImportResult {
	return ImportLegacyAuthConfig(context.Background(), LegacyAuthImportDeps{
		ProviderRepo: f.providers, AllowlistRepo: f.allowlist, Enc: f.enc, Logger: f.logger,
	}, auth)
}

func (f *legacyAuthFixture) noProviders() {
	f.providers.On("List", mock.Anything).Return([]*models.InstanceAuthProvider{}, nil).Once()
}

func (f *legacyAuthFixture) noAllowlist() {
	f.allowlist.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceAuthAllowlistNotFound).Once()
}

// captureProviders records the set InsertIfEmpty receives.
func (f *legacyAuthFixture) captureProviders(inserted bool, err error) *[]*models.InstanceAuthProvider {
	captured := &[]*models.InstanceAuthProvider{}
	f.providers.On("InsertIfEmpty", mock.Anything, mock.AnythingOfType("[]*models.InstanceAuthProvider")).
		Run(func(args mock.Arguments) {
			*captured = args.Get(1).([]*models.InstanceAuthProvider)
		}).Return(inserted, err).Once()
	return captured
}

func (f *legacyAuthFixture) captureAllowlist(inserted bool, err error) *models.InstanceAuthAllowlist {
	captured := &models.InstanceAuthAllowlist{}
	f.allowlist.On("InsertIfAbsent", mock.Anything, mock.AnythingOfType("*models.InstanceAuthAllowlist")).
		Run(func(args mock.Arguments) {
			*captured = *args.Get(1).(*models.InstanceAuthAllowlist)
		}).Return(inserted, err).Once()
	return captured
}

func (f *legacyAuthFixture) messagesAt(level slog.Level) []string {
	var out []string
	for _, entry := range f.logs.AllEntries() {
		if entry.Level == level {
			out = append(out, entry.Message)
		}
	}
	return out
}

// assertNoSecretLogged fails if any log message or attribute carries the
// plaintext secret.
func (f *legacyAuthFixture) assertNoSecretLogged(t *testing.T) {
	t.Helper()
	for _, entry := range f.logs.AllEntries() {
		assert.NotContains(t, entry.Message, legacyAuthSecret)
		assert.NotContains(t, fmt.Sprint(entry.Data), legacyAuthSecret)
	}
}

func TestLegacyAuthPopulated(t *testing.T) {
	tests := []struct {
		name string
		auth config.AuthConfig
		want bool
	}{
		{"zero value", config.AuthConfig{}, false},
		{"redirect uris only", config.AuthConfig{
			LegacyGoogle: config.GoogleAuthConfig{RedirectURI: "http://localhost:8080/api/v1/auth/callback"},
			LegacyGitHub: config.GitHubAuthConfig{RedirectURI: "http://localhost:8080/api/v1/auth/callback"},
			LegacyOIDC:   config.OIDCAuthConfig{RedirectURI: "http://localhost:8080/api/v1/auth/callback"},
		}, false},
		{"provider none", config.AuthConfig{LegacyProvider: "none"}, false},
		{"blank allowlist entries", config.AuthConfig{LegacyAccessAllowlist: config.AccessAllowlistConfig{
			Domains: config.EnvStringSlice{"", " "}, Emails: config.EnvStringSlice{""},
		}}, false},
		{"provider enabled", config.AuthConfig{LegacyProvider: "google"}, true},
		{"providers list", config.AuthConfig{LegacyProviders: []string{"github"}}, true},
		{"credentials only", config.AuthConfig{LegacyGitHub: config.GitHubAuthConfig{ClientID: "id"}}, true},
		{"oidc issuer only", config.AuthConfig{LegacyOIDC: config.OIDCAuthConfig{IssuerURL: legacyAuthIssuer}}, true},
		{"allowlist email", config.AuthConfig{LegacyAccessAllowlist: config.AccessAllowlistConfig{
			Emails: config.EnvStringSlice{"a@example.com"},
		}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LegacyAuthPopulated(tt.auth))
		})
	}
}

// TestLegacyAuthProviderCandidates is the mapping table: which legacy
// providers are imported, how, and why the others are not.
func TestLegacyAuthProviderCandidates(t *testing.T) {
	google := config.GoogleAuthConfig{ClientID: "google-id", ClientSecret: legacyAuthSecret}
	github := config.GitHubAuthConfig{ClientID: "github-id", ClientSecret: legacyAuthSecret}
	oidc := config.OIDCAuthConfig{IssuerURL: legacyAuthIssuer, ClientID: "oidc-id", ClientSecret: legacyAuthSecret}

	tests := []struct {
		name        string
		auth        config.AuthConfig
		wantSlugs   []string
		wantSkipped map[string]string
	}{
		{
			name:      "providers wins over provider, alphabetical order",
			auth:      config.AuthConfig{LegacyProviders: []string{"oidc", "github"}, LegacyProvider: "google", LegacyGoogle: google, LegacyGitHub: github, LegacyOIDC: oidc},
			wantSlugs: []string{"github", "oidc"},
			wantSkipped: map[string]string{
				"google": legacyAuthSkipNotEnabled,
			},
		},
		{
			name:        "single provider shim, trimmed and lower-cased",
			auth:        config.AuthConfig{LegacyProvider: "  GitHub ", LegacyGitHub: github},
			wantSlugs:   []string{"github"},
			wantSkipped: map[string]string{},
		},
		{
			name:        "duplicates and none dropped",
			auth:        config.AuthConfig{LegacyProviders: []string{"Google", "google", "none", ""}, LegacyGoogle: google},
			wantSlugs:   []string{"google"},
			wantSkipped: map[string]string{},
		},
		{
			name:        "unknown name",
			auth:        config.AuthConfig{LegacyProviders: []string{"okta", "google"}, LegacyGoogle: google},
			wantSlugs:   []string{"google"},
			wantSkipped: map[string]string{"okta": legacyAuthSkipUnknown},
		},
		{
			name: "google missing secret",
			auth: config.AuthConfig{LegacyProvider: "google", LegacyGoogle: config.GoogleAuthConfig{
				ClientID: "google-id", ClientSecret: "   ",
			}},
			wantSlugs:   nil,
			wantSkipped: map[string]string{"google": legacyAuthSkipIncompleteOAuth},
		},
		{
			name: "github missing client id",
			auth: config.AuthConfig{LegacyProvider: "github", LegacyGitHub: config.GitHubAuthConfig{
				ClientSecret: legacyAuthSecret,
			}},
			wantSlugs:   nil,
			wantSkipped: map[string]string{"github": legacyAuthSkipIncompleteOAuth},
		},
		{
			name: "oidc missing issuer",
			auth: config.AuthConfig{LegacyProvider: "oidc", LegacyOIDC: config.OIDCAuthConfig{
				ClientID: "oidc-id", ClientSecret: legacyAuthSecret,
			}},
			wantSlugs:   nil,
			wantSkipped: map[string]string{"oidc": legacyAuthSkipIncompleteOIDC},
		},
		{
			name: "oidc missing secret",
			auth: config.AuthConfig{LegacyProvider: "oidc", LegacyOIDC: config.OIDCAuthConfig{
				IssuerURL: legacyAuthIssuer, ClientID: "oidc-id",
			}},
			wantSlugs:   nil,
			wantSkipped: map[string]string{"oidc": legacyAuthSkipIncompleteOIDC},
		},
		{
			name:        "enabled but no credentials at all",
			auth:        config.AuthConfig{LegacyProvider: "github"},
			wantSlugs:   nil,
			wantSkipped: map[string]string{"github": legacyAuthSkipIncompleteOAuth},
		},
		{
			name: "credentials but nothing enabled",
			auth: config.AuthConfig{LegacyGoogle: google, LegacyOIDC: oidc},
			wantSkipped: map[string]string{
				"google": legacyAuthSkipNotEnabled,
				"oidc":   legacyAuthSkipNotEnabled,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates, skipped := legacyAuthProviderCandidates(tt.auth)

			var slugs []string
			for i, c := range candidates {
				slugs = append(slugs, c.provider.Slug)
				assert.Equal(t, i, c.provider.SortOrder, "sort_order follows the alphabetical picker order")
				assert.True(t, c.provider.Enabled)
				assert.Nil(t, c.provider.ClientSecretEncrypted, "the mapping never holds ciphertext")
				assert.Equal(t, legacyAuthSecret, c.secret)
			}
			assert.Equal(t, tt.wantSlugs, slugs)
			assert.Equal(t, tt.wantSkipped, skipped)
		})
	}
}

// TestLegacyAuthProviderCandidates_InvalidIssuer: a provider that fails the
// admin API's validation is skipped with the validation message.
func TestLegacyAuthProviderCandidates_InvalidIssuer(t *testing.T) {
	candidates, skipped := legacyAuthProviderCandidates(config.AuthConfig{
		LegacyProvider: "oidc",
		LegacyOIDC: config.OIDCAuthConfig{
			IssuerURL: "http://sso.example.com", ClientID: "oidc-id", ClientSecret: legacyAuthSecret,
		},
	})
	assert.Empty(t, candidates)
	require.Contains(t, skipped, "oidc")
	assert.Contains(t, skipped["oidc"], "invalid:")
	assert.Contains(t, skipped["oidc"], "https")
}

// TestLegacyAuthProviderCandidates_ReproducesTodaysLogin is the upgrade-safety
// assertion (#1232 AC; the /auth/providers round trip lands with #1234): the
// imported rows carry exactly the names, labels and order the login screen
// shows today — the registry's alphabetical Enabled() order and the labels of
// providerDisplayName.
func TestLegacyAuthProviderCandidates_ReproducesTodaysLogin(t *testing.T) {
	auth := config.AuthConfig{
		LegacyProviders: []string{"oidc", "google", "github"},
		LegacyGoogle:    config.GoogleAuthConfig{ClientID: "g", ClientSecret: legacyAuthSecret},
		LegacyGitHub:    config.GitHubAuthConfig{ClientID: "gh", ClientSecret: legacyAuthSecret},
		LegacyOIDC:      config.OIDCAuthConfig{IssuerURL: legacyAuthIssuer, ClientID: "o", ClientSecret: legacyAuthSecret},
	}
	candidates, skipped := legacyAuthProviderCandidates(auth)
	require.Empty(t, skipped)

	type row struct {
		Slug, Type, Label string
		Order             int
	}
	var got []row
	for _, c := range candidates {
		got = append(got, row{c.provider.Slug, string(c.provider.Type), c.provider.DisplayName, c.provider.SortOrder})
	}
	assert.Equal(t, []row{
		{"github", "github", "GitHub", 0},
		{"google", "google", "Google", 1},
		{"oidc", "oidc", "Single Sign-On", 2},
	}, got)

	require.NotNil(t, candidates[2].provider.IssuerURL)
	assert.Equal(t, legacyAuthIssuer, *candidates[2].provider.IssuerURL)
	assert.Nil(t, candidates[0].provider.IssuerURL)
	assert.Equal(t, "gh", candidates[0].provider.ClientID)
}

func TestImportLegacyAuthConfig_ImportsProvidersAndAllowlist(t *testing.T) {
	f := newLegacyAuthFixture(t)
	f.noProviders()
	f.noAllowlist()
	captured := f.captureProviders(true, nil)
	allowlist := f.captureAllowlist(true, nil)

	result := f.run(completeLegacyAuth())

	assert.Equal(t, []string{"google", "oidc"}, result.ProvidersImported)
	assert.Empty(t, result.ProvidersSkipped)
	assert.True(t, result.AllowlistImported)

	require.Len(t, *captured, 2)
	for _, p := range *captured {
		require.NotNil(t, p.ClientSecretEncrypted, "%s secret is stored", p.Slug)
		assert.NotEqual(t, legacyAuthSecret, *p.ClientSecretEncrypted, "only ciphertext is stored")
		plain, err := f.enc.Decrypt(*p.ClientSecretEncrypted)
		require.NoError(t, err)
		assert.Equal(t, legacyAuthSecret, plain, "the ciphertext decrypts to the config value")
	}
	assert.Equal(t, []string{"example.com"}, allowlist.Domains, "normalized and de-duplicated")
	assert.Equal(t, []string{"alice@other.com"}, allowlist.Emails)
	f.assertNoSecretLogged(t)
}

func TestImportLegacyAuthConfig_NothingPopulatedTouchesNothing(t *testing.T) {
	f := newLegacyAuthFixture(t)

	result := f.run(config.AuthConfig{LegacyGoogle: config.GoogleAuthConfig{
		RedirectURI: "http://localhost:8080/api/v1/auth/callback",
	}})

	assert.Equal(t, LegacyAuthImportResult{}, result)
	assert.Empty(t, f.logs.AllEntries(), "an unconfigured install logs nothing")
}

func TestImportLegacyAuthConfig_DatabaseWins(t *testing.T) {
	f := newLegacyAuthFixture(t)
	f.providers.On("List", mock.Anything).
		Return([]*models.InstanceAuthProvider{{Slug: "google"}}, nil).Once()
	f.allowlist.On("Get", mock.Anything).Return(&models.InstanceAuthAllowlist{}, nil).Once()

	result := f.run(completeLegacyAuth())

	assert.True(t, result.ProvidersIgnored)
	assert.True(t, result.AllowlistIgnored)
	assert.Empty(t, result.ProvidersImported)
	assert.False(t, result.AllowlistImported)
	warns := f.messagesAt(slog.LevelWarn)
	require.Len(t, warns, 2)
	assert.Contains(t, warns[0], "ignored")
	assert.Contains(t, warns[1], "ignored")
}

func TestImportLegacyAuthConfig_LostRaceIsNotAnImport(t *testing.T) {
	f := newLegacyAuthFixture(t)
	f.noProviders()
	f.noAllowlist()
	f.captureProviders(false, nil)
	f.captureAllowlist(false, nil)

	result := f.run(completeLegacyAuth())

	assert.Empty(t, result.ProvidersImported)
	assert.False(t, result.AllowlistImported)
	assert.Empty(t, f.messagesAt(slog.LevelError))
}

func TestImportLegacyAuthConfig_SkipsAreLoggedWithReasons(t *testing.T) {
	f := newLegacyAuthFixture(t)
	f.noProviders()

	result := f.run(config.AuthConfig{
		LegacyProviders: []string{"okta", "github"},
		LegacyGitHub:    config.GitHubAuthConfig{ClientID: "id"},
	})

	assert.Empty(t, result.ProvidersImported)
	assert.Equal(t, map[string]string{
		"github": legacyAuthSkipIncompleteOAuth,
		"okta":   legacyAuthSkipUnknown,
	}, result.ProvidersSkipped)
	var reasons []any
	for _, entry := range f.logs.AllEntries() {
		if entry.Level == slog.LevelWarn {
			reasons = append(reasons, entry.Data["provider"], entry.Data["reason"])
		}
	}
	assert.Equal(t, []any{"github", legacyAuthSkipIncompleteOAuth, "okta", legacyAuthSkipUnknown}, reasons)
}

func TestImportLegacyAuthConfig_FailuresNeverFailBoot(t *testing.T) {
	boom := errors.New("boom")

	t.Run("provider list fails", func(t *testing.T) {
		f := newLegacyAuthFixture(t)
		f.providers.On("List", mock.Anything).Return(nil, boom).Once()
		f.noAllowlist()
		f.captureAllowlist(true, nil)

		result := f.run(completeLegacyAuth())
		assert.Empty(t, result.ProvidersImported)
		assert.True(t, result.AllowlistImported, "the allowlist import is independent")
		assert.Len(t, f.messagesAt(slog.LevelError), 1)
	})

	t.Run("provider insert fails", func(t *testing.T) {
		f := newLegacyAuthFixture(t)
		f.noProviders()
		f.captureProviders(false, boom)
		f.noAllowlist()
		f.captureAllowlist(true, nil)

		result := f.run(completeLegacyAuth())
		assert.Empty(t, result.ProvidersImported)
		assert.Equal(t, []string{legacyAuthImportFailedMsg}, f.messagesAt(slog.LevelError))
		f.assertNoSecretLogged(t)
	})

	t.Run("encryption unavailable imports nothing", func(t *testing.T) {
		f := newLegacyAuthFixture(t)
		f.enc = nil
		f.noProviders()

		result := f.run(config.AuthConfig{
			LegacyProvider: "google",
			LegacyGoogle:   config.GoogleAuthConfig{ClientID: "id", ClientSecret: legacyAuthSecret},
		})
		assert.Empty(t, result.ProvidersImported)
		assert.Equal(t, []string{legacyAuthImportFailedMsg}, f.messagesAt(slog.LevelError))
		f.providers.AssertNotCalled(t, "InsertIfEmpty", mock.Anything, mock.Anything)
	})

	t.Run("allowlist read fails", func(t *testing.T) {
		f := newLegacyAuthFixture(t)
		f.allowlist.On("Get", mock.Anything).Return(nil, boom).Once()

		result := f.run(config.AuthConfig{LegacyAccessAllowlist: config.AccessAllowlistConfig{
			Domains: config.EnvStringSlice{"example.com"},
		}})
		assert.False(t, result.AllowlistImported)
		assert.Len(t, f.messagesAt(slog.LevelError), 1)
	})

	t.Run("allowlist insert fails", func(t *testing.T) {
		f := newLegacyAuthFixture(t)
		f.noAllowlist()
		f.captureAllowlist(false, boom)

		result := f.run(config.AuthConfig{LegacyAccessAllowlist: config.AccessAllowlistConfig{
			Domains: config.EnvStringSlice{"example.com"},
		}})
		assert.False(t, result.AllowlistImported)
		assert.Equal(t, []string{legacyAllowlistImportFailedMsg}, f.messagesAt(slog.LevelError))
	})

	t.Run("invalid allowlist is not imported", func(t *testing.T) {
		f := newLegacyAuthFixture(t)
		f.noAllowlist()

		result := f.run(config.AuthConfig{LegacyAccessAllowlist: config.AccessAllowlistConfig{
			Domains: config.EnvStringSlice{"@example.com"},
		}})
		assert.False(t, result.AllowlistImported)
		errs := f.messagesAt(slog.LevelError)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "invalid")
		f.allowlist.AssertNotCalled(t, "InsertIfAbsent", mock.Anything, mock.Anything)
	})
}
