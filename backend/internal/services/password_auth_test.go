package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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

const (
	pwTestUserID   = "user-pw-1"
	pwTestEmail    = "ada@example.com"
	pwTestPassword = "correct horse battery staple"
)

// pwCheapParams keeps the suite fast; a production hash costs 19 MiB.
var pwCheapParams = password.Params{Memory: 64, Time: 1, Threads: 1, SaltLength: 16, KeyLength: 32}

// countingHasher is a real hasher that counts the argon2id computations a
// sign-in performs, split by kind.
type countingHasher struct {
	*password.Hasher
	verifies atomic.Int32
	dummies  atomic.Int32
	hashes   atomic.Int32
}

func (h *countingHasher) Hash(ctx context.Context, plaintext string) (string, error) {
	h.hashes.Add(1)
	return h.Hasher.Hash(ctx, plaintext)
}

func (h *countingHasher) Verify(ctx context.Context, encoded, plaintext string) (bool, bool, error) {
	h.verifies.Add(1)
	return h.Hasher.Verify(ctx, encoded, plaintext)
}

func (h *countingHasher) DummyVerify(ctx context.Context, plaintext string) error {
	h.dummies.Add(1)
	return h.Hasher.DummyVerify(ctx, plaintext)
}

// fixedAuthPolicy is an AuthPolicyResolver over a fixed policy.
type fixedAuthPolicy struct {
	policy models.InstanceAuthPolicy
	err    error
}

func (p fixedAuthPolicy) Policy(context.Context) (models.InstanceAuthPolicy, error) {
	return p.policy, p.err
}

func (fixedAuthPolicy) PasswordResetChannel(context.Context) (string, error) {
	return PasswordResetByAdmin, nil
}

var passwordLoginOn = fixedAuthPolicy{policy: models.InstanceAuthPolicy{
	PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeOpen,
}}

// pwFixture is a PasswordAuthService over repository mocks, a real cheap
// hasher that counts its work, a settable clock and a recording logger.
type pwFixture struct {
	svc         *PasswordAuthService
	users       *repo_mocks.MockUserRepository
	credentials *repo_mocks.MockUserCredentialRepository
	hasher      *countingHasher
	logs        *logtest.Recorder
	now         time.Time
	hash        string
	// derivationsAtStart is the hasher's computation count once the fixture is
	// built, so a test can count the work of the sign-in alone.
	derivationsAtStart int64
}

type pwOption func(*PasswordAuthDeps)

func withPolicy(p AuthPolicyResolver) pwOption {
	return func(d *PasswordAuthDeps) { d.Policy = p }
}

func withAllowlist(emails ...string) pwOption {
	return func(d *PasswordAuthDeps) { d.Allowlist = newStaticAllowlistResolver(d.Logger, emails) }
}

func newPwFixture(t *testing.T, opts ...pwOption) *pwFixture {
	t.Helper()
	logger, logs := logtest.New()
	f := &pwFixture{
		users:       repo_mocks.NewMockUserRepository(t),
		credentials: repo_mocks.NewMockUserCredentialRepository(t),
		hasher:      &countingHasher{Hasher: password.NewHasher(pwCheapParams)},
		logs:        logs,
		now:         time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC),
	}
	hash, err := f.hasher.Hasher.Hash(context.Background(), pwTestPassword)
	require.NoError(t, err)
	f.hash = hash
	f.derivationsAtStart = f.hasher.Derivations()

	deps := PasswordAuthDeps{
		Users:       f.users,
		Credentials: f.credentials,
		Policy:      passwordLoginOn,
		Allowlist:   newStaticAllowlistResolver(logger, nil),
		Hasher:      f.hasher,
		Logger:      logger,
		Now:         func() time.Time { return f.now },
	}
	for _, opt := range opts {
		opt(&deps)
	}
	f.svc = NewPasswordAuthService(deps)
	return f
}

func (f *pwFixture) activeUser() *models.User {
	return &models.User{ID: pwTestUserID, Email: pwTestEmail, Status: models.UserStatusActive, SessionEpoch: 3}
}

func (f *pwFixture) credential() *models.UserCredential {
	return &models.UserCredential{UserID: pwTestUserID, PasswordHash: f.hash}
}

// expectAccount makes the email resolve to user, and admits (and counts) each
// sign-in attempt against credential.
func (f *pwFixture) expectAccount(user *models.User, credential *models.UserCredential) {
	f.users.On("GetByEmail", mock.Anything, pwTestEmail).Return(user, nil)
	f.credentials.On("BeginAttempt", mock.Anything, user.ID, mock.Anything, models.DefaultPasswordLockout).
		Return(credential, nil)
}

