package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Server-side sessions (#1646): every sign-in stores a row, every cookie
// request needs it, and the provider tokens live in it and not in the cookie.

// sessionStoreContainer overrides the session store of any container.
type sessionStoreContainer struct {
	container.Container
	store services.SessionStoreInterface
}

func (c sessionStoreContainer) SessionStore() services.SessionStoreInterface { return c.store }

// withOwnSessionStore gives srv a session store of its own and returns its
// rows, so a test can look at exactly what was stored.
func withOwnSessionStore(srv *Server) *memSessionRepo {
	store, repo := newMemSessionStore()
	srv.container = sessionStoreContainer{Container: srv.container, store: store}
	return repo
}

// decryptedRowTokens returns the plaintext tokens of row.
func decryptedRowTokens(t *testing.T, row models.UserSession) (access, refresh string) {
	t.Helper()
	enc, err := services.NewEncryptionService(testSessionEncryptionKey)
	require.NoError(t, err)
	if row.AccessTokenEnc != "" {
		access, err = enc.Decrypt(row.AccessTokenEnc)
		require.NoError(t, err)
	}
	if row.RefreshTokenEnc != "" {
		refresh, err = enc.Decrypt(row.RefreshTokenEnc)
		require.NoError(t, err)
	}
	return access, refresh
}

// readCookie decrypts a vx_session cookie the way a request would.
func readCookie(t *testing.T, srv *Server, c *http.Cookie) *sesslib.Session {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	sess, err := srv.sessionManager.Read(req)
	require.NoError(t, err)
	return sess
}

func expectLoginActivity(f *epochFixture) {
	f.container.activityService.On("RecordAuthActivity", mock.Anything, epochUserID,
		activities.ActivityTypeAuthLogin, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
}

func TestSessionRows_ProviderSignInStoresTheTokensServerSide(t *testing.T) {
	f := newEpochFixture(t, 3)
	rows := withOwnSessionStore(f.srv)
	expectLoginActivity(f)
	user := &models.User{ID: epochUserID, Email: "o@example.com", Status: models.UserStatusActive, SessionEpoch: 3}
	expiry := time.Now().Add(time.Hour)

	rec := httptest.NewRecorder()
	f.srv.handleCallbackSuccess(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil),
		user, &idp.Tokens{AccessToken: "provider-at", RefreshToken: "provider-rt", ExpiresAt: expiry}, "state", "oidc", false)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, 1, rows.count(), "a sign-in stores one session row")
	row := rows.only()
	assert.Equal(t, epochUserID, row.UserID)
	assert.Equal(t, "oidc", row.Provider)
	assert.Equal(t, sesslib.AuthMethodOAuth, row.AuthMethod)
	assert.NotEqual(t, "provider-at", row.AccessTokenEnc, "tokens are stored encrypted")
	access, refresh := decryptedRowTokens(t, row)
	assert.Equal(t, "provider-at", access)
	assert.Equal(t, "provider-rt", refresh)

	minted := readCookie(t, f.srv, sessionCookieIn(rec))
	assert.NotEmpty(t, minted.ID)
	assert.Empty(t, minted.AccessToken, "the cookie carries no access token")
	assert.Empty(t, minted.RefreshToken, "the cookie carries no refresh token")
	assert.Equal(t, int64(3), minted.Epoch)

	// The next request gets the tokens back from the row.
	_, ctx, called := f.requireAuth(t, sessionCookieIn(rec))
	require.True(t, called)
	assert.Equal(t, "provider-rt", authenticatedSession(ctx).RefreshToken)
	assert.Equal(t, "provider-at", authenticatedSession(ctx).AccessToken)
}

func TestSessionRows_DevSignInStoresARow(t *testing.T) {
	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	expectLoginActivity(f)

	rec := httptest.NewRecorder()
	f.srv.handleDevLoginSuccess(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev/login", nil),
		&models.User{ID: epochUserID, Email: "dev@example.com"})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, rows.count())
	assert.Equal(t, sesslib.AuthMethodDev, rows.only().AuthMethod)
	assert.Empty(t, rows.only().RefreshTokenEnc, "a dev session is sliding")
	assert.Empty(t, readCookie(t, f.srv, sessionCookieIn(rec)).AccessToken)
}

