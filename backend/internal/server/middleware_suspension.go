package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
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

// # Session revocation (#1428)
//
// A session cookie is stateless, so the only way to end one early is to make
// the server stop accepting it. Every cookie carries the user's session_epoch
// as of the moment it was minted, and this chokepoint compares it with the user
// row it has already loaded: a mismatch means the user's sessions were revoked
// after the cookie was issued. The comparison costs no query. It applies to
// cookie authentication only: an API key and an MCP/OAuth token carry no epoch
// and have their own revocation.

// errUserSuspended is returned by authenticateUser when the resolved account is
// suspended. Required-auth paths translate it into their normal
// "not authenticated" response; optional-auth paths treat it as anonymous.
var errUserSuspended = errors.New("user account is suspended")

// errUserNotOnAllowlist is returned by authenticateUser when an access
// allowlist is active and the resolved account's email no longer matches it.
// It is handled exactly like errUserSuspended: required-auth paths reject with
// 401, optional-auth paths proceed anonymously.
var errUserNotOnAllowlist = errors.New("user is not on the access allowlist")

// errSessionRevoked is returned by authenticateUser when a session cookie was
// minted under an earlier session epoch than the user's current one: the
// user's sessions were revoked after it was issued. Required-auth paths clear
// the cookie and answer 401; optional-auth paths proceed anonymously.
var errSessionRevoked = errors.New("session was revoked")

// errTokenRevoked is returned by authenticateUser when a bearer access token was
// issued before the user's API access was last revoked (#1449). Such a token is
// a signed JWT that would verify until its own expiry; the user's
// api_tokens_valid_after cut-off is what ends it early. Required-auth paths
// answer 401; optional-auth paths proceed anonymously.
var errTokenRevoked = errors.New("access token was revoked")

// errSessionMissing is returned by authenticateUser when a cookie
// authentication reaches it without the session. It is a programming error,
// reported as an infrastructure failure so the request is refused: a cookie
// path that skipped the epoch check would make revocation bypassable.
var errSessionMissing = errors.New("cookie authentication without a session")

// sessionRevokedAuthDetail is the client-facing message for a revoked session.
// It is the message an expired session already gets: to the client the two are
// the same event, a session that no longer works and needs a new sign-in.
const sessionRevokedAuthDetail = "Session expired"

// rejectReasonSessionRevoked is the logged reason of an epoch rejection.
const rejectReasonSessionRevoked = "session_revoked"

// tokenRevokedAuthDetail is the client-facing message for a revoked access
// token: the wording an expired or malformed token already gets, so a client
// re-authorizes the same way.
const tokenRevokedAuthDetail = "Invalid token"

// rejectReasonTokenRevoked is the logged reason of a cut-off rejection.
const rejectReasonTokenRevoked = "token_revoked"

