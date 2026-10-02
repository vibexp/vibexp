package models

import "time"

// InstanceAuthSetup is the stored state of first-run authentication setup
// (#1236, epic #1230): the one-time setup token an operator exchanges for a
// setup session while the instance has no way to sign in.
//
// Only the token's SHA-256 is stored, and it is json:"-", so marshaling the row
// yields a credential-free audit snapshot. Whether setup mode is ACTIVE is not
// stored; services.SetupModeService computes it.
type InstanceAuthSetup struct {
	// TokenHash is the SHA-256 of the setup token; nil once consumed.
	TokenHash []byte `json:"-"`
	// ExpiresAt is when the token stops being exchangeable.
	ExpiresAt *time.Time `json:"expires_at"`
	// ConsumedAt is when a root instance admin's provider sign-in ended setup;
	// nil while the token is still outstanding.
	ConsumedAt *time.Time `json:"consumed_at"`
	// ConsumedBy is that root admin; nil when not consumed, and after the user
	// is deleted.
	ConsumedBy *string `json:"consumed_by"`
	// Rearmed is true while setup was deliberately re-armed and not yet
	// consumed: setup mode stays active even with a provider enabled.
	Rearmed bool `json:"rearmed"`
	// Generation is bumped by every mint and by consumption. A setup session
	// carries the generation it was issued at.
	Generation int64     `json:"generation"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// IsConsumed reports whether a root admin's sign-in has ended this setup.
func (s *InstanceAuthSetup) IsConsumed() bool {
	return s.ConsumedAt != nil
}

// HasLiveToken reports whether the row holds a token that can still be
// exchanged at now: one is stored, it is not consumed and it has not expired.
func (s *InstanceAuthSetup) HasLiveToken(now time.Time) bool {
	return len(s.TokenHash) > 0 && !s.IsConsumed() && s.ExpiresAt != nil && now.Before(*s.ExpiresAt)
}
