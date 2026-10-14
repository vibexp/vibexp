package models

import "time"

// Purposes of a one-time account link. The values are stored in
// account_tokens.purpose (its CHECK lists them) and published in the API;
// change all three together.
const (
	// AccountTokenPurposeSetup creates the account's first password.
	AccountTokenPurposeSetup = "setup"
	// AccountTokenPurposeReset replaces a forgotten password.
	AccountTokenPurposeReset = "reset"
)

// How a one-time account link reached its holder. The values are stored in
// account_tokens.delivery; its CHECK lists them.
const (
	// AccountTokenDeliveryEmail is a link sent to the account's mailbox.
	// Completing it proves the mailbox.
	AccountTokenDeliveryEmail = "email"
	// AccountTokenDeliveryLink is a link an administrator copied by hand. It
	// proves nothing about who holds it.
	AccountTokenDeliveryLink = "link"
)

// Lifetimes of a one-time account link (epic #1426, decision 14).
const (
	// AccountSetupLinkLifetime is how long a setup link stays usable, however
	// it was delivered: an invitation may sit in an inbox for days.
	AccountSetupLinkLifetime = 7 * 24 * time.Hour
	// AccountResetEmailLinkLifetime is how long an emailed reset link stays
	// usable. Short, because anyone who reads the mailbox can use it.
	AccountResetEmailLinkLifetime = time.Hour
	// AccountResetCopiedLinkLifetime is how long a reset link an administrator
	// generated stays usable: long enough to hand it over out of band.
	AccountResetCopiedLinkLifetime = 24 * time.Hour
)

// ValidAccountTokenPurpose reports whether purpose is a link purpose.
func ValidAccountTokenPurpose(purpose string) bool {
	return purpose == AccountTokenPurposeSetup || purpose == AccountTokenPurposeReset
}

// ValidAccountTokenDelivery reports whether delivery is a delivery channel.
func ValidAccountTokenDelivery(delivery string) bool {
	return delivery == AccountTokenDeliveryEmail || delivery == AccountTokenDeliveryLink
}

// AccountTokenLifetime returns how long a link of the given purpose and
// delivery stays usable. Callers cannot choose a lifetime: it follows from what
// the link is for and how it travels.
func AccountTokenLifetime(purpose, delivery string) time.Duration {
	switch {
	case purpose == AccountTokenPurposeSetup:
		return AccountSetupLinkLifetime
	case delivery == AccountTokenDeliveryEmail:
		return AccountResetEmailLinkLifetime
	default:
		return AccountResetCopiedLinkLifetime
	}
}

// AccountToken is one issued one-time account link (#1432). Only the SHA-256 of
// its token is ever held here; the token exists in the link alone. The hash is
// never serialized.
type AccountToken struct {
	ID       string `json:"id" db:"id"`
	UserID   string `json:"user_id" db:"user_id"`
	Purpose  string `json:"purpose" db:"purpose"`
	Delivery string `json:"delivery" db:"delivery"`
	// TokenHash is the SHA-256 of the token.
	TokenHash []byte    `json:"-" db:"token_hash"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	// ConsumedAt is set once, by the request that completed the link.
	ConsumedAt *time.Time `json:"consumed_at,omitempty" db:"consumed_at"`
	// CreatedBy is the administrator who issued the link; nil for a link the
	// system issued, or once that user is deleted.
	CreatedBy *string   `json:"created_by,omitempty" db:"created_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// IsLiveAt reports whether the link can still be completed at now: it was not
// consumed and has not expired. A link expires AT its expiry time, not after.
func (t *AccountToken) IsLiveAt(now time.Time) bool {
	return t.ConsumedAt == nil && now.Before(t.ExpiresAt)
}

// ProvesMailbox reports whether completing the link proves that its holder
// reads the account's mailbox: true only for a link that was emailed there.
func (t *AccountToken) ProvesMailbox() bool {
	return t.Delivery == AccountTokenDeliveryEmail
}
