package services

import (
	"context"
	"fmt"
	"log/slog"
)

// ensureEmailAllowed denies sign-in when email is not permitted by the access
// allowlist, returning ErrAccessRestricted. It is the one implementation of
// that rule for every sign-in method: AuthService.ensureAccessAllowed (provider
// and dev sign-in) and PasswordAuthService.Login both call it.
//
// With no active allowlist the instance is open and every email is allowed.
// The decision comes from the AccessAllowlistResolver, which reads the
// allowlist stored in the database (#1235) and exempts root instance admins;
// provider is included in the audit log for operator traceability.
//
// emailVerified reports whether the address counts as verified for this
// sign-in. An active allowlist rejects an unverified one (#218), root admins
// included: the allowlist and the root-admin exemption both grant access by
// address, so an address the user merely claimed must satisfy neither.
//
// An allowlist that is active but cannot be read fails closed: the resolver's
// error is returned and nobody but a root admin signs in.
func ensureEmailAllowed(
	ctx context.Context, allowlist AccessAllowlistResolver, logger *slog.Logger,
	email, provider string, emailVerified bool,
) error {
	allowed, active, err := allowlist.IsEmailAllowed(ctx, email)
	if err != nil {
		logger.With("email", email, "provider", provider, "error", err.Error()).
			Error("Sign-in refused: the access allowlist could not be evaluated")
		return fmt.Errorf("evaluate access allowlist: %w", err)
	}
	if !active || (allowed && emailVerified) {
		return nil
	}
	// Reaching here means the allowlist is ACTIVE and refused this identity.
	// Report which rule refused it: from the user's side both denials are
	// identical (?error=access_restricted), so only this log tells the operator
	// why.
	reason := "not_on_allowlist"
	if !emailVerified {
		reason = "unverified_email"
	}
	logger.With(
		"email", email,
		"provider", provider,
		"reason", reason,
	).Info("Sign-in denied by access allowlist")
	return ErrAccessRestricted
}
