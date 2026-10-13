// Package session provides AES-GCM encrypted httpOnly cookie session management.
// The session cookie name is "vx_session" and carries an encrypted JSON payload
// containing the user's access token, refresh token, expiry, IDP subject,
// internal user ID, sign-in method and time, and session epoch.
package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// CookieName is the name of the session cookie.
	CookieName = "vx_session"

	// cookiePath is the Path attribute for the session cookie.
	cookiePath = "/"

	// cookieMaxAge is the max-age for the cookie of a session that carries a
	// refresh token (24 hours). The cookie lifetime is deliberately generous;
	// actual session validity is governed by the access/refresh token expiry in
	// the payload.
	cookieMaxAge = 24 * 60 * 60 // 24 hours in seconds

	// SlidingLifetime is how long a sliding session (one with no refresh token,
	// see Session.IsSliding) stays valid after it was last issued (#1428).
	SlidingLifetime = 30 * 24 * time.Hour

	// slidingReissueInterval is the least time between two re-issues of a
	// sliding session: an active user gets a fresh cookie at most once a day,
	// not on every request.
	slidingReissueInterval = 24 * time.Hour

	// slidingCookieMaxAge is the max-age of a sliding session's cookie. It must
	// cover SlidingLifetime: with the 24-hour cookieMaxAge the browser would drop
	// the cookie after one idle day, long before the session itself expires.
	slidingCookieMaxAge = int(SlidingLifetime / time.Second)
)

// Sign-in methods recorded in Session.AuthMethod (#1428).
const (
	// AuthMethodOAuth is a sign-in through an identity provider; Session.Provider
	// names which one.
	AuthMethodOAuth = "oauth"
	// AuthMethodDev is the localhost-only dev login.
	AuthMethodDev = "dev"
	// AuthMethodPassword is an email and password sign-in.
	AuthMethodPassword = "password" // gitleaks:allow - the name of a sign-in method, not a credential
)

// ErrNoCookie is returned by Read when no session cookie is present.
var ErrNoCookie = errors.New("session: no session cookie")

// ErrInvalidSession is returned when the session cookie cannot be decrypted
// or its payload is malformed.
var ErrInvalidSession = errors.New("session: invalid or tampered session cookie")

// ErrCookieTooLarge is returned when the encoded ciphertext exceeds the
// safe browser cookie limit. Set conservatively below the 4096 byte hard
// limit to leave headroom for the cookie name and attributes.
var ErrCookieTooLarge = errors.New("session: encoded session exceeds maximum cookie size")

// maxCookieValueBytes is the largest acceptable encoded cookie value.
// Browsers enforce a per-cookie limit around 4096 bytes (name + value +
// attributes). Reserving ~600 bytes for name and attributes leaves ~3500
// for the value. If provider access tokens grow large, we surface a
// concrete error instead of silently dropping the cookie.
const maxCookieValueBytes = 3500

// Session represents the decrypted payload stored inside the session cookie.
type Session struct {
	// AccessToken is the IDP-issued access token.
	AccessToken string `json:"access_token"`
	// RefreshToken is the IDP-issued refresh token (may be empty for short-lived sessions).
	RefreshToken string `json:"refresh_token"`
	// ExpiresAt is when the access token expires.
	ExpiresAt time.Time `json:"expires_at"`
	// IDPSubject is the stable, unique provider-specific user identifier.
	IDPSubject string `json:"idp_subject"`
	// UserID is the internal application user ID.
	UserID string `json:"user_id"`
	// Provider is the canonical name of the identity provider that issued the
	// session (e.g. "google", "github"). It routes token refresh to the right
	// provider. Empty for sessions created before multi-provider support and
	// for dev login; the refresh path tolerates an empty value.
	Provider string `json:"provider,omitempty"`
	// AuthMethod is how the user signed in: one of the AuthMethod* constants.
	// Empty for a session minted before #1428.
	AuthMethod string `json:"auth_method,omitempty"`
	// AuthenticatedAt is when the user actually signed in. A refresh or a
	// sliding re-issue does not move it, so it answers "was this a recent
	// sign-in". Zero for a session minted before #1428, which therefore never
	// counts as recent.
	AuthenticatedAt time.Time `json:"authenticated_at"`
	// Epoch is the user's session_epoch at the time the cookie was minted. The
	// authentication chokepoint rejects the session when it differs from the
	// user row, which is how a user's sessions are revoked. A session minted
	// before #1428 has no epoch and decodes as 0, the column's default.
	Epoch int64 `json:"epoch,omitempty"`
}

