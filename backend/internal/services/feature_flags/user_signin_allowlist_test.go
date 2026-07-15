package feature_flags

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Helper function to create context with email for testing
func contextWithEmail(email string) context.Context {
	return context.WithValue(context.Background(), EmailContextKey, email)
}

// Helper function to create context with wrong type for testing
func contextWithWrongType(value interface{}) context.Context {
	return context.WithValue(context.Background(), EmailContextKey, value)
}

func TestNewUserSignInAllowlistFlag(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	allowedEmails := []string{"test@example.com", "admin@example.com"}

	flag := NewUserSignInAllowlistFlag(logger, nil, allowedEmails)

	assert.NotNil(t, flag)
	assert.NotNil(t, flag.logger)
	// Verify that emails were normalized
	assert.Len(t, flag.allowedEmails, len(allowedEmails))
}

func TestNewUserSignInAllowlistFlag_DropsBlankEntries(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	// A trailing comma in AUTH_ALLOWED_EMAILS / AUTH_ALLOWED_DOMAINS produces an
	// empty element; blanks (and whitespace-only entries) are dropped for both lists.
	flag := NewUserSignInAllowlistFlag(logger,
		[]string{"example.com", "", "  "},
		[]string{"test@example.com", "", "  "},
	)

	assert.Len(t, flag.allowedDomains, 1)
	assert.Len(t, flag.allowedEmails, 1)
}

func TestUserSignInAllowlistFlag_Name(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, nil, []string{"test@example.com"})

	assert.Equal(t, "user_signin_allowlist", flag.Name())
}

func TestUserSignInAllowlistFlag_Evaluate_WithEmailInContext(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	allowedEmails := []string{"test@example.com", "admin@example.com"}

	flag := NewUserSignInAllowlistFlag(logger, nil, allowedEmails)

	tests := []struct {
		name     string
		email    string
		expected bool
	}{
		{
			name:     "email in allowlist",
			email:    "test@example.com",
			expected: true,
		},
		{
			name:     "email in allowlist with different case",
			email:    "TEST@EXAMPLE.COM",
			expected: true,
		},
		{
			name:     "email in allowlist with spaces",
			email:    "  admin@example.com  ",
			expected: true,
		},
		{
			name:     "email not in allowlist",
			email:    "unauthorized@example.com",
			expected: false,
		},
		{
			name:     "empty email",
			email:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := contextWithEmail(tt.email)
			result := flag.Evaluate(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestUserSignInAllowlistFlag_Evaluate_EmailVerified covers #218: an ACTIVE
// allowlist grants access by address, so an address the identity provider did not
// verify must not satisfy it — otherwise a provider relaying an unverified claim
// lets an attacker simply assert an allowlisted address. Scoped to an active
// allowlist: an open instance is unchanged, and an absent key means verified so
// callers with no verification concept (dev login) keep working.
func TestUserSignInAllowlistFlag_Evaluate_EmailVerified(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	const allowed = "test@example.com"

	tests := []struct {
		name              string
		allowedEmails     []string
		email             string
		setVerified       bool
		verified          bool
		expected          bool
		expectedRationale string
	}{
		{
			name:              "active allowlist, allowlisted email, verified",
			allowedEmails:     []string{allowed},
			email:             allowed,
			setVerified:       true,
			verified:          true,
			expected:          true,
			expectedRationale: "a verified, allowlisted address is the normal grant",
		},
		{
			name:              "active allowlist, allowlisted email, NOT verified",
			allowedEmails:     []string{allowed},
			email:             allowed,
			setVerified:       true,
			verified:          false,
			expected:          false,
			expectedRationale: "an unverified address must never satisfy an active allowlist",
		},
		{
			name:              "active allowlist, allowlisted email, verification unknown",
			allowedEmails:     []string{allowed},
			email:             allowed,
			setVerified:       false,
			expected:          true,
			expectedRationale: "an absent key means verified, so pre-#218 callers are unaffected",
		},
		{
			name:              "open allowlist, NOT verified",
			allowedEmails:     nil,
			email:             "anyone@anywhere.test",
			setVerified:       true,
			verified:          false,
			expected:          true,
			expectedRationale: "with no allowlist there is nothing to bypass: open instances unchanged",
		},
		{
			name:              "active allowlist, non-allowlisted email, verified",
			allowedEmails:     []string{allowed},
			email:             "outsider@elsewhere.test",
			setVerified:       true,
			verified:          true,
			expected:          false,
			expectedRationale: "verification does not by itself grant access",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := NewUserSignInAllowlistFlag(logger, nil, tt.allowedEmails)

			ctx := contextWithEmail(tt.email)
			if tt.setVerified {
				ctx = context.WithValue(ctx, EmailVerifiedContextKey, tt.verified)
			}

			assert.Equal(t, tt.expected, flag.Evaluate(ctx), tt.expectedRationale)
		})
	}
}

// TestUserSignInAllowlistFlag_IsEmailAllowed_IgnoresVerification pins the split of
// responsibilities: IsEmailAllowed answers only "does this address match?", which
// is what the MCP consent re-check (#217) needs for an already-established
// identity. The verification rule lives in Evaluate, which has the IdP claims.
func TestUserSignInAllowlistFlag_IsEmailAllowed_IgnoresVerification(t *testing.T) {
	flag := NewUserSignInAllowlistFlag(slog.New(slog.DiscardHandler), nil, []string{"test@example.com"})

	assert.True(t, flag.IsEmailAllowed("test@example.com"),
		"IsEmailAllowed matches the address and says nothing about verification")
}

func TestUserSignInAllowlistFlag_Evaluate_NoEmailInContext(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, nil, []string{"test@example.com"})

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{
			name: "nil context value",
			ctx:  context.Background(),
		},
		{
			name: "wrong type in context",
			ctx:  contextWithWrongType(123),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := flag.Evaluate(tt.ctx)
			assert.False(t, result)
		})
	}
}

