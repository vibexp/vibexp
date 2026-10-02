package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	// SetupCookieName is the name of the setup-session cookie (#1236). It is a
	// different cookie from the user session on purpose: nothing that reads
	// vx_session ever looks at it.
	SetupCookieName = "vibexp_setup"

	// SetupSessionLifetime is how long a setup session lasts.
	SetupSessionLifetime = time.Hour

	// setupCookieAAD binds the ciphertext to the setup cookie. Both cookies are
	// sealed with the same key, so without it a setup cookie's value pasted into
	// vx_session (or the reverse) would still decrypt.
	setupCookieAAD = "vx-setup-session-v1"
)

// SetupSession is the decrypted payload of the setup cookie: a short-lived,
// USERLESS session issued in exchange for the one-time setup token. It names no
// user and carries no token, so it can never stand in for a user session; what
// it authorizes is decided by the server's setup guard alone.
type SetupSession struct {
	// Generation is the setup generation the session was issued at. The server
	// rejects the session once the stored generation has moved on.
	Generation int64 `json:"generation"`
	// ExpiresAt is when the session stops being valid.
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired reports whether the setup session has passed its expiry at now.
func (s *SetupSession) IsExpired(now time.Time) bool {
	return !now.Before(s.ExpiresAt)
}

// ReadSetup decrypts and returns the setup session from the request cookie.
// Returns ErrNoCookie when the cookie is absent and ErrInvalidSession when it
// does not decrypt as a setup session.
func (m *Manager) ReadSetup(r *http.Request) (*SetupSession, error) {
	cookie, err := r.Cookie(SetupCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, ErrNoCookie
		}
		return nil, fmt.Errorf("session: read setup cookie: %w", err)
	}

	plaintext, err := m.decrypt(cookie.Value, []byte(setupCookieAAD))
	if err != nil {
		return nil, ErrInvalidSession
	}

	var s SetupSession
	if err := json.Unmarshal(plaintext, &s); err != nil {
		return nil, ErrInvalidSession
	}
	return &s, nil
}

// SetupCookie returns the encrypted setup cookie for s. It is HttpOnly and
// SameSite=Strict: the setup page is same-origin, so the cookie never needs to
// ride a cross-site navigation.
func (m *Manager) SetupCookie(s *SetupSession) (*http.Cookie, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("session: marshal setup session: %w", err)
	}

	ciphertext, err := m.encrypt(payload, []byte(setupCookieAAD))
	if err != nil {
		return nil, fmt.Errorf("session: encrypt setup session: %w", err)
	}

	return m.setupCookie(ciphertext, int(SetupSessionLifetime/time.Second)), nil
}

// setupCookie is the one place the setup cookie's attributes are written, so
// the cookie that sets the session and the one that expires it can never drift
// apart: a browser only drops a cookie whose attributes match the original.
func (m *Manager) setupCookie(value string, maxAge int) *http.Cookie {
	// #nosec G124 -- same as Manager.Write: Secure comes from m.secure, which is
	// false only for local HTTP development and which G124 cannot evaluate. The
	// attributes are asserted in setup_test.go for both cases.
	return &http.Cookie{
		Name:     SetupCookieName,
		Value:    value,
		Path:     cookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteStrictMode,
	}
}

// WriteSetup encrypts s and sets the setup cookie on the response.
func (m *Manager) WriteSetup(w http.ResponseWriter, s *SetupSession) error {
	cookie, err := m.SetupCookie(s)
	if err != nil {
		return err
	}
	http.SetCookie(w, cookie)
	return nil
}

// ClearSetup expires the setup cookie immediately.
func (m *Manager) ClearSetup(w http.ResponseWriter) {
	http.SetCookie(w, m.setupCookie("", -1))
}
