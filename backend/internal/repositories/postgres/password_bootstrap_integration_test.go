//go:build integration

package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/pkg/events"
)

// Password-only bootstrap (#1438) over real Postgres: the two counts the
// way-in check and the lockout guard rest on, and the whole path from an empty
// instance to "not in setup mode" through the real services.

const bootstrapRootEmail = "root.admin@integration.test"

func resetPasswordBootstrap(t *testing.T) {
	t.Helper()
	resetIntegrationTables(t)
	resetInstanceAuthSettings(t)
	resetInstanceAuthSetup(t)
	resetInstanceAuthPolicy(t)
}

// userWithEmail inserts a user and returns its id.
func userWithEmail(t *testing.T, email string) string {
	t.Helper()
	id := insertTestUser(t)
	_, err := integrationDB.ExecContext(context.Background(), "UPDATE users SET email = $2 WHERE id = $1", id, email)
	require.NoError(t, err)
	return id
}

func TestIntegrationUserCredential_CountUsableForAdmins(t *testing.T) {
	resetPasswordBootstrap(t)
	ctx := context.Background()
	credentials := NewUserCredentialRepository(integrationDB)
	roots := []string{bootstrapRootEmail, "second.root@integration.test"}
	count := func(t *testing.T) int {
		t.Helper()
		n, err := credentials.CountUsableForAdmins(ctx, roots)
		require.NoError(t, err)
		return n
	}

	root := userWithEmail(t, bootstrapRootEmail)
	granted := insertTestUser(t)
	member := insertTestUser(t)
	_, err := NewInstanceAdminRepository(integrationDB).Grant(ctx, granted, nil)
	require.NoError(t, err)
	assert.Zero(t, count(t), "admins exist, none has a password")

	require.NoError(t, credentials.Upsert(ctx, member, credentialTestHash))
	assert.Zero(t, count(t), "a member's password is not a way in for an admin")

	require.NoError(t, credentials.Upsert(ctx, root, credentialTestHash))
	assert.Equal(t, 1, count(t), "a root admin by configuration")
	require.NoError(t, credentials.Upsert(ctx, granted, credentialTestHash))
	assert.Equal(t, 2, count(t), "and an admin by grant")

	_, err = integrationDB.ExecContext(ctx, "UPDATE user_credentials SET disabled_at = now() WHERE user_id = $1", granted)
	require.NoError(t, err)
	assert.Equal(t, 1, count(t), "a disabled password does not count")

	_, err = integrationDB.ExecContext(ctx,
		"UPDATE user_credentials SET locked_until = now() + interval '1 hour', failed_attempts = 9 WHERE user_id = $1", root)
	require.NoError(t, err)
	assert.Equal(t, 1, count(t), "a lock from failed attempts ends by itself and still counts")

	_, err = integrationDB.ExecContext(ctx, "UPDATE users SET status = $2 WHERE id = $1", root, models.UserStatusSuspended)
	require.NoError(t, err)
	assert.Zero(t, count(t), "a suspended admin cannot sign in")

	n, err := credentials.CountUsableForAdmins(ctx, nil)
	require.NoError(t, err)
	assert.Zero(t, n, "no configured root admin and no usable grant")
}

func TestIntegrationUserCredential_CountPasswordOnly(t *testing.T) {
	resetPasswordBootstrap(t)
	ctx := context.Background()
	credentials := NewUserCredentialRepository(integrationDB)
	identities := NewUserIdentityRepository(integrationDB)
	count := func(t *testing.T) int {
		t.Helper()
		n, err := credentials.CountPasswordOnly(ctx)
		require.NoError(t, err)
		return n
	}
	withPassword := func(t *testing.T, providers ...string) string {
		t.Helper()
		id := insertTestUser(t)
		require.NoError(t, credentials.Upsert(ctx, id, credentialTestHash))
		for _, provider := range providers {
			require.NoError(t, identities.Upsert(ctx, id, provider, provider+"-"+id))
		}
		return id
	}

	oauthOnly := insertTestUser(t)
	require.NoError(t, identities.Upsert(ctx, oauthOnly, "google", "g-"+oauthOnly))
	assert.Zero(t, count(t), "no password at all")

	withPassword(t)
	assert.Equal(t, 1, count(t), "a password and nothing else")
	withPassword(t, models.DevIdentityProvider)
	assert.Equal(t, 2, count(t), "the dev-login row is not a sign-in method")
	withPassword(t, "google")
	withPassword(t, models.LegacyGoogleIdentityProvider)
	assert.Equal(t, 2, count(t), "an identity from a provider is another way in")
}

