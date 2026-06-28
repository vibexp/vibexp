package session

import (
	"net/http"
	"net/http/httptest"
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

	ciphertext, err := mgr.encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)

	decrypted, err := mgr.decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncrypt_ProducesUniqueValues(t *testing.T) {
	mgr := newTestManager(t)
	plaintext := []byte("same plaintext")

	c1, err := mgr.encrypt(plaintext)
	require.NoError(t, err)

	c2, err := mgr.encrypt(plaintext)
	require.NoError(t, err)

	// Each encryption uses a fresh random nonce → different ciphertext
	assert.NotEqual(t, c1, c2)
}

func TestDecrypt_MissingDotSeparator(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.decrypt("nodotinvalue")
	assert.Error(t, err)
}

func TestDecrypt_InvalidNonceHex(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.decrypt("notvalidhex.body")
	assert.Error(t, err)
}

func TestDecrypt_InvalidCiphertextHex(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.decrypt("0102030405060708090a0b0c.notvalidhex")
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
