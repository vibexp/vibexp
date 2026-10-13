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

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Self-service account rules (#1437).

const accountTestUser = "user-acct-1"

// fakePasswords is the password service as AccountService sees it, recording
// what was asked of it.
type fakePasswords struct {
	PasswordAuthServiceInterface
	status    PasswordStatus
	verifyErr error
	setErr    error
	calls     []string
	// onRemove, when set, runs inside RemovePassword.
	onRemove func()
}

func (p *fakePasswords) PasswordStatus(context.Context, string) (PasswordStatus, error) {
	return p.status, nil
}

func (p *fakePasswords) VerifyPassword(_ context.Context, _, plaintext string) error {
	p.calls = append(p.calls, "verify:"+plaintext)
	return p.verifyErr
}

func (p *fakePasswords) SetPassword(_ context.Context, _, plaintext string) error {
	p.calls = append(p.calls, "set:"+plaintext)
	if p.setErr == nil {
		p.status = PasswordStatus{Set: true, Usable: true, ChangedAt: time.Now()}
	}
	return p.setErr
}

func (p *fakePasswords) RemovePassword(context.Context, string) (bool, error) {
	p.calls = append(p.calls, "remove")
	if p.onRemove != nil {
		p.onRemove()
	}
	removed := p.status.Set
	p.status = PasswordStatus{}
	return removed, nil
}

// fakeAPIAccess records revocations.
type fakeAPIAccess struct {
	revoked []string
	err     error
}

func (a *fakeAPIAccess) RevokeAPIAccess(_ context.Context, userID string) (*models.APIAccessRevocation, error) {
	if a.err != nil {
		return nil, a.err
	}
	a.revoked = append(a.revoked, userID)
	return &models.APIAccessRevocation{}, nil
}

// recordingNotifier counts password-changed notifications.
type recordingNotifier struct{ notified []string }

func (n *recordingNotifier) PasswordChanged(_ context.Context, userID string) {
	n.notified = append(n.notified, userID)
}

// accountFixture is an AccountService over fakes. By default password sign-in
// is on, providers "google" and "github" are enabled, and the account has no
// password and no identity.
// recordingLock is a SignInMethodLock that runs fn at once and records, for
// each call, the user it was taken for.
type recordingLock struct {
	held  bool
	users []string
}

func (l *recordingLock) WithLock(ctx context.Context, userID string, fn func(context.Context) error) error {
	l.users = append(l.users, userID)
	l.held = true
	defer func() { l.held = false }()
	return fn(ctx)
}

type accountFixture struct {
	lock       *recordingLock
	svc        *AccountService
	users      *repo_mocks.MockUserRepository
	identities *repo_mocks.MockUserIdentityRepository
	passwords  *fakePasswords
	apiAccess  *fakeAPIAccess
	notifier   *recordingNotifier
	held       []models.UserIdentity
}

func newAccountFixture(t *testing.T, policy AuthPolicyResolver) *accountFixture {
	t.Helper()
	logger, _ := logtest.New()
	f := &accountFixture{
		users:      &repo_mocks.MockUserRepository{},
		identities: &repo_mocks.MockUserIdentityRepository{},
		passwords:  &fakePasswords{},
		apiAccess:  &fakeAPIAccess{},
		notifier:   &recordingNotifier{},
		lock:       &recordingLock{},
	}
	f.users.On("GetByID", mock.Anything, accountTestUser).
		Return(&models.User{ID: accountTestUser, Email: "ada@example.com", Name: "Ada"}, nil).Maybe()
	f.identities.On("ListByUserID", mock.Anything, accountTestUser).
		Return(func(context.Context, string) ([]models.UserIdentity, error) { return f.held, nil }).Maybe()
	registry := idp.NewRegistryFromEntries(
		idp.Entry{Provider: slugProvider("google"), Type: idp.ProviderGoogle, DisplayName: "Google"},
		idp.Entry{Provider: slugProvider("github"), Type: idp.ProviderGitHub, DisplayName: "GitHub"},
	)
	f.svc = NewAccountService(AccountServiceDeps{
		Users: f.users, Identities: f.identities, MethodLock: f.lock, Passwords: f.passwords, Policy: policy,
		Providers: staticIDPResolver{reg: registry}, APIAccess: f.apiAccess, Notifier: f.notifier, Logger: logger,
	})
	return f
}

// holds gives the account an identity for provider and returns its id.
func (f *accountFixture) holds(provider string) string {
	id := "identity-" + provider
	f.held = append(f.held, models.UserIdentity{
		ID: id, UserID: accountTestUser, Provider: provider, Subject: "sub-" + provider,
		CreatedAt: time.Date(2026, 1, len(f.held)+1, 0, 0, 0, 0, time.UTC),
	})
	return id
}

func (f *accountFixture) hasPassword() *accountFixture {
	f.passwords.status = PasswordStatus{Set: true, Usable: true, ChangedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
	return f
}

var passwordLoginOff = fixedAuthPolicy{policy: models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeOpen}}

func TestAccountService_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("an account with nothing yet", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)

		view, err := f.svc.Get(ctx, accountTestUser)

		require.NoError(t, err)
		assert.Equal(t, "ada@example.com", view.User.Email)
		assert.False(t, view.Password.Set)
		assert.Nil(t, view.Password.ChangedAt)
		assert.True(t, view.Password.LoginEnabled)
		assert.NotNil(t, view.Identities, "an empty list, not nil")
		assert.Empty(t, view.Identities)
	})

	t.Run("password and identities, with internal rows hidden", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOff).hasPassword()
		googleID := f.holds("google")
		f.holds(models.LegacyGoogleIdentityProvider) // alias of the Google identity
		f.holds(devIDPProvider)                      // localhost dev login
		goneID := f.holds("old-sso")                 // its provider was removed

		view, err := f.svc.Get(ctx, accountTestUser)

		require.NoError(t, err)
		assert.True(t, view.Password.Set)
		require.NotNil(t, view.Password.ChangedAt)
		assert.False(t, view.Password.LoginEnabled)
		require.Len(t, view.Identities, 2, "the legacy alias and the dev identity are not sign-in methods to manage")
		assert.Equal(t, AccountIdentityView{
			ID: googleID, Provider: "google", DisplayName: "Google", Available: true,
			ConnectedAt: f.held[0].CreatedAt,
		}, view.Identities[0])
		assert.Equal(t, goneID, view.Identities[1].ID)
		assert.Equal(t, "old-sso", view.Identities[1].DisplayName, "the slug stands in for a provider that is gone")
		assert.False(t, view.Identities[1].Available)
	})

	t.Run("a policy that cannot be read is an error, not 'disabled'", func(t *testing.T) {
		boom := errors.New("db down")
		f := newAccountFixture(t, fixedAuthPolicy{err: boom})

		_, err := f.svc.Get(ctx, accountTestUser)

		require.ErrorIs(t, err, boom)
	})
}

