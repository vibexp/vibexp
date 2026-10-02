package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/testutils/memauthsettings"
)

const (
	authSettingsCallbackURL = "https://vibexp.test/api/v1/auth/callback"
	authSettingsRootID      = "11111111-1111-4111-8111-111111111111"
	authSettingsRootEmail   = "root@example.com"
	authSettingsDBAdminID   = "22222222-2222-4222-8222-222222222222"
	authSettingsMemberID    = "33333333-3333-4333-8333-333333333333"
	authSettingsMemberEmail = "Ada@Example.com"
	authSettingsSuspendedID = "44444444-4444-4444-8444-444444444444"
	authSettingsIssuer      = "https://issuer.example.com"
)

// authSettingsSecret is the plaintext client secret the fixtures store. It is
// built at run time so the fixture does not read as a committed credential.
var authSettingsSecret = strings.Repeat("s", 8) + "-client-value"

// fakeAuthResolver is an IdentityProviderResolver whose health and test
// outcome a test sets directly.
type fakeAuthResolver struct {
	health      map[string]ProviderHealth
	snapshotErr error
	testErr     error
	snapshots   int
	tested      []models.InstanceAuthProvider
	testSecrets []*string
}

func (f *fakeAuthResolver) Snapshot(context.Context) (*idp.Registry, error) {
	f.snapshots++
	return nil, f.snapshotErr
}

func (f *fakeAuthResolver) Provider(context.Context, string) (idp.IdentityProvider, bool, error) {
	return nil, false, nil
}

func (f *fakeAuthResolver) Health() map[string]ProviderHealth { return f.health }

func (f *fakeAuthResolver) TestProvider(
	_ context.Context, candidate models.InstanceAuthProvider, secret *string,
) error {
	f.tested = append(f.tested, candidate)
	f.testSecrets = append(f.testSecrets, secret)
	return f.testErr
}

// authSettingsUsers is a UserRepository answering by id and by exact email.
type authSettingsUsers struct {
	repositories.UserRepository
	users []*models.User
	err   error
}

func (u *authSettingsUsers) GetByID(_ context.Context, id string) (*models.User, error) {
	if u.err != nil {
		return nil, u.err
	}
	for _, user := range u.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, repositories.ErrUserNotFound
}

func (u *authSettingsUsers) GetByEmail(_ context.Context, email string) (*models.User, error) {
	if u.err != nil {
		return nil, u.err
	}
	for _, user := range u.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, repositories.ErrUserNotFound
}

type authSettingsFixture struct {
	svc      *InstanceAuthSettingsService
	store    *memauthsettings.Store
	resolver *fakeAuthResolver
	users    *authSettingsUsers
	enc      *EncryptionService
}

func newAuthSettingsFixture(t *testing.T) *authSettingsFixture {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	enc, err := NewEncryptionService(strings.Repeat("k", 32))
	require.NoError(t, err)

	f := &authSettingsFixture{
		store:    memauthsettings.New(),
		resolver: &fakeAuthResolver{},
		enc:      enc,
		users: &authSettingsUsers{users: []*models.User{
			{ID: authSettingsRootID, Email: authSettingsRootEmail, Name: "Root", Status: models.UserStatusActive},
			{ID: authSettingsDBAdminID, Email: "delegate@example.com", Status: models.UserStatusActive},
			{ID: authSettingsMemberID, Email: authSettingsMemberEmail, Name: "Ada", Status: models.UserStatusActive},
			{ID: authSettingsSuspendedID, Email: "gone@example.com", Status: models.UserStatusSuspended},
		}},
	}
	admins := NewInstanceAdminService([]string{authSettingsRootEmail}, f.store.Grants(), f.users, logger)
	f.svc = NewInstanceAuthSettingsService(InstanceAuthSettingsDeps{
		Providers:  f.store.Providers(),
		Allowlists: f.store.Allowlist(),
		Versions:   f.store.Versions(),
		Grants:     f.store.Grants(),
		Users:      f.users,
		Resolver:   f.resolver,
		AllowlistResolver: NewAccessAllowlistResolver(AccessAllowlistResolverDeps{
			Allowlists: f.store.Allowlist(), Versions: f.store.Versions(), RootAdmins: admins, Logger: logger,
		}),
		Admins:      admins,
		Enc:         enc,
		CallbackURL: authSettingsCallbackURL,
		Logger:      logger,
	})
	return f
}

func oidcCreate(slug string) models.CreateInstanceAuthProviderRequest {
	issuer := authSettingsIssuer
	return models.CreateInstanceAuthProviderRequest{
		Type: models.InstanceAuthProviderOIDC, Slug: slug, DisplayName: " " + slug + " ", Enabled: true,
		ClientID: " client-" + slug + " ", ClientSecret: authSettingsSecret, IssuerURL: &issuer,
	}
}