// bootstrapEvents records the user.created events the bootstrap publishes.
type bootstrapEvents struct{ published []events.Event }

func (e *bootstrapEvents) Publish(_ context.Context, event events.Event) error {
	e.published = append(e.published, event)
	return nil
}

// passwordBootstrapStack is the real services of a password-only instance over
// the test database, with the fastest hashing parameters.
type passwordBootstrapStack struct {
	setup     services.SetupModeService
	wayIn     services.AuthWayIn
	root      *services.RootAdminPasswordService
	passwords *services.PasswordAuthService
	events    *bootstrapEvents
}

func newPasswordBootstrapStack(withEvents bool) *passwordBootstrapStack {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	users := NewUserRepository(integrationDB)
	credentials := NewUserCredentialRepository(integrationDB)
	policies := NewInstanceAuthPolicyRepository(integrationDB)
	isRoot := func(email string) bool { return email == bootstrapRootEmail }
	stack := &passwordBootstrapStack{events: &bootstrapEvents{}}
	stack.wayIn = services.NewAuthWayIn(services.AuthWayInDeps{
		Providers: NewInstanceAuthProviderRepository(integrationDB), Policies: policies, Credentials: credentials,
		RootAdminEmails: func() []string { return []string{bootstrapRootEmail} },
	})
	stack.setup = services.NewSetupModeService(services.SetupModeDeps{
		Setup: NewInstanceAuthSetupRepository(integrationDB), WayIn: stack.wayIn, IsRootAdmin: isRoot,
		BaseURL: recoveryBaseURL, Logger: logger,
	})
	stack.passwords = services.NewPasswordAuthService(services.PasswordAuthDeps{
		Users: users, Credentials: credentials,
		Hasher: password.NewHasher(password.Params{Memory: 64, Time: 1, Threads: 1, SaltLength: 16, KeyLength: 32}),
		Logger: logger,
	})
	deps := services.RootAdminPasswordDeps{
		Users: users, Credentials: credentials, Policies: policies, Passwords: stack.passwords,
		IsRootAdmin: isRoot, Logger: logger,
	}
	if withEvents {
		deps.Events = stack.events
	}
	stack.root = services.NewRootAdminPasswordService(deps)
	return stack
}

func (s *passwordBootstrapStack) setupActive(t *testing.T) bool {
	t.Helper()
	active, err := s.setup.IsActive(context.Background())
	require.NoError(t, err)
	return active
}