func TestAccountService_UpdateName(t *testing.T) {
	ctx := context.Background()

	t.Run("stores the trimmed name", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.users.On("UpdateName", ctx, accountTestUser, "Ada Lovelace").Return(nil).Once()

		view, err := f.svc.UpdateName(ctx, accountTestUser, "  Ada Lovelace \t")

		require.NoError(t, err)
		assert.NotNil(t, view)
		f.users.AssertExpectations(t)
	})

	t.Run("255 characters fit, counted as characters", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		longest := strings.Repeat("é", AccountNameMaxLength)
		f.users.On("UpdateName", ctx, accountTestUser, longest).Return(nil).Once()

		_, err := f.svc.UpdateName(ctx, accountTestUser, longest)

		require.NoError(t, err)
	})

	for name, input := range map[string]string{
		"blank": "   ", "empty": "", "too long": strings.Repeat("a", AccountNameMaxLength+1),
	} {
		t.Run(name+" is refused and nothing is written", func(t *testing.T) {
			f := newAccountFixture(t, passwordLoginOn)

			_, err := f.svc.UpdateName(ctx, accountTestUser, input)

			require.ErrorIs(t, err, ErrAccountNameInvalid)
			f.users.AssertNotCalled(t, "UpdateName", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestAccountService_SetPassword(t *testing.T) {
	ctx := context.Background()
	const next = "a brand new long passphrase"

	t.Run("a first password needs a recent sign-in, and no current password", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)

		first, err := f.svc.SetPassword(ctx, accountTestUser, AccountPasswordChange{New: next, RecentSignIn: true})

		require.NoError(t, err)
		assert.True(t, first)
		assert.Equal(t, []string{"set:" + next}, f.passwords.calls, "nothing to verify against")
		assert.Equal(t, []string{accountTestUser}, f.notifier.notified)
	})

	t.Run("a first password without a recent sign-in is refused", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)

		_, err := f.svc.SetPassword(ctx, accountTestUser, AccountPasswordChange{New: next})

		require.ErrorIs(t, err, ErrReauthenticationRequired)
		assert.Empty(t, f.passwords.calls)
		assert.Empty(t, f.notifier.notified)
	})

	t.Run("a change needs the current password, not a recent sign-in", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn).hasPassword()

		first, err := f.svc.SetPassword(ctx, accountTestUser, AccountPasswordChange{Current: "the old one", New: next})

		require.NoError(t, err)
		assert.False(t, first)
		assert.Equal(t, []string{"verify:the old one", "set:" + next}, f.passwords.calls,
			"the current password is checked before anything is stored")
		assert.Equal(t, []string{accountTestUser}, f.notifier.notified)
	})

	for name, verifyErr := range map[string]error{
		"a wrong current password": ErrCurrentPasswordInvalid,
		"a locked password":        &PasswordLockedError{Until: time.Now().Add(time.Minute)},
	} {
		t.Run(name+" stores nothing", func(t *testing.T) {
			f := newAccountFixture(t, passwordLoginOn).hasPassword()
			f.passwords.verifyErr = verifyErr

			_, err := f.svc.SetPassword(ctx, accountTestUser,
				AccountPasswordChange{Current: "guess", New: next, RecentSignIn: true})

			require.ErrorIs(t, err, verifyErr)
			assert.Equal(t, []string{"verify:guess"}, f.passwords.calls, "a recent sign-in does not replace the current password")
			assert.Empty(t, f.notifier.notified)
		})
	}

	t.Run("a new password that breaks the policy is passed through", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.passwords.setErr = password.Validate("short", "ada@example.com")
		require.Error(t, f.passwords.setErr)

		_, err := f.svc.SetPassword(ctx, accountTestUser, AccountPasswordChange{New: "short", RecentSignIn: true})

		var policyErr *password.PolicyError
		require.ErrorAs(t, err, &policyErr)
		assert.Empty(t, f.notifier.notified, "nothing changed, nothing to announce")
	})

	t.Run("refused while password sign-in is off, before anything is checked", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOff).hasPassword()

		_, err := f.svc.SetPassword(ctx, accountTestUser,
			AccountPasswordChange{Current: "the old one", New: next, RecentSignIn: true})

		require.ErrorIs(t, err, ErrPasswordLoginDisabled)
		assert.Empty(t, f.passwords.calls)
	})
}

