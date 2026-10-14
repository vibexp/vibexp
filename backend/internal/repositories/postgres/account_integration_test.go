//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Real-Postgres coverage for the self-service account API's writes (#1437):
// when a password counts as "changed", disconnecting an identity, and renaming.

// password_changed_at moves only when a new password is stored. Attempts,
// successes and a rehash of the same password bump updated_at and must leave it
// alone, or the account page would show "changed just now" after every sign-in.
func TestIntegrationUserCredential_PasswordChangedAtMovesOnlyOnANewPassword(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))

	// Age the row, so "unchanged" and "set to now" cannot look alike.
	_, err := integrationDB.ExecContext(ctx,
		`UPDATE user_credentials
		    SET password_changed_at = now() - interval '30 days', updated_at = now() - interval '30 days'
		  WHERE user_id = $1`, userID)
	require.NoError(t, err)
	aged, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().Add(-30*24*time.Hour), aged.PasswordChangedAt, time.Hour)

	unchanged := func(t *testing.T, what string) {
		t.Helper()
		got, getErr := repo.Get(ctx, userID)
		require.NoError(t, getErr)
		assert.True(t, aged.PasswordChangedAt.Equal(got.PasswordChangedAt), "%s moved password_changed_at", what)
		assert.True(t, got.UpdatedAt.After(aged.UpdatedAt), "fixture: %s must have touched the row", what)
	}

	attempt, err := repo.BeginAttempt(ctx, userID, time.Now().UTC(), models.DefaultPasswordLockout)
	require.NoError(t, err)
	assert.True(t, aged.PasswordChangedAt.Equal(attempt.PasswordChangedAt), "BeginAttempt returns the column too")
	unchanged(t, "a sign-in attempt")

	require.NoError(t, repo.RecordSuccess(ctx, userID))
	unchanged(t, "a successful sign-in")

	updated, err := repo.UpdateHash(ctx, userID, credentialTestHash, credentialTestHash+"rehashed")
	require.NoError(t, err)
	require.True(t, updated)
	unchanged(t, "a rehash of the same password")

	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash+"new"))
	changed, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), changed.PasswordChangedAt, time.Minute, "a new password is a change")
}

// signInMethodFixture is one account's sign-in methods over the real tables.
type signInMethodFixture struct {
	t          *testing.T
	methods    repositories.SignInMethodRepository
	identities repositories.UserIdentityRepository
	passwords  repositories.UserCredentialRepository
}

func newSignInMethodFixture(t *testing.T) *signInMethodFixture {
	t.Helper()
	resetIntegrationTables(t)
	return &signInMethodFixture{
		t:          t,
		methods:    NewSignInMethodRepository(integrationDB),
		identities: NewUserIdentityRepository(integrationDB),
		passwords:  NewUserCredentialRepository(integrationDB),
	}
}

// user creates an account with a password (when asked) and one identity per
// provider, and returns its id.
func (f *signInMethodFixture) user(withPassword bool, providers ...string) string {
	f.t.Helper()
	ctx := context.Background()
	userID := insertTestUser(f.t)
	if withPassword {
		require.NoError(f.t, f.passwords.Upsert(ctx, userID, credentialTestHash))
	}
	for _, provider := range providers {
		require.NoError(f.t, f.identities.Upsert(ctx, userID, provider, provider+"-"+userID))
	}
	return userID
}

func (f *signInMethodFixture) identityID(userID, provider string) string {
	f.t.Helper()
	var id string
	require.NoError(f.t, integrationDB.QueryRowContext(context.Background(),
		"SELECT id FROM user_identities WHERE user_id = $1 AND provider = $2", userID, provider).Scan(&id))
	return id
}

func (f *signInMethodFixture) providersOf(userID string) []string {
	f.t.Helper()
	got, err := f.identities.ListByUserID(context.Background(), userID)
	require.NoError(f.t, err)
	providers := make([]string, 0, len(got))
	for _, identity := range got {
		providers = append(providers, identity.Provider)
	}
	return providers
}

func (f *signInMethodFixture) hasPassword(userID string) bool {
	f.t.Helper()
	_, err := f.passwords.Get(context.Background(), userID)
	if errors.Is(err, repositories.ErrUserCredentialNotFound) {
		return false
	}
	require.NoError(f.t, err)
	return true
}

