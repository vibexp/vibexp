package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrRegistrationClosed is returned by the provider sign-in entry points when
// the sign-in would create an account, the instance's registration mode is
// invite_only, and the person was not invited. Nothing is written. It is also
// returned in every mode for an unverified claim to a root instance admin's
// address (#1467).
var ErrRegistrationClosed = errors.New("registration is closed to this sign-in")

// RegistrationGate decides whether a sign-in may create a NEW account (#1434).
// It is never asked about an existing account: the registration mode does not
// affect people who already have one, including an account an administrator
// pre-created.
type RegistrationGate interface {
	// AllowNewAccount returns nil when an account may be created for email,
	// ErrRegistrationClosed when the registration mode forbids it, and any
	// other error when that could not be determined (the caller must then
	// refuse: an unreadable policy never opens registration).
	//
	// emailVerified reports whether the sign-in provider verified the address.
	// In every mode, an unverified claim to a root instance admin's address
	// (auth.instance_admins) is refused: the address is reserved for whoever
	// proves the mailbox (#1467). In invite_only mode an invitation and the root-admin exemption both admit
	// by address, so an address the person merely claimed satisfies neither.
	AllowNewAccount(ctx context.Context, email string, emailVerified bool) error
}

// openRegistrationGate admits everyone. It is the AuthService default, for the
// unit tests that are not about registration; the running server always
// installs the policy-backed gate (see providers.ProvideAuthService).
type openRegistrationGate struct{}

func (openRegistrationGate) AllowNewAccount(context.Context, string, bool) error { return nil }

// RegistrationGateDeps are the collaborators of the policy-backed gate.
type RegistrationGateDeps struct {
	Policy      AuthPolicyResolver
	Invitations repositories.TeamInvitationRepository
	RootAdmins  InstanceAdminResolver
	Logger      *slog.Logger
}

type registrationGate struct {
	policy      AuthPolicyResolver
	invitations repositories.TeamInvitationRepository
	rootAdmins  InstanceAdminResolver
	logger      *slog.Logger
}

// NewRegistrationGate returns the gate that enforces the instance's
// registration mode (instance_auth_policy.registration_mode):
//
//   - in every mode, an address in auth.instance_admins needs a
//     provider-verified email (#1467), so nobody claims a root admin's address
//     before its owner signs in;
//   - open: anyone else may create an account (the access allowlist, checked
//     separately, is the only boundary);
//   - invite_only: only someone whose provider-verified email has a pending,
//     unexpired team invitation, or is a root instance admin
//     (auth.instance_admins). The root exemption keeps an instance from locking
//     out its own operator.
//
// The invitation is not consumed: it admits the account, and the person still
// accepts or declines it in the app.
func NewRegistrationGate(deps RegistrationGateDeps) RegistrationGate {
	return &registrationGate{
		policy:      deps.Policy,
		invitations: deps.Invitations,
		rootAdmins:  deps.RootAdmins,
		logger:      deps.Logger,
	}
}

// AllowNewAccount implements RegistrationGate.
func (g *registrationGate) AllowNewAccount(ctx context.Context, email string, emailVerified bool) error {
	policy, err := g.policy.Policy(ctx)
	if err != nil {
		return fmt.Errorf("read the registration mode: %w", err)
	}
	if !emailVerified && g.rootAdmins.IsRootAdmin(email) {
		return g.closed(email, "unverified_root_admin")
	}
	if !policy.InviteOnly() {
		return nil
	}
	if !emailVerified {
		return g.closed(email, "unverified_email")
	}
	if g.rootAdmins.IsRootAdmin(email) {
		return nil
	}
	pending, err := g.invitations.GetPendingByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("look up pending invitations: %w", err)
	}
	if len(pending) == 0 {
		return g.closed(email, "not_invited")
	}
	return nil
}

// closed logs the refusal, which looks the same to the person signing in
// whatever the reason, and returns ErrRegistrationClosed.
func (g *registrationGate) closed(email, reason string) error {
	g.logger.With("email", email, "reason", reason).
		Info("Sign-up refused")
	return ErrRegistrationClosed
}

// WithRegistrationGate replaces the default gate, which admits everyone. A nil
// gate is ignored.
func WithRegistrationGate(gate RegistrationGate) AuthServiceOption {
	return func(as *AuthService) {
		if gate != nil {
			as.registrationGate = gate
		}
	}
}