func (f *authSettingsFixture) create(t *testing.T, req models.CreateInstanceAuthProviderRequest) *models.InstanceAuthProviderSaved {
	t.Helper()
	saved, err := f.svc.CreateProvider(context.Background(), authSettingsRootID, req)
	require.NoError(t, err)
	return saved
}

// updateOf is a whole-row update that changes nothing about row.
func updateOf(row models.InstanceAuthProvider, version int64) models.UpdateInstanceAuthProviderRequest {
	return models.UpdateInstanceAuthProviderRequest{
		DisplayName: row.DisplayName, Enabled: row.Enabled, SortOrder: row.SortOrder,
		ClientID: row.ClientID, IssuerURL: row.IssuerURL, ExpectedVersion: version,
	}
}

// oktaSecret returns the decrypted client secret stored for the "okta" provider.
func (f *authSettingsFixture) oktaSecret(t *testing.T) string {
	t.Helper()
	row := f.store.Provider("okta")
	require.NotNil(t, row)
	require.NotNil(t, row.ClientSecretEncrypted)
	plain, err := f.enc.Decrypt(*row.ClientSecretEncrypted)
	require.NoError(t, err)
	return plain
}

func settingsFieldOf(t *testing.T, err error) string {
	t.Helper()
	var fieldErr *SettingsFieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Len(t, fieldErr.Fields, 1)
	return fieldErr.Fields[0]
}

func TestInstanceAuthSettings_CreateProvider(t *testing.T) {
	f := newAuthSettingsFixture(t)
	before := f.store.Version()

	saved := f.create(t, oidcCreate("okta"))

	assert.Equal(t, before+1, saved.Version)
	assert.Equal(t, f.store.Version(), saved.Version, "the reported version is the stored one")
	p := saved.Provider.Provider
	assert.Equal(t, "okta", p.DisplayName, "the display name is trimmed")
	assert.Equal(t, "client-okta", p.ClientID, "the client id is trimmed")
	assert.Equal(t, authSettingsCallbackURL, saved.Provider.RedirectURI)
	assert.True(t, p.HasClientSecret())
	assert.NotContains(t, *p.ClientSecretEncrypted, authSettingsSecret, "the secret is stored encrypted")
	assert.Equal(t, authSettingsSecret, f.oktaSecret(t))

	entries := f.store.AuditEntries(models.InstanceSettingAuthProviders)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].ActorUserID)
	assert.Equal(t, authSettingsRootID, *entries[0].ActorUserID)
}

func TestInstanceAuthSettings_CreateProvider_SetupSessionHasNoActor(t *testing.T) {
	f := newAuthSettingsFixture(t)

	_, err := f.svc.CreateProvider(context.Background(), "", oidcCreate("okta"))
	require.NoError(t, err)

	entries := f.store.AuditEntries(models.InstanceSettingAuthProviders)
	require.Len(t, entries, 1)
	assert.Nil(t, entries[0].ActorUserID)
	assert.Nil(t, f.store.Provider("okta").UpdatedBy)
}

func TestInstanceAuthSettings_CreateProvider_Rejections(t *testing.T) {
	stale := int64(99)
	cases := []struct {
		name    string
		mutate  func(*models.CreateInstanceAuthProviderRequest)
		field   string
		wantErr error
	}{
		{name: "blank secret", field: "client_secret", wantErr: ErrInvalidInstanceAuthProvider,
			mutate: func(r *models.CreateInstanceAuthProviderRequest) { r.ClientSecret = "  " }},
		{name: "bad slug", field: "slug", wantErr: ErrInvalidInstanceAuthProvider,
			mutate: func(r *models.CreateInstanceAuthProviderRequest) { r.Slug = "Not A Slug" }},
		{name: "oidc without issuer", field: "issuer_url", wantErr: ErrInvalidInstanceAuthProvider,
			mutate: func(r *models.CreateInstanceAuthProviderRequest) { r.IssuerURL = nil }},
		{name: "github with issuer", field: "issuer_url", wantErr: ErrInvalidInstanceAuthProvider,
			mutate: func(r *models.CreateInstanceAuthProviderRequest) { r.Type = models.InstanceAuthProviderGitHub }},
		{name: "stale expected version", wantErr: repositories.ErrInstanceSettingsVersionConflict,
			mutate: func(r *models.CreateInstanceAuthProviderRequest) { r.ExpectedVersion = &stale }},
		{name: "taken slug", wantErr: repositories.ErrInstanceAuthProviderConflict,
			mutate: func(r *models.CreateInstanceAuthProviderRequest) { r.Slug = "taken" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			f.create(t, oidcCreate("taken"))
			version := f.store.Version()
			req := oidcCreate("okta")
			tc.mutate(&req)

			_, err := f.svc.CreateProvider(context.Background(), authSettingsRootID, req)

			require.ErrorIs(t, err, tc.wantErr)
			if tc.field != "" {
				assert.Equal(t, tc.field, settingsFieldOf(t, err))
			}
			assert.Equal(t, version, f.store.Version(), "nothing was written")
			assert.Nil(t, f.store.Provider("okta"))
		})
	}
}

