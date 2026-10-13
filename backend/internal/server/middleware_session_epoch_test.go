package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Session revocation, sign-in method and sliding sessions (#1428).
//
// The epoch check lives inside authenticateUser, so the guardrail in
// middleware_suspension_guardrail_test.go already forces every path through it.
// These tests drive the middlewares end to end, because revocation is only
// real if each path hands the session to the chokepoint and renders its error.

const epochUserID = "epoch-user"

// epochFixture is an auth test server whose user row carries a session epoch
// the test can move, the way a revocation moves it in the database.
type epochFixture struct {
	srv       *Server
	container *MockAuthContainer
	epoch     *atomic.Int64
}

func newEpochFixture(t *testing.T, epoch int64) *epochFixture {
	t.Helper()
	current := &atomic.Int64{}
	current.Store(epoch)

	users := repomocks.NewMockUserRepository(t)
	users.On("GetByID", mock.Anything, epochUserID).
		Return(func(_ context.Context, id string) (*models.User, error) {
			return &models.User{ID: id, Status: models.UserStatusActive, SessionEpoch: current.Load()}, nil
		}).Maybe()

	c := newMockAuthContainer(t)
	c.userRepository = users
	return &epochFixture{srv: createTestAuthServer(c), container: c, epoch: current}
}

// cookieFor encrypts sess into a request cookie.
func (f *epochFixture) cookieFor(t *testing.T, sess *sesslib.Session) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	require.NoError(t, f.srv.sessionManager.Write(rec, sess))
	c := sessionCookieIn(rec)
	require.NotNil(t, c, "session cookie not written")
	return c
}

// sessionCookieIn returns the vx_session cookie a response set, or nil.
func sessionCookieIn(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			return c
		}
	}
	return nil
}

// readSession decrypts the vx_session cookie a response set.
func (f *epochFixture) readSession(t *testing.T, rec *httptest.ResponseRecorder) *sesslib.Session {
	t.Helper()
	c := sessionCookieIn(rec)
	require.NotNil(t, c, "response set no session cookie")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	sess, err := f.srv.sessionManager.Read(req)
	require.NoError(t, err)
	return sess
}

// requireAuth runs the required-auth cookie path and reports the response, the
// context the handler saw, and whether the handler ran.
func (f *epochFixture) requireAuth(
	t *testing.T, cookie *http.Cookie,
) (*httptest.ResponseRecorder, context.Context, bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	var seen context.Context
	called := false
	f.srv.authenticateWithSession(rec, req, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		seen = r.Context()
		w.WriteHeader(http.StatusOK)
	}))
	return rec, seen, called
}

// slidingSession is a refresh-less session for the fixture user minted under
// epoch, last issued `age` ago.
func slidingSession(epoch int64, age time.Duration) *sesslib.Session {
	issued := time.Now().Add(-age)
	return &sesslib.Session{
		UserID:          epochUserID,
		AccessToken:     "at",
		ExpiresAt:       issued.Add(sesslib.SlidingLifetime),
		AuthMethod:      sesslib.AuthMethodDev,
		AuthenticatedAt: issued,
		Epoch:           epoch,
	}
}

