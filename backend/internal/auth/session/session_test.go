package session

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPassword is a 64-char hex string → 32-byte AES-256-GCM key.
// #nosec G101 - Test credential, not a real secret
const testPassword = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	mgr, err := NewManager(testPassword, true /* local */)
	require.NoError(t, err)
	return mgr
}

func TestNewManager_Success(t *testing.T) {
	mgr, err := NewManager(testPassword, true)
	require.NoError(t, err)
	assert.NotNil(t, mgr)
	assert.False(t, mgr.secure, "local mode should set secure=false")
}

func TestNewManager_SecureInProduction(t *testing.T) {
	mgr, err := NewManager(testPassword, false /* not local */)
	require.NoError(t, err)
	assert.True(t, mgr.secure)
}

func TestNewManager_EmptyPassword(t *testing.T) {
	_, err := NewManager("", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cookie password is required")
}

func TestNewManager_InvalidHex(t *testing.T) {
	_, err := NewManager("not-valid-hex!", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "valid hex string")
}

func TestNewManager_WrongKeyLength(t *testing.T) {
	// 16 bytes → 32 hex chars (only 16 bytes, not 32)
	shortHex := "0102030405060708090a0b0c0d0e0f10"
	_, err := NewManager(shortHex, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exactly 32 bytes")
}

func TestWriteAndRead_RoundTrip(t *testing.T) {
	mgr := newTestManager(t)

	sess := &Session{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		ExpiresAt:    time.Now().Add(time.Hour),
		IDPSubject:   "oidc-sub-123",
		UserID:       "user-abc",
	}

	// Write session to response
	rw := httptest.NewRecorder()
	err := mgr.Write(rw, sess)
	require.NoError(t, err)

	// Verify cookie was set
	cookies := rw.Result().Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	assert.Equal(t, CookieName, c.Name)
	assert.True(t, c.HttpOnly)
	assert.False(t, c.Secure, "local mode should not set Secure flag")
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)

	// Read session from request
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	readSess, err := mgr.Read(req)
	require.NoError(t, err)

	assert.Equal(t, sess.AccessToken, readSess.AccessToken)
	assert.Equal(t, sess.RefreshToken, readSess.RefreshToken)
	assert.Equal(t, sess.IDPSubject, readSess.IDPSubject)
	assert.Equal(t, sess.UserID, readSess.UserID)
	// Time comparison with tolerance
	assert.WithinDuration(t, sess.ExpiresAt, readSess.ExpiresAt, time.Second)
}

func TestRead_NoCookie(t *testing.T) {
	mgr := newTestManager(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := mgr.Read(req)
	assert.ErrorIs(t, err, ErrNoCookie)
}

func TestRead_InvalidCiphertext(t *testing.T) {
	mgr := newTestManager(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// #nosec G101 - Test cookie value, not a real credential
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "deadbeef.invalidciphertext"})

	_, err := mgr.Read(req)
	assert.ErrorIs(t, err, ErrInvalidSession)
}

func TestRead_TamperedPayload(t *testing.T) {
	mgr := newTestManager(t)

	sess := &Session{
		AccessToken: "token",
		ExpiresAt:   time.Now().Add(time.Hour),
		UserID:      "user-123",
	}
	rw := httptest.NewRecorder()
	require.NoError(t, mgr.Write(rw, sess))
	c := rw.Result().Cookies()[0]

	// Tamper with the cookie value
	tampered := c.Value + "x"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: tampered})

	_, err := mgr.Read(req)
	assert.ErrorIs(t, err, ErrInvalidSession)
}

func TestClear_SetsMaxAgeNegative(t *testing.T) {
	mgr := newTestManager(t)

	rw := httptest.NewRecorder()
	mgr.Clear(rw)

	cookies := rw.Result().Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	assert.Equal(t, CookieName, c.Name)
	assert.Equal(t, "", c.Value)
	assert.Equal(t, -1, c.MaxAge)

	// The expiry write must carry the same attributes as Write or the browser
	// will not match the original cookie and the clear becomes a no-op. gosec's
	// G124 is suppressed on both writes (#553), so this is what guards them.
	assert.True(t, c.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, cookiePath, c.Path)
}

func TestSession_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"not expired", time.Now().Add(time.Hour), false},
		{"just expired", time.Now().Add(-time.Millisecond), true},
		{"expired 1h ago", time.Now().Add(-time.Hour), true},
		// A zero expiry is in the past. It must never mean "does not expire".
		{"zero expiry", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := &Session{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.want, sess.IsExpired())
		})
	}
}