// What works on the test instance: Google and GitHub are enabled providers;
// "old-sso" is one that was removed.
func usableMethods(passwordLogin bool) repositories.UsableSignInMethods {
	return repositories.UsableSignInMethods{Providers: []string{"google", "github"}, PasswordLogin: passwordLogin}
}

func TestIntegrationSignInMethod_RemovePassword(t *testing.T) {
	ctx := context.Background()

	t.Run("removed when an identity of an enabled provider remains", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true, "google")

		require.NoError(t, f.methods.RemovePassword(ctx, userID, usableMethods(true), repositories.RemovePasswordOptions{}))

		assert.False(t, f.hasPassword(userID))
		assert.Equal(t, []string{"google"}, f.providersOf(userID))
	})

	t.Run("removed while password sign-in is off", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true, "github")

		require.NoError(t, f.methods.RemovePassword(ctx, userID, usableMethods(false), repositories.RemovePasswordOptions{}))
		assert.False(t, f.hasPassword(userID))
	})

	for name, providers := range map[string][]string{
		"no identity at all":                      nil,
		"only an identity of a removed provider":  {"old-sso"},
		"only the legacy alias and the dev login": {models.LegacyGoogleIdentityProvider, "dev"},
	} {
		t.Run("refused as the last usable method: "+name, func(t *testing.T) {
			f := newSignInMethodFixture(t)
			userID := f.user(true, providers...)

			err := f.methods.RemovePassword(ctx, userID, usableMethods(true), repositories.RemovePasswordOptions{})

			require.ErrorIs(t, err, repositories.ErrLastSignInMethod)
			assert.True(t, f.hasPassword(userID), "nothing was removed")
		})
	}

	t.Run("an account with no password, and another user's identity does not count", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(false, "google")
		loner := f.user(true)

		require.ErrorIs(t, f.methods.RemovePassword(ctx, userID, usableMethods(true), repositories.RemovePasswordOptions{}),
			repositories.ErrUserCredentialNotFound)
		require.ErrorIs(t, f.methods.RemovePassword(ctx, loner, usableMethods(true), repositories.RemovePasswordOptions{}),
			repositories.ErrLastSignInMethod, "the first user's Google identity is not the loner's")
	})
}

