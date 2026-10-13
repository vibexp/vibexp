package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
)

const (
	rootPwEmail  = "root@example.com"
	rootPwUserID = "user-root-1"
	rootPwPhrase = "a long operator passphrase"
)

// rootPwSetter records the passwords it was asked to store.
type rootPwSetter struct {
	calls []string
	err   error
}

func (s *rootPwSetter) SetPassword(_ context.Context, userID, plaintext string) error {
	if s.err != nil {
		return s.err
	}
	s.calls = append(s.calls, userID+":"+plaintext)
	return nil
}

// rootPwEvents records published events.
type rootPwEvents struct {
	published []events.Event
	err       error
}

func (e *rootPwEvents) Publish(_ context.Context, event events.Event) error {
	if e.err != nil {
		return e.err
	}
	e.published = append(e.published, event)
	return nil
}

type rootPwFixture struct {
	svc         *RootAdminPasswordService
	users       *repo_mocks.MockUserRepository
	credentials *repo_mocks.MockUserCredentialRepository
	policies    *repo_mocks.MockInstanceAuthPolicyRepository
	setter      *rootPwSetter
	events      *rootPwEvents
	logs        *logtest.Recorder
	now         time.Time
}

// newRootPwFixture builds the service. withEvents false is the CLI: it cannot
// create accounts.
func newRootPwFixture(t *testing.T, withEvents bool) *rootPwFixture {
	t.Helper()
	logger, logs := logtest.New()
	f := &rootPwFixture{
		users:       repo_mocks.NewMockUserRepository(t),
		credentials: repo_mocks.NewMockUserCredentialRepository(t),
		policies:    repo_mocks.NewMockInstanceAuthPolicyRepository(t),
		setter:      &rootPwSetter{},
		events:      &rootPwEvents{},
		logs:        logs,
		now:         time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC),
	}
	deps := RootAdminPasswordDeps{
		Users: f.users, Credentials: f.credentials, Policies: f.policies, Passwords: f.setter,
		IsRootAdmin: func(email string) bool { return email == rootPwEmail },
		Logger:      logger,
	}
	if withEvents {
		deps.Events = f.events
	}
	f.svc = NewRootAdminPasswordService(deps)
	f.svc.now = func() time.Time { return f.now }
	return f
}

// existing arranges an account for the root admin, with or without a password.
func (f *rootPwFixture) existing(user *models.User, hasPassword bool) {
	f.users.On("GetByEmail", mock.Anything, rootPwEmail).Return(user, nil)
	if hasPassword {
		f.credentials.On("Get", mock.Anything, user.ID).Return(&models.UserCredential{UserID: user.ID}, nil)
	} else {
		f.credentials.On("Get", mock.Anything, user.ID).Return(nil, repositories.ErrUserCredentialNotFound)
	}
}

func (f *rootPwFixture) storedPolicy(policy *models.InstanceAuthPolicy) {
	if policy == nil {
		f.policies.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceAuthPolicyNotFound)
		return
	}
	f.policies.On("Get", mock.Anything).Return(policy, nil)
}

func TestRootAdminPassword_ExistingAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("sets the password, verifies the email, ends sessions, switches login on", func(t *testing.T) {
		f := newRootPwFixture(t, true)
		user := &models.User{ID: rootPwUserID, Email: rootPwEmail, Status: models.UserStatusActive}
		f.existing(user, false)
		f.users.On("Update", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
			return u.ID == rootPwUserID && u.EmailVerifiedAt != nil && u.EmailVerifiedAt.Equal(f.now)
		})).Return(nil).Once()
		f.users.On("BumpSessionEpoch", mock.Anything, rootPwUserID).Return(int64(1), nil).Twice()
		f.storedPolicy(&models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeInviteOnly, Version: 3})
		f.policies.On("UpsertAudited", mock.Anything, mock.MatchedBy(func(p *models.InstanceAuthPolicy) bool {
			return p.PasswordLoginEnabled && p.RegistrationMode == models.RegistrationModeInviteOnly
		}), (*string)(nil), versionIs(3)).Return(nil).Once()

		// Letter case and surrounding space in the address do not matter.
		result, err := f.svc.Set(ctx, "  ROOT@Example.com ", rootPwPhrase, true)

		require.NoError(t, err)
		assert.Equal(t, &RootAdminPasswordResult{UserID: rootPwUserID, Email: rootPwEmail, LoginSwitchedOn: true}, result)
		assert.Equal(t, []string{rootPwUserID + ":" + rootPwPhrase}, f.setter.calls)
		assert.Empty(t, f.events.published)
		for _, entry := range f.logs.AllEntries() {
			assert.NotContains(t, fmt.Sprint(entry.Message, entry.Data), rootPwPhrase, "the password is never logged")
		}
	})

	t.Run("an already verified email is left alone; login already on is not rewritten", func(t *testing.T) {
		f := newRootPwFixture(t, true)
		verified := f.now.Add(-time.Hour)
		f.existing(&models.User{ID: rootPwUserID, Email: rootPwEmail, EmailVerifiedAt: &verified}, false)
		f.users.On("BumpSessionEpoch", mock.Anything, rootPwUserID).Return(int64(1), nil).Twice()
		f.storedPolicy(&models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeOpen})

		result, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.NoError(t, err)
		assert.False(t, result.LoginSwitchedOn)
		f.users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		f.policies.AssertNotCalled(t, "UpsertAudited", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("without enableLogin the policy is not read", func(t *testing.T) {
		f := newRootPwFixture(t, false)
		verified := f.now
		f.existing(&models.User{ID: rootPwUserID, Email: rootPwEmail, EmailVerifiedAt: &verified}, false)
		f.users.On("BumpSessionEpoch", mock.Anything, rootPwUserID).Return(int64(1), nil).Twice()

		result, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, false)

		require.NoError(t, err)
		assert.False(t, result.LoginSwitchedOn)
	})

	t.Run("replacing a password is reported and logged as a warning", func(t *testing.T) {
		f := newRootPwFixture(t, false)
		verified := f.now
		f.existing(&models.User{ID: rootPwUserID, Email: rootPwEmail, EmailVerifiedAt: &verified}, true)
		f.users.On("BumpSessionEpoch", mock.Anything, rootPwUserID).Return(int64(4), nil).Twice()

		result, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, false)

		require.NoError(t, err)
		assert.True(t, result.PasswordReplaced)
		last := f.logs.LastEntry()
		require.NotNil(t, last)
		assert.Equal(t, slog.LevelWarn, last.Level)
		assert.Contains(t, last.Message, "existing password was replaced")
	})
}