// IsExpired returns true when the access token has passed its expiry. A zero
// ExpiresAt is in the past, so it reads as expired: it never means "does not
// expire".
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsSliding reports whether the session has no refresh token and so cannot be
// kept alive by an identity provider. Such a session lives for SlidingLifetime
// after it was last issued and is extended by activity (Slide). That is every
// dev-login and password session, and a provider session whose provider issued
// no refresh token (GitHub never does).
func (s *Session) IsSliding() bool {
	return s.RefreshToken == ""
}

// StartSliding sets a sliding session's expiry to SlidingLifetime from now. A
// mint site calls it for a session with no refresh token, whatever expiry the
// provider reported for its access token: that token is never presented to the
// provider again, so its expiry says nothing about the session. It is a no-op
// for a session with a refresh token.
func (s *Session) StartSliding(now time.Time) {
	if s.IsSliding() {
		s.ExpiresAt = now.Add(SlidingLifetime)
	}
}

// Slide extends an unexpired sliding session to SlidingLifetime from now and
// reports whether it did, in which case the caller must write the cookie again.
// It extends only when the session was last issued more than
// slidingReissueInterval ago, so an active user is re-issued a cookie at most
// once a day. An expired session is never revived, and a session with a refresh
// token is left to the refresh path.
func (s *Session) Slide(now time.Time) bool {
	if !s.IsSliding() || now.After(s.ExpiresAt) {
		return false
	}
	if s.ExpiresAt.Sub(now) > SlidingLifetime-slidingReissueInterval {
		return false
	}
	s.ExpiresAt = now.Add(SlidingLifetime)
	return true
}

// maxAge returns the Max-Age of the cookie that carries the session.
func (s *Session) maxAge() int {
	if s.IsSliding() {
		return slidingCookieMaxAge
	}
	return cookieMaxAge
}

// Manager handles reading and writing the encrypted session cookie.
// It is safe for concurrent use.
type Manager struct {
	key    []byte // 32-byte AES-256-GCM key
	secure bool   // Secure flag on the Set-Cookie header
}

// NewManager constructs a Manager from the hex-encoded cookie password.
// cookiePassword must be a hex string that decodes to exactly 32 bytes.
// isLocal should be true when running in local development (drops the Secure flag).
func NewManager(cookiePassword string, isLocal bool) (*Manager, error) {
	if cookiePassword == "" {
		return nil, fmt.Errorf("session: cookie password is required")
	}

	key, err := hex.DecodeString(cookiePassword)
	if err != nil {
		return nil, fmt.Errorf("session: cookie password must be a valid hex string: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("session: cookie password must decode to exactly 32 bytes (got %d)", len(key))
	}

	return &Manager{key: key, secure: !isLocal}, nil
}

// stateMACDomain is a domain-separation tag used when deriving the HMAC
// key for the OAuth state cookie. Mixing this label into the derived key
// ensures the AES-GCM session key and the state HMAC key are different,
// satisfying NIST SP 800-108 key-separation guidance even though both
// derive from the same SESSION_ENCRYPTION_KEY secret.
const stateMACDomain = "vx-state-mac-v1"

// DeriveStateMACKey returns a 32-byte key derived from the manager's
// master key and the state-cookie domain-separation tag. Used by callers
// that need to HMAC-sign OAuth state without re-using the AES-GCM key.
func (m *Manager) DeriveStateMACKey() []byte {
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(stateMACDomain))
	return mac.Sum(nil)
}