func TestAccountService_RemovePassword(t *testing.T) {
	ctx := context.Background()

	t.Run("removed when another usable method remains", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn).hasPassword()
		f.holds("google")

		require.NoError(t, f.svc.RemovePassword(ctx, accountTestUser, true))

		assert.Equal(t, []string{"remove"}, f.passwords.calls)
		assert.Equal(t, []string{accountTestUser}, f.notifier.notified)
	})

	t.Run("works while password sign-in is off", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOff).hasPassword()
		f.holds("github")

		require.NoError(t, f.svc.RemovePassword(ctx, accountTestUser, true))
	})

	t.Run("needs a recent sign-in", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn).hasPassword()
		f.holds("google")

		require.ErrorIs(t, f.svc.RemovePassword(ctx, accountTestUser, false), ErrReauthenticationRequired)
		assert.Empty(t, f.passwords.calls)
	})

	t.Run("refused when it is the only usable method", func(t *testing.T) {
		for name, arrange := range map[string]func(*accountFixture){
			"no identity at all":                     func(*accountFixture) {},
			"only an identity of a removed provider": func(f *accountFixture) { f.holds("old-sso") },
			"only the legacy alias and dev login": func(f *accountFixture) {
				f.holds(models.LegacyGoogleIdentityProvider)
				f.holds(devIDPProvider)
			},
		} {
			t.Run(name, func(t *testing.T) {
				f := newAccountFixture(t, passwordLoginOn).hasPassword()
				arrange(f)

				require.ErrorIs(t, f.svc.RemovePassword(ctx, accountTestUser, true), ErrLastSignInMethod)
				assert.Empty(t, f.passwords.calls)
				assert.Empty(t, f.notifier.notified)
			})
		}
	})

	t.Run("an account with no password", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.holds("google")

		require.ErrorIs(t, f.svc.RemovePassword(ctx, accountTestUser, true), ErrAccountPasswordNotSet)
	})
}

