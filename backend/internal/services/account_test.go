package services

import (
	"context"
	"errors"
	"fmt"
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
}

func (p *fakePasswords) PasswordStatus(context.Context, string) (PasswordStatus, error) {
	return p.status, nil
}

func (p *fakePasswords) VerifyPassword(_ context.Context, _, plaintext string) error {
	p.calls = append(p.calls, "verify:"+plaintext)
	return p.verifyErr
}

func (p *fakePasswords) ChangeOwnPassword(_ context.Context, _, plaintext string) error {
	p.calls = append(p.calls, "set:"+plaintext)
	if p.setErr == nil {
		p.status = PasswordStatus{Set: true, Usable: true, ChangedAt: time.Now()}
	}
	return p.setErr
}

func (p *fakePasswords) RemovePassword(context.Context, string) (bool, error) {
	p.calls = append(p.calls, "remove")
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
type recordingNotifier struct {
	notified []string
	kinds    []AccountChangeKind
}

func (n *recordingNotifier) PasswordChanged(_ context.Context, userID string, kind AccountChangeKind) {
	n.notified = append(n.notified, userID)
	n.kinds = append(n.kinds, kind)
}

// accountFixture is an AccountService over fakes. By default password sign-in
// is on, providers "google" and "github" are enabled, and the account has no
// password and no identity.
type accountFixture struct {
	methods    *repo_mocks.MockSignInMethodRepository
	svc        *AccountService
	users      *repo_mocks.MockUserRepository
	identities *repo_mocks.MockUserIdentityRepository
	passwords  *fakePasswords
	apiAccess  *fakeAPIAccess
	notifier   *recordingNotifier
	held       []models.UserIdentity
	// disconnected is what ListDisconnected answers (#1460).
	disconnected []models.DisconnectedIdentity
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
		methods:    repo_mocks.NewMockSignInMethodRepository(t),
	}
	f.users.On("GetByID", mock.Anything, accountTestUser).
		Return(&models.User{ID: accountTestUser, Email: "ada@example.com", Name: "Ada"}, nil).Maybe()
	f.identities.On("ListByUserID", mock.Anything, accountTestUser).
		Return(func(context.Context, string) ([]models.UserIdentity, error) { return f.held, nil }).Maybe()
	f.methods.EXPECT().ListDisconnected(mock.Anything, accountTestUser).
		RunAndReturn(func(context.Context, string) ([]models.DisconnectedIdentity, error) {
			return f.disconnected, nil
		}).Maybe()
	registry := idp.NewRegistryFromEntries(
		idp.Entry{Provider: slugProvider("google"), Type: idp.ProviderGoogle, DisplayName: "Google"},
		idp.Entry{Provider: slugProvider("github"), Type: idp.ProviderGitHub, DisplayName: "GitHub"},
	)
	f.svc = NewAccountService(AccountServiceDeps{
		Users: f.users, Identities: f.identities, Methods: f.methods, Passwords: f.passwords, Policy: policy,
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

// What works on the fixture's instance: its two enabled providers, and the
// password according to the policy.
func usableWith(passwordLogin bool) repositories.UsableSignInMethods {
	return repositories.UsableSignInMethods{Providers: []string{"google", "github"}, PasswordLogin: passwordLogin}
}

// The "another usable method remains" rule itself is the repository's, where
// it runs under a lock with the delete (TestIntegrationSignInMethod_*). Here:
// what the service tells it works, and how its answers are reported.
func TestAccountService_RemovePassword(t *testing.T) {
	ctx := context.Background()

	t.Run("removed, and the notifier is told", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.methods.EXPECT().RemovePassword(ctx, accountTestUser, usableWith(true), repositories.RemovePasswordOptions{}).Return(nil).Once()

		require.NoError(t, f.svc.RemovePassword(ctx, accountTestUser, true))

		assert.Equal(t, []string{accountTestUser}, f.notifier.notified)
	})

	t.Run("works while password sign-in is off, and says so to the repository", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOff)
		f.methods.EXPECT().RemovePassword(ctx, accountTestUser, usableWith(false), repositories.RemovePasswordOptions{}).Return(nil).Once()

		require.NoError(t, f.svc.RemovePassword(ctx, accountTestUser, true))
	})

	t.Run("needs a recent sign-in", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn) // a strict mock: nothing is asked of the repository

		require.ErrorIs(t, f.svc.RemovePassword(ctx, accountTestUser, false), ErrReauthenticationRequired)
		assert.Empty(t, f.notifier.notified)
	})

	for name, tc := range map[string]struct{ repoErr, want error }{
		"the only usable method": {repositories.ErrLastSignInMethod, ErrLastSignInMethod},
		"no password":            {repositories.ErrUserCredentialNotFound, ErrAccountPasswordNotSet},
		"a disabled password":    {repositories.ErrUserCredentialDisabled, ErrPasswordDisabled},
	} {
		t.Run(name, func(t *testing.T) {
			f := newAccountFixture(t, passwordLoginOn)
			f.methods.EXPECT().RemovePassword(ctx, accountTestUser, usableWith(true), repositories.RemovePasswordOptions{}).Return(tc.repoErr).Once()

			require.ErrorIs(t, f.svc.RemovePassword(ctx, accountTestUser, true), tc.want)
			assert.Empty(t, f.notifier.notified)
		})
	}

	t.Run("a storage failure is not reported as a refusal", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.methods.EXPECT().RemovePassword(ctx, accountTestUser, usableWith(true), repositories.RemovePasswordOptions{}).Return(errors.New("pq: down")).Once()

		err := f.svc.RemovePassword(ctx, accountTestUser, true)

		require.Error(t, err)
		require.NotErrorIs(t, err, ErrLastSignInMethod)
		require.NotErrorIs(t, err, ErrAccountPasswordNotSet)
	})
}

