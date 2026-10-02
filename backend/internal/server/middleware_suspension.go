package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// User-suspension enforcement (#454).
//
// # Why this is a chokepoint and not six inline checks
//
// Suspension is only as strong as its weakest authentication path: one path
// that resolves a user id without consulting the status column is a complete
// bypass. There are SIX places in this package that turn a resolved user id
// into an authenticated context (three required-auth, three optional-auth), and
// "remember to add the check to all of them, and to any future one" is not a
// property anyone can verify by reading a diff.
//
// So authenticatedContext — which already existed as the single place that
// builds the authenticated context shape — is now reachable only through
// authenticateUser, which cannot construct that context without first clearing
// the status check. A new auth path physically cannot skip the check without
// calling authenticatedContext directly, and
// TestAuthenticatedContextIsOnlyReachedThroughAuthenticateUser
// (middleware_suspension_guardrail_test.go) fails the build if it does.
//
// The check costs one indexed primary-key read per authenticated request. That
// is the price of making revocation immediate: it is evaluated per request, not
// at token issuance, so a suspended user holding an unexpired session cookie,
// API key or MCP/OAuth bearer token is cut off on their very next call.

// # Access allowlist (#1235)
//
// The same chokepoint enforces the access allowlist, for the same reason:
// removing someone from the allowlist is an access revocation, and it has to
// reach a session cookie, an API key and an MCP/OAuth token that were issued
// while the user still matched. The check reuses the user row the suspension
// check already loaded, and the allowlist resolver answers from a compiled list
// held in memory (it re-reads the auth settings version at most once every few
// seconds), so it adds no database round trip of its own. Root instance admins
// are exempt inside the resolver.

// errUserSuspended is returned by authenticateUser when the resolved account is
// suspended. Required-auth paths translate it into their normal
// "not authenticated" response; optional-auth paths treat it as anonymous.
var errUserSuspended = errors.New("user account is suspended")

// errUserNotOnAllowlist is returned by authenticateUser when an access
// allowlist is active and the resolved account's email no longer matches it.
// It is handled exactly like errUserSuspended: required-auth paths reject with
// 401, optional-auth paths proceed anonymously.
var errUserNotOnAllowlist = errors.New("user is not on the access allowlist")

// suspendedAuthDetail is the client-facing message for a rejected suspended
// account. It is deliberately explicit rather than a generic auth failure: the
// caller already holds a valid credential for this account, so naming the cause
// leaks nothing and lets a client render something better than "invalid token".
const suspendedAuthDetail = "Account suspended"

// notOnAllowlistAuthDetail is the client-facing message for an account the
// access allowlist no longer admits. Like suspendedAuthDetail it names the
// cause: the caller holds a valid credential for the account.
const notOnAllowlistAuthDetail = "Access restricted by the access allowlist"

// rejectReasonNotOnAllowlist is the logged reason of an allowlist rejection,
// the same value sign-in logs for the same denial.
const rejectReasonNotOnAllowlist = "not_on_allowlist"

// authenticateUser is the ONLY way to build an authenticated request context.
// It rejects suspended accounts, and accounts the access allowlist no longer
// admits, before delegating to authenticatedContext.
//
// Errors are of two kinds and callers must distinguish them:
//   - errUserSuspended / errUserNotOnAllowlist — the account may not
//     authenticate. Required-auth paths reject; the optional-auth paths proceed
//     anonymously.
//   - anything else — an infrastructure failure looking the account up, or
//     evaluating an allowlist that is known to be active. This must NOT be
//     treated as "allowed": failing open here would make suspension and the
//     allowlist bypassable by inducing database errors. Required-auth paths
//     return 500; optional-auth paths proceed anonymously (they already do that
//     for every other failure, and they grant no access on their own).
func (s *Server) authenticateUser(
	ctx context.Context, userID, authType string, extra []any,
) (context.Context, error) {
	if err := s.rejectIfSuspended(ctx, userID); err != nil {
		return nil, err
	}
	return authenticatedContext(ctx, userID, authType, extra), nil
}