func TestUserSignInAllowlistFlag_IsEmailAllowed(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	allowedEmails := []string{"test@example.com", "admin@example.com"}

	flag := NewUserSignInAllowlistFlag(logger, nil, allowedEmails)

	tests := []struct {
		name     string
		email    string
		expected bool
	}{
		{
			name:     "email in allowlist",
			email:    "test@example.com",
			expected: true,
		},
		{
			name:     "email in allowlist with different case",
			email:    "TEST@EXAMPLE.COM",
			expected: true,
		},
		{
			name:     "email in allowlist with spaces",
			email:    "  admin@example.com  ",
			expected: true,
		},
		{
			name:     "email not in allowlist",
			email:    "unauthorized@example.com",
			expected: false,
		},
		{
			name:     "empty email",
			email:    "",
			expected: false,
		},
		{
			name:     "email with mixed case in allowlist",
			email:    "aDmIn@ExAmPlE.cOm",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := flag.IsEmailAllowed(tt.email)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestUserSignInAllowlistFlag_DomainMatching exhaustively covers the
// email-domain matching semantics added in #214: exact-domain match on the part
// after the LAST "@", case/whitespace-insensitive, with NO subdomain or
// substring matching, and denial of inputs without an "@".
func TestUserSignInAllowlistFlag_DomainMatching(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	// Domains only (no exact-email entries) to isolate domain matching. Entries
	// are deliberately mixed-case / padded to prove normalization at construction.
	flag := NewUserSignInAllowlistFlag(logger,
		[]string{"Example.com", "  vibexp.io  "},
		nil,
	)

	tests := []struct {
		name     string
		email    string
		expected bool
	}{
		{"exact domain match", "alice@example.com", true},
		{"exact domain match, second domain", "bob@vibexp.io", true},
		{"domain match, uppercase input", "ALICE@EXAMPLE.COM", true},
		{"domain match, surrounding whitespace", "  alice@example.com  ", true},
		{"subdomain must NOT match", "a@sub.vibexp.io", false},
		{"lookalike domain must NOT match", "a@evil-vibexp.io", false},
		{"superstring domain must NOT match", "a@vibexp.io.attacker.com", false},
		{"unrelated domain rejected", "a@other.com", false},
		{"no @ is denied when allowlist active", "not-an-email", false},
		{"domain matched on last @ (multiple @)", `"a@b"@example.com`, true},
		{"trailing @ (empty domain) denied", "alice@", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, flag.IsEmailAllowed(tt.email))
			assert.Equal(t, tt.expected, flag.Evaluate(contextWithEmail(tt.email)))
		})
	}
}

// TestUserSignInAllowlistFlag_DomainsAndEmailsCombined verifies a user is allowed
// if EITHER list matches: exact email OR domain.
func TestUserSignInAllowlistFlag_DomainsAndEmailsCombined(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger,
		[]string{"example.com"},
		[]string{"special@other.com"},
	)

	// Allowed by domain.
	assert.True(t, flag.IsEmailAllowed("anyone@example.com"))
	// Allowed by exact email even though its domain is not listed.
	assert.True(t, flag.IsEmailAllowed("special@other.com"))
	// A different address at an unlisted domain is denied.
	assert.False(t, flag.IsEmailAllowed("nobody@other.com"))
}