func TestIntegrationSignInMethod_RemoveIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("removed when another usable identity remains", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(false, "google", "github")

		provider, err := f.methods.RemoveIdentity(ctx, userID, f.identityID(userID, "google"), usableMethods(true))

		require.NoError(t, err)
		assert.Equal(t, "google", provider)
		assert.Equal(t, []string{"github"}, f.providersOf(userID))
	})

	t.Run("removed when a usable password remains", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true, "google")

		_, err := f.methods.RemoveIdentity(ctx, userID, f.identityID(userID, "google"), usableMethods(true))

		require.NoError(t, err)
		assert.Empty(t, f.providersOf(userID))
		assert.True(t, f.hasPassword(userID))
	})

	t.Run("an identity of a removed provider can be cleaned up when something usable remains", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(false, "old-sso", "google")

		provider, err := f.methods.RemoveIdentity(ctx, userID, f.identityID(userID, "old-sso"), usableMethods(true))

		require.NoError(t, err)
		assert.Equal(t, "old-sso", provider)
	})

	for name, tc := range map[string]struct {
		password      bool
		disable       bool
		passwordLogin bool
		others        []string
	}{
		"the only identity, no password":          {passwordLogin: true},
		"a password, but password sign-in is off": {password: true},
		"a password an administrator disabled":    {password: true, disable: true, passwordLogin: true},
		"another identity whose provider is gone": {passwordLogin: true, others: []string{"old-sso"}},
		"only the legacy alias and the dev login": {passwordLogin: true, others: []string{models.LegacyGoogleIdentityProvider, "dev"}},
	} {
		t.Run("refused when nothing usable would remain: "+name, func(t *testing.T) {
			f := newSignInMethodFixture(t)
			userID := f.user(tc.password, append([]string{"github"}, tc.others...)...)
			if tc.disable {
				_, err := integrationDB.ExecContext(ctx,
					"UPDATE user_credentials SET disabled_at = now() WHERE user_id = $1", userID)
				require.NoError(t, err)
			}

			_, err := f.methods.RemoveIdentity(ctx, userID, f.identityID(userID, "github"), usableMethods(tc.passwordLogin))

			require.ErrorIs(t, err, repositories.ErrLastSignInMethod)
			assert.Contains(t, f.providersOf(userID), "github", "nothing was removed")
		})
	}

	t.Run("unknown, someone else's and internal identities are not found", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true, "google", models.LegacyGoogleIdentityProvider, "dev")
		other := f.user(true, "google")

		for name, id := range map[string]string{
			"unknown":        uuid.New().String(),
			"someone else's": f.identityID(other, "google"),
			"legacy alias":   f.identityID(userID, models.LegacyGoogleIdentityProvider),
			"dev login":      f.identityID(userID, "dev"),
		} {
			_, err := f.methods.RemoveIdentity(ctx, userID, id, usableMethods(true))
			require.ErrorIs(t, err, repositories.ErrUserIdentityNotFound, name)
		}
		assert.Len(t, f.providersOf(userID), 3)
		assert.Len(t, f.providersOf(other), 1)
	})

	t.Run("disconnecting Google removes its legacy alias too, and only the owner's", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		owner := insertTestUser(t)
		other := insertTestUser(t)
		// The same subject string under both users' aliases proves the scoping.
		for _, row := range []struct{ user, provider, subject string }{
			{owner, "google", "g-subject"},
			{owner, models.LegacyGoogleIdentityProvider, "g-subject"},
			{owner, "github", "gh-owner"},
			{other, "google", "g-other"},
			{other, models.LegacyGoogleIdentityProvider, "g-subject-other"},
		} {
			require.NoError(t, f.identities.Upsert(ctx, row.user, row.provider, row.subject))
		}

		_, err := f.methods.RemoveIdentity(ctx, owner, f.identityID(owner, "google"), usableMethods(true))

		require.NoError(t, err)
		assert.Equal(t, []string{"github"}, f.providersOf(owner),
			"left behind, the alias would keep that Google account able to sign in")
		assert.ElementsMatch(t, []string{"google", models.LegacyGoogleIdentityProvider}, f.providersOf(other))
		_, err = NewUserRepository(integrationDB).GetByIDPSubject(ctx, models.LegacyGoogleIdentityProvider, "g-subject")
		require.ErrorIs(t, err, repositories.ErrUserNotFound)
	})

	t.Run("removing it twice is not found", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true, "google")
		id := f.identityID(userID, "google")
		_, err := f.methods.RemoveIdentity(ctx, userID, id, usableMethods(true))
		require.NoError(t, err)

		_, err = f.methods.RemoveIdentity(ctx, userID, id, usableMethods(true))
		require.ErrorIs(t, err, repositories.ErrUserIdentityNotFound)
	})
}

// Two removals sent together must not leave an account with no way in: each
// would see the other's method still present if the check and the delete were
// not serialized. Repeated over many accounts so a lost race would show.
func TestIntegrationSignInMethod_ConcurrentRemovalsKeepOneMethod(t *testing.T) {
	f := newSignInMethodFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const accounts = 20
	for range accounts {
		userID := f.user(true, "google")
		identityID := f.identityID(userID, "google")
		errs := make([]error, 2)
		var wg sync.WaitGroup
		wg.Go(func() {
			errs[0] = f.methods.RemovePassword(ctx, userID, usableMethods(true), repositories.RemovePasswordOptions{})
		})
		wg.Go(func() { _, errs[1] = f.methods.RemoveIdentity(ctx, userID, identityID, usableMethods(true)) })
		wg.Wait()

		refused := 0
		for _, err := range errs {
			if err != nil {
				require.ErrorIs(t, err, repositories.ErrLastSignInMethod)
				refused++
			}
		}
		require.Equal(t, 1, refused, "exactly one of the two removals is refused")
		remaining := len(f.providersOf(userID))
		if f.hasPassword(userID) {
			remaining++
		}
		require.Equal(t, 1, remaining, "one sign-in method is left")
	}
}

