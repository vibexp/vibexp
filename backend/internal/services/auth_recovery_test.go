package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

type authRecoveryFixture struct {
	svc       *AuthRecoveryService
	providers *mocks.MockInstanceAuthProviderRepository
	allowlist *mocks.MockInstanceAuthAllowlistRepository
	version   *mocks.MockInstanceAuthSettingsVersionRepository
	setup     *memSetupRepo
}

func newAuthRecoveryFixture(t *testing.T) *authRecoveryFixture {
	t.Helper()
	f := &authRecoveryFixture{
		providers: mocks.NewMockInstanceAuthProviderRepository(t),
		allowlist: mocks.NewMockInstanceAuthAllowlistRepository(t),
		version:   mocks.NewMockInstanceAuthSettingsVersionRepository(t),
		setup:     &memSetupRepo{},
	}
	f.svc = NewAuthRecoveryService(AuthRecoveryDeps{
		Providers: f.providers,
		Allowlist: f.allowlist,
		Setup:     f.setup,
		Version:   f.version,
		BaseURL:   "https://vibexp.example.com/",
	})
	return f
}

// cliAudited matches a context marked as a CLI write; plainCtx one that is not.
var (
	cliAudited = mock.MatchedBy(func(ctx context.Context) bool {
		return repositories.AuditSourceFromContext(ctx) == repositories.AuditSourceCLI
	})
	errRecoveryDB = errors.New("db down")
)

func recoveryProvider(slug string, enabled bool) *models.InstanceAuthProvider {
	secret := "enc:v1:ciphertext"
	return &models.InstanceAuthProvider{
		ID: "id-" + slug, Slug: slug, Type: models.InstanceAuthProviderOIDC, DisplayName: "SSO",
		Enabled: enabled, ClientID: "client", ClientSecretEncrypted: &secret,
	}
}

func TestAuthRecovery_ListProviders(t *testing.T) {
	ctx := context.Background()

	f := newAuthRecoveryFixture(t)
	rows := []*models.InstanceAuthProvider{recoveryProvider("sso", true)}
	f.providers.EXPECT().List(ctx).Return(rows, nil).Once()
	got, err := f.svc.ListProviders(ctx)
	require.NoError(t, err)
	assert.Equal(t, rows, got)

	f = newAuthRecoveryFixture(t)
	f.providers.EXPECT().List(ctx).Return(nil, errRecoveryDB).Once()
	_, err = f.svc.ListProviders(ctx)
	require.ErrorIs(t, err, errRecoveryDB)
}