// Read decrypts and returns the session from the request cookie.
// Returns ErrNoCookie when the cookie is absent, ErrInvalidSession on
// decryption or unmarshal failure.
func (m *Manager) Read(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, ErrNoCookie
		}
		return nil, fmt.Errorf("session: read cookie: %w", err)
	}

	plaintext, err := m.decrypt(cookie.Value, nil)
	if err != nil {
		return nil, ErrInvalidSession
	}

	var s Session
	if err := json.Unmarshal(plaintext, &s); err != nil {
		return nil, ErrInvalidSession
	}

	return &s, nil
}

// Cookie encrypts the session and returns the vx_session cookie that carries
// it. Write is the usual way to set it; a handler with no ResponseWriter of its
// own (a generated strict-server operation returns its headers as data) sends
// Cookie(...).String() as the Set-Cookie header instead.
func (m *Manager) Cookie(s *Session) (*http.Cookie, error) {
	// #nosec G117 - AccessToken is the session payload; it is encrypted with AES-GCM before
	// being written to the cookie. The token never touches the wire in plaintext.
	payload, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("session: marshal session: %w", err)
	}

	ciphertext, err := m.encrypt(payload, nil)
	if err != nil {
		return nil, fmt.Errorf("session: encrypt session: %w", err)
	}

	if len(ciphertext) > maxCookieValueBytes {
		return nil, fmt.Errorf("%w: got %d bytes, limit %d", ErrCookieTooLarge, len(ciphertext), maxCookieValueBytes)
	}

	// #nosec G124 -- HttpOnly and SameSite are set below and Secure is set from
	// m.secure, which is false only for local HTTP development. G124 requires a
	// literal `Secure: true` and cannot see through the variable; hard-coding it
	// would break local dev and the localhost e2e stack. The attributes are
	// asserted in session_test.go for both the local and production cases.
	return &http.Cookie{
		Name:     CookieName,
		Value:    ciphertext,
		Path:     cookiePath,
		MaxAge:   s.maxAge(),
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

// Write encrypts the session and sets the vx_session cookie on the response.
func (m *Manager) Write(w http.ResponseWriter, s *Session) error {
	cookie, err := m.Cookie(s)
	if err != nil {
		return err
	}
	http.SetCookie(w, cookie)
	return nil
}

// Clear sets the session cookie to expire immediately, effectively logging
// the user out of the cookie-based session.
func (m *Manager) Clear(w http.ResponseWriter) {
	// #nosec G124 -- same as Write: Secure comes from m.secure, which G124 cannot
	// evaluate. This is the expiry write (MaxAge -1) and carries the identical
	// attribute set so the browser matches and drops the original cookie —
	// asserted in TestClear_SetsMaxAgeNegative.
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// encrypt seals plaintext with AES-256-GCM. The output is:
//
//	hex(nonce) + "." + hex(ciphertext)
//
// The nonce is randomly generated on every call. aad is authenticated but not
// encrypted: a value sealed under one aad does not open under another, which is
// what keeps one cookie kind from being replayed as a different one. The user
// session passes nil.
func (m *Manager) encrypt(plaintext, aad []byte) (string, error) {
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", fmt.Errorf("session: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("session: create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("session: generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	return hex.EncodeToString(nonce) + "." + hex.EncodeToString(ciphertext), nil
}

// decrypt reverses encrypt. It expects the format: hex(nonce) + "." + hex(ciphertext),
// sealed under the same aad.
func (m *Manager) decrypt(encoded string, aad []byte) ([]byte, error) {
	dotIdx := strings.IndexByte(encoded, '.')
	if dotIdx < 0 {
		return nil, fmt.Errorf("session: invalid encoded format")
	}

	nonce, err := hex.DecodeString(encoded[:dotIdx])
	if err != nil {
		return nil, fmt.Errorf("session: decode nonce: %w", err)
	}

	ciphertext, err := hex.DecodeString(encoded[dotIdx+1:])
	if err != nil {
		return nil, fmt.Errorf("session: decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(m.key)
	if err != nil {
		return nil, fmt.Errorf("session: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("session: create GCM: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("session: nonce size mismatch: got %d, want %d", len(nonce), gcm.NonceSize())
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("session: decrypt: %w", err)
	}

	return plaintext, nil
}