// productionMaxOpenConns is the connection cap of the server's pool
// (database.NewConnection).
const productionMaxOpenConns = 25

// openCappedDB opens a SECOND pool to the test database with the production
// connection cap. The shared integrationDB has no cap, so a test of pool
// starvation on it could never fail.
func openCappedDB(t *testing.T) *database.DB {
	t.Helper()
	dsn, ok := os.LookupEnv("POSTGRES_TEST_DSN")
	if !ok || dsn == "" {
		dsn = defaultTestDSN
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(productionMaxOpenConns)
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("failed to close the capped database: %v", closeErr)
		}
	})
	return &database.DB{DB: db}
}

// More callers than the pool has connections, all for one user: each waits for
// the lock holding ONE connection, and the holder needs no second one, so they
// finish one after another and the pool stays usable. A lock held on one
// connection while the work ran on others starved the pool: with the
// production cap every caller failed at its deadline and unrelated queries
// timed out. This runs on a pool with that cap, so it would fail the same way.
func TestIntegrationSignInMethod_ManyCallersDoNotStarveThePool(t *testing.T) {
	f := newSignInMethodFixture(t)
	userID := f.user(true, "google", "github")
	identityID := f.identityID(userID, "google")
	capped := openCappedDB(t)
	methods := NewSignInMethodRepository(capped)
	const callers = 4 * productionMaxOpenConns
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var removed, notFound atomic.Int32
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			_, err := methods.RemoveIdentity(ctx, userID, identityID, usableMethods(true))
			switch {
			case err == nil:
				removed.Add(1)
			case errors.Is(err, repositories.ErrUserIdentityNotFound):
				notFound.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	// An unrelated query on the same pool, while the callers are in flight.
	var one int
	bystander, bystanderCancel := context.WithTimeout(ctx, 10*time.Second)
	defer bystanderCancel()
	require.NoError(t, capped.QueryRowContext(bystander, "SELECT 1").Scan(&one), "the pool still answers")
	wg.Wait()

	assert.EqualValues(t, 1, removed.Load(), "the identity is removed once")
	assert.EqualValues(t, callers-1, notFound.Load(), "everyone else finds it gone")
}

// The internal rows never count as what remains, even on an instance that
// gives a real provider the slug "dev".
func TestIntegrationSignInMethod_InternalRowsNeverCountAsRemaining(t *testing.T) {
	ctx := context.Background()
	withDevProvider := repositories.UsableSignInMethods{Providers: []string{"dev", "github"}, PasswordLogin: true}

	t.Run("removing the password", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true, "dev")

		require.ErrorIs(t, f.methods.RemovePassword(ctx, userID, withDevProvider, repositories.RemovePasswordOptions{}), repositories.ErrLastSignInMethod)
		assert.True(t, f.hasPassword(userID))
	})

	t.Run("disconnecting an identity", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(false, "github", "dev")

		_, err := f.methods.RemoveIdentity(ctx, userID, f.identityID(userID, "github"), withDevProvider)

		require.ErrorIs(t, err, repositories.ErrLastSignInMethod)
	})
}