func TestAuthRecovery_SetProviderEnabled(t *testing.T) {
	ctx := context.Background()
	version := int64(7)

	t.Run("disable writes the row as read, with no actor, as a CLI-audited compare-and-set", func(t *testing.T) {
		f := newAuthRecoveryFixture(t)
		stored := recoveryProvider("sso", true)
		f.version.EXPECT().Get(ctx).Return(version, nil).Once()
		f.providers.EXPECT().GetBySlug(ctx, "sso").Return(stored, nil).Once()
		f.providers.EXPECT().
			Update(cliAudited, mock.MatchedBy(func(p *models.InstanceAuthProvider) bool {
				// The whole row is written, so everything but enabled must be what was read.
				return !p.Enabled && p.ID == "id-sso" && p.ClientSecretEncrypted != nil && p.ClientID == "client"
			}), (*string)(nil), &version).
			Return(nil).Once()
		f.providers.EXPECT().List(ctx).
			Return([]*models.InstanceAuthProvider{recoveryProvider("sso", false), recoveryProvider("other", true)}, nil).Once()

		got, err := f.svc.SetProviderEnabled(ctx, "sso", false)
		require.NoError(t, err)
		assert.Equal(t, ProviderToggle{Changed: true, NoneEnabled: false}, got)
	})

	t.Run("disabling the last enabled provider is allowed and reported", func(t *testing.T) {
		f := newAuthRecoveryFixture(t)
		f.version.EXPECT().Get(ctx).Return(version, nil).Once()
		f.providers.EXPECT().GetBySlug(ctx, "sso").Return(recoveryProvider("sso", true), nil).Once()
		f.providers.EXPECT().Update(cliAudited, mock.Anything, (*string)(nil), &version).Return(nil).Once()
		f.providers.EXPECT().List(ctx).
			Return([]*models.InstanceAuthProvider{recoveryProvider("sso", false)}, nil).Once()

		got, err := f.svc.SetProviderEnabled(ctx, "sso", false)
		require.NoError(t, err)
		assert.Equal(t, ProviderToggle{Changed: true, NoneEnabled: true}, got)
	})

	t.Run("enable flips a disabled provider", func(t *testing.T) {
		f := newAuthRecoveryFixture(t)
		f.version.EXPECT().Get(ctx).Return(version, nil).Once()
		f.providers.EXPECT().GetBySlug(ctx, "sso").Return(recoveryProvider("sso", false), nil).Once()
		f.providers.EXPECT().
			Update(cliAudited, mock.MatchedBy(func(p *models.InstanceAuthProvider) bool { return p.Enabled }),
				(*string)(nil), &version).
			Return(nil).Once()
		f.providers.EXPECT().List(ctx).
			Return([]*models.InstanceAuthProvider{recoveryProvider("sso", true)}, nil).Once()

		got, err := f.svc.SetProviderEnabled(ctx, "sso", true)
		require.NoError(t, err)
		assert.Equal(t, ProviderToggle{Changed: true}, got)
	})

	for _, enabled := range []bool{true, false} {
		t.Run("already in the requested state: nothing is written", func(t *testing.T) {
			f := newAuthRecoveryFixture(t) // an Update call would fail the mock
			f.version.EXPECT().Get(ctx).Return(version, nil).Once()
			f.providers.EXPECT().GetBySlug(ctx, "sso").Return(recoveryProvider("sso", enabled), nil).Once()
			f.providers.EXPECT().List(ctx).
				Return([]*models.InstanceAuthProvider{recoveryProvider("sso", enabled)}, nil).Once()

			got, err := f.svc.SetProviderEnabled(ctx, "sso", enabled)
			require.NoError(t, err)
			assert.Equal(t, ProviderToggle{Changed: false, NoneEnabled: !enabled}, got)
		})
	}

	t.Run("an unknown slug is ErrInstanceAuthProviderNotFound", func(t *testing.T) {
		f := newAuthRecoveryFixture(t)
		f.version.EXPECT().Get(ctx).Return(version, nil).Once()
		f.providers.EXPECT().GetBySlug(ctx, "nope").Return(nil, repositories.ErrInstanceAuthProviderNotFound).Once()

		_, err := f.svc.SetProviderEnabled(ctx, "nope", false)
		require.ErrorIs(t, err, repositories.ErrInstanceAuthProviderNotFound)
	})

	t.Run("a concurrent settings write is re-read, not overwritten", func(t *testing.T) {
		f := newAuthRecoveryFixture(t)
		next := version + 1
		rotated := recoveryProvider("sso", true)
		rotatedSecret := "enc:v1:rotated"
		rotated.ClientSecretEncrypted = &rotatedSecret

		f.version.EXPECT().Get(ctx).Return(version, nil).Once()
		f.providers.EXPECT().GetBySlug(ctx, "sso").Return(recoveryProvider("sso", true), nil).Once()
		f.providers.EXPECT().Update(cliAudited, mock.Anything, (*string)(nil), &version).
			Return(repositories.ErrInstanceSettingsVersionConflict).Once()
		f.version.EXPECT().Get(ctx).Return(next, nil).Once()
		f.providers.EXPECT().GetBySlug(ctx, "sso").Return(rotated, nil).Once()
		f.providers.EXPECT().
			Update(cliAudited, mock.MatchedBy(func(p *models.InstanceAuthProvider) bool {
				return !p.Enabled && *p.ClientSecretEncrypted == rotatedSecret
			}), (*string)(nil), &next).
			Return(nil).Once()
		f.providers.EXPECT().List(ctx).Return([]*models.InstanceAuthProvider{rotated}, nil).Once()

		got, err := f.svc.SetProviderEnabled(ctx, "sso", false)
		require.NoError(t, err)
		assert.True(t, got.Changed)
	})

	t.Run("it gives up after repeated conflicts", func(t *testing.T) {
		f := newAuthRecoveryFixture(t)
		f.version.EXPECT().Get(ctx).Return(version, nil).Times(authRecoveryToggleAttempts)
		f.providers.EXPECT().GetBySlug(ctx, "sso").
			RunAndReturn(func(context.Context, string) (*models.InstanceAuthProvider, error) {
				return recoveryProvider("sso", true), nil
			}).Times(authRecoveryToggleAttempts)
		f.providers.EXPECT().Update(cliAudited, mock.Anything, (*string)(nil), &version).
			Return(repositories.ErrInstanceSettingsVersionConflict).Times(authRecoveryToggleAttempts)

		_, err := f.svc.SetProviderEnabled(ctx, "sso", false)
		require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)
	})

	t.Run("read and write failures are returned", func(t *testing.T) {
		f := newAuthRecoveryFixture(t)
		f.version.EXPECT().Get(ctx).Return(int64(0), errRecoveryDB).Once()
		_, err := f.svc.SetProviderEnabled(ctx, "sso", false)
		require.ErrorIs(t, err, errRecoveryDB)

		f = newAuthRecoveryFixture(t)
		f.version.EXPECT().Get(ctx).Return(version, nil).Once()
		f.providers.EXPECT().GetBySlug(ctx, "sso").Return(recoveryProvider("sso", true), nil).Once()
		f.providers.EXPECT().Update(cliAudited, mock.Anything, (*string)(nil), &version).Return(errRecoveryDB).Once()
		_, err = f.svc.SetProviderEnabled(ctx, "sso", false)
		require.ErrorIs(t, err, errRecoveryDB)

		f = newAuthRecoveryFixture(t)
		f.version.EXPECT().Get(ctx).Return(version, nil).Once()
		f.providers.EXPECT().GetBySlug(ctx, "sso").Return(recoveryProvider("sso", false), nil).Once()
		f.providers.EXPECT().List(ctx).Return(nil, errRecoveryDB).Once()
		_, err = f.svc.SetProviderEnabled(ctx, "sso", false)
		require.ErrorIs(t, err, errRecoveryDB)
	})
}