func TestRootAdminPassword_NewAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("created verified, provisioned by user.created, password stored", func(t *testing.T) {
		f := newRootPwFixture(t, true)
		f.users.On("GetByEmail", mock.Anything, rootPwEmail).Return(nil, repositories.ErrUserNotFound)
		f.users.On("Create", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
			return u.Email == rootPwEmail && u.Name == "root" && u.EmailVerifiedAt != nil
		})).Run(func(args mock.Arguments) { args.Get(1).(*models.User).ID = "user-new" }).Return(nil).Once()
		f.storedPolicy(nil)
		f.policies.On("UpsertAudited", mock.Anything, mock.MatchedBy(func(p *models.InstanceAuthPolicy) bool {
			return p.PasswordLoginEnabled && p.RegistrationMode == models.RegistrationModeOpen
		}), (*string)(nil), versionIs(repositories.InstanceSettingsNoStoredVersion)).Return(nil).Once()

		result, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.NoError(t, err)
		assert.True(t, result.AccountCreated)
		assert.False(t, result.PasswordReplaced)
		assert.Equal(t, []string{"user-new:" + rootPwPhrase}, f.setter.calls)
		require.Len(t, f.events.published, 1)
		// A new account has no session to end and is already verified.
		f.users.AssertNotCalled(t, "BumpSessionEpoch", mock.Anything, mock.Anything)
		f.users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("a caller with no event publisher cannot create one", func(t *testing.T) {
		f := newRootPwFixture(t, false)
		f.users.On("GetByEmail", mock.Anything, rootPwEmail).Return(nil, repositories.ErrUserNotFound)

		_, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.ErrorIs(t, err, ErrRootAdminAccountMissing)
		assert.Empty(t, f.setter.calls)
	})

	t.Run("a failed provisioning removes the row again", func(t *testing.T) {
		f := newRootPwFixture(t, true)
		f.events.err = errors.New("bus down")
		f.users.On("GetByEmail", mock.Anything, rootPwEmail).Return(nil, repositories.ErrUserNotFound)
		f.users.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { args.Get(1).(*models.User).ID = "user-new" }).Return(nil).Once()
		f.users.On("DeleteByID", mock.Anything, "user-new").Return(nil).Once()

		_, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.Error(t, err)
		assert.Empty(t, f.setter.calls, "no password for an account that does not exist")
	})
}