// expectUnavailable makes the email resolve to user and refuses the attempt
// with reason, one of the two forms of ErrUserCredentialUnavailable.
func (f *pwFixture) expectUnavailable(user *models.User, reason error) {
	f.users.On("GetByEmail", mock.Anything, pwTestEmail).Return(user, nil)
	f.credentials.On("BeginAttempt", mock.Anything, user.ID, mock.Anything, models.DefaultPasswordLockout).
		Return(nil, reason)
}

// computations is how many argon2id computations the service performed.
func (f *pwFixture) computations() (verifies, dummies int32) {
	return f.hasher.verifies.Load(), f.hasher.dummies.Load()
}

func requireLoginRefusal(t *testing.T, err error, reason, userID string) {
	t.Helper()
	require.ErrorIs(t, err, ErrInvalidCredentials)
	var refusal *PasswordLoginError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, reason, refusal.Reason)
	assert.Equal(t, userID, refusal.UserID)
}

func TestPasswordAuthService_Login_Success(t *testing.T) {
	f := newPwFixture(t)
	f.expectAccount(f.activeUser(), f.credential())
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(nil)

	// The address is normalised before it is looked up.
	user, err := f.svc.Login(context.Background(), "  Ada@Example.COM ", pwTestPassword)

	require.NoError(t, err)
	assert.Equal(t, pwTestUserID, user.ID)
	assert.Equal(t, int64(3), user.SessionEpoch, "the caller mints the session under this epoch")
	verifies, dummies := f.computations()
	assert.Equal(t, int32(1), verifies)
	assert.Zero(t, dummies)
	assert.Zero(t, f.hasher.hashes.Load(), "a hash under the current parameters is not replaced")
}

// Every account-dependent refusal is the same error and costs exactly one
// argon2id computation, so neither the response nor its timing tells an
// anonymous caller which accounts exist.
func TestPasswordAuthService_Login_RefusalsAreUniformAndCostOneComputation(t *testing.T) {
	cases := []struct {
		name       string
		arrange    func(f *pwFixture)
		password   string
		wantReason string
		wantUserID string
		wantDummy  bool
	}{
		{
			name: "unknown email",
			arrange: func(f *pwFixture) {
				f.users.On("GetByEmail", mock.Anything, pwTestEmail).Return(nil, repositories.ErrUserNotFound)
			},
			password: pwTestPassword, wantReason: SignInRefusalUnknownEmail, wantDummy: true,
		},
		{
			name: "account with no password",
			arrange: func(f *pwFixture) {
				f.users.On("GetByEmail", mock.Anything, pwTestEmail).Return(f.activeUser(), nil)
				f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, mock.Anything, mock.Anything).
					Return(nil, repositories.ErrUserCredentialNotFound)
			},
			password: pwTestPassword, wantReason: SignInRefusalNoCredential, wantUserID: pwTestUserID,
			wantDummy: true,
		},
		{
			name: "disabled password, even with the right one",
			arrange: func(f *pwFixture) {
				f.expectUnavailable(f.activeUser(), repositories.ErrUserCredentialDisabled)
			},
			password: pwTestPassword, wantReason: SignInRefusalDisabled, wantUserID: pwTestUserID,
			wantDummy: true,
		},
		{
			name: "locked account, even with the right password",
			arrange: func(f *pwFixture) {
				f.expectUnavailable(f.activeUser(), repositories.ErrUserCredentialLocked)
			},
			password: pwTestPassword, wantReason: SignInRefusalLocked, wantUserID: pwTestUserID,
			wantDummy: true,
		},
		{
			name: "wrong password",
			arrange: func(f *pwFixture) {
				counted := f.credential()
				counted.FailedAttempts = 1
				f.expectAccount(f.activeUser(), counted)
			},
			password: "not the password at all", wantReason: SignInRefusalMismatch,
			wantUserID: pwTestUserID,
		},
		{
			name: "a stored hash that cannot be read",
			arrange: func(f *pwFixture) {
				f.expectAccount(f.activeUser(), &models.UserCredential{UserID: pwTestUserID, PasswordHash: "not-a-hash"})
			},
			password: pwTestPassword, wantReason: SignInRefusalUnreadable, wantUserID: pwTestUserID,
			wantDummy: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPwFixture(t)
			tc.arrange(f)

			user, err := f.svc.Login(context.Background(), pwTestEmail, tc.password)

			assert.Nil(t, user)
			requireLoginRefusal(t, err, tc.wantReason, tc.wantUserID)
			_, dummies := f.computations()
			derivations := f.hasher.Derivations() - f.derivationsAtStart
			assert.Equal(t, int64(1), derivations, "exactly one argon2id computation")
			if tc.wantDummy {
				assert.Equal(t, int32(1), dummies, "nothing to verify against: the work is spent on a dummy")
			} else {
				assert.Zero(t, dummies)
			}
		})
	}
}