func TestAuthRecovery_ClearAllowlist(t *testing.T) {
	ctx := context.Background()

	for _, deleted := range []bool{true, false} {
		f := newAuthRecoveryFixture(t)
		f.allowlist.EXPECT().DeleteAudited(cliAudited, (*string)(nil)).Return(deleted, nil).Once()
		cleared, err := f.svc.ClearAllowlist(ctx)
		require.NoError(t, err)
		assert.Equal(t, deleted, cleared)
	}

	f := newAuthRecoveryFixture(t)
	f.allowlist.EXPECT().DeleteAudited(cliAudited, (*string)(nil)).Return(false, errRecoveryDB).Once()
	_, err := f.svc.ClearAllowlist(ctx)
	require.ErrorIs(t, err, errRecoveryDB)
}

// TestAuthRecovery_RearmSetup proves the printed URL's token completes the
// #1236 exchange on a server sharing the setup state, and that the previous
// token and session stop working.
func TestAuthRecovery_RearmSetup(t *testing.T) {
	ctx := context.Background()
	const prefix = "https://vibexp.example.com/setup?token="

	server := newSetupFixture(t, false)
	server.enableProvider() // a healthy-looking instance: setup mode is off
	_, err := server.svc.ExchangeToken(ctx, "anything")
	require.ErrorIs(t, err, ErrSetupNotActive)

	recovery := NewAuthRecoveryService(AuthRecoveryDeps{Setup: server.repo, BaseURL: "https://vibexp.example.com/"})
	recovery.now = func() time.Time { return server.now }

	firstURL, err := recovery.RearmSetup(ctx)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(firstURL, prefix), firstURL)
	first := strings.TrimPrefix(firstURL, prefix)
	sess, err := server.svc.ExchangeToken(ctx, first)
	require.NoError(t, err, "the printed token exchanges even though a provider is enabled")
	require.NoError(t, server.svc.ValidateSession(ctx, sess))
	assert.Equal(t, server.now.Add(SetupTokenLifetime), *server.repo.row.ExpiresAt)

	secondURL, err := recovery.RearmSetup(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, firstURL, secondURL)
	_, err = server.svc.ExchangeToken(ctx, first)
	require.ErrorIs(t, err, ErrSetupTokenInvalid, "re-arming replaces the previous token")
	require.ErrorIs(t, server.svc.ValidateSession(ctx, sess), ErrSetupSessionInvalid,
		"and ends the previous setup session")
	_, err = server.svc.ExchangeToken(ctx, strings.TrimPrefix(secondURL, prefix))
	require.NoError(t, err)

	server.repo.writeErr = errRecoveryDB
	_, err = recovery.RearmSetup(ctx)
	require.ErrorIs(t, err, errRecoveryDB)
}