func TestSessionEpoch_RequiredPath(t *testing.T) {
	t.Run("a matching epoch authenticates", func(t *testing.T) {
		f := newEpochFixture(t, 3)
		rec, _, called := f.requireAuth(t, f.cookieFor(t, slidingSession(3, 0)))

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, called)
	})

	t.Run("a bumped epoch ends the session and clears the cookie", func(t *testing.T) {
		f := newEpochFixture(t, 3)
		cookie := f.cookieFor(t, slidingSession(3, 0))

		rec, _, called := f.requireAuth(t, cookie)
		require.True(t, called, "the session must work before it is revoked")
		require.Equal(t, http.StatusOK, rec.Code)

		f.epoch.Store(4)
		rec, _, called = f.requireAuth(t, cookie)

		assert.False(t, called, "a revoked session must not reach the handler")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), sessionRevokedAuthDetail)
		cleared := sessionCookieIn(rec)
		require.NotNil(t, cleared, "a revoked session's cookie must be cleared")
		assert.Negative(t, cleared.MaxAge)
		assert.Empty(t, cleared.Value)
	})

	t.Run("a cookie from a later epoch than the row is rejected too", func(t *testing.T) {
		// Equality, not "at least": a cookie claiming an epoch the user has not
		// reached is not one this server minted for the current row.
		f := newEpochFixture(t, 1)
		rec, _, called := f.requireAuth(t, f.cookieFor(t, slidingSession(2, 0)))

		assert.False(t, called)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("a cookie minted before epochs existed works while the epoch is 0", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		legacy := &sesslib.Session{
			UserID: epochUserID, AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour), Provider: "google",
			RefreshToken: "rt",
		}
		rec, ctx, called := f.requireAuth(t, f.cookieFor(t, legacy))

		require.True(t, called, "the upgrade must not sign anyone out")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, contextkeys.GetAuthMethod(ctx))
		assert.True(t, contextkeys.GetAuthenticatedAt(ctx).IsZero(),
			"a session with no recorded sign-in time must never read as a recent sign-in")
	})

	t.Run("a cookie minted before epochs existed stops working once the epoch is bumped", func(t *testing.T) {
		f := newEpochFixture(t, 1)
		legacy := &sesslib.Session{UserID: epochUserID, AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour)}
		rec, _, called := f.requireAuth(t, f.cookieFor(t, legacy))

		assert.False(t, called)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestSessionEpoch_OptionalPathGoesAnonymous(t *testing.T) {
	f := newEpochFixture(t, 2)
	cookie := f.cookieFor(t, slidingSession(1, 0))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	called := false
	f.srv.optionalAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Nil(t, r.Context().Value(contextkeys.UserID), "a revoked session must not carry a user")
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assert.True(t, called, "the optional path proceeds, anonymously")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestSessionEpoch_DoesNotApplyToOtherAuthTypes pins the scope of revocation:
// an API key and a bearer token carry no epoch, so bumping the user's epoch
// leaves them working.
func TestSessionEpoch_DoesNotApplyToOtherAuthTypes(t *testing.T) {
	f := newEpochFixture(t, 7)

	for _, authType := range []string{"api_key", "oauth"} {
		t.Run(authType, func(t *testing.T) {
			ctx, err := f.srv.authenticateUser(context.Background(), epochUserID, authType, nil, nil)

			require.NoError(t, err)
			assert.Equal(t, epochUserID, ctx.Value(contextkeys.UserID))
			assert.Empty(t, contextkeys.GetAuthMethod(ctx))
			assert.True(t, contextkeys.GetAuthenticatedAt(ctx).IsZero())
		})
	}
}

// TestAuthenticateUser_CookieWithoutSessionIsRefused pins the fail-closed
// guard: a cookie path that reached the chokepoint without its session could
// not have its epoch checked, so it must not authenticate.
func TestAuthenticateUser_CookieWithoutSessionIsRefused(t *testing.T) {
	f := newEpochFixture(t, 0)

	ctx, err := f.srv.authenticateUser(context.Background(), epochUserID, authTypeCookie, nil, nil)

	require.ErrorIs(t, err, errSessionMissing)
	assert.Nil(t, ctx)

	rec := httptest.NewRecorder()
	f.srv.writeSuspensionAuthError(rec, httptest.NewRequest(http.MethodGet, "/", nil), err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "a wiring fault is not a credential problem")
}

func TestAuthenticatedContext_CarriesSignInMethodAndTime(t *testing.T) {
	f := newEpochFixture(t, 0)
	sess := slidingSession(0, 5*time.Minute)

	_, ctx, called := f.requireAuth(t, f.cookieFor(t, sess))

	require.True(t, called)
	assert.Equal(t, sesslib.AuthMethodDev, contextkeys.GetAuthMethod(ctx))
	assert.WithinDuration(t, sess.AuthenticatedAt, contextkeys.GetAuthenticatedAt(ctx), time.Second)
}

func TestSlidingSession_ReissuedAtMostOnceADay(t *testing.T) {
	t.Run("a session issued under a day ago sets no cookie", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		rec, _, called := f.requireAuth(t, f.cookieFor(t, slidingSession(0, 23*time.Hour)))

		require.True(t, called)
		assert.Nil(t, sessionCookieIn(rec), "an active user must not get a new cookie on every request")
	})

	t.Run("a session issued over a day ago is extended to 30 days", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		before := slidingSession(0, 25*time.Hour)

		rec, _, called := f.requireAuth(t, f.cookieFor(t, before))

		require.True(t, called)
		after := f.readSession(t, rec)
		assert.WithinDuration(t, time.Now().Add(sesslib.SlidingLifetime), after.ExpiresAt, time.Minute)
		assert.Equal(t, int(sesslib.SlidingLifetime/time.Second), sessionCookieIn(rec).MaxAge)
		assert.WithinDuration(t, before.AuthenticatedAt, after.AuthenticatedAt, time.Second,
			"sliding must not move the sign-in time, or an old session would read as a recent sign-in")
		assert.Equal(t, before.Epoch, after.Epoch)
		assert.Equal(t, before.AuthMethod, after.AuthMethod)
	})

	t.Run("a session idle for 30 days is expired and is never refreshed", func(t *testing.T) {
		// The fixture's AuthService mock has no RefreshTokens expectation, so a
		// call to it would fail the test.
		f := newEpochFixture(t, 0)
		rec, _, called := f.requireAuth(t, f.cookieFor(t, slidingSession(0, sesslib.SlidingLifetime+time.Minute)))

		assert.False(t, called)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Nil(t, sessionCookieIn(rec), "an expired session must not be revived")
	})

	t.Run("a revoked session is not extended", func(t *testing.T) {
		f := newEpochFixture(t, 1)
		rec, _, called := f.requireAuth(t, f.cookieFor(t, slidingSession(0, 25*time.Hour)))

		assert.False(t, called)
		cleared := sessionCookieIn(rec)
		require.NotNil(t, cleared)
		assert.Negative(t, cleared.MaxAge, "the only cookie written is the one that clears it")
	})

	t.Run("a session with a refresh token does not slide", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		idpSession := &sesslib.Session{
			UserID: epochUserID, AccessToken: "at", RefreshToken: "rt", Provider: "google",
			ExpiresAt: time.Now().Add(10 * time.Minute), AuthMethod: sesslib.AuthMethodOAuth,
		}
		rec, _, called := f.requireAuth(t, f.cookieFor(t, idpSession))

		require.True(t, called)
		assert.Nil(t, sessionCookieIn(rec))
	})
}