func TestSessionRows_PasswordSignInStoresARow(t *testing.T) {
	f := newPwLoginFixture(t)
	rows := withOwnSessionStore(f.srv)
	f.expectActivity(activities.ActivityTypeAuthLogin)

	login := f.post(t, pwLoginEmail, pwLoginPassword)

	require.Equal(t, http.StatusOK, login.Code, login.Body.String())
	require.Equal(t, 1, rows.count())
	assert.Equal(t, sesslib.AuthMethodPassword, rows.only().AuthMethod)
	assert.Equal(t, pwLoginUserID, rows.only().UserID)
	assert.NotEmpty(t, readCookie(t, f.srv, sessionCookieIn(login)).ID)
}

func TestSessionRows_NoStoreFailsTheSignInClosed(t *testing.T) {
	f := newEpochFixture(t, 0)
	f.srv.container = sessionStoreContainer{Container: f.srv.container, store: nil}
	f.container.activityService.On("RecordAuthActivity", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	rec := httptest.NewRecorder()
	f.srv.handleDevLoginSuccess(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev/login", nil),
		&models.User{ID: epochUserID, Email: "dev@example.com"})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Nil(t, sessionCookieIn(rec), "no cookie without a row")
}

func TestSessionRows_RowIsRequired(t *testing.T) {
	t.Run("a deleted row no longer authenticates and the cookie is cleared", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		withOwnSessionStore(f.srv)
		sess := slidingSession(0, time.Hour)
		cookie := storedSessionCookie(t, f.srv, sess)
		require.NoError(t, f.srv.sessionStore().Delete(context.Background(), sess.ID))

		rec, _, called := f.requireAuth(t, cookie)

		assert.False(t, called)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		cleared := sessionCookieIn(rec)
		require.NotNil(t, cleared)
		assert.Negative(t, cleared.MaxAge)
	})

	t.Run("a row that cannot be read is a 500 and keeps the cookie", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		rows := withOwnSessionStore(f.srv)
		cookie := storedSessionCookie(t, f.srv, slidingSession(0, time.Hour))
		rows.setFail(errors.New("db down"))

		rec, _, called := f.requireAuth(t, cookie)

		assert.False(t, called)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "an outage is not a sign-out")
		assert.Nil(t, sessionCookieIn(rec))
	})

	t.Run("the optional path treats a missing row as anonymous", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		withOwnSessionStore(f.srv)
		sess := slidingSession(0, time.Hour)
		cookie := storedSessionCookie(t, f.srv, sess)
		require.NoError(t, f.srv.sessionStore().Delete(context.Background(), sess.ID))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
		req.AddCookie(cookie)
		var user any
		f.srv.optionalAuthMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			user = r.Context().Value(contextkeys.UserID)
		})).ServeHTTP(httptest.NewRecorder(), req)

		assert.Nil(t, user)
	})

	t.Run("the row's tokens win over a stale cookie", func(t *testing.T) {
		// Another request refreshed the session after this cookie was written:
		// the row holds unexpired tokens, so this request uses them and never
		// presents the spent refresh token (the fixture's AuthService has no
		// RefreshTokens expectation).
		f := newEpochFixture(t, 0)
		withOwnSessionStore(f.srv)
		sess := &sesslib.Session{
			UserID: epochUserID, AccessToken: "old-at", RefreshToken: "spent-rt", Provider: "oidc",
			ExpiresAt: time.Now().Add(-time.Minute), AuthMethod: sesslib.AuthMethodOAuth,
		}
		cookie := storedSessionCookie(t, f.srv, sess)
		refreshed := *sess
		refreshed.AccessToken, refreshed.RefreshToken = "fresh-at", "fresh-rt"
		refreshed.ExpiresAt = time.Now().Add(time.Hour)
		require.NoError(t, f.srv.sessionStore().SaveTokens(context.Background(), &refreshed))

		_, ctx, called := f.requireAuth(t, cookie)

		require.True(t, called)
		assert.Equal(t, "fresh-at", authenticatedSession(ctx).AccessToken)
		assert.Equal(t, "fresh-rt", authenticatedSession(ctx).RefreshToken)
	})
}

