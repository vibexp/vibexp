package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Instance authentication policy at runtime (#1429, epic #1426).
//
// The policy (the password login switch and the registration mode) is stored in
// instance_auth_policy and changed in the admin UI, so it is resolved at
// runtime and a change takes effect without a restart.

const (
	// authPolicyVersionProbeInterval is how long a resolved policy is served
	// from memory before the shared auth settings version is read again. It
	// bounds how long a replica keeps acting on a policy that was just
	// changed.
	authPolicyVersionProbeInterval = 2 * time.Second
	// authPolicyRefreshTimeout bounds one refresh (the version read plus, when
	// the version changed, the policy read).
	authPolicyRefreshTimeout = 5 * time.Second
)

// Password reset channels reported by AuthPolicyResolver.PasswordResetChannel.
const (
	// PasswordResetByEmail means a reset link can be emailed: the instance has
	// an email provider configured.
	PasswordResetByEmail = "email"
	// PasswordResetByAdmin means the user must ask an administrator for a
	// reset link: the instance has no email provider.
	PasswordResetByAdmin = "admin"
)

// AuthPolicyResolver serves the instance authentication policy to the sign-in
// page and the authentication services.
//
// # Caching
//
// The policy is cached by the shared auth settings version, which every policy
// write bumps in its own transaction. The version is read at most once per
// authPolicyVersionProbeInterval; in between, the policy is served from memory
// with no database read. An unchanged version reuses the cached policy without
// reading the policy table.
//
// # When the policy cannot be read
//
// Policy returns the error. It never falls back to the defaults or to the last
// policy it read: the default registration mode is the permissive one, and a
// stale policy may be more permissive than the stored one, so either would let
// a database fault open up an invite-only instance. The caller decides what a
// failure means for its own operation.
type AuthPolicyResolver interface {
	// Policy returns the instance authentication policy:
	// models.DefaultInstanceAuthPolicy when none is stored. A non-nil error
	// means the policy could not be determined.
	Policy(ctx context.Context) (models.InstanceAuthPolicy, error)
	// PasswordResetChannel reports how a forgotten password is reset on this
	// instance: PasswordResetByEmail when an instance email provider is stored,
	// PasswordResetByAdmin otherwise. It reads the stored configuration only and
	// never sends or attempts mail, because an instance with no provider
	// "sends" successfully through a stub that discards the message.
	PasswordResetChannel(ctx context.Context) (string, error)
}

// AuthPolicyResolverDeps are the collaborators of the resolver.
type AuthPolicyResolverDeps struct {
	Policies repositories.InstanceAuthPolicyRepository
	Versions repositories.InstanceAuthSettingsVersionRepository
	// EmailProviders answers whether an instance email provider is stored.
	EmailProviders repositories.InstanceEmailProviderRepository
	Logger         *slog.Logger
	// ProbeInterval overrides how long a resolved policy is served before the
	// auth settings version is read again. Zero means
	// authPolicyVersionProbeInterval; production wiring leaves it zero.
	ProbeInterval time.Duration
}

// resolvedAuthPolicy is one immutable policy and the auth settings version it
// was read at.
type resolvedAuthPolicy struct {
	// gen is the invalidation generation the refresh started under.
	gen     uint64
	version int64
	policy  models.InstanceAuthPolicy
	// probedAt is when the version was last confirmed.
	probedAt time.Time
}

// authPolicyResolver implements AuthPolicyResolver.
type authPolicyResolver struct {
	policies       repositories.InstanceAuthPolicyRepository
	versions       repositories.InstanceAuthSettingsVersionRepository
	emailProviders repositories.InstanceEmailProviderRepository
	logger         *slog.Logger

	// Injectable for tests.
	now           func() time.Time
	probeInterval time.Duration

	snap  atomic.Pointer[resolvedAuthPolicy]
	group singleflight.Group
	settingsGeneration
}

var (
	_ AuthPolicyResolver = (*authPolicyResolver)(nil)
	_ AuthSettingsCache  = (*authPolicyResolver)(nil)
)

// NewAuthPolicyResolver creates the runtime authentication policy resolver.
func NewAuthPolicyResolver(deps AuthPolicyResolverDeps) AuthPolicyResolver {
	return newAuthPolicyResolver(deps)
}

func newAuthPolicyResolver(deps AuthPolicyResolverDeps) *authPolicyResolver {
	probeInterval := deps.ProbeInterval
	if probeInterval == 0 {
		probeInterval = authPolicyVersionProbeInterval
	}
	return &authPolicyResolver{
		policies:       deps.Policies,
		versions:       deps.Versions,
		emailProviders: deps.EmailProviders,
		logger:         deps.Logger,
		now:            time.Now,
		probeInterval:  probeInterval,
	}
}

// Policy implements AuthPolicyResolver. It serves the cached policy while its
// version was confirmed within probeInterval and refreshes it otherwise.
func (r *authPolicyResolver) Policy(ctx context.Context) (models.InstanceAuthPolicy, error) {
	if s := r.snap.Load(); r.fresh(s) {
		return s.policy, nil
	}
	// One refresh at a time, shared by every caller that arrives meanwhile. It
	// runs detached from the caller's cancellation, so one aborted request
	// cannot fail the refresh for everyone waiting on it.
	gen := r.generation()
	ch := r.group.DoChan(refreshKey(gen), func() (any, error) {
		if s := r.snap.Load(); r.fresh(s) {
			return s, nil
		}
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authPolicyRefreshTimeout)
		defer cancel()
		return r.refresh(refreshCtx, gen)
	})
	select {
	case <-ctx.Done():
		return models.InstanceAuthPolicy{}, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return models.InstanceAuthPolicy{}, res.Err
		}
		return res.Val.(*resolvedAuthPolicy).policy, nil
	}
}