func TestAccountService_DisconnectIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("disconnected when another identity is usable", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		googleID := f.holds("google")
		f.holds("github")
		f.identities.On("DeleteForUser", ctx, accountTestUser, googleID).Return(nil).Once()

		provider, err := f.svc.DisconnectIdentity(ctx, accountTestUser, googleID, true)

		require.NoError(t, err)
		assert.Equal(t, "google", provider)
		f.identities.AssertExpectations(t)
		assert.Empty(t, f.notifier.notified, "the password did not change")
	})

	t.Run("disconnected when a usable password remains", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn).hasPassword()
		googleID := f.holds("google")
		f.identities.On("DeleteForUser", ctx, accountTestUser, googleID).Return(nil).Once()

		_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, googleID, true)

		require.NoError(t, err)
	})

	t.Run("an identity of a removed provider can be cleaned up when something usable remains", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		goneID := f.holds("old-sso")
		f.holds("google")
		f.identities.On("DeleteForUser", ctx, accountTestUser, goneID).Return(nil).Once()

		_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, goneID, true)

		require.NoError(t, err)
	})

	t.Run("refused when nothing usable would remain", func(t *testing.T) {
		for name, tc := range map[string]struct {
			policy  AuthPolicyResolver
			arrange func(*accountFixture)
		}{
			"the only identity, no password": {passwordLoginOn, func(*accountFixture) {}},
			"a password, but password sign-in is off": {passwordLoginOff, func(f *accountFixture) {
				f.hasPassword()
			}},
			"a password an administrator disabled": {passwordLoginOn, func(f *accountFixture) {
				f.passwords.status = PasswordStatus{Set: true, Usable: false}
			}},
			"another identity whose provider is gone": {passwordLoginOn, func(f *accountFixture) {
				f.holds("old-sso")
			}},
		} {
			t.Run(name, func(t *testing.T) {
				f := newAccountFixture(t, tc.policy)
				tc.arrange(f)
				googleID := f.holds("google")

				_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, googleID, true)

				require.ErrorIs(t, err, ErrLastSignInMethod)
				f.identities.AssertNotCalled(t, "DeleteForUser", mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("needs a recent sign-in", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		googleID := f.holds("google")
		f.holds("github")

		_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, googleID, false)

		require.ErrorIs(t, err, ErrReauthenticationRequired)
		f.identities.AssertNotCalled(t, "DeleteForUser", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("an unknown, internal or someone else's identity is not found", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.holds("google")
		f.holds("github")
		aliasID := f.holds(models.LegacyGoogleIdentityProvider)

		for _, id := range []string{"identity-of-another-user", aliasID} {
			_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, id, true)
			require.ErrorIs(t, err, ErrAccountIdentityNotFound, id)
		}
		f.identities.AssertNotCalled(t, "DeleteForUser", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("an identity that vanished between the check and the delete is not found", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		googleID := f.holds("google")
		f.holds("github")
		f.identities.On("DeleteForUser", ctx, accountTestUser, googleID).Return(repositories.ErrUserIdentityNotFound)

		_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, googleID, true)

		require.ErrorIs(t, err, ErrAccountIdentityNotFound)
	})
}

func TestAccountService_RevokeAPIAccess(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t, passwordLoginOn)

	require.NoError(t, f.svc.RevokeAPIAccess(ctx, accountTestUser))
	assert.Equal(t, []string{accountTestUser}, f.apiAccess.revoked)

	boom := errors.New("db down")
	f.apiAccess.err = boom
	require.ErrorIs(t, f.svc.RevokeAPIAccess(ctx, accountTestUser), boom)
}

// With no notifier installed nothing is announced and nothing breaks.
func TestAccountService_DefaultNotifierIsANoop(t *testing.T) {
	logger, _ := logtest.New()
	passwords := &fakePasswords{}
	svc := NewAccountService(AccountServiceDeps{
		Passwords: passwords, Policy: passwordLoginOn, Logger: logger,
	})

	first, err := svc.SetPassword(context.Background(), accountTestUser,
		AccountPasswordChange{New: "a brand new long passphrase", RecentSignIn: true})

	require.NoError(t, err)
	assert.True(t, first)
}

// The guard and the delete must run under the user's sign-in method lock:
// without it two removals running together each see the other's method still
// present, and the account ends with none.
func TestAccountService_RemovalsReadAndDeleteUnderTheLock(t *testing.T) {
	ctx := context.Background()

	t.Run("removing the password", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn).hasPassword()
		f.holds("google")
		var listedUnderLock bool
		f.identities.ExpectedCalls = nil
		f.identities.On("ListByUserID", mock.Anything, accountTestUser).
			Return(func(context.Context, string) ([]models.UserIdentity, error) {
				listedUnderLock = f.lock.held
				return f.held, nil
			})
		f.passwords.onRemove = func() { assert.True(t, f.lock.held, "the delete runs under the lock") }

		require.NoError(t, f.svc.RemovePassword(ctx, accountTestUser, true))

		assert.True(t, listedUnderLock, "the guard reads the identities under the lock")
		assert.Equal(t, []string{accountTestUser}, f.lock.users)
	})

	t.Run("disconnecting an identity", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn).hasPassword()
		id := f.holds("google")
		var listedUnderLock, deletedUnderLock bool
		f.identities.ExpectedCalls = nil
		f.identities.On("ListByUserID", mock.Anything, accountTestUser).
			Return(func(context.Context, string) ([]models.UserIdentity, error) {
				listedUnderLock = f.lock.held
				return f.held, nil
			})
		f.identities.On("DeleteForUser", mock.Anything, accountTestUser, id).
			Return(func(context.Context, string, string) error {
				deletedUnderLock = f.lock.held
				return nil
			})

		_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, id, true)

		require.NoError(t, err)
		assert.True(t, listedUnderLock, "the guard reads the identities under the lock")
		assert.True(t, deletedUnderLock, "the delete runs under the lock")
		assert.Equal(t, []string{accountTestUser}, f.lock.users)
	})

	t.Run("a refusal for a stale sign-in takes no lock", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn).hasPassword()

		require.ErrorIs(t, f.svc.RemovePassword(ctx, accountTestUser, false), ErrReauthenticationRequired)

		assert.Empty(t, f.lock.users)
	})
}

// An omitted current password cannot be right, and is refused without
// spending one of the account's attempts.
func TestAccountService_SetPassword_EmptyCurrentPasswordSpendsNoAttempt(t *testing.T) {
	f := newAccountFixture(t, passwordLoginOn).hasPassword()

	_, err := f.svc.SetPassword(context.Background(), accountTestUser,
		AccountPasswordChange{New: "a brand new long passphrase", RecentSignIn: true})

	require.ErrorIs(t, err, ErrCurrentPasswordInvalid, "the same answer as a wrong one")
	assert.Empty(t, f.passwords.calls, "nothing was verified, so nothing was counted")
}