// rejectIfSuspended returns errUserSuspended when the account is suspended, a
// wrapped repository error when the lookup fails, and nil when the account may
// authenticate.
//
// An account that no longer exists is treated as suspended rather than as an
// infrastructure error: a credential referencing a deleted user must not
// authenticate (this is also the path a #455 hard delete leaves behind for any
// still-live session).
func (s *Server) rejectIfSuspended(ctx context.Context, userID string) error {
	// Fail CLOSED when the repository is unavailable. Returning nil here would
	// authenticate everyone the moment the container is partially wired, and a
	// panic would turn it into a 500 storm; an error keeps the semantics honest
	// (required-auth paths 500, optional-auth paths go anonymous).
	if s.container == nil {
		return fmt.Errorf("container unavailable for suspension check of %q", userID)
	}
	repo := s.container.UserRepository()
	if repo == nil {
		return fmt.Errorf("user repository unavailable for suspension check of %q", userID)
	}

	user, err := repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return errUserSuspended
		}
		return fmt.Errorf("failed to load user %q for suspension check: %w", userID, err)
	}
	if user.IsSuspended() {
		return errUserSuspended
	}
	return s.rejectIfNotOnAllowlist(ctx, user)
}

// rejectIfNotOnAllowlist returns errUserNotOnAllowlist when an access allowlist
// is active and user's email does not match it, a wrapped error when an active
// allowlist could not be evaluated, and nil when the account may authenticate.
//
// The fail-closed/fail-open split on a read failure belongs to the resolver
// (services.AccessAllowlistResolver): it returns an error only while the last
// allowlist it compiled is active, and allows access when none is known to be.
func (s *Server) rejectIfNotOnAllowlist(ctx context.Context, user *models.User) error {
	resolver := s.container.AccessAllowlistResolver()
	if resolver == nil {
		return fmt.Errorf("access allowlist resolver unavailable for check of %q", user.ID)
	}
	allowed, _, err := resolver.IsEmailAllowed(ctx, user.Email)
	if err != nil {
		return fmt.Errorf("failed to evaluate the access allowlist for user %q: %w", user.ID, err)
	}
	if !allowed {
		return errUserNotOnAllowlist
	}
	return nil
}

// logSuspendedRejection records a blocked request at Info — this is a security
// event an operator wants to see, but it is expected behaviour, not an error.
func (s *Server) logSuspendedRejection(ctx context.Context, middleware, authType, userID string, err error) {
	logger := contextkeys.GetLoggerFromContext(ctx).With(
		"middleware", middleware,
		"auth_type", authType,
		"user_id", userID,
	)
	if errors.Is(err, errUserSuspended) {
		logger.Info("Rejected request from suspended account")
		return
	}
	if errors.Is(err, errUserNotOnAllowlist) {
		logger.With("reason", rejectReasonNotOnAllowlist).
			Info("Rejected request from account no longer on the access allowlist")
		return
	}
	logger.With("error", fmt.Sprintf("%+v", err)).
		Error("Account status check failed; refusing to authenticate")
}

// writeSuspensionAuthError renders the response for a required-auth path whose
// suspension check failed. A suspended account reuses that path's ordinary
// "credential rejected" response (401) so the failure shape does not change; an
// infrastructure failure is a 500, never a 401, so a database outage is never
// mistaken for a credential problem.
func (s *Server) writeSuspensionAuthError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errUserSuspended) {
		apierrors.WriteJSONError(w, r, apierrors.NewAuthInvalidError(suspendedAuthDetail))
		return
	}
	if errors.Is(err, errUserNotOnAllowlist) {
		apierrors.WriteJSONError(w, r, apierrors.NewAuthInvalidError(notOnAllowlistAuthDetail))
		return
	}
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(""))
}