// A refused attempt is refused by the admission itself: the service performs no
// write of its own for it, so hammering a locked account cannot push the lock
// further out.
func TestPasswordAuthService_Login_LockedAttemptWritesNothing(t *testing.T) {
	f := newPwFixture(t)
	f.expectUnavailable(f.activeUser(), repositories.ErrUserCredentialLocked)

	for range 3 {
		_, err := f.svc.Login(context.Background(), pwTestEmail, "wrong password again")
		requireLoginRefusal(t, err, SignInRefusalLocked, pwTestUserID)
	}

	f.credentials.AssertNotCalled(t, "RecordSuccess", mock.Anything, mock.Anything)
	f.credentials.AssertNotCalled(t, "UpdateHash", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// The attempt is admitted with the service's clock and schedule, which is what
// lets a lock end by itself: the repository compares the lock with this time.
func TestPasswordAuthService_Login_AdmitsWithTheClockAndSchedule(t *testing.T) {
	custom := models.PasswordLockout{Threshold: 2, Base: time.Second, Max: time.Minute}
	f := newPwFixture(t, func(d *PasswordAuthDeps) { d.Lockout = custom })
	f.users.On("GetByEmail", mock.Anything, pwTestEmail).Return(f.activeUser(), nil)
	f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, custom).Return(f.credential(), nil).Once()
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(nil).Once()

	user, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

	require.NoError(t, err)
	assert.Equal(t, pwTestUserID, user.ID)
}

// The attempt that reaches the threshold is the one that locks; when its
// password is wrong the lock is logged once, with no secret in the entry.
func TestPasswordAuthService_Login_WrongPasswordThatLocksIsLogged(t *testing.T) {
	f := newPwFixture(t)
	counted := f.credential()
	until := f.now.Add(time.Minute)
	counted.FailedAttempts, counted.LockedUntil = 5, &until
	f.expectAccount(f.activeUser(), counted)

	_, err := f.svc.Login(context.Background(), pwTestEmail, "wrong password again")

	requireLoginRefusal(t, err, SignInRefusalMismatch, pwTestUserID)
	last := f.logs.LastEntry()
	require.NotNil(t, last)
	assert.Equal(t, "Password sign-in locked after repeated failures", last.Message)
	assert.EqualValues(t, 5, last.Data["failed_attempts"])
}

// The attempt that reaches the threshold with the RIGHT password signs in, and
// its success clears the lock its own admission set.
func TestPasswordAuthService_Login_RightPasswordOnTheThresholdAttemptSignsIn(t *testing.T) {
	f := newPwFixture(t)
	counted := f.credential()
	until := f.now.Add(time.Minute)
	counted.FailedAttempts, counted.LockedUntil = 5, &until
	f.expectAccount(f.activeUser(), counted)
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(nil).Once()

	user, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

	require.NoError(t, err)
	assert.Equal(t, pwTestUserID, user.ID)
}

// TestPasswordAuthService_Login_ConcurrentGuessesAreBoundedByTheThreshold is
// the throttle under the load it exists for: many wrong passwords arriving
// together. The credential store here admits attempts the way the SQL does,
// one at a time, refusing once the lock is set. However many arrive, no more
// than the threshold are ever checked against the hash.
func TestPasswordAuthService_Login_ConcurrentGuessesAreBoundedByTheThreshold(t *testing.T) {
	f := newPwFixture(t)
	store := &serialCredentialStore{credential: *f.credential()}
	f.svc.credentials = store
	f.users.On("GetByEmail", mock.Anything, pwTestEmail).Return(f.activeUser(), nil)

	const guesses = 100
	var wg sync.WaitGroup
	for i := range guesses {
		wg.Go(func() {
			_, err := f.svc.Login(context.Background(), pwTestEmail, fmt.Sprintf("guess number %d of many", i))
			assert.ErrorIs(t, err, ErrInvalidCredentials)
		})
	}
	wg.Wait()

	verifies, dummies := f.computations()
	assert.Equal(t, int32(models.DefaultPasswordLockout.Threshold), verifies,
		"only the threshold's worth of guesses may be checked against the hash")
	assert.Equal(t, int32(guesses)-verifies, dummies, "every other guess is refused on a dummy")
	assert.Equal(t, models.DefaultPasswordLockout.Threshold, store.credential.FailedAttempts,
		"a refused attempt is not counted")
}

// serialCredentialStore is a one-row credential store whose BeginAttempt
// admits attempts one at a time and applies the schedule, as the row lock and
// the UPDATE's WHERE clause do in PostgreSQL.
type serialCredentialStore struct {
	repositories.UserCredentialRepository
	mu         sync.Mutex
	credential models.UserCredential
}

func (s *serialCredentialStore) BeginAttempt(
	_ context.Context, _ string, now time.Time, lockout models.PasswordLockout,
) (*models.UserCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.credential.IsDisabled() {
		return nil, repositories.ErrUserCredentialDisabled
	}
	if s.credential.IsLockedAt(now) {
		return nil, repositories.ErrUserCredentialLocked
	}
	s.credential.FailedAttempts++
	if lock := lockout.LockFor(s.credential.FailedAttempts); lock > 0 {
		until := now.Add(lock)
		s.credential.LockedUntil = &until
	}
	c := s.credential
	return &c, nil
}

func TestPasswordAuthService_Login_Disabled(t *testing.T) {
	t.Run("password login switched off", func(t *testing.T) {
		f := newPwFixture(t, withPolicy(fixedAuthPolicy{policy: models.DefaultInstanceAuthPolicy()}))

		_, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

		require.ErrorIs(t, err, ErrPasswordLoginDisabled)
		verifies, dummies := f.computations()
		assert.Zero(t, verifies+dummies, "a disabled endpoint does no work")
	})

	t.Run("an unreadable policy is not the same as switched off", func(t *testing.T) {
		boom := errors.New("connection refused")
		f := newPwFixture(t, withPolicy(fixedAuthPolicy{err: boom}))

		_, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, ErrPasswordLoginDisabled)
		assert.NotErrorIs(t, err, ErrInvalidCredentials)
	})
}