// A GitHub sign-in yields no refresh token and no access-token expiry
// (idp/github: "ExpiresAt as the zero value"). Before #1428 the session was
// minted with that zero expiry, which IsExpired reads as already expired, and
// with no refresh token the first authenticated request answered 401 "Session
// expired". These two tests pin both halves: what the callback mints now, and
// what happens to a cookie that was minted the old way.
func TestGitHubShapedSession(t *testing.T) {
	githubTokens := &idp.Tokens{AccessToken: "gho_token"} // no RefreshToken, zero ExpiresAt

	t.Run("the callback mints a sliding session that authenticates", func(t *testing.T) {
		f := newEpochFixture(t, 2)
		f.container.activityService.On("RecordAuthActivity", mock.Anything, epochUserID,
			activities.ActivityTypeAuthLogin, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		user := &models.User{ID: epochUserID, Email: "gh@example.com", Status: models.UserStatusActive, SessionEpoch: 2}

		rec := httptest.NewRecorder()
		f.srv.handleCallbackSuccess(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil),
			user, githubTokens, "state", "github", false)

		require.Equal(t, http.StatusFound, rec.Code)
		minted := f.readSession(t, rec)
		assert.Equal(t, sesslib.AuthMethodOAuth, minted.AuthMethod)
		assert.Equal(t, "github", minted.Provider)
		assert.Equal(t, int64(2), minted.Epoch, "the cookie carries the user's epoch at sign-in")
		assert.WithinDuration(t, time.Now(), minted.AuthenticatedAt, time.Minute)
		assert.WithinDuration(t, time.Now().Add(sesslib.SlidingLifetime), minted.ExpiresAt, time.Minute)
		assert.Equal(t, int(sesslib.SlidingLifetime/time.Second), sessionCookieIn(rec).MaxAge)

		authRec, _, called := f.requireAuth(t, sessionCookieIn(rec))
		assert.True(t, called, "the first authenticated request of a GitHub session must succeed")
		assert.Equal(t, http.StatusOK, authRec.Code)
	})

	t.Run("a cookie minted before #1428 with a zero expiry stays expired", func(t *testing.T) {
		// Deliberate: a zero ExpiresAt never means "does not expire". Such a
		// cookie never authenticated, so nothing is taken away; the user signs
		// in again and gets a sliding session.
		f := newEpochFixture(t, 0)
		legacy := &sesslib.Session{UserID: epochUserID, AccessToken: "gho_token", Provider: "github"}

		rec, _, called := f.requireAuth(t, f.cookieFor(t, legacy))

		assert.False(t, called)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Nil(t, sessionCookieIn(rec), "an expired session must not be slid back to life")
	})
}

func TestHandleCallbackSuccess_RefreshableSessionKeepsProviderExpiry(t *testing.T) {
	f := newEpochFixture(t, 0)
	f.container.activityService.On("RecordAuthActivity", mock.Anything, epochUserID,
		activities.ActivityTypeAuthLogin, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	user := &models.User{ID: epochUserID, Email: "g@example.com", Status: models.UserStatusActive}
	expiry := time.Now().Add(time.Hour)

	rec := httptest.NewRecorder()
	f.srv.handleCallbackSuccess(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil),
		user, &idp.Tokens{AccessToken: "at", RefreshToken: "rt", ExpiresAt: expiry}, "state", "google", false)

	minted := f.readSession(t, rec)
	assert.WithinDuration(t, expiry, minted.ExpiresAt, time.Second, "a refreshable session keeps the provider's expiry")
	assert.Equal(t, 24*60*60, sessionCookieIn(rec).MaxAge, "and the 24-hour cookie")
	assert.Equal(t, sesslib.AuthMethodOAuth, minted.AuthMethod)
}

