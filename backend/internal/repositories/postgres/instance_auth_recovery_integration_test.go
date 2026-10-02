//go:build integration

package postgres

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// Break-glass auth recovery (#1237) against real Postgres with no server: the
// service behind `vibexp admin auth` over the real repositories, the CLI audit
// marker, the shared version bump a running server resolves on, idempotency,
// and a re-armed token completing the #1236 exchange. The tables are global to
// the shared test database, so no test runs in parallel.

const recoveryBaseURL = "https://vibexp.example.com"

func newIntegrationAuthRecovery() *services.AuthRecoveryService {
	return services.NewAuthRecoveryService(services.AuthRecoveryDeps{
		Providers: NewInstanceAuthProviderRepository(integrationDB),
		Allowlist: NewInstanceAuthAllowlistRepository(integrationDB),
		Setup:     NewInstanceAuthSetupRepository(integrationDB),
		Version:   NewInstanceAuthSettingsVersionRepository(integrationDB),
		BaseURL:   recoveryBaseURL,
	})
}

func TestIntegrationAuthRecovery_ProviderToggle(t *testing.T) {
	resetInstanceAuthSettings(t)
	ctx := context.Background()
	providers := NewInstanceAuthProviderRepository(integrationDB)
	recovery := newIntegrationAuthRecovery()

	ciphertext := providerCiphertext
	require.NoError(t, providers.Create(ctx, googleProviderFixture(&ciphertext), nil, nil))
	require.NoError(t, providers.Create(ctx, oidcProviderFixture("corp-sso", 1), nil, nil))
	start := authSettingsVersion(t)
	require.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 2)

	listed, err := recovery.ListProviders(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 2)

	t.Run("disable is audited as a CLI write and bumps the shared version", func(t *testing.T) {
		got, err := recovery.SetProviderEnabled(ctx, "google", false)
		require.NoError(t, err)
		assert.Equal(t, services.ProviderToggle{Changed: true, NoneEnabled: true}, got,
			"the OIDC fixture is disabled, so disabling google leaves none enabled")
		assert.Equal(t, start+1, authSettingsVersion(t))

		row, err := providers.GetBySlug(ctx, "google")
		require.NoError(t, err)
		assert.False(t, row.Enabled)
		require.NotNil(t, row.ClientSecretEncrypted, "the whole-row write keeps the stored secret")
		assert.Equal(t, providerCiphertext, *row.ClientSecretEncrypted)
		assert.Equal(t, "Google", row.DisplayName)
		assert.Nil(t, row.UpdatedBy)

		entries := authAuditEntries(t, models.InstanceSettingAuthProviders) // newest first
		require.Len(t, entries, 3)
		assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entries[0].Action)
		assert.Nil(t, entries[0].ActorUserID, "a CLI write has no acting user")
		after := decodeAuditDoc(t, entries[0].After)
		assert.Equal(t, repositories.AuditSourceCLI, after["source"])
		assert.Equal(t, false, after["enabled"])
		assert.Equal(t, models.InstanceSettingsAuditSecretUnchanged, after["client_secret"])
		assert.NotContains(t, decodeAuditDoc(t, entries[0].Before), "source", "only the written state is marked")
		assert.NotContains(t, decodeAuditDoc(t, entries[1].After), "source",
			"a write that did not come from the CLI carries no source")
	})

	t.Run("disable again: no change, no audit entry, no version bump", func(t *testing.T) {
		got, err := recovery.SetProviderEnabled(ctx, "google", false)
		require.NoError(t, err)
		assert.False(t, got.Changed)
		assert.True(t, got.NoneEnabled)
		assert.Equal(t, start+1, authSettingsVersion(t))
		assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 3)
	})

	t.Run("enable, then enable again", func(t *testing.T) {
		got, err := recovery.SetProviderEnabled(ctx, "google", true)
		require.NoError(t, err)
		assert.Equal(t, services.ProviderToggle{Changed: true}, got)
		assert.Equal(t, start+2, authSettingsVersion(t))
		entries := authAuditEntries(t, models.InstanceSettingAuthProviders)
		require.Len(t, entries, 4)
		after := decodeAuditDoc(t, entries[0].After)
		assert.Equal(t, repositories.AuditSourceCLI, after["source"])
		assert.Equal(t, true, after["enabled"])

		got, err = recovery.SetProviderEnabled(ctx, "google", true)
		require.NoError(t, err)
		assert.Equal(t, services.ProviderToggle{}, got)
		assert.Equal(t, start+2, authSettingsVersion(t))
		assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 4)
	})

	t.Run("an unknown slug writes nothing", func(t *testing.T) {
		_, err := recovery.SetProviderEnabled(ctx, "nope", false)
		require.ErrorIs(t, err, repositories.ErrInstanceAuthProviderNotFound)
		assert.Equal(t, start+2, authSettingsVersion(t))
		assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 4)
	})
}

func TestIntegrationAuthRecovery_ClearAllowlist(t *testing.T) {
	resetInstanceAuthSettings(t)
	ctx := context.Background()
	allowlist := NewInstanceAuthAllowlistRepository(integrationDB)
	recovery := newIntegrationAuthRecovery()

	require.NoError(t, allowlist.UpsertAudited(ctx,
		&models.InstanceAuthAllowlist{Domains: []string{"example.com"}}, nil, nil))
	start := authSettingsVersion(t)

	cleared, err := recovery.ClearAllowlist(ctx)
	require.NoError(t, err)
	assert.True(t, cleared)
	_, err = allowlist.Get(ctx)
	require.ErrorIs(t, err, repositories.ErrInstanceAuthAllowlistNotFound, "open access again")
	assert.Equal(t, start+1, authSettingsVersion(t))

	entries := authAuditEntries(t, models.InstanceSettingAuthAllowlist) // newest first
	require.Len(t, entries, 2)
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entries[0].Action)
	assert.Nil(t, entries[0].ActorUserID)
	assert.Nil(t, entries[0].After, "a delete keeps a nil after; the marker goes on the removed state")
	before := decodeAuditDoc(t, entries[0].Before)
	assert.Equal(t, repositories.AuditSourceCLI, before["source"])
	assert.Equal(t, []any{"example.com"}, before["domains"])

	again, err := recovery.ClearAllowlist(ctx)
	require.NoError(t, err)
	assert.False(t, again, "nothing stored: no change")
	assert.Equal(t, start+1, authSettingsVersion(t))
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthAllowlist), 2)
}