// An active allowlist is the hard boundary for password sign-in too: an
// account whose address is not on it is refused even with the right password,
// before the account is looked up.
func TestPasswordAuthService_Login_OffAllowlistAccountIsRefused(t *testing.T) {
	f := newPwFixture(t, withAllowlist("someone-else@example.com"))

	_, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

	require.ErrorIs(t, err, ErrAccessRestricted)
	f.users.AssertNotCalled(t, "GetByEmail", mock.Anything, mock.Anything)
	verifies, dummies := f.computations()
	assert.Zero(t, verifies+dummies)
}

// What is waived for password sign-in is only provider verification of the
// address: an address that IS on the allowlist signs in although no identity
// provider ever verified it (EmailVerifiedAt nil).
func TestPasswordAuthService_Login_OnAllowlistNeedsNoProviderVerification(t *testing.T) {
	f := newPwFixture(t, withAllowlist(pwTestEmail))
	user := f.activeUser()
	require.Nil(t, user.EmailVerifiedAt)
	f.expectAccount(user, f.credential())
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(nil)

	got, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

	require.NoError(t, err)
	assert.Equal(t, pwTestUserID, got.ID)
}

// A suspended account is told so only once its password verified, the refusal
// names the account so it can be recorded against it, and the success is not
// recorded.
func TestPasswordAuthService_Login_Suspended(t *testing.T) {
	suspended := func(f *pwFixture) {
		user := f.activeUser()
		user.Status = models.UserStatusSuspended
		f.expectAccount(user, f.credential())
	}

	t.Run("right password", func(t *testing.T) {
		f := newPwFixture(t)
		suspended(f)

		_, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

		require.ErrorIs(t, err, ErrAccountSuspended)
		var refusal *AccountSuspendedError
		require.ErrorAs(t, err, &refusal)
		assert.Equal(t, pwTestUserID, refusal.UserID)
		assert.NotErrorIs(t, err, ErrInvalidCredentials)
		f.credentials.AssertNotCalled(t, "RecordSuccess", mock.Anything, mock.Anything)
	})

	t.Run("wrong password learns nothing about the suspension", func(t *testing.T) {
		f := newPwFixture(t)
		suspended(f)

		_, err := f.svc.Login(context.Background(), pwTestEmail, "not the password at all")

		requireLoginRefusal(t, err, SignInRefusalMismatch, pwTestUserID)
		assert.NotErrorIs(t, err, ErrAccountSuspended)
	})
}

// oldParamsHash is a hash of the test password made under weaker parameters
// than the fixture's hasher uses.
func oldParamsHash(t *testing.T) string {
	t.Helper()
	old, err := password.NewHasher(password.Params{Memory: 32, Time: 1, Threads: 1, SaltLength: 16, KeyLength: 32}).
		Hash(context.Background(), pwTestPassword)
	require.NoError(t, err)
	return old
}

// A hash made under weaker parameters still signs in and is replaced by one
// made under the current parameters, as a compare-and-swap on the hash that
// was just verified.
func TestPasswordAuthService_Login_RehashesAnOutdatedHash(t *testing.T) {
	f := newPwFixture(t)
	old := oldParamsHash(t)
	f.expectAccount(f.activeUser(), &models.UserCredential{UserID: pwTestUserID, PasswordHash: old})
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(nil)
	var stored string
	f.credentials.On("UpdateHash", mock.Anything, pwTestUserID, old, mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) { stored = args.String(3) }).Return(true, nil)

	_, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(stored, "$argon2id$v=19$m=64,t=1,p=1$"), "stored under the current parameters")
	ok, needsRehash, err := f.hasher.Hasher.Verify(context.Background(), stored, pwTestPassword)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, needsRehash)
}