func TestSessionRows_RefreshRotatesTheTokensInTheRow(t *testing.T) {
	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	newExpiry := time.Now().Add(time.Hour)
	f.container.authService.On("RefreshTokens", mock.Anything, "oidc", "rt-1").
		Return(&idp.Tokens{AccessToken: "at-2", RefreshToken: "rt-2", ExpiresAt: newExpiry}, nil).Once()
	cookie := storedSessionCookie(t, f.srv, &sesslib.Session{
		UserID: epochUserID, AccessToken: "at-1", RefreshToken: "rt-1", Provider: "oidc",
		ExpiresAt: time.Now().Add(-time.Minute), AuthMethod: sesslib.AuthMethodOAuth,
	})

	rec, _, called := f.requireAuth(t, cookie)

	require.True(t, called)
	access, refresh := decryptedRowTokens(t, rows.only())
	assert.Equal(t, "at-2", access)
	assert.Equal(t, "rt-2", refresh)
	assert.WithinDuration(t, newExpiry, rows.only().AccessExpiresAt, time.Second)
	assert.NotNil(t, rows.only().LastRefreshedAt)
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), rows.only().ExpiresAt, time.Minute)
	reissued := readCookie(t, f.srv, sessionCookieIn(rec))
	assert.Empty(t, reissued.RefreshToken, "the rotated token goes to the row, not the cookie")
	assert.Equal(t, readCookie(t, f.srv, cookie).ID, reissued.ID, "a refresh keeps the session")
}

func TestSessionRows_SlideExtendsTheRow(t *testing.T) {
	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	cookie := storedSessionCookie(t, f.srv, slidingSession(0, 2*24*time.Hour))

	_, _, called := f.requireAuth(t, cookie)

	require.True(t, called)
	want := time.Now().Add(sesslib.SlidingLifetime)
	assert.WithinDuration(t, want, rows.only().ExpiresAt, time.Minute)
	assert.WithinDuration(t, want, rows.only().AccessExpiresAt, time.Minute)
}

func TestSessionRows_LegacyCookieIsConvertedOnFirstUse(t *testing.T) {
	legacyPayload := func(epoch int64) map[string]any {
		return map[string]any{
			"user_id": epochUserID, "access_token": "legacy-at", "refresh_token": "legacy-rt",
			"provider": "oidc", "auth_method": sesslib.AuthMethodOAuth, "epoch": epoch,
			"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		}
	}

	t.Run("an accepted legacy cookie gets a row and a slim cookie", func(t *testing.T) {
		f := newEpochFixture(t, 2)
		rows := withOwnSessionStore(f.srv)

		rec, ctx, called := f.requireAuth(t, legacySessionCookie(t, legacyPayload(2)))

		require.True(t, called, "nobody is signed out by the upgrade")
		require.Equal(t, 1, rows.count())
		access, refresh := decryptedRowTokens(t, rows.only())
		assert.Equal(t, "legacy-at", access)
		assert.Equal(t, "legacy-rt", refresh)
		assert.Equal(t, "legacy-rt", authenticatedSession(ctx).RefreshToken)

		converted := sessionCookieIn(rec)
		require.NotNil(t, converted, "the cookie is re-issued")
		slim := readCookie(t, f.srv, converted)
		assert.NotEmpty(t, slim.ID)
		assert.Empty(t, slim.RefreshToken)
		assert.Equal(t, int64(2), slim.Epoch)

		// The converted cookie authenticates through its row from now on.
		_, ctx, called = f.requireAuth(t, converted)
		require.True(t, called)
		assert.Equal(t, "legacy-rt", authenticatedSession(ctx).RefreshToken)
		assert.Equal(t, 1, rows.count(), "a converted cookie is not converted again")
	})

	t.Run("a revoked legacy cookie is never converted", func(t *testing.T) {
		f := newEpochFixture(t, 3)
		rows := withOwnSessionStore(f.srv)

		rec, _, called := f.requireAuth(t, legacySessionCookie(t, legacyPayload(2)))

		assert.False(t, called)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Zero(t, rows.count())
	})

	t.Run("a failed conversion is a 500 and keeps the cookie", func(t *testing.T) {
		f := newEpochFixture(t, 2)
		rows := withOwnSessionStore(f.srv)
		rows.setFail(errors.New("db down"))

		rec, _, called := f.requireAuth(t, legacySessionCookie(t, legacyPayload(2)))

		assert.False(t, called)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Nil(t, sessionCookieIn(rec), "the legacy cookie is kept for the next try")
	})

	t.Run("the optional path accepts a legacy cookie without converting it", func(t *testing.T) {
		f := newEpochFixture(t, 2)
		rows := withOwnSessionStore(f.srv)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
		req.AddCookie(legacySessionCookie(t, legacyPayload(2)))
		var user any
		f.srv.optionalAuthMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			user = r.Context().Value(contextkeys.UserID)
		})).ServeHTTP(httptest.NewRecorder(), req)

		assert.Equal(t, epochUserID, user)
		assert.Zero(t, rows.count())
	})
}

