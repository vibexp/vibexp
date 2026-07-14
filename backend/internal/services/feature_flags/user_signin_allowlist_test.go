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

	flag := NewUserSignInAllowlistFlag(logger, allowedEmails)

	assert.NotNil(t, flag)
	assert.NotNil(t, flag.logger)
	// Verify that emails were normalized
	assert.Len(t, flag.allowedEmails, len(allowedEmails))
}

func TestNewUserSignInAllowlistFlag_DropsBlankEntries(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	// A trailing comma in AUTH_ALLOWED_EMAILS produces an empty element.
	flag := NewUserSignInAllowlistFlag(logger, []string{"test@example.com", "", "  "})

	assert.Len(t, flag.allowedEmails, 1)
}

func TestUserSignInAllowlistFlag_Name(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, []string{"test@example.com"})

	assert.Equal(t, "user_signin_allowlist", flag.Name())
}

func TestUserSignInAllowlistFlag_Evaluate_WithEmailInContext(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	allowedEmails := []string{"test@example.com", "admin@example.com"}

	flag := NewUserSignInAllowlistFlag(logger, allowedEmails)

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

func TestUserSignInAllowlistFlag_Evaluate_NoEmailInContext(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, []string{"test@example.com"})

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

	flag := NewUserSignInAllowlistFlag(logger, allowedEmails)

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

// TestUserSignInAllowlistFlag_EmptyAllowlist verifies open-registration
// semantics: when the allowlist is empty (unconfigured), every email is
// allowed to sign in. This is the default for self-hosted instances.
func TestUserSignInAllowlistFlag_EmptyAllowlist(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, []string{})

	ctx := contextWithEmail("test@example.com")
	assert.True(t, flag.Evaluate(ctx))
	assert.True(t, flag.IsEmailAllowed("test@example.com"))

	// Even an arbitrary, never-seen address is allowed.
	assert.True(t, flag.Evaluate(contextWithEmail("anyone@example.com")))
	assert.True(t, flag.IsEmailAllowed("anyone@example.com"))
}

// TestUserSignInAllowlistFlag_NilAllowlist verifies that a nil slice (the
// zero value of config.Auth.AccessAllowlist.Emails when the env var is unset) also
// yields open registration.
func TestUserSignInAllowlistFlag_NilAllowlist(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	flag := NewUserSignInAllowlistFlag(logger, nil)

	assert.True(t, flag.Evaluate(contextWithEmail("anyone@example.com")))
	assert.True(t, flag.IsEmailAllowed("anyone@example.com"))
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

	flag := NewUserSignInAllowlistFlag(logger, allowedEmails)

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

	flag := NewUserSignInAllowlistFlag(logger, allowedEmails)

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