// A password changed while the sign-in was in flight is not overwritten: the
// swap finds a different hash stored, writes nothing, and the sign-in (whose
// password was correct when it was checked) still succeeds.
func TestPasswordAuthService_Login_RehashLosesToAConcurrentPasswordChange(t *testing.T) {
	f := newPwFixture(t)
	old := oldParamsHash(t)
	f.expectAccount(f.activeUser(), &models.UserCredential{UserID: pwTestUserID, PasswordHash: old})
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(nil)
	f.credentials.On("UpdateHash", mock.Anything, pwTestUserID, old, mock.Anything).Return(false, nil).Once()

	user, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

	require.NoError(t, err)
	assert.Equal(t, pwTestUserID, user.ID)
	assert.Equal(t, "Skipped a password hash upgrade: the password was changed meanwhile", f.logs.LastEntry().Message)
}

// Neither a failed throttle reset nor a failed rehash turns a correct password
// into a refusal.
func TestPasswordAuthService_Login_BookkeepingFailuresDoNotFailTheSignIn(t *testing.T) {
	f := newPwFixture(t)
	old := oldParamsHash(t)
	f.expectAccount(f.activeUser(), &models.UserCredential{UserID: pwTestUserID, PasswordHash: old})
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(errors.New("connection refused"))
	f.credentials.On("UpdateHash", mock.Anything, pwTestUserID, old, mock.Anything).
		Return(false, errors.New("connection refused"))

	user, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

	require.NoError(t, err)
	assert.Equal(t, pwTestUserID, user.ID)
}

func TestPasswordAuthService_Login_InfrastructureFailuresAreNotRefusals(t *testing.T) {
	boom := errors.New("connection refused")

	t.Run("account lookup", func(t *testing.T) {
		f := newPwFixture(t)
		f.users.On("GetByEmail", mock.Anything, pwTestEmail).Return(nil, boom)

		_, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("admitting the attempt", func(t *testing.T) {
		f := newPwFixture(t)
		f.users.On("GetByEmail", mock.Anything, pwTestEmail).Return(f.activeUser(), nil)
		f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, mock.Anything, mock.Anything).Return(nil, boom)

		_, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, ErrInvalidCredentials)
		verifies, dummies := f.computations()
		assert.Zero(t, verifies+dummies, "an attempt that could not be counted is never checked")
	})

	t.Run("a request abandoned before it could be checked", func(t *testing.T) {
		f := newPwFixture(t)
		f.expectAccount(f.activeUser(), f.credential())
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		// Every hashing slot busy, as under a flood.
		blocked := &blockedHasher{}
		f.svc.hasher = blocked

		_, err := f.svc.Login(cancelled, pwTestEmail, pwTestPassword)

		require.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, ErrInvalidCredentials, "an abandoned request is not told its password was wrong")
	})

	t.Run("an unavailable credential of an unnamed kind is refused as locked", func(t *testing.T) {
		f := newPwFixture(t)
		f.expectUnavailable(f.activeUser(), repositories.ErrUserCredentialUnavailable)

		_, err := f.svc.Login(context.Background(), pwTestEmail, pwTestPassword)

		requireLoginRefusal(t, err, SignInRefusalLocked, pwTestUserID)
	})
}

// blockedHasher is a hasher whose every slot is taken: each call returns the
// context's error, as *password.Hasher does when ctx ends while waiting.
type blockedHasher struct{}

func (blockedHasher) Hash(ctx context.Context, _ string) (string, error) { return "", ctx.Err() }
func (blockedHasher) Verify(ctx context.Context, _, _ string) (bool, bool, error) {
	return false, false, ctx.Err()
}
func (blockedHasher) DummyVerify(ctx context.Context, _ string) error { return ctx.Err() }