func TestSessionRows_SignOutDeletesTheRow(t *testing.T) {
	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	f.container.activityService.On("RecordAuthActivity", mock.Anything, epochUserID,
		activities.ActivityTypeAuthLogout, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	cookie := storedSessionCookie(t, f.srv, slidingSession(0, time.Hour))
	require.Equal(t, 1, rows.count())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	f.srv.handleLogout(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, rows.count(), "sign-out deletes the row")
	// A copy of the cookie kept from before the sign-out is dead.
	again, _, called := f.requireAuth(t, cookie)
	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, again.Code)
}

func TestSessionRows_RevokedSessionLosesItsRow(t *testing.T) {
	f := newEpochFixture(t, 1)
	rows := withOwnSessionStore(f.srv)
	cookie := storedSessionCookie(t, f.srv, slidingSession(0, time.Hour))

	rec, _, called := f.requireAuth(t, cookie)

	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, rows.count(), "a session the epoch revoked is deleted with its cookie")
}

func TestRunSessionSweep(t *testing.T) {
	t.Run("deletes expired rows and stops with its context", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		rows := withOwnSessionStore(f.srv)
		storedSessionCookie(t, f.srv, slidingSession(0, time.Hour))
		require.NoError(t, rows.Create(context.Background(), &models.UserSession{
			IDHash: []byte("expired"), UserID: epochUserID, ExpiresAt: time.Now().Add(-time.Minute),
		}))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		f.srv.runSessionSweep(ctx, time.Hour)

		assert.Equal(t, 1, rows.count(), "the start-up sweep removed only the expired row")
	})

	t.Run("does nothing without a store", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		f.srv.container = sessionStoreContainer{Container: f.srv.container, store: nil}
		f.srv.runSessionSweep(context.Background(), time.Hour) // returns at once
	})
}

func TestSessionRows_PermanentRefreshFailureDeletesTheRow(t *testing.T) {
	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	f.container.authService.On("RefreshTokens", mock.Anything, "oidc", "rt-1").
		Return(nil, errors.New("token endpoint returned 400: invalid_grant")).Once()
	cookie := storedSessionCookie(t, f.srv, &sesslib.Session{
		UserID: epochUserID, AccessToken: "at-1", RefreshToken: "rt-1", Provider: "oidc",
		ExpiresAt: time.Now().Add(-time.Minute), AuthMethod: sesslib.AuthMethodOAuth,
	})

	rec, _, called := f.requireAuth(t, cookie)

	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, rows.count(), "a session the provider refused is deleted with its cookie")
}
