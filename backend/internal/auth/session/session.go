// Package session provides AES-GCM encrypted httpOnly cookie session management.
// The session cookie name is "vx_session" and carries an encrypted JSON payload
// containing the user's access token, refresh token, expiry, IDP subject, and
// internal user ID.
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

	// cookieMaxAge is the max-age for the session cookie (24 hours).
	// The cookie lifetime is deliberately generous; actual session validity
	// is governed by the access/refresh token expiry in the payload.
	cookieMaxAge = 24 * 60 * 60 // 24 hours in seconds
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
// for the value. If WorkOS access tokens grow large, we surface a
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
}

// IsExpired returns true when the access token has passed its expiry.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
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
// derive from the same WORKOS_COOKIE_PASSWORD secret.
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

	plaintext, err := m.decrypt(cookie.Value)
	if err != nil {
		return nil, ErrInvalidSession
	}

	var s Session
	if err := json.Unmarshal(plaintext, &s); err != nil {
		return nil, ErrInvalidSession
	}

	return &s, nil
}

// Write encrypts the session and sets the vx_session cookie on the response.
func (m *Manager) Write(w http.ResponseWriter, s *Session) error {
	// #nosec G117 - AccessToken is the session payload; it is encrypted with AES-GCM before
	// being written to the cookie. The token never touches the wire in plaintext.
	payload, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("session: marshal session: %w", err)
	}

	ciphertext, err := m.encrypt(payload)
	if err != nil {
		return fmt.Errorf("session: encrypt session: %w", err)
	}

	if len(ciphertext) > maxCookieValueBytes {
		return fmt.Errorf("%w: got %d bytes, limit %d", ErrCookieTooLarge, len(ciphertext), maxCookieValueBytes)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    ciphertext,
		Path:     cookiePath,
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Clear sets the session cookie to expire immediately, effectively logging
// the user out of the cookie-based session.
func (m *Manager) Clear(w http.ResponseWriter) {
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
// The nonce is randomly generated on every call.
func (m *Manager) encrypt(plaintext []byte) (string, error) {
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

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return hex.EncodeToString(nonce) + "." + hex.EncodeToString(ciphertext), nil
}

// decrypt reverses encrypt. It expects the format: hex(nonce) + "." + hex(ciphertext).
func (m *Manager) decrypt(encoded string) ([]byte, error) {
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

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("session: decrypt: %w", err)
	}

	return plaintext, nil
}
