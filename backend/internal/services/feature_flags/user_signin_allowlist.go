package feature_flags

import (
	"context"
	"log/slog"
	"strings"
)

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

// EmailContextKey is the context key for storing email in context for feature flag evaluation
const EmailContextKey ContextKey = "email"

// FlagUserSignInAllowlist is the registered name of the sign-in allowlist flag.
// It is the lookup key callers pass to FeatureFlagService.IsEnabled.
const FlagUserSignInAllowlist = "user_signin_allowlist"

// AllowedSignInUsers is the default sign-in allowlist. It is intentionally
// empty: an unconfigured instance allows open registration. Configure the
// allowlist via auth.access_allowlist.domains / .emails
// (AUTH_ALLOWED_DOMAINS / AUTH_ALLOWED_EMAILS) rather than mutating this default.
var AllowedSignInUsers = []string{}

// UserSignInAllowlistFlag implements sign-in access control using feature flags.
// It validates whether a user's email address is permitted to sign in, by exact
// email address and/or by email domain (the part after the last "@").
type UserSignInAllowlistFlag struct {
	allowedDomains []string
	allowedEmails  []string
	logger         *slog.Logger
}

// Ensure UserSignInAllowlistFlag implements FeatureFlagEvaluator
var _ FeatureFlagEvaluator = (*UserSignInAllowlistFlag)(nil)

// NewUserSignInAllowlistFlag creates a new UserSignInAllowlistFlag.
//
// domains and emails are the configured allowlist (typically
// config.Auth.AccessAllowlist.Domains / .Emails). When BOTH are empty, sign-in
// is open: every email is allowed. Pass AllowedSignInUsers to fall back to the
// (empty) package default.
func NewUserSignInAllowlistFlag(logger *slog.Logger, domains, emails []string) *UserSignInAllowlistFlag {
	return &UserSignInAllowlistFlag{
		allowedDomains: normalizeAllowlist(domains),
		allowedEmails:  normalizeAllowlist(emails),
		logger:         logger,
	}
}

// normalizeAllowlist lowercases and trims each entry once at construction,
// dropping any blank entries (e.g. from a trailing comma in an env var).
func normalizeAllowlist(items []string) []string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		if n := strings.ToLower(strings.TrimSpace(item)); n != "" {
			normalized = append(normalized, n)
		}
	}
	return normalized
}

// openAccess reports whether the allowlist is unconfigured (both lists empty),
// in which case every user may sign in — the default for self-hosted instances.
func (f *UserSignInAllowlistFlag) openAccess() bool {
	return len(f.allowedDomains) == 0 && len(f.allowedEmails) == 0
}

// Name returns the unique identifier for this feature flag
func (f *UserSignInAllowlistFlag) Name() string {
	return FlagUserSignInAllowlist
}

// Evaluate checks if the email from context is permitted to sign in.
// Email should be stored in context during the authentication flow.
//
// When the allowlist is empty (unconfigured), sign-in is open and every email
// is allowed — this is the default for self-hosted/open-source instances. The
// matching decision is delegated to IsEmailAllowed; Evaluate adds the context
// plumbing and denial/grant logging.
func (f *UserSignInAllowlistFlag) Evaluate(ctx context.Context) bool {
	// Empty allowlist means open registration: allow everyone.
	if f.openAccess() {
		return true
	}

	// Extract email from context
	email, ok := ctx.Value(EmailContextKey).(string)
	if !ok || email == "" {
		f.logger.With(
			"service", "vibexp-api",
			"component", "feature-flags",
			"flag", "user_signin_allowlist",
		).Debug("No email found in context, denying sign-in access")
		return false
	}

	if f.IsEmailAllowed(email) {
		f.logger.With(
			"service", "vibexp-api",
			"component", "feature-flags",
			"flag", "user_signin_allowlist",
			"email", strings.ToLower(strings.TrimSpace(email)),
		).Debug("Email is in sign-in allowlist, granting access")
		return true
	}

	f.logger.With(
		"service", "vibexp-api",
		"component", "feature-flags",
		"flag", "user_signin_allowlist",
		"email", strings.ToLower(strings.TrimSpace(email)),
	).Debug("Email not in sign-in allowlist, denying access")

	return false
}

// IsEmailAllowed reports whether email may sign in, without needing a context.
// It is the single source of truth for the allow/deny decision.
//
// When the allowlist is empty (unconfigured), every email is allowed. Otherwise
// the email is allowed iff (a) its normalized form exactly matches a configured
// email, or (b) the domain — the part after the LAST "@" — exactly matches a
// configured domain (case- and whitespace-insensitive). Matching is exact: no
// subdomain or substring matching, so "a@sub.example.com" and "a@evil-example.com"
// do NOT match domain "example.com". An input without an "@" is denied when the
// allowlist is active.
func (f *UserSignInAllowlistFlag) IsEmailAllowed(email string) bool {
	if f.openAccess() {
		return true
	}

	email = strings.ToLower(strings.TrimSpace(email))

	// Exact email match.
	for _, allowedEmail := range f.allowedEmails {
		if allowedEmail == email {
			return true
		}
	}

	// Exact domain match on the part after the last "@". No "@" ⇒ deny.
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, allowedDomain := range f.allowedDomains {
		if allowedDomain == domain {
			return true
		}
	}

	return false
}
