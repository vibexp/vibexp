package models

import (
	"strings"
	"time"
)

// LegacyGoogleIdentityProvider is the reserved user_identities.provider value
// for a Google subject recorded before providers had slugs (the old
// users.google_id). Only a Google-type provider may claim such a row. Provider
// slugs cannot contain an underscore, so it never collides with a real slug.
const LegacyGoogleIdentityProvider = "legacy_google_id"

// DevIdentityProvider is the user_identities.provider value of the
// localhost-only dev login. It is not a sign-in method outside local
// development.
const DevIdentityProvider = "dev"

// UserIdentity is one way a user can sign in: a (provider, subject) pair. A
// user may hold several.
type UserIdentity struct {
	ID       string `json:"id" db:"id"`
	UserID   string `json:"user_id" db:"user_id"`
	Provider string `json:"provider" db:"provider"`
	Subject  string `json:"subject" db:"subject"`
	// LastUsedAt is nil for an identity that was backfilled and has not signed
	// in since.
	LastUsedAt *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
}

// NormalizeEmail returns the form every users.email value is stored and looked
// up in: surrounding whitespace removed, lower case. The users_email_lowercase
// CHECK rejects anything else, so every write path must go through it.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
