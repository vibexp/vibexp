package services

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// fakeAuthPolicyStore is an in-memory instance_auth_policy, the shared auth
// settings version and the instance email provider row. It counts reads, so
// tests can prove what the resolver did and did not touch.
type fakeAuthPolicyStore struct {
	repositories.InstanceAuthPolicyRepository

	mu           sync.Mutex
	row          *models.InstanceAuthPolicy
	version      int64
	versionErr   error
	policyErr    error
	versionReads int
	rowReads     int

	emailProvider bool
	emailErr      error
	emailReads    int
}

// set stores a policy and bumps the version, as every audited write does.
func (f *fakeAuthPolicyStore) set(passwordLogin bool, mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version++
	f.row = &models.InstanceAuthPolicy{PasswordLoginEnabled: passwordLogin, RegistrationMode: mode, Version: 1}
}

func (f *fakeAuthPolicyStore) fail(versionErr, policyErr error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versionErr, f.policyErr = versionErr, policyErr
}

func (f *fakeAuthPolicyStore) reads() (versionReads, rowReads int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.versionReads, f.rowReads
}

// policyRepo, versionRepo and emailRepo expose the store as the three
// repositories the resolver reads. Each has one Get, so they cannot be methods
// of one type.
type (
	fakeAuthPolicyRepo        struct{ *fakeAuthPolicyStore }
	fakeAuthPolicyVersionRepo struct{ s *fakeAuthPolicyStore }
	fakeAuthPolicyEmailRepo   struct {
		repositories.InstanceEmailProviderRepository
		s *fakeAuthPolicyStore
	}
)

func (r fakeAuthPolicyRepo) Get(context.Context) (*models.InstanceAuthPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rowReads++
	if r.policyErr != nil {
		return nil, r.policyErr
	}
	if r.row == nil {
		return nil, repositories.ErrInstanceAuthPolicyNotFound
	}
	c := *r.row
	return &c, nil
}

func (r fakeAuthPolicyVersionRepo) Get(context.Context) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.versionReads++
	return r.s.version, r.s.versionErr
}

func (r fakeAuthPolicyEmailRepo) Get(context.Context) (*models.InstanceEmailProvider, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.emailReads++
	if r.s.emailErr != nil {
		return nil, r.s.emailErr
	}
	if !r.s.emailProvider {
		return nil, repositories.ErrInstanceEmailProviderNotFound
	}
	return &models.InstanceEmailProvider{}, nil
}

// authPolicyClock is a settable clock.
type authPolicyClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *authPolicyClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *authPolicyClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestAuthPolicyResolver(store *fakeAuthPolicyStore) (*authPolicyResolver, *authPolicyClock) {
	clock := &authPolicyClock{now: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	r := newAuthPolicyResolver(AuthPolicyResolverDeps{
		Policies:       fakeAuthPolicyRepo{store},
		Versions:       fakeAuthPolicyVersionRepo{store},
		EmailProviders: fakeAuthPolicyEmailRepo{s: store},
		Logger:         slog.New(slog.DiscardHandler),
	})
	r.now = clock.Now
	return r, clock
}

func TestAuthPolicyResolver_DefaultsWhenNothingIsStored(t *testing.T) {
	r, _ := newTestAuthPolicyResolver(&fakeAuthPolicyStore{version: 1})

	policy, err := r.Policy(context.Background())

	require.NoError(t, err)
	assert.Equal(t, models.DefaultInstanceAuthPolicy(), policy)
	assert.False(t, policy.PasswordLoginEnabled)
	assert.False(t, policy.InviteOnly())
	assert.False(t, policy.IsStored())
}

func TestAuthPolicyResolver_ReturnsTheStoredPolicy(t *testing.T) {
	store := &fakeAuthPolicyStore{}
	store.set(true, models.RegistrationModeInviteOnly)
	r, _ := newTestAuthPolicyResolver(store)

	policy, err := r.Policy(context.Background())

	require.NoError(t, err)
	assert.True(t, policy.PasswordLoginEnabled)
	assert.True(t, policy.InviteOnly())
	assert.True(t, policy.IsStored())
}

func TestAuthPolicyResolver_Cache(t *testing.T) {
	ctx := context.Background()
	store := &fakeAuthPolicyStore{}
	store.set(false, models.RegistrationModeOpen)
	r, clock := newTestAuthPolicyResolver(store)

	for range 5 {
		_, err := r.Policy(ctx)
		require.NoError(t, err)
	}
	versionReads, rowReads := store.reads()
	assert.Equal(t, 1, versionReads, "within the probe interval the policy is served from memory")
	assert.Equal(t, 1, rowReads)

	// A change is not seen until the version is probed again...
	store.set(true, models.RegistrationModeInviteOnly)
	policy, err := r.Policy(ctx)
	require.NoError(t, err)
	assert.False(t, policy.PasswordLoginEnabled)

	// ...which happens once the probe interval has passed.
	clock.advance(authPolicyVersionProbeInterval)
	policy, err = r.Policy(ctx)
	require.NoError(t, err)
	assert.True(t, policy.PasswordLoginEnabled)
	assert.True(t, policy.InviteOnly())
	versionReads, rowReads = store.reads()
	assert.Equal(t, 2, versionReads)
	assert.Equal(t, 2, rowReads)

	// An unchanged version re-stamps the cache without reading the policy.
	clock.advance(authPolicyVersionProbeInterval)
	_, err = r.Policy(ctx)
	require.NoError(t, err)
	versionReads, rowReads = store.reads()
	assert.Equal(t, 3, versionReads)
	assert.Equal(t, 2, rowReads, "the row is read only when the version changed")
}

func TestAuthPolicyResolver_ConcurrentCallersShareOneRefresh(t *testing.T) {
	store := &fakeAuthPolicyStore{}
	store.set(true, models.RegistrationModeOpen)
	r, _ := newTestAuthPolicyResolver(store)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			policy, err := r.Policy(context.Background())
			assert.NoError(t, err)
			assert.True(t, policy.PasswordLoginEnabled)
		})
	}
	wg.Wait()

	_, rowReads := store.reads()
	assert.Equal(t, 1, rowReads)
}