func TestInstanceAuthSettings_CreateProvider_NoEncryptionKey(t *testing.T) {
	f := newAuthSettingsFixture(t)
	f.svc.deps.Enc = nil

	_, err := f.svc.CreateProvider(context.Background(), authSettingsRootID, oidcCreate("okta"))

	require.ErrorIs(t, err, ErrEncryptionUnavailable)
	assert.Nil(t, f.store.Provider("okta"))
}

func TestInstanceAuthSettings_UpdateProvider_SecretRules(t *testing.T) {
	replacement := "replacement-" + authSettingsSecret
	blank := " "
	otherIssuer := "https://other.example.com"

	t.Run("an omitted secret keeps the stored one", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		saved := f.create(t, oidcCreate("okta"))
		req := updateOf(saved.Provider.Provider, saved.Version)
		req.DisplayName = "Renamed"

		updated, err := f.svc.UpdateProvider(context.Background(), authSettingsRootID, saved.Provider.Provider.ID, req)

		require.NoError(t, err)
		assert.Equal(t, saved.Version+1, updated.Version)
		assert.Equal(t, "Renamed", f.store.Provider("okta").DisplayName)
		assert.Equal(t, authSettingsSecret, f.oktaSecret(t))
	})

	t.Run("a submitted secret replaces the stored one", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		saved := f.create(t, oidcCreate("okta"))
		req := updateOf(saved.Provider.Provider, saved.Version)
		req.ClientSecret = &replacement

		_, err := f.svc.UpdateProvider(context.Background(), authSettingsRootID, saved.Provider.Provider.ID, req)

		require.NoError(t, err)
		assert.Equal(t, replacement, f.oktaSecret(t))
	})

	t.Run("an empty secret is rejected", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		saved := f.create(t, oidcCreate("okta"))
		req := updateOf(saved.Provider.Provider, saved.Version)
		req.ClientSecret = &blank

		_, err := f.svc.UpdateProvider(context.Background(), authSettingsRootID, saved.Provider.Provider.ID, req)

		require.ErrorIs(t, err, ErrInvalidInstanceAuthProvider)
		assert.Equal(t, "client_secret", settingsFieldOf(t, err))
		assert.Equal(t, authSettingsSecret, f.oktaSecret(t))
	})

	t.Run("the stored secret is not kept across an issuer change", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		saved := f.create(t, oidcCreate("okta"))
		req := updateOf(saved.Provider.Provider, saved.Version)
		req.IssuerURL = &otherIssuer

		_, err := f.svc.UpdateProvider(context.Background(), authSettingsRootID, saved.Provider.Provider.ID, req)

		require.ErrorIs(t, err, ErrInvalidInstanceAuthProvider)
		assert.Equal(t, "client_secret", settingsFieldOf(t, err))
		assert.Equal(t, authSettingsIssuer, *f.store.Provider("okta").IssuerURL, "nothing was written")

		req.ClientSecret = &replacement
		_, err = f.svc.UpdateProvider(context.Background(), authSettingsRootID, saved.Provider.Provider.ID, req)
		require.NoError(t, err, "an issuer change with its own secret is accepted")
		assert.Equal(t, otherIssuer, *f.store.Provider("okta").IssuerURL)
		assert.Equal(t, replacement, f.oktaSecret(t))
	})
}

func TestInstanceAuthSettings_UpdateProvider_SlugAndTypeAreImmutable(t *testing.T) {
	f := newAuthSettingsFixture(t)
	saved := f.create(t, oidcCreate("okta"))

	_, err := f.svc.UpdateProvider(context.Background(), authSettingsRootID, saved.Provider.Provider.ID,
		updateOf(saved.Provider.Provider, saved.Version))

	require.NoError(t, err)
	stored := f.store.Provider("okta")
	require.NotNil(t, stored, "the slug is unchanged")
	assert.Equal(t, models.InstanceAuthProviderOIDC, stored.Type)
}

