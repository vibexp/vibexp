package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/vibexp/vibexp/internal/repositories"
)

// A working way in (#1438, epic #1426).
//
// Setup mode and the lockout guard both ask one question: can an administrator
// still sign in and repair the authentication settings? Before password login
// the answer was "an identity provider is enabled". It is now that, OR password
// login is switched on and at least one instance admin has a usable password.
// Both callers ask AuthWayIn, so they cannot disagree.

// AuthWayIn answers whether the instance has a sign-in method an administrator
// can use.
type AuthWayIn interface {
	// PasswordWorks reports whether password login is a working way in:
	// switched on, and at least one instance admin (root by configuration, or
	// granted) who is not suspended has a password that is not disabled.
	PasswordWorks(ctx context.Context) (bool, error)
	// Any reports whether any way in works: an identity provider is enabled,
	// or PasswordWorks.
	Any(ctx context.Context) (bool, error)
}

// AuthWayInDeps are the collaborators of AuthWayIn. Everything is read from
// the stored rows on each call, not from the cached resolvers: the resolver
// snapshot leaves out an enabled provider that failed to build, and a cached
// policy would let setup mode lag behind the change that ends it.
type AuthWayInDeps struct {
	Providers   repositories.InstanceAuthProviderRepository
	Policies    repositories.InstanceAuthPolicyRepository
	Credentials repositories.UserCredentialRepository
	// RootAdminEmails returns the auth.instance_admins addresses, lower case
	// (InstanceAdminResolver.RootAdminEmails).
	RootAdminEmails func() []string
}

type authWayIn struct {
	deps AuthWayInDeps
}

// NewAuthWayIn creates the working-way-in check.
func NewAuthWayIn(deps AuthWayInDeps) AuthWayIn {
	return &authWayIn{deps: deps}
}

// PasswordWorks implements AuthWayIn.
func (w *authWayIn) PasswordWorks(ctx context.Context) (bool, error) {
	policy, err := w.deps.Policies.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceAuthPolicyNotFound) {
		return false, nil // no stored policy: password login is off by default
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the authentication policy: %w", err)
	}
	if !policy.PasswordLoginEnabled {
		return false, nil
	}
	admins, err := w.deps.Credentials.CountUsableForAdmins(ctx, w.deps.RootAdminEmails())
	if err != nil {
		return false, err
	}
	return admins > 0, nil
}

// Any implements AuthWayIn.
func (w *authWayIn) Any(ctx context.Context) (bool, error) {
	enabled, err := hasEnabledAuthProvider(ctx, w.deps.Providers)
	if err != nil || enabled {
		return enabled, err
	}
	return w.PasswordWorks(ctx)
}

// hasEnabledAuthProvider reads the provider ROWS rather than the resolver's
// built snapshot: the snapshot leaves out an enabled provider that failed to
// build, so judging by it would open setup mode whenever an identity provider
// had an outage.
func hasEnabledAuthProvider(ctx context.Context, providers repositories.InstanceAuthProviderRepository) (bool, error) {
	rows, err := providers.List(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list identity providers: %w", err)
	}
	for _, row := range rows {
		if row.Enabled {
			return true, nil
		}
	}
	return false, nil
}