// A read failure is returned, never papered over with the defaults or with the
// last policy read: either could be more permissive than the stored policy.
func TestAuthPolicyResolver_ReadFailureIsReported(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("connection refused")

	t.Run("version unreadable, nothing cached", func(t *testing.T) {
		store := &fakeAuthPolicyStore{}
		store.fail(boom, nil)
		r, _ := newTestAuthPolicyResolver(store)

		_, err := r.Policy(ctx)

		require.ErrorIs(t, err, boom)
	})

	t.Run("policy unreadable", func(t *testing.T) {
		store := &fakeAuthPolicyStore{version: 1}
		store.fail(nil, boom)
		r, _ := newTestAuthPolicyResolver(store)

		_, err := r.Policy(ctx)

		require.ErrorIs(t, err, boom)
	})

	t.Run("a cached invite-only policy is not replaced by the open default", func(t *testing.T) {
		store := &fakeAuthPolicyStore{}
		store.set(true, models.RegistrationModeInviteOnly)
		r, clock := newTestAuthPolicyResolver(store)
		_, err := r.Policy(ctx)
		require.NoError(t, err)

		store.fail(boom, nil)
		clock.advance(authPolicyVersionProbeInterval)
		policy, err := r.Policy(ctx)

		require.ErrorIs(t, err, boom)
		assert.Equal(t, models.InstanceAuthPolicy{}, policy, "no policy is handed out with an error")

		// The failure is not cached: the next call reads again and recovers.
		store.fail(nil, nil)
		policy, err = r.Policy(ctx)
		require.NoError(t, err)
		assert.True(t, policy.InviteOnly())
	})

	t.Run("a cancelled caller gets its own context error", func(t *testing.T) {
		r, _ := newTestAuthPolicyResolver(&fakeAuthPolicyStore{version: 1})
		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		_, err := r.Policy(cancelled)

		// The refresh is detached from the caller, so either the caller's
		// cancellation or the completed refresh may win the select.
		if err != nil {
			require.ErrorIs(t, err, context.Canceled)
		}
	})
}

func TestAuthPolicyResolver_PasswordResetChannel(t *testing.T) {
	ctx := context.Background()

	t.Run("no email provider stored: an administrator resets", func(t *testing.T) {
		r, _ := newTestAuthPolicyResolver(&fakeAuthPolicyStore{})

		channel, err := r.PasswordResetChannel(ctx)

		require.NoError(t, err)
		assert.Equal(t, PasswordResetByAdmin, channel)
	})

	t.Run("an email provider is stored: reset by email", func(t *testing.T) {
		store := &fakeAuthPolicyStore{emailProvider: true}
		r, _ := newTestAuthPolicyResolver(store)

		channel, err := r.PasswordResetChannel(ctx)

		require.NoError(t, err)
		assert.Equal(t, PasswordResetByEmail, channel)
		// The store embeds a nil repository interface: any call other than Get,
		// such as a send or a health write, would panic. One Get is all it takes.
		assert.Equal(t, 1, store.emailReads)
	})

	t.Run("an unreadable email provider is reported, not guessed", func(t *testing.T) {
		boom := errors.New("connection refused")
		r, _ := newTestAuthPolicyResolver(&fakeAuthPolicyStore{emailErr: boom})

		_, err := r.PasswordResetChannel(ctx)

		require.ErrorIs(t, err, boom)
	})
}

func TestNewAuthPolicyResolver_ProbeIntervalOverride(t *testing.T) {
	store := &fakeAuthPolicyStore{}
	store.set(false, models.RegistrationModeOpen)
	resolver := NewAuthPolicyResolver(AuthPolicyResolverDeps{
		Policies:       fakeAuthPolicyRepo{store},
		Versions:       fakeAuthPolicyVersionRepo{store},
		EmailProviders: fakeAuthPolicyEmailRepo{s: store},
		Logger:         slog.New(slog.DiscardHandler),
		ProbeInterval:  time.Nanosecond,
	})

	_, err := resolver.Policy(context.Background())
	require.NoError(t, err)
	store.set(true, models.RegistrationModeOpen)
	time.Sleep(time.Millisecond)
	policy, err := resolver.Policy(context.Background())

	require.NoError(t, err)
	assert.True(t, policy.PasswordLoginEnabled, "a nanosecond probe interval sees the change at once")
}
