//go:build integration

package postgres

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// The boot-time import of the legacy config.yaml auth providers and allowlist
// (#1232) against real Postgres: a fresh import, a second boot that changes
// nothing, replicas racing, and the stored secret and audit trail.

// legacyAuthIntegrationSecret is the legacy client secret. It must only ever
// be stored encrypted, and never appear in an audit entry.
const legacyAuthIntegrationSecret = "legacy-auth-integration-sentinel"

func legacyAuthIntegrationConfig() config.AuthConfig {
	return config.AuthConfig{
		LegacyProviders: []string{"oidc", "google"},
		LegacyGoogle:    config.GoogleAuthConfig{ClientID: "google-id", ClientSecret: legacyAuthIntegrationSecret},
		LegacyOIDC: config.OIDCAuthConfig{
			IssuerURL: "https://sso.example.com", ClientID: "oidc-id", ClientSecret: legacyAuthIntegrationSecret,
		},
		LegacyAccessAllowlist: config.AccessAllowlistConfig{
			Domains: config.EnvStringSlice{"Example.com"},
			Emails:  config.EnvStringSlice{"alice@other.com"},
		},
	}
}

func legacyAuthIntegrationDeps(t *testing.T) (services.LegacyAuthImportDeps, services.EncryptionServiceInterface) {
	t.Helper()
	enc, err := services.NewEncryptionService(strings.Repeat("k", 32))
	require.NoError(t, err)
	return services.LegacyAuthImportDeps{
		ProviderRepo:  NewInstanceAuthProviderRepository(integrationDB),
		AllowlistRepo: NewInstanceAuthAllowlistRepository(integrationDB),
		Enc:           enc,
	}, enc
}

func TestIntegrationLegacyAuthImport_FreshThenSecondBoot(t *testing.T) {
	resetInstanceAuthSettings(t)
	ctx := context.Background()
	deps, enc := legacyAuthIntegrationDeps(t)
	start := authSettingsVersion(t)

	result := services.ImportLegacyAuthConfig(ctx, deps, legacyAuthIntegrationConfig())
	assert.Equal(t, []string{"google", "oidc"}, result.ProvidersImported)
	assert.True(t, result.AllowlistImported)
	assert.Equal(t, start+2, authSettingsVersion(t), "one bump for the provider set, one for the allowlist")

	providers, err := deps.ProviderRepo.List(ctx)
	require.NoError(t, err)
	require.Len(t, providers, 2)
	google, oidc := providers[0], providers[1]
	assert.Equal(t, "google", google.Slug)
	assert.Equal(t, models.InstanceAuthProviderGoogle, google.Type)
	assert.Equal(t, "Google", google.DisplayName)
	assert.Equal(t, 0, google.SortOrder)
	assert.Equal(t, "oidc", oidc.Slug)
	assert.Equal(t, "Single Sign-On", oidc.DisplayName)
	assert.Equal(t, 1, oidc.SortOrder)
	require.NotNil(t, oidc.IssuerURL)
	assert.Equal(t, "https://sso.example.com", *oidc.IssuerURL)
	for _, p := range providers {
		assert.True(t, p.Enabled)
		assert.Nil(t, p.UpdatedBy, "an import has no editor")
		require.NotNil(t, p.ClientSecretEncrypted)
		assert.NotContains(t, *p.ClientSecretEncrypted, legacyAuthIntegrationSecret, "stored only encrypted")
		plain, err := enc.Decrypt(*p.ClientSecretEncrypted)
		require.NoError(t, err)
		assert.Equal(t, legacyAuthIntegrationSecret, plain)
	}

	allowlist, err := deps.AllowlistRepo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com"}, allowlist.Domains)
	assert.Equal(t, []string{"alice@other.com"}, allowlist.Emails)

	providerAudits := authAuditEntries(t, models.InstanceSettingAuthProviders)
	require.Len(t, providerAudits, 2, "one import entry per provider")
	for _, e := range providerAudits {
		assert.Equal(t, models.InstanceSettingsAuditActionImport, e.Action)
		assert.Nil(t, e.ActorUserID)
		assert.NotContains(t, string(e.After), legacyAuthIntegrationSecret)
		assert.Equal(t, models.InstanceSettingsAuditSecretChanged, decodeAuditDoc(t, e.After)["client_secret"])
	}
	allowlistAudits := authAuditEntries(t, models.InstanceSettingAuthAllowlist)
	require.Len(t, allowlistAudits, 1)
	assert.Equal(t, models.InstanceSettingsAuditActionImport, allowlistAudits[0].Action)
	assert.Nil(t, allowlistAudits[0].ActorUserID)

	// Second boot with the same keys: the database wins and nothing changes.
	again := services.ImportLegacyAuthConfig(ctx, deps, legacyAuthIntegrationConfig())
	assert.Equal(t, services.LegacyAuthImportResult{ProvidersIgnored: true, AllowlistIgnored: true}, again)
	assert.Equal(t, start+2, authSettingsVersion(t))
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 2)
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthAllowlist), 1)
}

// barrierListProviderRepo holds every replica after its List until all have
// read, so each sees an empty table and the repository alone decides.
type barrierListProviderRepo struct {
	repositories.InstanceAuthProviderRepository
	readers *sync.WaitGroup
}

func (r barrierListProviderRepo) List(ctx context.Context) ([]*models.InstanceAuthProvider, error) {
	rows, err := r.InstanceAuthProviderRepository.List(ctx)
	r.readers.Done()
	r.readers.Wait()
	return rows, err
}

// barrierGetAllowlistRepo does the same for the allowlist read.
type barrierGetAllowlistRepo struct {
	repositories.InstanceAuthAllowlistRepository
	readers *sync.WaitGroup
}

func (r barrierGetAllowlistRepo) Get(ctx context.Context) (*models.InstanceAuthAllowlist, error) {
	row, err := r.InstanceAuthAllowlistRepository.Get(ctx)
	r.readers.Done()
	r.readers.Wait()
	return row, err
}

func TestIntegrationLegacyAuthImport_ConcurrentBootsStoreOneSet(t *testing.T) {
	resetInstanceAuthSettings(t)
	legacy := legacyAuthIntegrationConfig()

	const replicas = 3
	results := make([]services.LegacyAuthImportResult, replicas)
	var providerReaders, allowlistReaders, wg sync.WaitGroup
	providerReaders.Add(replicas)
	allowlistReaders.Add(replicas)
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deps, _ := legacyAuthIntegrationDeps(t)
			deps.ProviderRepo = barrierListProviderRepo{deps.ProviderRepo, &providerReaders}
			deps.AllowlistRepo = barrierGetAllowlistRepo{deps.AllowlistRepo, &allowlistReaders}
			results[i] = services.ImportLegacyAuthConfig(context.Background(), deps, legacy)
		}()
	}
	wg.Wait()

	providers, err := NewInstanceAuthProviderRepository(integrationDB).List(context.Background())
	require.NoError(t, err)
	assert.Len(t, providers, 2, "no duplicate and no partial provider set")
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 2)
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthAllowlist), 1)

	providerImports, allowlistImports := 0, 0
	for _, r := range results {
		if len(r.ProvidersImported) > 0 {
			providerImports++
		}
		if r.AllowlistImported {
			allowlistImports++
		}
	}
	assert.Equal(t, 1, providerImports, "exactly one replica imports the providers")
	assert.Equal(t, 1, allowlistImports, "exactly one replica imports the allowlist")
}