func TestEncryptDecrypt_Idempotent(t *testing.T) {
	mgr := newTestManager(t)
	plaintext := []byte(`{"user_id":"test","access_token":"tok","expires_at":"2099-01-01T00:00:00Z"}`)

	ciphertext, err := mgr.encrypt(plaintext, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)

	decrypted, err := mgr.decrypt(ciphertext, nil)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncrypt_ProducesUniqueValues(t *testing.T) {
	mgr := newTestManager(t)
	plaintext := []byte("same plaintext")

	c1, err := mgr.encrypt(plaintext, nil)
	require.NoError(t, err)

	c2, err := mgr.encrypt(plaintext, nil)
	require.NoError(t, err)

	// Each encryption uses a fresh random nonce → different ciphertext
	assert.NotEqual(t, c1, c2)
}

func TestDecrypt_MissingDotSeparator(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.decrypt("nodotinvalue", nil)
	assert.Error(t, err)
}

func TestDecrypt_InvalidNonceHex(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.decrypt("notvalidhex.body", nil)
	assert.Error(t, err)
}

func TestDecrypt_InvalidCiphertextHex(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.decrypt("0102030405060708090a0b0c.notvalidhex", nil)
	assert.Error(t, err)
}

func TestSecureFlag_Production(t *testing.T) {
	mgr, err := NewManager(testPassword, false /* production */)
	require.NoError(t, err)

	sess := &Session{
		AccessToken: "token",
		ExpiresAt:   time.Now().Add(time.Hour),
		UserID:      "user-1",
	}

	rw := httptest.NewRecorder()
	require.NoError(t, mgr.Write(rw, sess))

	c := rw.Result().Cookies()[0]
	assert.True(t, c.Secure, "Secure flag should be set in production mode")
}

func TestWrite_RejectsOversizedCookie(t *testing.T) {
	mgr := newTestManager(t)

	// Build a session whose encoded ciphertext exceeds the safe browser
	// cookie limit. AES-GCM doesn't expand the plaintext, but hex encoding
	// doubles the size — so a 2KB token plus structural overhead easily
	// produces >3500 bytes encoded.
	huge := make([]byte, 2500)
	for i := range huge {
		huge[i] = 'A'
	}
	sess := &Session{
		AccessToken: string(huge),
		ExpiresAt:   time.Now().Add(time.Hour),
		UserID:      "user-1",
	}

	rw := httptest.NewRecorder()
	err := mgr.Write(rw, sess)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCookieTooLarge)
}

func TestDeriveStateMACKey_DistinctFromMasterKey(t *testing.T) {
	mgr := newTestManager(t)

	stateKey := mgr.DeriveStateMACKey()
	assert.Len(t, stateKey, 32, "derived state key should be 32 bytes (SHA-256 output)")
	assert.NotEqual(t, mgr.key, stateKey, "state MAC key must differ from AES-GCM master key")

	// Stable across calls
	stateKey2 := mgr.DeriveStateMACKey()
	assert.Equal(t, stateKey, stateKey2, "DeriveStateMACKey must be deterministic")
}

func TestDeriveStateMACKey_DiffersAcrossManagers(t *testing.T) {
	mgr1 := newTestManager(t)

	otherPassword := "0202020202020202020202020202020202020202020202020202020202020202"
	mgr2, err := NewManager(otherPassword, true)
	require.NoError(t, err)

	assert.NotEqual(t, mgr1.DeriveStateMACKey(), mgr2.DeriveStateMACKey())
}

// TestRead_PayloadFromBeforeEpochs decodes the exact JSON a cookie minted before
// #1428 carries: no epoch, no sign-in method, no sign-in time. It must decode,
// with the epoch reading as 0 (the users.session_epoch default), so the upgrade
// signs nobody out.
func TestRead_PayloadFromBeforeEpochs(t *testing.T) {
	mgr := newTestManager(t)
	legacy := []byte(`{"access_token":"tok","refresh_token":"rt","expires_at":"2099-01-01T00:00:00Z",` +
		`"idp_subject":"sub","user_id":"u-1","provider":"google"}`)
	value, err := mgr.encrypt(legacy, nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: value})
	sess, err := mgr.Read(req)

	require.NoError(t, err)
	assert.Equal(t, "u-1", sess.UserID)
	assert.Equal(t, int64(0), sess.Epoch)
	assert.Empty(t, sess.AuthMethod)
	assert.True(t, sess.AuthenticatedAt.IsZero())
}