// A fresh instance with no provider leaves setup mode once a root admin has a
// password and password login is on, the admin's sign-in consumes the setup
// token, and setup mode does not come back while that stays true.
func TestIntegrationPasswordBootstrap_FromEmptyInstanceToWorkingSignIn(t *testing.T) {
	resetPasswordBootstrap(t)
	ctx := context.Background()
	stack := newPasswordBootstrapStack(true)
	const passphrase = "an operator passphrase 1438"

	require.True(t, stack.setupActive(t), "nothing configured: setup mode")
	_, minted, err := stack.setup.EnsureTokenAtBoot(ctx)
	require.NoError(t, err)
	require.True(t, minted)

	result, err := stack.root.Set(ctx, "Root.Admin@Integration.TEST", passphrase, true)

	require.NoError(t, err)
	assert.True(t, result.AccountCreated)
	assert.True(t, result.LoginSwitchedOn)
	require.Len(t, stack.events.published, 1, "user.created provisions the workspace")
	user, err := NewUserRepository(integrationDB).GetByEmail(ctx, bootstrapRootEmail)
	require.NoError(t, err)
	assert.NotNil(t, user.EmailVerifiedAt, "the operator vouches for a configured address")
	policy, err := NewInstanceAuthPolicyRepository(integrationDB).Get(ctx)
	require.NoError(t, err)
	assert.True(t, policy.PasswordLoginEnabled)
	assert.Equal(t, models.RegistrationModeOpen, policy.RegistrationMode)
	assert.False(t, stack.setupActive(t), "an admin with a usable password is a working way in")

	// The setup session stays valid until the admin signs in, then it ends.
	require.NoError(t, stack.setup.ConsumeOnRootLogin(ctx, user))
	_, minted, err = stack.setup.EnsureTokenAtBoot(ctx)
	require.NoError(t, err)
	assert.False(t, minted, "the next boot mints no setup URL")

	t.Run("a later verified provider sign-in links and keeps the password", func(t *testing.T) {
		credentials := NewUserCredentialRepository(integrationDB)
		auth := services.NewAuthService(
			NewUserRepository(integrationDB), NewUserIdentityRepository(integrationDB),
			nil, nil, slog.New(slog.DiscardHandler), openAllowlist{},
			services.WithUnverifiedAccountClaimHook(
				services.NewUnverifiedAccountCleanup(credentials, nil, slog.New(slog.DiscardHandler))))

		linked, err := auth.ProvisionFromClaims(ctx, "corp-sso",
			&idp.Claims{Subject: "sub-root", Email: bootstrapRootEmail, EmailVerified: true})

		require.NoError(t, err)
		assert.Equal(t, user.ID, linked.ID, "the same account")
		_, err = credentials.Get(ctx, user.ID)
		require.NoError(t, err, "the account was verified, so the claim hook did not remove the password")
	})

	t.Run("setup mode comes back when the way in is lost", func(t *testing.T) {
		_, err := integrationDB.ExecContext(ctx,
			"UPDATE user_credentials SET disabled_at = now() WHERE user_id = $1", user.ID)
		require.NoError(t, err)
		assert.True(t, stack.setupActive(t), "the only admin's password is disabled")

		_, err = integrationDB.ExecContext(ctx,
			"UPDATE user_credentials SET disabled_at = NULL WHERE user_id = $1", user.ID)
		require.NoError(t, err)
		assert.False(t, stack.setupActive(t))
		off := &models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeOpen}
		require.NoError(t, NewInstanceAuthPolicyRepository(integrationDB).UpsertAudited(ctx, off, nil, nil))
		assert.True(t, stack.setupActive(t), "password login is switched off and no provider is enabled")
	})
}

// The CLI path: no event publisher, so only an existing account, and the
// policy change is audited with source "cli".
func TestIntegrationPasswordBootstrap_CLISetsAnExistingRootAdminsPassword(t *testing.T) {
	resetPasswordBootstrap(t)
	ctx := repositories.WithAuditSource(context.Background(), repositories.AuditSourceCLI)
	stack := newPasswordBootstrapStack(false)
	const passphrase = "a recovery passphrase 1438"

	_, err := stack.root.Set(ctx, bootstrapRootEmail, passphrase, true)
	require.ErrorIs(t, err, services.ErrRootAdminAccountMissing, "the CLI cannot create an account")
	var users int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&users))
	assert.Zero(t, users)

	userID := userWithEmail(t, bootstrapRootEmail)
	require.NoError(t, stack.passwords.SetPassword(ctx, userID, "the password nobody remembers"))

	result, err := stack.root.Set(ctx, bootstrapRootEmail, passphrase, true)

	require.NoError(t, err)
	assert.True(t, result.PasswordReplaced)
	assert.True(t, result.LoginSwitchedOn)
	user, err := NewUserRepository(integrationDB).GetByID(ctx, userID)
	require.NoError(t, err)
	assert.NotNil(t, user.EmailVerifiedAt)
	assert.EqualValues(t, 1, user.SessionEpoch, "earlier sessions are ended")
	assert.False(t, stack.setupActive(t))

	entries := authAuditEntries(t, models.InstanceSettingAuthPolicy) // newest first
	require.Len(t, entries, 1)
	assert.Nil(t, entries[0].ActorUserID, "a CLI write has no acting user")
	assert.Equal(t, repositories.AuditSourceCLI, decodeAuditDoc(t, entries[0].After)["source"])

	_, err = stack.root.Set(ctx, "someone.else@integration.test", passphrase, true)
	require.ErrorIs(t, err, services.ErrNotRootAdmin)
	_, err = stack.root.Set(ctx, bootstrapRootEmail, "short", true)
	var policyErr *password.PolicyError
	require.ErrorAs(t, err, &policyErr)
}