func TestHandleDevLogin_MintsSlidingSessionUnderTheUsersEpoch(t *testing.T) {
	f := newEpochFixture(t, 5)
	f.container.activityService.On("RecordAuthActivity", mock.Anything, epochUserID,
		activities.ActivityTypeAuthLogin, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	rec := httptest.NewRecorder()
	f.srv.handleDevLoginSuccess(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev/login", nil),
		&models.User{ID: epochUserID, Email: "dev@example.com", SessionEpoch: 5})

	minted := f.readSession(t, rec)
	assert.Equal(t, sesslib.AuthMethodDev, minted.AuthMethod)
	assert.Equal(t, int64(5), minted.Epoch)
	assert.WithinDuration(t, time.Now(), minted.AuthenticatedAt, time.Minute)
	assert.WithinDuration(t, time.Now().Add(sesslib.SlidingLifetime), minted.ExpiresAt, time.Minute)

	_, _, called := f.requireAuth(t, sessionCookieIn(rec))
	assert.True(t, called)
}

// TestRevokeThenReissue_OnlyTheCallerStaysSignedIn drives the two halves a
// "sign out my other sessions" handler composes, AuthService.RevokeUserSessions
// and session.Manager.ReissueUnderEpoch, against the real chokepoint: the
// caller's re-issued cookie keeps working, and every cookie minted before the
// bump, the caller's own old one included, stops.
func TestRevokeThenReissue_OnlyTheCallerStaysSignedIn(t *testing.T) {
	f := newEpochFixture(t, 0)
	mine := f.cookieFor(t, slidingSession(0, time.Hour))
	other := f.cookieFor(t, slidingSession(0, 2*time.Hour))

	// The bump, as the repository performs it.
	newEpoch := f.epoch.Add(1)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/anything", nil)
	req.AddCookie(mine)
	rec := httptest.NewRecorder()
	reissued, err := f.srv.sessionManager.ReissueUnderEpoch(rec, req, epochUserID, newEpoch)
	require.NoError(t, err)
	require.True(t, reissued)

	_, _, called := f.requireAuth(t, sessionCookieIn(rec))
	assert.True(t, called, "the caller's re-issued session must keep working")

	otherRec, _, called := f.requireAuth(t, other)
	assert.False(t, called, "another session of the same user must end")
	assert.Equal(t, http.StatusUnauthorized, otherRec.Code)

	staleRec, _, called := f.requireAuth(t, mine)
	assert.False(t, called, "the caller's cookie from before the bump must end too")
	assert.Equal(t, http.StatusUnauthorized, staleRec.Code)
}

func TestHandleLogout_RecordsLogoutActivity(t *testing.T) {
	t.Run("a valid cookie records one auth_logout for its user", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		f.container.activityService.On("RecordAuthActivity", mock.Anything, epochUserID,
			activities.ActivityTypeAuthLogout, (*string)(nil),
			mock.MatchedBy(func(md map[string]interface{}) bool {
				return md["auth_method"] == sesslib.AuthMethodOAuth && md["provider"] == "github"
			}), mock.Anything, mock.Anything).Return(nil).Once()

		sess := slidingSession(0, time.Hour)
		sess.AuthMethod, sess.Provider = sesslib.AuthMethodOAuth, "github"
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(f.cookieFor(t, sess))
		rec := httptest.NewRecorder()
		f.srv.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		cleared := sessionCookieIn(rec)
		require.NotNil(t, cleared)
		assert.Negative(t, cleared.MaxAge)
		f.container.activityService.AssertExpectations(t)
	})

	t.Run("an expired or revoked cookie still records who signed out", func(t *testing.T) {
		f := newEpochFixture(t, 9)
		f.container.activityService.On("RecordAuthActivity", mock.Anything, epochUserID,
			activities.ActivityTypeAuthLogout, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil).Once()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(f.cookieFor(t, slidingSession(0, sesslib.SlidingLifetime+time.Hour)))
		rec := httptest.NewRecorder()
		f.srv.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		f.container.activityService.AssertExpectations(t)
	})

	t.Run("no cookie records nothing", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		rec := httptest.NewRecorder()
		f.srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))

		assert.Equal(t, http.StatusOK, rec.Code)
		f.container.activityService.AssertNotCalled(t, "RecordAuthActivity")
	})

	t.Run("a cookie this server did not encrypt records nothing", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: "forged.value"})
		rec := httptest.NewRecorder()
		f.srv.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		f.container.activityService.AssertNotCalled(t, "RecordAuthActivity")
	})
}