func TestWriteAndRead_RoundTripsEpochMethodAndSignInTime(t *testing.T) {
	mgr := newTestManager(t)
	signedIn := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	rec := httptest.NewRecorder()
	require.NoError(t, mgr.Write(rec, &Session{
		UserID: "u-1", AccessToken: "tok", ExpiresAt: time.Now().Add(time.Hour),
		AuthMethod: AuthMethodPassword, AuthenticatedAt: signedIn, Epoch: 7,
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	sess, err := mgr.Read(req)

	require.NoError(t, err)
	assert.Equal(t, AuthMethodPassword, sess.AuthMethod)
	assert.True(t, signedIn.Equal(sess.AuthenticatedAt))
	assert.Equal(t, int64(7), sess.Epoch)
}

func TestSession_IsSliding(t *testing.T) {
	assert.True(t, (&Session{}).IsSliding(), "no refresh token: nothing but activity can keep it alive")
	assert.False(t, (&Session{RefreshToken: "rt"}).IsSliding())
}

func TestSession_StartSliding(t *testing.T) {
	now := time.Now()

	t.Run("a refresh-less session gets the sliding lifetime whatever expiry it had", func(t *testing.T) {
		for name, had := range map[string]time.Time{
			"zero (GitHub)":         {},
			"one hour (OIDC token)": now.Add(time.Hour),
		} {
			t.Run(name, func(t *testing.T) {
				sess := &Session{ExpiresAt: had}
				sess.StartSliding(now)
				assert.Equal(t, now.Add(SlidingLifetime), sess.ExpiresAt)
			})
		}
	})

	t.Run("a session with a refresh token keeps the provider's expiry", func(t *testing.T) {
		expiry := now.Add(time.Hour)
		sess := &Session{RefreshToken: "rt", ExpiresAt: expiry}
		sess.StartSliding(now)
		assert.Equal(t, expiry, sess.ExpiresAt)
	})
}

func TestSession_Slide(t *testing.T) {
	now := time.Now()
	// issued returns a sliding session last issued `age` ago.
	issued := func(age time.Duration) *Session {
		return &Session{ExpiresAt: now.Add(-age).Add(SlidingLifetime)}
	}

	tests := []struct {
		name string
		sess *Session
		want bool
	}{
		{"just issued", issued(0), false},
		{"issued a minute short of a day ago", issued(slidingReissueInterval - time.Minute), false},
		{"issued exactly a day ago", issued(slidingReissueInterval), true},
		{"issued 29 days ago", issued(29 * 24 * time.Hour), true},
		{"expires this instant", issued(SlidingLifetime), true},
		{"expired a second ago", issued(SlidingLifetime + time.Second), false},
		{"zero expiry", &Session{}, false},
		{"has a refresh token", &Session{RefreshToken: "rt", ExpiresAt: now.Add(time.Minute)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.sess.ExpiresAt

			got := tt.sess.Slide(now)

			assert.Equal(t, tt.want, got)
			if tt.want {
				assert.Equal(t, now.Add(SlidingLifetime), tt.sess.ExpiresAt)
			} else {
				assert.Equal(t, before, tt.sess.ExpiresAt, "a session that does not slide must be left untouched")
			}
		})
	}
}

// TestWrite_CookieMaxAgeFollowsTheSessionKind pins the cookie lifetime to the
// session's: a sliding session needs a cookie that outlives an idle day, and a
// refreshable session keeps the 24-hour cookie it always had.
func TestWrite_CookieMaxAgeFollowsTheSessionKind(t *testing.T) {
	mgr := newTestManager(t)
	maxAgeOf := func(sess *Session) int {
		rec := httptest.NewRecorder()
		require.NoError(t, mgr.Write(rec, sess))
		cookies := rec.Result().Cookies()
		require.Len(t, cookies, 1)
		return cookies[0].MaxAge
	}

	assert.Equal(t, 30*24*60*60, maxAgeOf(&Session{UserID: "u", ExpiresAt: time.Now().Add(SlidingLifetime)}))
	assert.Equal(t, 24*60*60,
		maxAgeOf(&Session{UserID: "u", RefreshToken: "rt", ExpiresAt: time.Now().Add(time.Hour)}))
}

// Cookie returns exactly the cookie Write sets, so a handler that sends it as a
// header itself issues the same session.
func TestCookie_IsWhatWriteSets(t *testing.T) {
	mgr := newTestManager(t)
	sess := &Session{UserID: "u-1", AccessToken: "tok", ExpiresAt: time.Now().Add(SlidingLifetime), Epoch: 3}

	cookie, err := mgr.Cookie(sess)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	require.NoError(t, mgr.Write(rec, sess))
	written := rec.Result().Cookies()
	require.Len(t, written, 1)

	assert.Equal(t, CookieName, cookie.Name)
	assert.Equal(t, written[0].MaxAge, cookie.MaxAge)
	assert.Equal(t, written[0].Path, cookie.Path)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)

	// The header form round-trips through a browser-style request.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cookie", CookieName+"="+cookie.Value)
	got, err := mgr.Read(req)
	require.NoError(t, err)
	assert.Equal(t, "u-1", got.UserID)
	assert.Equal(t, int64(3), got.Epoch)
}

func TestCookie_RejectsOversizedSession(t *testing.T) {
	mgr := newTestManager(t)

	cookie, err := mgr.Cookie(&Session{UserID: "u-1", AccessToken: strings.Repeat("a", maxCookieValueBytes)})

	require.ErrorIs(t, err, ErrCookieTooLarge)
	assert.Nil(t, cookie)
}