func TestPasswordAuthService_SetPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("stores a hash of a password that meets the policy", func(t *testing.T) {
		f := newPwFixture(t)
		f.users.On("GetByID", ctx, pwTestUserID).Return(f.activeUser(), nil)
		var stored string
		f.credentials.On("Upsert", ctx, pwTestUserID, mock.AnythingOfType("string")).
			Run(func(args mock.Arguments) { stored = args.String(2) }).Return(nil)

		require.NoError(t, f.svc.SetPassword(ctx, pwTestUserID, pwTestPassword))

		assert.NotContains(t, stored, pwTestPassword)
		ok, _, err := f.hasher.Hasher.Verify(context.Background(), stored, pwTestPassword)
		require.NoError(t, err)
		assert.True(t, ok, "what was stored verifies the password that was set")
	})

	t.Run("a password that fails the policy is rejected and nothing is stored", func(t *testing.T) {
		for name, candidate := range map[string]string{
			"too short":         "short",
			"common":            "123456789012",
			"the account email": "ADA@example.com",
		} {
			t.Run(name, func(t *testing.T) {
				f := newPwFixture(t)
				f.users.On("GetByID", ctx, pwTestUserID).Return(f.activeUser(), nil)

				err := f.svc.SetPassword(ctx, pwTestUserID, candidate)

				var policyErr *password.PolicyError
				require.ErrorAs(t, err, &policyErr)
				assert.NotEmpty(t, policyErr.Violations)
				f.credentials.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything)
				assert.Zero(t, f.hasher.hashes.Load(), "a rejected password is never hashed")
			})
		}
	})

	t.Run("an unknown user stays recognisable", func(t *testing.T) {
		f := newPwFixture(t)
		f.users.On("GetByID", ctx, "ghost").Return(nil, repositories.ErrUserNotFound)

		require.ErrorIs(t, f.svc.SetPassword(ctx, "ghost", pwTestPassword), repositories.ErrUserNotFound)
	})

	t.Run("a storage failure is reported", func(t *testing.T) {
		f := newPwFixture(t)
		boom := errors.New("connection refused")
		f.users.On("GetByID", ctx, pwTestUserID).Return(f.activeUser(), nil)
		f.credentials.On("Upsert", ctx, pwTestUserID, mock.Anything).Return(boom)

		require.ErrorIs(t, f.svc.SetPassword(ctx, pwTestUserID, pwTestPassword), boom)
	})
}

func TestPasswordAuthService_RemoveAndHasPassword(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("connection refused")

	t.Run("remove reports whether there was one", func(t *testing.T) {
		f := newPwFixture(t)
		f.credentials.On("Delete", ctx, pwTestUserID).Return(true, nil).Once()
		f.credentials.On("Delete", ctx, pwTestUserID).Return(false, nil).Once()
		f.credentials.On("Delete", ctx, pwTestUserID).Return(false, boom).Once()

		removed, err := f.svc.RemovePassword(ctx, pwTestUserID)
		require.NoError(t, err)
		assert.True(t, removed)
		removed, err = f.svc.RemovePassword(ctx, pwTestUserID)
		require.NoError(t, err)
		assert.False(t, removed)
		_, err = f.svc.RemovePassword(ctx, pwTestUserID)
		require.ErrorIs(t, err, boom)
	})

	t.Run("has", func(t *testing.T) {
		f := newPwFixture(t)
		f.credentials.On("Get", ctx, pwTestUserID).Return(f.credential(), nil).Once()
		f.credentials.On("Get", ctx, pwTestUserID).Return(nil, repositories.ErrUserCredentialNotFound).Once()
		f.credentials.On("Get", ctx, pwTestUserID).Return(nil, boom).Once()

		has, err := f.svc.HasPassword(ctx, pwTestUserID)
		require.NoError(t, err)
		assert.True(t, has)
		has, err = f.svc.HasPassword(ctx, pwTestUserID)
		require.NoError(t, err)
		assert.False(t, has)
		_, err = f.svc.HasPassword(ctx, pwTestUserID)
		require.ErrorIs(t, err, boom)
	})
}

// TestPasswordAuthService_NothingSecretIsLoggedOrReturned drives every path
// that handles a password and scans what the service logged and returned for
// the password and for the stored hash.
func TestPasswordAuthService_NothingSecretIsLoggedOrReturned(t *testing.T) {
	const wrong = "a wrong but long password"
	ctx := context.Background()
	f := newPwFixture(t)
	counted := f.credential()
	until := f.now.Add(time.Minute)
	counted.FailedAttempts, counted.LockedUntil = 5, &until
	f.expectAccount(f.activeUser(), counted)
	f.users.On("GetByID", ctx, pwTestUserID).Return(f.activeUser(), nil)
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(errors.New("reset failed"))
	f.credentials.On("Upsert", ctx, pwTestUserID, mock.Anything).Return(errors.New("store failed")).Once()
	f.credentials.On("Upsert", ctx, pwTestUserID, mock.Anything).Return(nil)

	var returned []string
	collect := func(err error) {
		if err != nil {
			returned = append(returned, err.Error(), fmt.Sprintf("%+v", err))
		}
	}
	_, err := f.svc.Login(ctx, pwTestEmail, pwTestPassword)
	collect(err)
	_, err = f.svc.Login(ctx, pwTestEmail, wrong)
	collect(err)
	collect(f.svc.SetPassword(ctx, pwTestUserID, pwTestPassword)) // storage failure
	collect(f.svc.SetPassword(ctx, pwTestUserID, pwTestPassword)) // success
	collect(f.svc.SetPassword(ctx, pwTestUserID, "123456789012")) // policy failure

	secrets := []string{pwTestPassword, wrong, "123456789012", f.hash}
	require.NotEmpty(t, f.logs.AllEntries(), "the paths above log")
	for _, entry := range f.logs.AllEntries() {
		line := entry.Message + " " + fmt.Sprintf("%v", entry.Data)
		for _, secret := range secrets {
			assert.NotContains(t, line, secret, "log entry %q", entry.Message)
		}
	}
	require.NotEmpty(t, returned)
	for _, text := range returned {
		for _, secret := range secrets {
			assert.NotContains(t, text, secret)
		}
	}
}

