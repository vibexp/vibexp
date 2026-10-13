package password

import (
	_ "embed" // for the go:embed directive on commonPasswordList
	"strings"
	"sync"
	"unicode/utf8"
)

// The password policy (#1431, epic #1426 decision 7): a length range, no
// composition rules, not a commonly used password, and not the account's own
// email address. It is deliberately not configurable.
const (
	// MinLength and MaxLength are counted in characters (runes), not bytes.
	MinLength = 12
	// MaxLength is generous. It exists so one request cannot make the server
	// hash megabytes of input.
	MaxLength = 128
)

// Violation is one reason a password fails the policy. The values are stable:
// clients map them to their own wording.
type Violation string

const (
	// TooShort: fewer than MinLength characters.
	TooShort Violation = "too_short"
	// TooLong: more than MaxLength characters.
	TooLong Violation = "too_long"
	// Common: on the built-in list of commonly used passwords.
	Common Violation = "common"
	// MatchesEmail: the account's email address, or the part before the "@".
	MatchesEmail Violation = "matches_email"
)

// Message is a sentence a person can act on. It never contains the password.
func (v Violation) Message() string {
	switch v {
	case TooShort:
		return "Use at least 12 characters."
	case TooLong:
		return "Use at most 128 characters."
	case Common:
		return "This password is too common. Choose one that is harder to guess."
	case MatchesEmail:
		return "Do not use your email address as your password."
	default:
		return "This password is not allowed."
	}
}

// PolicyError is returned by Validate for a password that fails the policy. It
// names the rules broken and never carries the password.
type PolicyError struct {
	Violations []Violation
}

func (e *PolicyError) Error() string {
	parts := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		parts[i] = string(v)
	}
	return "password does not meet the policy: " + strings.Join(parts, ", ")
}

// commonPasswordList holds commonly used passwords of MinLength characters or
// more, one per line, lower-cased and sorted. Shorter ones are left out because
// the length rule already rejects them.
//
// Source: the "100k most used passwords" list published by the UK National
// Cyber Security Centre with Have I Been Pwned, as distributed in SecLists
// (github.com/danielmiessler/SecLists, MIT licence), filtered to printable
// ASCII entries of 12 to 128 characters.
//
//go:embed common_passwords.txt
var commonPasswordList string

var (
	commonOnce sync.Once
	commonSet  map[string]struct{}
)

func isCommon(password string) bool {
	commonOnce.Do(func() {
		lines := strings.Split(commonPasswordList, "\n")
		commonSet = make(map[string]struct{}, len(lines))
		for _, line := range lines {
			if line = strings.TrimSpace(line); line != "" {
				commonSet[line] = struct{}{}
			}
		}
	})
	_, found := commonSet[strings.ToLower(password)]
	return found
}

// Validate checks password against the policy for the account with the given
// email, and returns nil or a *PolicyError listing every rule broken. email may
// be empty when there is no account yet.
//
// The password is checked exactly as typed: it is not trimmed or normalized, so
// leading and trailing spaces count and are kept. Comparisons with the common
// list and the email ignore letter case.
func Validate(password, email string) error {
	var violations []Violation

	switch length := utf8.RuneCountInString(password); {
	case length < MinLength:
		violations = append(violations, TooShort)
	case length > MaxLength:
		violations = append(violations, TooLong)
	}
	if isCommon(password) {
		violations = append(violations, Common)
	}
	if matchesEmail(password, email) {
		violations = append(violations, MatchesEmail)
	}

	if len(violations) == 0 {
		return nil
	}
	return &PolicyError{Violations: violations}
}

// matchesEmail reports whether password is the email address or its local
// part, ignoring case and surrounding space in the email.
func matchesEmail(password, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	candidate := strings.ToLower(password)
	if candidate == email {
		return true
	}
	local, _, found := strings.Cut(email, "@")
	return found && local != "" && candidate == local
}
