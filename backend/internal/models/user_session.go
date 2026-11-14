package models

import "time"

// UserSession is the server-side row of a vx_session cookie session (#1646).
// The cookie carries an opaque session id; IDHash is its SHA-256, the only form
// in which the id is stored. The identity-provider tokens are held here
// encrypted (AccessTokenEnc, RefreshTokenEnc), never in the cookie; an empty
// value means the session has no such token.
type UserSession struct {
	IDHash          []byte
	UserID          string
	Provider        string
	AuthMethod      string
	AuthenticatedAt time.Time
	AccessTokenEnc  string
	RefreshTokenEnc string
	// AccessExpiresAt is the access token's expiry, or the session's own expiry
	// for a sliding session (one with no refresh token).
	AccessExpiresAt time.Time
	LastRefreshedAt *time.Time
	CreatedAt       time.Time
	// ExpiresAt is when the session stops authenticating; the sweep deletes the
	// row after it.
	ExpiresAt time.Time
}