func TestInstanceAuthSettings_UpdateProvider_Rejections(t *testing.T) {
	f := newAuthSettingsFixture(t)
	saved := f.create(t, oidcCreate("okta"))
	id := saved.Provider.Provider.ID

	_, err := f.svc.UpdateProvider(context.Background(), authSettingsRootID, id,
		updateOf(saved.Provider.Provider, saved.Version-1))
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)

	_, err = f.svc.UpdateProvider(context.Background(), authSettingsRootID, "no-such-id",
		updateOf(saved.Provider.Provider, saved.Version))
	require.ErrorIs(t, err, repositories.ErrInstanceAuthProviderNotFound)

	invalid := updateOf(saved.Provider.Provider, saved.Version)
	invalid.ClientID = " "
	_, err = f.svc.UpdateProvider(context.Background(), authSettingsRootID, id, invalid)
	require.ErrorIs(t, err, ErrInvalidInstanceAuthProvider)
	assert.Equal(t, "client_id", settingsFieldOf(t, err))

	assert.Equal(t, saved.Version, f.store.Version(), "nothing was written")
}

func TestCheckAuthLockoutRisk(t *testing.T) {
	rows := func(enabled ...bool) []*models.InstanceAuthProvider {
		out := make([]*models.InstanceAuthProvider, 0, len(enabled))
		for i, on := range enabled {
			slug := string(rune('a' + i))
			out = append(out, &models.InstanceAuthProvider{ID: "id-" + slug, Slug: slug, Enabled: on})
		}
		return out
	}
	cases := []struct {
		name         string
		current      []*models.InstanceAuthProvider
		target       string
		enabledAfter bool
		caller       models.InstanceAuthLockoutContext
		want         string
	}{
		{name: "disabling the last enabled provider", current: rows(true, false), target: "id-a",
			want: AuthLockoutNoEnabledProvider},
		{name: "deleting the only provider", current: rows(true), target: "id-a",
			want: AuthLockoutNoEnabledProvider},
		{name: "disabling one of two enabled providers", current: rows(true, true), target: "id-a"},
		{name: "disabling the caller's own provider", current: rows(true, true), target: "id-a",
			caller: models.InstanceAuthLockoutContext{SessionProvider: "a"}, want: AuthLockoutOwnProvider},
		{name: "the last enabled provider outranks own provider", current: rows(true), target: "id-a",
			caller: models.InstanceAuthLockoutContext{SessionProvider: "a"}, want: AuthLockoutNoEnabledProvider},
		{name: "disabling another provider than the caller's", current: rows(true, true), target: "id-b",
			caller: models.InstanceAuthLockoutContext{SessionProvider: "a"}},
		{name: "keeping the provider enabled", current: rows(true), target: "id-a", enabledAfter: true,
			caller: models.InstanceAuthLockoutContext{SessionProvider: "a"}},
		{name: "changing an already disabled provider", current: rows(false), target: "id-a"},
		{name: "deleting a disabled provider when none is enabled", current: rows(false, false), target: "id-b"},
		{name: "an unknown target", current: rows(true), target: "id-z"},
		{name: "a confirmed change", current: rows(true), target: "id-a",
			caller: models.InstanceAuthLockoutContext{SessionProvider: "a", Confirmed: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkAuthLockoutRisk(tc.current, tc.target, tc.enabledAfter, tc.caller)
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			var risk *ErrAuthLockoutRisk
			require.ErrorAs(t, err, &risk)
			assert.Equal(t, tc.want, risk.Reason)
			assert.NotEmpty(t, risk.Error())
		})
	}
}

func TestInstanceAuthSettings_LockoutGuardOnUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	f := newAuthSettingsFixture(t)
	okta := f.create(t, oidcCreate("okta"))
	id := okta.Provider.Provider.ID

	disable := updateOf(okta.Provider.Provider, okta.Version)
	disable.Enabled = false
	_, err := f.svc.UpdateProvider(ctx, authSettingsRootID, id, disable)
	var risk *ErrAuthLockoutRisk
	require.ErrorAs(t, err, &risk)
	assert.Equal(t, AuthLockoutNoEnabledProvider, risk.Reason)
	assert.True(t, f.store.Provider("okta").Enabled, "nothing was written")

	err = f.svc.DeleteProvider(ctx, authSettingsRootID, id, models.DeleteInstanceAuthProviderRequest{})
	require.ErrorAs(t, err, &risk)
	assert.NotNil(t, f.store.Provider("okta"))

	// A second enabled provider lifts the zero-provider rule, but not the
	// own-provider one.
	f.create(t, oidcCreate("backup"))
	own := models.InstanceAuthLockoutContext{SessionProvider: "okta"}
	err = f.svc.DeleteProvider(ctx, authSettingsRootID, id, models.DeleteInstanceAuthProviderRequest{Lockout: own})
	require.ErrorAs(t, err, &risk)
	assert.Equal(t, AuthLockoutOwnProvider, risk.Reason)

	own.Confirmed = true
	require.NoError(t, f.svc.DeleteProvider(ctx, authSettingsRootID, id,
		models.DeleteInstanceAuthProviderRequest{Lockout: own}))
	assert.Nil(t, f.store.Provider("okta"))
}