func TestPasswordLoginError(t *testing.T) {
	err := error(&PasswordLoginError{Reason: SignInRefusalLocked, UserID: "u"})

	assert.ErrorIs(t, err, ErrInvalidCredentials)
	assert.NotErrorIs(t, err, ErrAccountSuspended)
	assert.Equal(t, "invalid email or password: locked", err.Error())

	suspended := error(&AccountSuspendedError{UserID: "u"})
	assert.ErrorIs(t, suspended, ErrAccountSuspended)
	assert.NotErrorIs(t, suspended, ErrInvalidCredentials)
	assert.Equal(t, "account is suspended", suspended.Error())
}

func TestNewPasswordAuthService_Defaults(t *testing.T) {
	svc := NewPasswordAuthService(PasswordAuthDeps{})

	assert.Equal(t, models.DefaultPasswordLockout, svc.lockout)
	assert.WithinDuration(t, time.Now(), svc.now(), time.Minute)
}

// recordingAPIAccess is an APIAccessServiceInterface that records, in order,
// what it was asked to revoke.
type recordingAPIAccess struct {
	mu      sync.Mutex
	revoked []string
	events  *[]string
	err     error
}

func (a *recordingAPIAccess) RevokeAPIAccess(_ context.Context, userID string) (*models.APIAccessRevocation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return nil, a.err
	}
	a.revoked = append(a.revoked, userID)
	if a.events != nil {
		*a.events = append(*a.events, "API access revoked")
	}
	return &models.APIAccessRevocation{}, nil
}

func TestUnverifiedAccountCleanup(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("connection refused")
	type fixture struct {
		cleanup     *UnverifiedAccountCleanup
		credentials *repo_mocks.MockUserCredentialRepository
		apiAccess   *recordingAPIAccess
		logs        *logtest.Recorder
	}
	newFixture := func(t *testing.T) fixture {
		logger, logs := logtest.New()
		credentials := repo_mocks.NewMockUserCredentialRepository(t)
		apiAccess := &recordingAPIAccess{}
		return fixture{NewUnverifiedAccountCleanup(credentials, apiAccess, logger), credentials, apiAccess, logs}
	}

	t.Run("removes the password, revokes API access, and says so", func(t *testing.T) {
		f := newFixture(t)
		f.credentials.On("Delete", ctx, pwTestUserID).Return(true, nil).Once()

		require.NoError(t, f.cleanup.OnUnverifiedAccountClaimed(ctx, pwTestUserID))

		assert.Equal(t, []string{pwTestUserID}, f.apiAccess.revoked)
		require.NotNil(t, f.logs.LastEntry())
		assert.Equal(t, pwTestUserID, f.logs.LastEntry().Data["user_id"])
	})

	t.Run("is idempotent: an account with no password still has its API access revoked, each time", func(t *testing.T) {
		f := newFixture(t)
		f.credentials.On("Delete", ctx, pwTestUserID).Return(false, nil).Twice()

		require.NoError(t, f.cleanup.OnUnverifiedAccountClaimed(ctx, pwTestUserID))
		require.NoError(t, f.cleanup.OnUnverifiedAccountClaimed(ctx, pwTestUserID))

		assert.Equal(t, []string{pwTestUserID, pwTestUserID}, f.apiAccess.revoked)
		assert.Empty(t, f.logs.AllEntries())
	})

	t.Run("a password that cannot be removed stops the claim before API access is touched", func(t *testing.T) {
		f := newFixture(t)
		f.credentials.On("Delete", ctx, pwTestUserID).Return(false, boom).Once()

		require.ErrorIs(t, f.cleanup.OnUnverifiedAccountClaimed(ctx, pwTestUserID), boom)
		assert.Empty(t, f.apiAccess.revoked)
	})

	t.Run("API access that cannot be revoked stops the claim", func(t *testing.T) {
		f := newFixture(t)
		f.apiAccess.err = boom
		f.credentials.On("Delete", ctx, pwTestUserID).Return(true, nil).Once()

		require.ErrorIs(t, f.cleanup.OnUnverifiedAccountClaimed(ctx, pwTestUserID), boom)
	})
}

// oneCredentialStore is a credential store holding at most one user's password,
// enough to watch it being removed.
type oneCredentialStore struct {
	repositories.UserCredentialRepository
	mu     sync.Mutex
	userID string
	events *[]string
}