// An administrator's disable (#1461): the owner sees it and cannot clear it.
func TestAccountService_DisabledPassword(t *testing.T) {
	ctx := context.Background()
	disabled := func(f *accountFixture) *accountFixture {
		f.passwords.status = PasswordStatus{Set: true, Usable: false, ChangedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
		return f
	}

	t.Run("the account shows it", func(t *testing.T) {
		f := disabled(newAccountFixture(t, passwordLoginOn))
		f.users.On("GetByID", mock.Anything, accountTestUser).Return(&models.User{ID: accountTestUser}, nil).Maybe()
		f.identities.On("ListByUserID", mock.Anything, accountTestUser).Return(nil, nil).Maybe()

		view, err := f.svc.Get(ctx, accountTestUser)

		require.NoError(t, err)
		assert.True(t, view.Password.Disabled)
		assert.True(t, view.Password.Set)
	})

	t.Run("a change is refused before the current password is checked", func(t *testing.T) {
		f := disabled(newAccountFixture(t, passwordLoginOn))

		_, err := f.svc.SetPassword(ctx, accountTestUser,
			AccountPasswordChange{Current: "the old one", New: "a brand new long passphrase", RecentSignIn: true})

		require.ErrorIs(t, err, ErrPasswordDisabled)
		assert.Empty(t, f.passwords.calls, "no attempt is spent and nothing is stored")
		assert.Empty(t, f.notifier.notified)
	})

	t.Run("a disable that lands during the change wins", func(t *testing.T) {
		// The status read said usable; the store, which checks again in the
		// same statement, finds the mark.
		f := newAccountFixture(t, passwordLoginOn).hasPassword()
		f.passwords.setErr = ErrPasswordDisabled

		_, err := f.svc.SetPassword(ctx, accountTestUser,
			AccountPasswordChange{Current: "the old one", New: "a brand new long passphrase"})

		require.ErrorIs(t, err, ErrPasswordDisabled)
		assert.Empty(t, f.notifier.notified)
	})
}

func TestAccountService_NoticeKinds(t *testing.T) {
	ctx := context.Background()
	const next = "a brand new long passphrase"

	f := newAccountFixture(t, passwordLoginOn)
	_, err := f.svc.SetPassword(ctx, accountTestUser, AccountPasswordChange{New: next, RecentSignIn: true})
	require.NoError(t, err)
	_, err = f.svc.SetPassword(ctx, accountTestUser, AccountPasswordChange{Current: next, New: next + "!"})
	require.NoError(t, err)
	f.methods.EXPECT().RemovePassword(ctx, accountTestUser, usableWith(true), repositories.RemovePasswordOptions{}).Return(nil).Once()
	require.NoError(t, f.svc.RemovePassword(ctx, accountTestUser, true))

	assert.Equal(t, []AccountChangeKind{AccountChangeSet, AccountChangeChanged, AccountChangeRemoved}, f.notifier.kinds)
}

func TestAccountService_AdminRemovePassword(t *testing.T) {
	ctx := context.Background()

	for _, allowLast := range []bool{false, true} {
		t.Run(fmt.Sprintf("allowLast=%v removes a disabled one too and tells the owner", allowLast), func(t *testing.T) {
			f := newAccountFixture(t, passwordLoginOn)
			f.methods.EXPECT().RemovePassword(ctx, accountTestUser, usableWith(true),
				repositories.RemovePasswordOptions{AllowLast: allowLast, AllowDisabled: true}).Return(nil).Once()

			require.NoError(t, f.svc.AdminRemovePassword(ctx, accountTestUser, allowLast))
			assert.Equal(t, []AccountChangeKind{AccountChangeRemovedByAdmin}, f.notifier.kinds)
		})
	}

	t.Run("the last method is refused unless allowed", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.methods.EXPECT().RemovePassword(ctx, accountTestUser, usableWith(true),
			repositories.RemovePasswordOptions{AllowDisabled: true}).Return(repositories.ErrLastSignInMethod).Once()

		require.ErrorIs(t, f.svc.AdminRemovePassword(ctx, accountTestUser, false), ErrLastSignInMethod)
		assert.Empty(t, f.notifier.notified)
	})
}

