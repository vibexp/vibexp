package feature_flags

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
)

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

// EmailContextKey is the context key for storing email in context for feature flag evaluation
const EmailContextKey ContextKey = "email"

// AllowedSignInUsers is the default sign-in allowlist. It is intentionally
// empty: an unconfigured instance allows open registration. Configure the
// allowlist via the SIGNIN_ALLOWED_EMAILS environment variable
// (config.Config.SignInAllowedEmails) rather than mutating this default.
var AllowedSignInUsers = []string{}

// UserSignInAllowlistFlag implements sign-in access control using feature flags.
// It validates whether a user's email address is in the allowlist for signing in to the platform.
type UserSignInAllowlistFlag struct {
	allowedEmails []string
	logger        *logrus.Logger
}

// Ensure UserSignInAllowlistFlag implements FeatureFlagEvaluator
var _ FeatureFlagEvaluator = (*UserSignInAllowlistFlag)(nil)

// NewUserSignInAllowlistFlag creates a new UserSignInAllowlistFlag.
//
// allowedEmails is the configured allowlist (typically config.SignInAllowedEmails).
// An empty list means open registration: every email is allowed. Pass
// AllowedSignInUsers to fall back to the (empty) package default.
func NewUserSignInAllowlistFlag(logger *logrus.Logger, allowedEmails []string) *UserSignInAllowlistFlag {
	// Normalize allowed emails to lowercase and trim spaces once at creation,
	// dropping any blank entries (e.g. from a trailing comma in the env var).
	normalizedEmails := make([]string, 0, len(allowedEmails))
	for _, email := range allowedEmails {
		normalized := strings.ToLower(strings.TrimSpace(email))
		if normalized != "" {
			normalizedEmails = append(normalizedEmails, normalized)
		}
	}
	return &UserSignInAllowlistFlag{
		allowedEmails: normalizedEmails,
		logger:        logger,
	}
}

// Name returns the unique identifier for this feature flag
func (f *UserSignInAllowlistFlag) Name() string {
	return "user_signin_allowlist"
}

// Evaluate checks if the email from context is in the sign-in allowlist
// Email should be stored in context during the authentication flow.
//
// When the allowlist is empty (unconfigured), sign-in is open and every email
// is allowed — this is the default for self-hosted/open-source instances.
func (f *UserSignInAllowlistFlag) Evaluate(ctx context.Context) bool {
	// Empty allowlist means open registration: allow everyone.
	if len(f.allowedEmails) == 0 {
		return true
	}

	// Extract email from context
	email, ok := ctx.Value(EmailContextKey).(string)
	if !ok || email == "" {
		f.logger.WithFields(logrus.Fields{
			"service":   "vibexp-api",
			"component": "feature-flags",
			"flag":      "user_signin_allowlist",
		}).Debug("No email found in context, denying sign-in access")
		return false
	}

	// Normalize email for comparison
	email = strings.ToLower(strings.TrimSpace(email))

	// Check if email is in the allowlist
	for _, allowedEmail := range f.allowedEmails {
		if strings.EqualFold(allowedEmail, email) {
			f.logger.WithFields(logrus.Fields{
				"service":   "vibexp-api",
				"component": "feature-flags",
				"flag":      "user_signin_allowlist",
				"email":     email,
			}).Debug("Email is in sign-in allowlist, granting access")
			return true
		}
	}

	f.logger.WithFields(logrus.Fields{
		"service":   "vibexp-api",
		"component": "feature-flags",
		"flag":      "user_signin_allowlist",
		"email":     email,
	}).Debug("Email not in sign-in allowlist, denying access")

	return false
}

// IsEmailAllowed is a helper method to check if an email is in the sign-in allowlist
// This can be used directly without requiring a context.
//
// When the allowlist is empty (unconfigured), every email is allowed.
func (f *UserSignInAllowlistFlag) IsEmailAllowed(email string) bool {
	if len(f.allowedEmails) == 0 {
		return true
	}
	email = strings.ToLower(strings.TrimSpace(email))
	for _, allowedEmail := range f.allowedEmails {
		if strings.EqualFold(allowedEmail, email) {
			return true
		}
	}
	return false
}