func TestInstanceAuthSettings_DeleteProvider_Rejections(t *testing.T) {
	ctx := context.Background()
	f := newAuthSettingsFixture(t)
	okta := f.create(t, oidcCreate("okta"))
	f.create(t, oidcCreate("backup"))
	stale := okta.Version

	err := f.svc.DeleteProvider(ctx, authSettingsRootID, okta.Provider.Provider.ID,
		models.DeleteInstanceAuthProviderRequest{ExpectedVersion: &stale})
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)

	err = f.svc.DeleteProvider(ctx, authSettingsRootID, "no-such-id", models.DeleteInstanceAuthProviderRequest{})
	require.ErrorIs(t, err, repositories.ErrInstanceAuthProviderNotFound)
	assert.NotNil(t, f.store.Provider("okta"))
}

func TestInstanceAuthSettings_ListProviders_Health(t *testing.T) {
	ctx := context.Background()
	f := newAuthSettingsFixture(t)
	f.create(t, oidcCreate("healthy"))
	f.create(t, oidcCreate("broken"))
	f.create(t, oidcCreate("unresolved"))
	off := oidcCreate("off")
	off.Enabled = false
	f.create(t, off)

	checkedAt := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	f.resolver.health = map[string]ProviderHealth{
		"healthy": {Healthy: true, CheckedAt: checkedAt},
		"broken":  {LastError: "discovery failed", CheckedAt: checkedAt},
		// A stale outcome for a provider that has since been disabled.
		"off": {Healthy: true, CheckedAt: checkedAt},
	}
	f.resolver.snapshots = 0

	list, err := f.svc.ListProviders(ctx)

	require.NoError(t, err)
	assert.Equal(t, f.store.Version(), list.Version)
	assert.Equal(t, 1, f.resolver.snapshots, "the providers are resolved before their health is read")
	got := map[string]models.InstanceAuthProviderHealth{}
	for _, view := range list.Providers {
		got[view.Provider.Slug] = view.Health
		assert.Equal(t, authSettingsCallbackURL, view.RedirectURI)
	}
	brokenErr := "discovery failed"
	assert.Equal(t, models.InstanceAuthProviderHealth{
		Status: models.InstanceAuthProviderHealthy, CheckedAt: &checkedAt}, got["healthy"])
	assert.Equal(t, models.InstanceAuthProviderHealth{
		Status: models.InstanceAuthProviderUnhealthy, LastError: &brokenErr, CheckedAt: &checkedAt}, got["broken"])
	assert.Equal(t, models.InstanceAuthProviderHealth{Status: models.InstanceAuthProviderHealthUnknown}, got["unresolved"])
	assert.Equal(t, models.InstanceAuthProviderHealth{Status: models.InstanceAuthProviderDisabled}, got["off"])

	// When the providers cannot be resolved, a remembered outcome is not served.
	f.resolver.snapshotErr = errors.New("database unavailable")
	list, err = f.svc.ListProviders(ctx)
	require.NoError(t, err)
	for _, view := range list.Providers {
		if view.Provider.Enabled {
			assert.Equal(t, models.InstanceAuthProviderHealthUnknown, view.Health.Status, view.Provider.Slug)
		}
	}
}

func TestInstanceAuthSettings_StoreFailures(t *testing.T) {
	ctx := context.Background()
	f := newAuthSettingsFixture(t)
	saved := f.create(t, oidcCreate("okta"))
	boom := errors.New("database unavailable")
	f.store.Err = boom

	_, err := f.svc.ListProviders(ctx)
	require.ErrorIs(t, err, boom)
	_, err = f.svc.CreateProvider(ctx, authSettingsRootID, oidcCreate("other"))
	require.ErrorIs(t, err, boom)
	_, err = f.svc.UpdateProvider(ctx, authSettingsRootID, saved.Provider.Provider.ID,
		updateOf(saved.Provider.Provider, saved.Version))
	require.ErrorIs(t, err, boom)
	require.ErrorIs(t, f.svc.DeleteProvider(ctx, authSettingsRootID, saved.Provider.Provider.ID,
		models.DeleteInstanceAuthProviderRequest{}), boom)
	_, err = f.svc.GetAllowlist(ctx)
	require.ErrorIs(t, err, boom)
	_, err = f.svc.UpdateAllowlist(ctx, authSettingsRootID, []string{"example.com"}, nil, nil)
	require.ErrorIs(t, err, boom)
	require.ErrorIs(t, f.svc.ResetAllowlist(ctx, authSettingsRootID), boom)
	_, err = f.svc.ListAdmins(ctx)
	require.ErrorIs(t, err, boom)
	id := saved.Provider.Provider.ID
	_, err = f.svc.TestProvider(ctx, models.TestInstanceAuthProviderRequest{ID: &id})
	require.ErrorIs(t, err, boom)
}

