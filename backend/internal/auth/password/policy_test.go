package password

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func violationsOf(t *testing.T, err error) []Violation {
	t.Helper()
	var policyErr *PolicyError
	require.True(t, errors.As(err, &policyErr), "want a *PolicyError, got %v", err)
	return policyErr.Violations
}

func TestValidate_AcceptsWithoutCompositionRules(t *testing.T) {
	accepted := []string{
		"correct horse battery staple",
		"alllowercaseletters",       // no digit, symbol or capital is required
		"quiet-harbor-lantern",      //
		strings.Repeat("é", 12),     // 12 characters, 24 bytes
		"  spaces are kept as is  ", //
		strings.Repeat("x9", 64),    // exactly the maximum
	}
	for _, password := range accepted {
		t.Run(password, func(t *testing.T) {
			assert.NoError(t, Validate(password, "ada@example.com"))
		})
	}
}

func TestValidate_Length(t *testing.T) {
	t.Run("eleven characters is too short", func(t *testing.T) {
		assert.Equal(t, []Violation{TooShort}, violationsOf(t, Validate("elevenchars", "")))
	})
	t.Run("length is counted in characters, not bytes", func(t *testing.T) {
		// 11 characters but 22 bytes: a byte count would let it through.
		assert.Equal(t, []Violation{TooShort}, violationsOf(t, Validate(strings.Repeat("é", 11), "")))
	})
	t.Run("the empty password is too short", func(t *testing.T) {
		assert.Equal(t, []Violation{TooShort}, violationsOf(t, Validate("", "")))
	})
	t.Run("129 characters is too long", func(t *testing.T) {
		assert.Equal(t, []Violation{TooLong}, violationsOf(t, Validate(strings.Repeat("a1", 64)+"x", "")))
	})
}

func TestValidate_CommonPasswords(t *testing.T) {
	for _, password := range []string{"123456789012", "passwordpassword", "PasswordPassword", "QWERTYUIOPASDFGHJKL"} {
		t.Run(password, func(t *testing.T) {
			if !isCommon(password) {
				t.Skipf("%q is not on the embedded list", password)
			}
			assert.Equal(t, []Violation{Common}, violationsOf(t, Validate(password, "")))
		})
	}

	t.Run("the list is loaded and holds only passwords the length rule lets through", func(t *testing.T) {
		require.True(t, isCommon("123456789012"), "a canonical entry must be on the list")
		require.Greater(t, len(commonSet), 1000)
		for entry := range commonSet {
			require.GreaterOrEqual(t, len(entry), MinLength, entry)
			require.Equal(t, strings.ToLower(entry), entry, "entries are stored lower-cased")
		}
	})
}

func TestValidate_MatchesEmail(t *testing.T) {
	const email = "Grace.Hopper.1906@Example.com"

	for name, password := range map[string]string{
		"the address":                 "grace.hopper.1906@example.com",
		"the address in another case": "GRACE.HOPPER.1906@EXAMPLE.COM",
		"the local part":              "grace.hopper.1906",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, []Violation{MatchesEmail}, violationsOf(t, Validate(password, email)))
		})
	}

	t.Run("a password that only contains the local part is allowed", func(t *testing.T) {
		assert.NoError(t, Validate("grace.hopper.1906-and-more", email))
	})
	t.Run("no email means no email rule", func(t *testing.T) {
		assert.NoError(t, Validate("grace.hopper.1906", ""))
	})
	t.Run("an address with no @ never matches a local part", func(t *testing.T) {
		assert.NoError(t, Validate("some-long-password", "not-an-address"))
	})
}

func TestValidate_ReportsEveryViolation(t *testing.T) {
	// Short and equal to the local part.
	violations := violationsOf(t, Validate("ada", "ada@example.com"))

	assert.ElementsMatch(t, []Violation{TooShort, MatchesEmail}, violations)
}

// The error and every message are safe to log and to return: neither ever
// contains the password.
func TestPolicyError_NeverCarriesThePassword(t *testing.T) {
	const password = "hunter2"
	err := Validate(password, "hunter2@example.com")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), password)
	assert.Equal(t, "password does not meet the policy: too_short, matches_email", err.Error())
	for _, v := range []Violation{TooShort, TooLong, Common, MatchesEmail, Violation("unknown")} {
		assert.NotEmpty(t, v.Message())
		assert.NotContains(t, v.Message(), password)
	}
}