func (s *oneCredentialStore) Delete(_ context.Context, userID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userID != userID {
		return false, nil
	}
	s.userID = ""
	*s.events = append(*s.events, "password removed")
	return true, nil
}

func (s *oneCredentialStore) has(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.userID != "" && s.userID == userID
}

// TestAuthService_VerifiedSignInClaimsAPasswordAccount is the pre-hijack
// defence end to end at the service: an account that was given a password
// through a bearer link (its email never verified, no sign-in identity) is
// claimed by a provider sign-in that PROVES ownership of the address. The
// password set by whoever held the link is removed and the account's API keys
// and grants are revoked, in that order and before the account is linked, so
// the previous holder cannot enter it again by any route.
func TestAuthService_VerifiedSignInClaimsAPasswordAccount(t *testing.T) {
	ctx := context.Background()
	logger, _ := logtest.New()
	var events []string
	account := &models.User{ID: "invited-1", Email: "ada@example.com", Status: models.UserStatusActive}
	credentials := &oneCredentialStore{userID: account.ID, events: &events}
	apiAccess := &recordingAPIAccess{events: &events}

	users := repo_mocks.NewMockUserRepository(t)
	identities := repo_mocks.NewMockUserIdentityRepository(t)
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-owner").Return(nil, repositories.ErrUserNotFound)
	users.On("GetByEmail", ctx, "ada@example.com").Return(account, nil)
	identities.On("ListByUserID", ctx, account.ID).Return([]models.UserIdentity{}, nil)
	users.On("BumpSessionEpoch", ctx, account.ID).
		Run(func(mock.Arguments) { events = append(events, "sessions ended") }).Return(int64(1), nil)
	users.On("Update", ctx, mock.Anything).
		Run(func(mock.Arguments) { events = append(events, "account verified") }).Return(nil)
	identities.On("Upsert", ctx, account.ID, "corp-sso", "sub-owner").
		Run(func(mock.Arguments) { events = append(events, "identity linked") }).Return(nil)

	svc := NewAuthService(users, identities, staticIDPResolver{reg: idp.NewRegistry()}, nil, logger,
		newStaticAllowlistResolver(logger, nil),
		WithUnverifiedAccountClaimHook(NewUnverifiedAccountCleanup(credentials, apiAccess, logger)))

	user, isNew, err := svc.createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC,
		&idp.Claims{Subject: "sub-owner", Email: "ada@example.com", Name: "Ada", EmailVerified: true})

	require.NoError(t, err)
	assert.False(t, isNew)
	assert.Equal(t, account.ID, user.ID)
	assert.False(t, credentials.has(account.ID), "the claimed account's password is gone")
	require.NotEmpty(t, events)
	assert.Equal(t, []string{account.ID}, apiAccess.revoked, "the claimed account's API keys and grants are revoked")
	require.GreaterOrEqual(t, len(events), 4, "%v", events)
	assert.Equal(t, []string{"password removed", "API access revoked", "sessions ended"}, events[:3],
		"everything the link's holder could use is gone before anything is linked: %v", events)
	assert.Contains(t, events[3:], "identity linked")
}

// An account whose email is already verified is linked without being claimed:
// its password is its owner's, and stays.
func TestAuthService_VerifiedSignInToAVerifiedAccountKeepsItsPassword(t *testing.T) {
	ctx := context.Background()
	logger, _ := logtest.New()
	var events []string
	verifiedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	account := &models.User{
		ID: "owner-1", Email: "ada@example.com", Status: models.UserStatusActive, EmailVerifiedAt: &verifiedAt,
	}
	credentials := &oneCredentialStore{userID: account.ID, events: &events}
	apiAccess := &recordingAPIAccess{events: &events}

	users := repo_mocks.NewMockUserRepository(t)
	identities := repo_mocks.NewMockUserIdentityRepository(t)
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-owner").Return(nil, repositories.ErrUserNotFound)
	users.On("GetByEmail", ctx, "ada@example.com").Return(account, nil)
	users.On("Update", ctx, mock.Anything).Return(nil).Maybe()
	identities.On("Upsert", ctx, account.ID, "corp-sso", "sub-owner").Return(nil)

	svc := NewAuthService(users, identities, staticIDPResolver{reg: idp.NewRegistry()}, nil, logger,
		newStaticAllowlistResolver(logger, nil),
		WithUnverifiedAccountClaimHook(NewUnverifiedAccountCleanup(credentials, apiAccess, logger)))

	_, _, err := svc.createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC,
		&idp.Claims{Subject: "sub-owner", Email: "ada@example.com", Name: "Ada", EmailVerified: true})

	require.NoError(t, err)
	assert.True(t, credentials.has(account.ID), "a verified account keeps its password")
	assert.Empty(t, apiAccess.revoked, "and its API keys")
	assert.Empty(t, events)
}