func TestInstanceAuthSettings_TestProvider(t *testing.T) {
	ctx := context.Background()
	oidcType, githubType := models.InstanceAuthProviderOIDC, models.InstanceAuthProviderGitHub
	clientID, issuer, otherIssuer := "candidate-client", authSettingsIssuer, "https://other.example.com"
	secret, blank := authSettingsSecret, ""

	t.Run("a stored provider is tested with its stored secret", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		id := f.create(t, oidcCreate("okta")).Provider.Provider.ID

		result, err := f.svc.TestProvider(ctx, models.TestInstanceAuthProviderRequest{ID: &id})

		require.NoError(t, err)
		assert.Equal(t, &models.InstanceAuthProviderTestResult{Valid: true}, result)
		require.Len(t, f.resolver.tested, 1)
		assert.Equal(t, "okta", f.resolver.tested[0].Slug)
		assert.Nil(t, f.resolver.testSecrets[0], "nil means the stored secret")
	})

	t.Run("a stored provider with replaced fields keeps its stored secret", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		id := f.create(t, oidcCreate("okta")).Provider.Provider.ID

		_, err := f.svc.TestProvider(ctx, models.TestInstanceAuthProviderRequest{
			ID: &id, ClientID: &clientID, IssuerURL: &otherIssuer})

		require.NoError(t, err)
		tested := f.resolver.tested[0]
		assert.Equal(t, clientID, tested.ClientID)
		assert.Equal(t, otherIssuer, *tested.IssuerURL)
		assert.True(t, tested.HasClientSecret())
		assert.Nil(t, f.resolver.testSecrets[0])
		assert.Equal(t, authSettingsIssuer, *f.store.Provider("okta").IssuerURL, "nothing is stored")
	})

	t.Run("an unsaved candidate is tested with its own secret", func(t *testing.T) {
		f := newAuthSettingsFixture(t)

		result, err := f.svc.TestProvider(ctx, models.TestInstanceAuthProviderRequest{
			Type: &oidcType, ClientID: &clientID, ClientSecret: &secret, IssuerURL: &issuer})

		require.NoError(t, err)
		assert.True(t, result.Valid)
		assert.False(t, f.resolver.tested[0].HasClientSecret())
		assert.Equal(t, &secret, f.resolver.testSecrets[0])
		assert.Empty(t, f.store.AuditEntries(models.InstanceSettingAuthProviders), "nothing is audited")
	})

	t.Run("a failed test is a result, not an error", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		f.resolver.testErr = ErrIdentityProviderCredentialsInvalid

		result, err := f.svc.TestProvider(ctx, models.TestInstanceAuthProviderRequest{
			Type: &githubType, ClientID: &clientID, ClientSecret: &secret})

		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, ErrIdentityProviderCredentialsInvalid.Error(), result.Message)
	})

	rejections := []struct {
		name  string
		req   func(storedID string) models.TestInstanceAuthProviderRequest
		field string
	}{
		{name: "a candidate without a type", field: "type",
			req: func(string) models.TestInstanceAuthProviderRequest {
				return models.TestInstanceAuthProviderRequest{ClientID: &clientID, ClientSecret: &secret}
			}},
		{name: "a candidate without a secret", field: "client_secret",
			req: func(string) models.TestInstanceAuthProviderRequest {
				return models.TestInstanceAuthProviderRequest{Type: &githubType, ClientID: &clientID}
			}},
		{name: "a candidate without a client id", field: "client_id",
			req: func(string) models.TestInstanceAuthProviderRequest {
				return models.TestInstanceAuthProviderRequest{Type: &githubType, ClientSecret: &secret}
			}},
		{name: "an oidc candidate without an issuer", field: "issuer_url",
			req: func(string) models.TestInstanceAuthProviderRequest {
				return models.TestInstanceAuthProviderRequest{Type: &oidcType, ClientID: &clientID, ClientSecret: &secret}
			}},
		{name: "a stored provider with another type", field: "type",
			req: func(id string) models.TestInstanceAuthProviderRequest {
				return models.TestInstanceAuthProviderRequest{ID: &id, Type: &githubType}
			}},
		{name: "a stored provider with an empty secret", field: "client_secret",
			req: func(id string) models.TestInstanceAuthProviderRequest {
				return models.TestInstanceAuthProviderRequest{ID: &id, ClientSecret: &blank}
			}},
	}
	for _, tc := range rejections {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			id := f.create(t, oidcCreate("okta")).Provider.Provider.ID

			_, err := f.svc.TestProvider(ctx, tc.req(id))

			require.ErrorIs(t, err, ErrInvalidInstanceAuthProvider)
			assert.Equal(t, tc.field, settingsFieldOf(t, err))
			assert.Empty(t, f.resolver.tested, "an invalid candidate is never tested")
		})
	}

	t.Run("an unknown stored provider", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		missing := "no-such-id"
		_, err := f.svc.TestProvider(ctx, models.TestInstanceAuthProviderRequest{ID: &missing})
		require.ErrorIs(t, err, repositories.ErrInstanceAuthProviderNotFound)
	})
}