// fresh reports whether s can be served without probing the version: read
// under the current generation and confirmed within probeInterval.
func (r *authPolicyResolver) fresh(s *resolvedAuthPolicy) bool {
	return s != nil && s.gen == r.generation() && r.now().Before(s.probedAt.Add(r.probeInterval))
}

// refresh reads the auth settings version and re-reads the policy only when it
// changed. An unchanged version re-stamps the cached policy and reads nothing
// else.
func (r *authPolicyResolver) refresh(ctx context.Context, gen uint64) (*resolvedAuthPolicy, error) {
	version, err := r.versions.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("read auth settings version: %w", err)
	}
	if s := r.snap.Load(); s != nil && s.version == version {
		confirmed := *s
		confirmed.gen = gen
		confirmed.probedAt = r.now()
		r.snap.Store(&confirmed)
		return &confirmed, nil
	}

	policy := models.DefaultInstanceAuthPolicy()
	stored, err := r.policies.Get(ctx)
	switch {
	case errors.Is(err, repositories.ErrInstanceAuthPolicyNotFound):
		// None stored: the defaults apply.
	case err != nil:
		return nil, fmt.Errorf("read instance auth policy: %w", err)
	default:
		policy = *stored
	}
	resolved := &resolvedAuthPolicy{gen: gen, version: version, policy: policy, probedAt: r.now()}
	r.snap.Store(resolved)
	r.logger.With(
		"version", version,
		"password_login_enabled", policy.PasswordLoginEnabled,
		"registration_mode", policy.RegistrationMode,
	).Info("Resolved the instance authentication policy")
	return resolved, nil
}

// PasswordResetChannel implements AuthPolicyResolver. It is not cached: an
// email provider write does not bump the auth settings version, and the read is
// one singleton row on a page that is opened by people signing in, not by every
// request.
func (r *authPolicyResolver) PasswordResetChannel(ctx context.Context) (string, error) {
	_, err := r.emailProviders.Get(ctx)
	switch {
	case errors.Is(err, repositories.ErrInstanceEmailProviderNotFound):
		return PasswordResetByAdmin, nil
	case err != nil:
		return "", fmt.Errorf("read instance email provider: %w", err)
	default:
		return PasswordResetByEmail, nil
	}
}