// TestUserSignInAllowlistFlag_EmptyAllowlist verifies open-registration
// semantics: when the allowlist is empty (unconfigured), every email is
// allowed to sign in. This is the default for self-hosted instances.
func TestUserSignInAllowlistFlag_EmptyAllowlist(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, []string{}, []string{})

	ctx := contextWithEmail("test@example.com")
	assert.True(t, flag.Evaluate(ctx))
	assert.True(t, flag.IsEmailAllowed("test@example.com"))

	// Even an arbitrary, never-seen address is allowed.
	assert.True(t, flag.Evaluate(contextWithEmail("anyone@example.com")))
	assert.True(t, flag.IsEmailAllowed("anyone@example.com"))
	// An input without an "@" is also allowed under open access.
	assert.True(t, flag.IsEmailAllowed("not-an-email"))
}

// TestUserSignInAllowlistFlag_NilAllowlist verifies that nil slices (the zero
// value of config.Auth.AccessAllowlist.Domains / .Emails when the env vars are
// unset) also yield open registration.
func TestUserSignInAllowlistFlag_NilAllowlist(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, nil, nil)

	assert.True(t, flag.Evaluate(contextWithEmail("anyone@example.com")))
	assert.True(t, flag.IsEmailAllowed("anyone@example.com"))
}

// TestUserSignInAllowlistFlag_OnlyDomainsConfigured verifies open access is NOT
// granted when only the domains list is populated (an exact-email-only check
// would wrongly treat this as open).
func TestUserSignInAllowlistFlag_OnlyDomainsConfigured(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, []string{"example.com"}, nil)

	assert.True(t, flag.IsEmailAllowed("alice@example.com"))
	assert.False(t, flag.IsEmailAllowed("alice@other.com"))
}

func TestUserSignInAllowlistFlag_MultipleEmailsInAllowlist(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	allowedEmails := []string{
		"user1@example.com",
		"user2@example.com",
		"user3@example.com",
		"admin@example.com",
		"support@example.com",
	}

	flag := NewUserSignInAllowlistFlag(logger, nil, allowedEmails)

	// Test all allowed emails
	for _, email := range allowedEmails {
		ctx := contextWithEmail(email)
		result := flag.Evaluate(ctx)
		assert.True(t, result, "Expected %s to be allowed", email)

		result = flag.IsEmailAllowed(email)
		assert.True(t, result, "Expected %s to be allowed", email)
	}

	// Test unauthorized email
	ctx := contextWithEmail("unauthorized@example.com")
	result := flag.Evaluate(ctx)
	assert.False(t, result)

	result = flag.IsEmailAllowed("unauthorized@example.com")
	assert.False(t, result)
}

func TestUserSignInAllowlistFlag_CaseSensitivity(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	allowedEmails := []string{"Test@Example.Com"}

	flag := NewUserSignInAllowlistFlag(logger, nil, allowedEmails)

	tests := []struct {
		name  string
		email string
	}{
		{"lowercase", "test@example.com"},
		{"uppercase", "TEST@EXAMPLE.COM"},
		{"mixed case", "TeSt@ExAmPlE.cOm"},
		{"original case", "Test@Example.Com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := contextWithEmail(tt.email)
			result := flag.Evaluate(ctx)
			assert.True(t, result)

			result = flag.IsEmailAllowed(tt.email)
			assert.True(t, result)
		})
	}
}