// authTypeOAuth is the auth type of a request authenticated by a bearer access
// token (the MCP endpoint and the /api/v1 bearer path).
const authTypeOAuth = "oauth"

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
// It rejects suspended accounts, accounts the access allowlist no longer
// admits, revoked cookie sessions and revoked bearer tokens, before delegating
// to authenticatedContext.
//
// sess is the decrypted session of a cookie authentication and nil for every
// other auth type. A cookie authentication must pass it: without it the epoch
// could not be checked, so that is refused rather than waved through.
//
// tokenIssuedAt is the iat of a bearer access token (authTypeOAuth) and the
// zero time for every other auth type. Once the user has a bearer-token
// cut-off, a token issued before it is refused, and so is one whose issue time
// is unknown: a token that cannot be placed after the revocation is not trusted
// to be. For a token of the embedded Authorization Server, iat is when the
// GRANT was first exchanged; a refreshed token inherits it, so refreshing never
// carries a token past the cut-off.
//
// Errors are of two kinds and callers must distinguish them:
//   - errUserSuspended / errUserNotOnAllowlist / errSessionRevoked /
//     errTokenRevoked — the credential may not authenticate. Required-auth paths reject; the
//     optional-auth paths proceed anonymously.
//   - anything else — an infrastructure failure looking the account up, or
//     evaluating an allowlist that is known to be active. This must NOT be
//     treated as "allowed": failing open here would make suspension and the
//     allowlist bypassable by inducing database errors. Required-auth paths
//     return 500; optional-auth paths proceed anonymously (they already do that
//     for every other failure, and they grant no access on their own).
func (s *Server) authenticateUser(
	ctx context.Context, userID, authType string, extra []any, sess *sesslib.Session, tokenIssuedAt time.Time,
) (context.Context, error) {
	if authType == authTypeCookie && sess == nil {
		return nil, fmt.Errorf("%w for user %q", errSessionMissing, userID)
	}
	user, err := s.rejectIfSuspended(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sess != nil && sess.Epoch != user.SessionEpoch {
		return nil, errSessionRevoked
	}
	if authType == authTypeOAuth && tokenRevoked(user, tokenIssuedAt) {
		return nil, errTokenRevoked
	}
	return authenticatedContext(ctx, userID, authType, extra, sess), nil
}

// tokenRevoked reports whether a bearer token issued at issuedAt predates the
// user's cut-off. With no cut-off nothing is revoked. With one, an unknown issue
// time (the zero time) counts as revoked.
//
// iat has one-second resolution and the cut-off does not, so a token issued in
// the same second as the revocation, even just after it, is refused once and
// the client re-authorizes. That errs on the side the revocation exists for.
//
// The cut-off is the DATABASE clock and iat the clock of the server that minted
// the token. If the minting server's clock runs ahead of the database by N
// seconds, a grant first exchanged in the N seconds before a revocation reads
// as issued after it and keeps working until it expires (its rows are gone, so
// it cannot be refreshed). Clocks kept in sync, as any deployment needs for
// token expiry anyway, make that window negligible.
func tokenRevoked(user *models.User, issuedAt time.Time) bool {
	if user.APITokensValidAfter == nil {
		return false
	}
	return issuedAt.IsZero() || issuedAt.Before(*user.APITokensValidAfter)
}

// rejectIfSuspended returns the account when it may authenticate,
// errUserSuspended when it is suspended, and a wrapped repository error when
// the lookup fails. The returned row is what the session epoch is checked
// against, so that check needs no second read.
//
// An account that no longer exists is treated as suspended rather than as an
// infrastructure error: a credential referencing a deleted user must not
// authenticate (this is also the path a #455 hard delete leaves behind for any
// still-live session).
func (s *Server) rejectIfSuspended(ctx context.Context, userID string) (*models.User, error) {
	// Fail CLOSED when the repository is unavailable. Returning nil here would
	// authenticate everyone the moment the container is partially wired, and a
	// panic would turn it into a 500 storm; an error keeps the semantics honest
	// (required-auth paths 500, optional-auth paths go anonymous).
	if s.container == nil {
		return nil, fmt.Errorf("container unavailable for suspension check of %q", userID)
	}
	repo := s.container.UserRepository()
	if repo == nil {
		return nil, fmt.Errorf("user repository unavailable for suspension check of %q", userID)
	}

	user, err := repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return nil, errUserSuspended
		}
		return nil, fmt.Errorf("failed to load user %q for suspension check: %w", userID, err)
	}
	if user.IsSuspended() {
		return nil, errUserSuspended
	}
	if err := s.rejectIfNotOnAllowlist(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
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
	if errors.Is(err, errSessionRevoked) {
		logger.With("reason", rejectReasonSessionRevoked).
			Info("Rejected request on a revoked session")
		return
	}
	if errors.Is(err, errTokenRevoked) {
		logger.With("reason", rejectReasonTokenRevoked).
			Info("Rejected request with a revoked access token")
		return
	}
	logger.With("error", fmt.Sprintf("%+v", err)).
		Error("Account status check failed; refusing to authenticate")
}

// writeSuspensionAuthError renders the response for a required-auth path whose
// suspension check failed. A suspended account reuses that path's ordinary
// "credential rejected" response (401) so the failure shape does not change, and
// so does a revoked session, whose cookie is cleared as well; an
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
	if errors.Is(err, errSessionRevoked) {
		// The cookie can never authenticate again, so drop it instead of
		// letting the browser present it on every request until it expires.
		if s.sessionManager != nil {
			s.sessionManager.Clear(w)
		}
		apierrors.WriteJSONError(w, r, apierrors.NewAuthInvalidError(sessionRevokedAuthDetail))
		return
	}
	if errors.Is(err, errTokenRevoked) {
		apierrors.WriteJSONError(w, r, apierrors.NewAuthInvalidError(tokenRevokedAuthDetail))
		return
	}
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(""))
}