func TestIntegrationUser_UpdateName(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	users := NewUserRepository(integrationDB)
	userID := insertTestUser(t)
	bystander := insertTestUser(t)
	before, err := users.GetByID(ctx, userID)
	require.NoError(t, err)

	require.NoError(t, users.UpdateName(ctx, userID, "Ada Lovelace"))

	after, err := users.GetByID(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", after.Name)
	assert.Equal(t, before.Email, after.Email)
	assert.Equal(t, before.Version+1, after.Version,
		"the version moves, so a sign-in that loaded the old row cannot write the old name back")
	untouched, err := users.GetByID(ctx, bystander)
	require.NoError(t, err)
	assert.Equal(t, "Integration Test User", untouched.Name)

	// A sign-in that read the row BEFORE the rename must not undo it.
	before.Name = "Stale Name From An Old Read"
	require.Error(t, users.Update(ctx, before), "the stale write is rejected by the version check")
	current, err := users.GetByID(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", current.Name)

	require.ErrorIs(t, users.UpdateName(ctx, uuid.New().String(), "Nobody"), repositories.ErrUserNotFound)
}

// #1460: a disconnect records the provider account, under the same lock and in
// the same statement as the removal, and "allow again" deletes the record.
func TestIntegrationSignInMethod_DisconnectIsRemembered(t *testing.T) {
	ctx := context.Background()
	f := newSignInMethodFixture(t)
	userID := f.user(true, "google", "github")
	otherUser := f.user(true, "google")
	// The legacy Google alias of the same subject goes with the identity and is
	// not recorded on its own.
	require.NoError(t, f.identities.Upsert(ctx, userID, models.LegacyGoogleIdentityProvider, "google-"+userID))

	provider, err := f.methods.RemoveIdentity(ctx, userID, f.identityID(userID, "google"), usableMethods(true))
	require.NoError(t, err)
	require.Equal(t, "google", provider)

	disconnected, err := f.methods.ListDisconnected(ctx, userID)
	require.NoError(t, err)
	require.Len(t, disconnected, 1)
	assert.Equal(t, "google", disconnected[0].Provider)
	assert.Equal(t, "google-"+userID, disconnected[0].Subject)
	assert.WithinDuration(t, time.Now(), disconnected[0].DisconnectedAt, time.Minute)

	isOut, err := f.methods.IsDisconnected(ctx, userID, "google", "google-"+userID)
	require.NoError(t, err)
	assert.True(t, isOut)
	isOut, err = f.methods.IsDisconnected(ctx, otherUser, "google", "google-"+otherUser)
	require.NoError(t, err)
	assert.False(t, isOut, "another account's identity is untouched")
	assert.Equal(t, []string{"github"}, f.providersOf(userID), "the alias went with the identity")

	_, allowed, err := f.methods.AllowIdentityAgain(ctx, otherUser, disconnected[0].ID)
	require.NoError(t, err)
	assert.False(t, allowed, "another account cannot allow it")

	allowedProvider, allowed, err := f.methods.AllowIdentityAgain(ctx, userID, disconnected[0].ID)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, "google", allowedProvider)
	isOut, err = f.methods.IsDisconnected(ctx, userID, "google", "google-"+userID)
	require.NoError(t, err)
	assert.False(t, isOut)

	_, allowed, err = f.methods.AllowIdentityAgain(ctx, userID, disconnected[0].ID)
	require.NoError(t, err)
	assert.False(t, allowed, "already allowed")
}

// A refused disconnect (the last way in) records nothing, and the records go
// with the account.
func TestIntegrationSignInMethod_DisconnectRecordLifecycle(t *testing.T) {
	ctx := context.Background()
	f := newSignInMethodFixture(t)

	loner := f.user(false, "google")
	_, err := f.methods.RemoveIdentity(ctx, loner, f.identityID(loner, "google"), usableMethods(true))
	require.ErrorIs(t, err, repositories.ErrLastSignInMethod)
	disconnected, err := f.methods.ListDisconnected(ctx, loner)
	require.NoError(t, err)
	assert.Empty(t, disconnected, "nothing was removed, so nothing is recorded")

	userID := f.user(true, "google")
	_, err = f.methods.RemoveIdentity(ctx, userID, f.identityID(userID, "google"), usableMethods(true))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	require.NoError(t, err)
	var left int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT count(*) FROM user_disconnected_identities WHERE user_id = $1`, userID).Scan(&left))
	assert.Zero(t, left, "the records go with the account")
}

// A recorded disconnect is not a sign-in method: it never counts as "another
// usable method remains" (#1460).
func TestIntegrationSignInMethod_ADisconnectedRecordIsNotAWayIn(t *testing.T) {
	ctx := context.Background()
	f := newSignInMethodFixture(t)
	userID := f.user(true, "google")
	_, err := f.methods.RemoveIdentity(ctx, userID, f.identityID(userID, "google"), usableMethods(true))
	require.NoError(t, err, "the password remains, so google can go")

	err = f.methods.RemovePassword(ctx, userID, usableMethods(true), repositories.RemovePasswordOptions{})

	require.ErrorIs(t, err, repositories.ErrLastSignInMethod, "the disconnected google record does not count")
	assert.True(t, f.hasPassword(userID))
}