func TestAccountService_DisconnectIdentity(t *testing.T) {
	ctx := context.Background()
	const identityID = "identity-google"

	t.Run("disconnected; the password did not change", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.methods.EXPECT().RemoveIdentity(ctx, accountTestUser, identityID, usableWith(true)).Return("google", nil).Once()

		provider, err := f.svc.DisconnectIdentity(ctx, accountTestUser, identityID, true)

		require.NoError(t, err)
		assert.Equal(t, "google", provider)
		assert.Empty(t, f.notifier.notified)
	})

	t.Run("with password sign-in off, a password is not offered as what remains", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOff)
		f.methods.EXPECT().RemoveIdentity(ctx, accountTestUser, identityID, usableWith(false)).
			Return("", repositories.ErrLastSignInMethod).Once()

		_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, identityID, true)

		require.ErrorIs(t, err, ErrLastSignInMethod)
	})

	t.Run("needs a recent sign-in", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn) // a strict mock: nothing is asked of the repository

		_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, identityID, false)

		require.ErrorIs(t, err, ErrReauthenticationRequired)
	})

	t.Run("an unknown, internal or someone else's identity is not found", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.methods.EXPECT().RemoveIdentity(ctx, accountTestUser, identityID, usableWith(true)).
			Return("", repositories.ErrUserIdentityNotFound).Once()

		_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, identityID, true)

		require.ErrorIs(t, err, ErrAccountIdentityNotFound)
	})
}

// When the instance's providers or policy cannot be read, nothing is removed:
// the service cannot say what would remain usable.
func TestAccountService_RemovalsFailClosedWhenUsableMethodsAreUnknown(t *testing.T) {
	ctx := context.Background()
	for name, arrange := range map[string]func(t *testing.T) *accountFixture{
		"the policy cannot be read": func(t *testing.T) *accountFixture {
			return newAccountFixture(t, fixedAuthPolicy{err: errors.New("pq: down")})
		},
		"the providers cannot be read": func(t *testing.T) *accountFixture {
			f := newAccountFixture(t, passwordLoginOn)
			f.svc.providers = staticIDPResolver{err: errors.New("pq: down")}
			return f
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := arrange(t) // the repository is a strict mock: nothing is asked of it

			require.Error(t, f.svc.RemovePassword(ctx, accountTestUser, true))
			_, err := f.svc.DisconnectIdentity(ctx, accountTestUser, "identity-google", true)
			require.Error(t, err)
			assert.Empty(t, f.notifier.notified)
		})
	}
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

// An omitted current password cannot be right, and is refused without
// spending one of the account's attempts.
func TestAccountService_SetPassword_EmptyCurrentPasswordSpendsNoAttempt(t *testing.T) {
	f := newAccountFixture(t, passwordLoginOn).hasPassword()

	_, err := f.svc.SetPassword(context.Background(), accountTestUser,
		AccountPasswordChange{New: "a brand new long passphrase", RecentSignIn: true})

	require.ErrorIs(t, err, ErrCurrentPasswordInvalid, "the same answer as a wrong one")
	assert.Empty(t, f.passwords.calls, "nothing was verified, so nothing was counted")
}

// #1460: the provider accounts the owner disconnected, and allowing one again.
func TestAccountService_DisconnectedIdentities(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

	t.Run("listed with the provider's name, or its slug when it is not enabled", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.disconnected = []models.DisconnectedIdentity{
			{ID: "d-1", Provider: "google", Subject: "g-1", DisconnectedAt: at},
			{ID: "d-2", Provider: "old-sso", Subject: "o-1", DisconnectedAt: at.Add(-time.Hour)},
		}

		view, err := f.svc.Get(ctx, accountTestUser)

		require.NoError(t, err)
		assert.Equal(t, []AccountDisconnectedView{
			{ID: "d-1", Provider: "google", DisplayName: "Google", DisconnectedAt: at},
			{ID: "d-2", Provider: "old-sso", DisplayName: "old-sso", DisconnectedAt: at.Add(-time.Hour)},
		}, view.Disconnected)
	})

	t.Run("an empty list, not nil", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)

		view, err := f.svc.Get(ctx, accountTestUser)

		require.NoError(t, err)
		assert.NotNil(t, view.Disconnected)
		assert.Empty(t, view.Disconnected)
	})

	t.Run("allow again needs a recent sign-in", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)

		_, err := f.svc.AllowIdentityAgain(ctx, accountTestUser, "d-1", false)
		require.ErrorIs(t, err, ErrReauthenticationRequired)
		f.methods.AssertNotCalled(t, "AllowIdentityAgain", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("allow again deletes the record", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.methods.EXPECT().AllowIdentityAgain(ctx, accountTestUser, "d-1").Return("google", true, nil).Once()

		provider, err := f.svc.AllowIdentityAgain(ctx, accountTestUser, "d-1", true)
		require.NoError(t, err)
		assert.Equal(t, "google", provider)
	})

	t.Run("an unknown record", func(t *testing.T) {
		f := newAccountFixture(t, passwordLoginOn)
		f.methods.EXPECT().AllowIdentityAgain(ctx, accountTestUser, "nope").Return("", false, nil).Once()

		_, err := f.svc.AllowIdentityAgain(ctx, accountTestUser, "nope", true)
		require.ErrorIs(t, err, ErrDisconnectedIdentityNotFound)
	})
}