func TestInstanceAuthSettings_Allowlist(t *testing.T) {
	ctx := context.Background()
	f := newAuthSettingsFixture(t)

	stored, err := f.svc.GetAllowlist(ctx)
	require.NoError(t, err)
	assert.Nil(t, stored, "nothing stored is open access")

	saved, err := f.svc.UpdateAllowlist(ctx, authSettingsRootID,
		[]string{" Example.COM ", "example.com"}, []string{"Bob@Partner.example"}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com"}, saved.Domains, "entries are normalized and de-duplicated")
	assert.Equal(t, []string{"bob@partner.example"}, saved.Emails)
	assert.Equal(t, int64(1), saved.Version)

	stale := saved.Version + 5
	_, err = f.svc.UpdateAllowlist(ctx, authSettingsRootID, []string{"other.example"}, nil, &stale)
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)

	_, err = f.svc.UpdateAllowlist(ctx, authSettingsRootID, []string{"not a domain"}, nil, nil)
	require.ErrorIs(t, err, ErrInvalidInstanceAuthAllowlist)
	assert.Equal(t, "domains", settingsFieldOf(t, err))

	stored, err = f.svc.GetAllowlist(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com"}, stored.Domains, "a rejected save changes nothing")

	require.NoError(t, f.svc.ResetAllowlist(ctx, ""))
	stored, err = f.svc.GetAllowlist(ctx)
	require.NoError(t, err)
	assert.Nil(t, stored)
	require.NoError(t, f.svc.ResetAllowlist(ctx, ""), "resetting nothing is a no-op")

	entries := f.store.AuditEntries(models.InstanceSettingAuthAllowlist)
	require.Len(t, entries, 2, "one upsert and one delete")
	assert.Nil(t, entries[1].ActorUserID, "a setup session reset has no actor")
}

func TestInstanceAuthSettings_PreviewAllowlist(t *testing.T) {
	ctx := context.Background()
	f := newAuthSettingsFixture(t)
	var gotDomains, gotExempt []string
	f.store.Outside = func(domains, _, exempt []string) (int, []string) {
		gotDomains, gotExempt = domains, exempt
		return 3, []string{"a@other.example", "b@other.example"}
	}

	impact, err := f.svc.PreviewAllowlist(ctx, []string{"Example.com"}, nil)

	require.NoError(t, err)
	assert.Equal(t, &AllowlistImpact{
		Count: 3, Sample: []string{"a@other.example", "b@other.example"}, SampleTruncated: true}, impact)
	assert.Equal(t, []string{"example.com"}, gotDomains)
	assert.Equal(t, []string{authSettingsRootEmail}, gotExempt, "root admins are exempt")
	stored, err := f.svc.GetAllowlist(ctx)
	require.NoError(t, err)
	assert.Nil(t, stored, "a preview stores nothing")

	_, err = f.svc.PreviewAllowlist(ctx, nil, []string{"not-an-email"})
	require.ErrorIs(t, err, ErrInvalidInstanceAuthAllowlist)
}

func TestInstanceAuthSettings_GrantAndRevokeAdmin(t *testing.T) {
	ctx := context.Background()
	f := newAuthSettingsFixture(t)

	view, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{UserID: authSettingsDBAdminID})
	require.NoError(t, err)
	assert.Equal(t, authSettingsDBAdminID, view.UserID)
	require.NotNil(t, view.GrantedBy)
	assert.Equal(t, authSettingsRootID, *view.GrantedBy)

	// By email, in a different case than the address is stored in.
	view, err = f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{Email: " " + authSettingsMemberEmail + " "})
	require.NoError(t, err)
	assert.Equal(t, authSettingsMemberID, view.UserID)
	assert.Equal(t, "Ada", view.Name)

	again, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{UserID: authSettingsMemberID})
	require.NoError(t, err, "granting an existing admin is idempotent")
	assert.Equal(t, view.GrantedAt, again.GrantedAt)
	assert.Len(t, f.store.AuditEntries(models.InstanceSettingInstanceAdmins), 2)

	list, err := f.svc.ListAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{authSettingsRootEmail}, list.RootAdmins)
	require.Len(t, list.Admins, 2)
	assert.Equal(t, authSettingsDBAdminID, list.Admins[0].UserID, "oldest first")

	require.NoError(t, f.svc.RevokeAdmin(ctx, authSettingsRootID, authSettingsMemberID))
	var notGranted *ErrInstanceAdminNotGranted
	require.ErrorAs(t, f.svc.RevokeAdmin(ctx, authSettingsRootID, authSettingsMemberID), &notGranted)
}