// TestIntegrationAuthRecovery_RearmSetup runs `setup rearm` against the
// database and completes the #1236 exchange with the printed token through a
// setup-mode service standing in for the running server.
func TestIntegrationAuthRecovery_RearmSetup(t *testing.T) {
	resetInstanceAuthSettings(t)
	resetInstanceAuthSetup(t)
	ctx := context.Background()
	providers := NewInstanceAuthProviderRepository(integrationDB)
	recovery := newIntegrationAuthRecovery()

	ciphertext := providerCiphertext
	require.NoError(t, providers.Create(ctx, googleProviderFixture(&ciphertext), nil, nil)) // enabled
	server := services.NewSetupModeService(services.SetupModeDeps{
		Setup:       NewInstanceAuthSetupRepository(integrationDB),
		Providers:   providers,
		IsRootAdmin: func(string) bool { return false },
		BaseURL:     recoveryBaseURL,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	active, err := server.IsActive(ctx)
	require.NoError(t, err)
	require.False(t, active, "a provider is enabled, so setup mode is off before the re-arm")
	versionBefore := authSettingsVersion(t)

	const prefix = recoveryBaseURL + "/setup?token="
	firstURL, err := recovery.RearmSetup(ctx)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(firstURL, prefix), firstURL)

	active, err = server.IsActive(ctx)
	require.NoError(t, err)
	assert.True(t, active, "re-arming forces setup mode on with a provider enabled")
	sess, err := server.ExchangeToken(ctx, strings.TrimPrefix(firstURL, prefix))
	require.NoError(t, err, "the printed token completes the exchange")
	require.NoError(t, server.ValidateSession(ctx, sess))

	entries := authAuditEntries(t, models.InstanceSettingAuthSetup)
	require.Len(t, entries, 1)
	assert.Nil(t, entries[0].ActorUserID)
	after := decodeAuditDoc(t, entries[0].After)
	assert.Equal(t, "rearmed", after["event"])
	assert.Equal(t, repositories.AuditSourceCLI, after["source"])
	assert.NotContains(t, after, "token_hash")

	secondURL, err := recovery.RearmSetup(ctx)
	require.NoError(t, err)
	require.NotEqual(t, firstURL, secondURL)
	_, err = server.ExchangeToken(ctx, strings.TrimPrefix(firstURL, prefix))
	require.ErrorIs(t, err, services.ErrSetupTokenInvalid, "the previous token stops working")
	require.ErrorIs(t, server.ValidateSession(ctx, sess), services.ErrSetupSessionInvalid,
		"and so does the setup session issued for it")
	_, err = server.ExchangeToken(ctx, strings.TrimPrefix(secondURL, prefix))
	require.NoError(t, err)
	assert.Equal(t, versionBefore, authSettingsVersion(t), "setup state is not part of the settings caches")
}

func TestIntegrationInstanceAuthSetup_MintReplacing(t *testing.T) {
	resetInstanceAuthSetup(t)
	repo := NewInstanceAuthSetupRepository(integrationDB)
	ctx := context.Background()
	now := time.Now().UTC()

	row, err := repo.MintReplacing(ctx, setupTokenHash("a"), now.Add(24*time.Hour))
	require.NoError(t, err)
	assert.False(t, row.Rearmed, "the recovery-mode mint does not re-arm")
	assert.Equal(t, int64(1), row.Generation)

	// Unlike MintIfAbsentOrExpired, a live token does not stop it.
	row, err = repo.MintReplacing(ctx, setupTokenHash("b"), now.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, setupTokenHash("b"), row.TokenHash)
	assert.Equal(t, int64(2), row.Generation, "outstanding setup sessions are invalidated")
	assert.False(t, row.Rearmed)

	entries := authAuditEntries(t, models.InstanceSettingAuthSetup)
	require.Len(t, entries, 2)
	for _, entry := range entries {
		after := decodeAuditDoc(t, entry.After)
		assert.Equal(t, "token_minted", after["event"])
		assert.NotContains(t, after, "source")
		assert.Nil(t, entry.ActorUserID)
	}

	t.Run("it keeps an earlier re-arm that was never consumed", func(t *testing.T) {
		_, err := repo.ForceMint(ctx, setupTokenHash("c"), now.Add(24*time.Hour))
		require.NoError(t, err)
		row, err := repo.MintReplacing(ctx, setupTokenHash("d"), now.Add(24*time.Hour))
		require.NoError(t, err)
		assert.True(t, row.Rearmed)
	})

	t.Run("it clears a previous consumption", func(t *testing.T) {
		consumed, err := repo.Consume(ctx, insertTestUser(t))
		require.NoError(t, err)
		require.True(t, consumed)
		row, err := repo.MintReplacing(ctx, setupTokenHash("e"), now.Add(24*time.Hour))
		require.NoError(t, err)
		assert.False(t, row.IsConsumed())
		assert.False(t, row.Rearmed)
	})
}