func TestRootAdminPassword_RefusalsChangeNothing(t *testing.T) {
	ctx := context.Background()

	t.Run("not a root admin", func(t *testing.T) {
		for _, email := range []string{"member@example.com", "", "   "} {
			f := newRootPwFixture(t, true) // strict mocks: nothing is read or written

			_, err := f.svc.Set(ctx, email, rootPwPhrase, true)

			require.ErrorIs(t, err, ErrNotRootAdmin, email)
		}
	})

	t.Run("a password that breaks the policy, before any account is created", func(t *testing.T) {
		for name, plaintext := range map[string]string{
			"too short": "short", "the email itself": rootPwEmail, "too long": strings.Repeat("x", 2000),
		} {
			f := newRootPwFixture(t, true) // strict mocks: nothing is read or written

			_, err := f.svc.Set(ctx, rootPwEmail, plaintext, true)

			var policyErr *password.PolicyError
			require.ErrorAs(t, err, &policyErr, name)
		}
	})

	t.Run("a suspended account", func(t *testing.T) {
		f := newRootPwFixture(t, true)
		f.users.On("GetByEmail", mock.Anything, rootPwEmail).
			Return(&models.User{ID: rootPwUserID, Email: rootPwEmail, Status: models.UserStatusSuspended}, nil)

		_, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.ErrorIs(t, err, ErrRootAdminSuspended)
		assert.Empty(t, f.setter.calls)
	})

	t.Run("a failure to store the password leaves login off", func(t *testing.T) {
		f := newRootPwFixture(t, true)
		f.setter.err = errors.New("pq: down")
		verified := f.now
		f.existing(&models.User{ID: rootPwUserID, Email: rootPwEmail, EmailVerifiedAt: &verified}, false)
		f.users.On("BumpSessionEpoch", mock.Anything, rootPwUserID).Return(int64(1), nil).Once()

		_, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.Error(t, err)
		f.policies.AssertNotCalled(t, "UpsertAudited", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	// Sessions end BEFORE the password changes, so no failure can leave a new
	// password with the old sessions still alive.
	t.Run("a failure to end the sessions stores no password", func(t *testing.T) {
		f := newRootPwFixture(t, true)
		verified := f.now
		f.existing(&models.User{ID: rootPwUserID, Email: rootPwEmail, EmailVerifiedAt: &verified}, true)
		f.users.On("BumpSessionEpoch", mock.Anything, rootPwUserID).Return(int64(0), errors.New("pq: down")).Once()

		_, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.Error(t, err)
		assert.Empty(t, f.setter.calls)
	})
}

// versionIs matches the expected version handed to UpsertAudited.
func versionIs(want int64) any {
	return mock.MatchedBy(func(got *int64) bool { return got != nil && *got == want })
}

// Switching password login on is a compare-and-set on the policy it read: a
// policy saved in between is re-read, so its registration mode is not
// overwritten with a stale one.
func TestRootAdminPassword_SwitchLoginOnDoesNotOverwriteAConcurrentPolicyChange(t *testing.T) {
	ctx := context.Background()
	arrange := func(t *testing.T) *rootPwFixture {
		t.Helper()
		f := newRootPwFixture(t, true)
		verified := f.now
		f.existing(&models.User{ID: rootPwUserID, Email: rootPwEmail, EmailVerifiedAt: &verified}, false)
		f.users.On("BumpSessionEpoch", mock.Anything, rootPwUserID).Return(int64(1), nil).Twice()
		return f
	}

	t.Run("the retry writes on top of the newer policy", func(t *testing.T) {
		f := arrange(t)
		f.policies.On("Get", mock.Anything).
			Return(&models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeOpen, Version: 3}, nil).Once()
		f.policies.On("UpsertAudited", mock.Anything, mock.Anything, (*string)(nil), versionIs(3)).
			Return(repositories.ErrInstanceSettingsVersionConflict).Once()
		// An admin saved invite_only in between.
		f.policies.On("Get", mock.Anything).
			Return(&models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeInviteOnly, Version: 4}, nil).Once()
		f.policies.On("UpsertAudited", mock.Anything, mock.MatchedBy(func(p *models.InstanceAuthPolicy) bool {
			return p.PasswordLoginEnabled && p.RegistrationMode == models.RegistrationModeInviteOnly
		}), (*string)(nil), versionIs(4)).Return(nil).Once()

		result, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.NoError(t, err)
		assert.True(t, result.LoginSwitchedOn)
	})

	t.Run("a second conflict is an error", func(t *testing.T) {
		f := arrange(t)
		f.policies.On("Get", mock.Anything).
			Return(&models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeOpen, Version: 3}, nil).Twice()
		f.policies.On("UpsertAudited", mock.Anything, mock.Anything, (*string)(nil), versionIs(3)).
			Return(repositories.ErrInstanceSettingsVersionConflict).Twice()

		_, err := f.svc.Set(ctx, rootPwEmail, rootPwPhrase, true)

		require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)
	})
}

// The sessions are ended before AND after the password is stored. A sign-in
// with the old password that lands while the new one is being hashed gets a
// session at the already bumped epoch; only the second bump ends it.
func TestRootAdminPassword_EndsSessionsOnBothSidesOfTheChange(t *testing.T) {
	f := newRootPwFixture(t, false)
	verified := f.now
	f.existing(&models.User{ID: rootPwUserID, Email: rootPwEmail, EmailVerifiedAt: &verified}, true)
	var order []string
	f.users.On("BumpSessionEpoch", mock.Anything, rootPwUserID).
		Run(func(mock.Arguments) { order = append(order, fmt.Sprintf("bump(stored=%d)", len(f.setter.calls))) }).
		Return(int64(1), nil).Twice()

	_, err := f.svc.Set(context.Background(), rootPwEmail, rootPwPhrase, false)

	require.NoError(t, err)
	assert.Equal(t, []string{"bump(stored=0)", "bump(stored=1)"}, order)
}