func TestInstanceAuthSettings_GrantAdmin_ByLowerCasedEmail(t *testing.T) {
	f := newAuthSettingsFixture(t)
	f.users.users[2].Email = "ada@example.com"

	view, err := f.svc.GrantAdmin(context.Background(), authSettingsRootID,
		models.InstanceAdminGrantTarget{Email: "ADA@Example.com"})

	require.NoError(t, err)
	assert.Equal(t, authSettingsMemberID, view.UserID)
}

func TestInstanceAuthSettings_GrantAdmin_Rejections(t *testing.T) {
	ctx := context.Background()
	var (
		notRoot    *ErrInstanceAdminNotRoot
		targetRoot *ErrInstanceAdminTargetIsRoot
		invalid    *ErrInstanceAdminTargetInvalid
	)

	t.Run("neither or both of user id and email", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		_, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{})
		require.ErrorIs(t, err, ErrInstanceAdminGrantTargetAmbiguous)
		_, err = f.svc.GrantAdmin(ctx, authSettingsRootID,
			models.InstanceAdminGrantTarget{UserID: authSettingsMemberID, Email: authSettingsMemberEmail})
		require.ErrorIs(t, err, ErrInstanceAdminGrantTargetAmbiguous)
	})

	t.Run("a non-root actor learns nothing about the target", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		_, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{UserID: authSettingsDBAdminID})
		require.NoError(t, err)

		for _, email := range []string{authSettingsMemberEmail, "nobody@example.com"} {
			_, err = f.svc.GrantAdmin(ctx, authSettingsDBAdminID, models.InstanceAdminGrantTarget{Email: email})
			require.ErrorAs(t, err, &notRoot, email)
		}
		require.ErrorAs(t, f.svc.RevokeAdmin(ctx, authSettingsDBAdminID, authSettingsDBAdminID), &notRoot)
	})

	t.Run("an unknown user", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		_, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{Email: "nobody@example.com"})
		require.ErrorAs(t, err, &invalid)
		assert.True(t, invalid.IsUnknownUser())
		_, err = f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{UserID: "no-such-id"})
		require.ErrorAs(t, err, &invalid)
		assert.True(t, invalid.IsUnknownUser())
	})

	t.Run("a suspended user", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		_, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{UserID: authSettingsSuspendedID})
		require.ErrorAs(t, err, &invalid)
		assert.False(t, invalid.IsUnknownUser())
	})

	t.Run("a root admin", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		_, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{Email: authSettingsRootEmail})
		require.ErrorAs(t, err, &targetRoot)
	})

	t.Run("a user lookup failure", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		boom := errors.New("database unavailable")
		f.users.err = boom
		_, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{UserID: authSettingsMemberID})
		require.ErrorIs(t, err, boom)
	})
}

func TestInstanceAuthSettings_ListAdmins_SkipsADeletedUser(t *testing.T) {
	ctx := context.Background()
	f := newAuthSettingsFixture(t)
	_, err := f.svc.GrantAdmin(ctx, authSettingsRootID, models.InstanceAdminGrantTarget{UserID: authSettingsDBAdminID})
	require.NoError(t, err)
	f.users.users = f.users.users[:1] // only the root admin is left

	list, err := f.svc.ListAdmins(ctx)

	require.NoError(t, err)
	assert.Empty(t, list.Admins)
	assert.NotNil(t, list.Admins)
}
