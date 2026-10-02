package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestManager(t *testing.T, isLocal bool) *Manager {
	t.Helper()
	mgr, err := NewManager("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f", isLocal)
	require.NoError(t, err)
	return mgr
}

// requestWithCookie returns a request carrying value under name.
func requestWithCookie(name, value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: name, Value: value})
	return r
}

func writtenCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %s cookie written", name)
	return nil
}

func TestSetupSession_RoundTrip(t *testing.T) {
	mgr := setupTestManager(t, false)
	want := &SetupSession{Generation: 7, ExpiresAt: time.Date(2026, 10, 2, 13, 0, 0, 0, time.UTC)}

	w := httptest.NewRecorder()
	require.NoError(t, mgr.WriteSetup(w, want))
	cookie := writtenCookie(t, w, SetupCookieName)

	got, err := mgr.ReadSetup(requestWithCookie(SetupCookieName, cookie.Value))
	require.NoError(t, err)
	assert.Equal(t, want.Generation, got.Generation)
	assert.True(t, want.ExpiresAt.Equal(got.ExpiresAt))
}

func TestSetupCookie_Attributes(t *testing.T) {
	for name, tc := range map[string]struct {
		isLocal    bool
		wantSecure bool
	}{
		"production":        {isLocal: false, wantSecure: true},
		"local development": {isLocal: true, wantSecure: false},
	} {
		t.Run(name, func(t *testing.T) {
			mgr := setupTestManager(t, tc.isLocal)
			cookie, err := mgr.SetupCookie(&SetupSession{Generation: 1, ExpiresAt: time.Now().Add(time.Hour)})
			require.NoError(t, err)

			assert.Equal(t, "vibexp_setup", cookie.Name)
			assert.NotEqual(t, CookieName, cookie.Name, "never the user session cookie")
			assert.Equal(t, "/", cookie.Path)
			assert.Equal(t, 3600, cookie.MaxAge)
			assert.True(t, cookie.HttpOnly)
			assert.Equal(t, tc.wantSecure, cookie.Secure)
			assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
			assert.NotContains(t, cookie.Value, "generation", "the payload is encrypted")
		})
	}
}

func TestClearSetup_ExpiresTheCookieWithTheSameAttributes(t *testing.T) {
	for name, tc := range map[string]struct {
		isLocal    bool
		wantSecure bool
	}{
		"production":        {isLocal: false, wantSecure: true},
		"local development": {isLocal: true, wantSecure: false},
	} {
		t.Run(name, func(t *testing.T) {
			mgr := setupTestManager(t, tc.isLocal)
			w := httptest.NewRecorder()
			mgr.ClearSetup(w)

			cookie := writtenCookie(t, w, SetupCookieName)
			assert.Empty(t, cookie.Value)
			assert.Equal(t, -1, cookie.MaxAge)
			assert.Equal(t, "/", cookie.Path)
			assert.True(t, cookie.HttpOnly)
			assert.Equal(t, tc.wantSecure, cookie.Secure)
			assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
		})
	}
}

func TestReadSetup_Rejections(t *testing.T) {
	mgr := setupTestManager(t, false)

	t.Run("no cookie", func(t *testing.T) {
		_, err := mgr.ReadSetup(httptest.NewRequest(http.MethodGet, "/", nil))
		assert.ErrorIs(t, err, ErrNoCookie)
	})

	t.Run("garbage", func(t *testing.T) {
		_, err := mgr.ReadSetup(requestWithCookie(SetupCookieName, "not-a-cookie"))
		assert.ErrorIs(t, err, ErrInvalidSession)
	})

	t.Run("sealed with another key", func(t *testing.T) {
		other, err := NewManager("ff0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f", false)
		require.NoError(t, err)
		cookie, err := other.SetupCookie(&SetupSession{Generation: 1, ExpiresAt: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		_, err = mgr.ReadSetup(requestWithCookie(SetupCookieName, cookie.Value))
		assert.ErrorIs(t, err, ErrInvalidSession)
	})

	t.Run("a payload that is not a setup session", func(t *testing.T) {
		sealed, err := mgr.encrypt([]byte(`"just a string"`), []byte(setupCookieAAD))
		require.NoError(t, err)
		_, err = mgr.ReadSetup(requestWithCookie(SetupCookieName, sealed))
		assert.ErrorIs(t, err, ErrInvalidSession)
	})
}

// The two cookies share one key, so each must refuse the other's ciphertext: a
// setup cookie can never be replayed as a user session, nor the reverse.
func TestSetupAndUserSessionCookiesAreNotInterchangeable(t *testing.T) {
	mgr := setupTestManager(t, false)

	setupCookie, err := mgr.SetupCookie(&SetupSession{Generation: 1, ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	_, err = mgr.Read(requestWithCookie(CookieName, setupCookie.Value))
	assert.ErrorIs(t, err, ErrInvalidSession, "a setup cookie's value is not a user session")

	w := httptest.NewRecorder()
	require.NoError(t, mgr.Write(w, &Session{UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)}))
	userCookie := writtenCookie(t, w, CookieName)
	_, err = mgr.ReadSetup(requestWithCookie(SetupCookieName, userCookie.Value))
	assert.ErrorIs(t, err, ErrInvalidSession, "a user session's value is not a setup session")

	// And the user session still reads back, so the domain separation changed
	// nothing for it.
	got, err := mgr.Read(requestWithCookie(CookieName, userCookie.Value))
	require.NoError(t, err)
	assert.Equal(t, "user-1", got.UserID)
}

func TestSetupSession_IsExpired(t *testing.T) {
	at := time.Date(2026, 10, 2, 13, 0, 0, 0, time.UTC)
	s := &SetupSession{ExpiresAt: at}
	assert.False(t, s.IsExpired(at.Add(-time.Nanosecond)))
	assert.True(t, s.IsExpired(at))
	assert.True(t, s.IsExpired(at.Add(time.Second)))
}
