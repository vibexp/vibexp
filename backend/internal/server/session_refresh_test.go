package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

const refreshSubject = "idp-sub-1"

// refreshableSession is a session of the epoch fixture's user with a refresh
// token, whose access token expires at accessExpiry.
func refreshableSession(refreshToken string, accessExpiry time.Time) *sesslib.Session {
	return &sesslib.Session{
		UserID: epochUserID, AccessToken: "at-1", RefreshToken: refreshToken, Provider: "oidc",
		ExpiresAt: accessExpiry, AuthMethod: sesslib.AuthMethodOAuth, IDPSubject: refreshSubject,
	}
}

func TestRefreshDue(t *testing.T) {
	srv := &Server{config: &config.Config{Auth: config.AuthConfig{SessionRecheckInterval: 15 * time.Minute}}}
	now := time.Now()
	tests := []struct {
		name        string
		refresh     string
		expiresAt   time.Time
		lastRefresh time.Time
		due         bool
	}{
		{"access token expired", "rt", now.Add(-time.Second), now, true},
		{"interval passed", "rt", now.Add(time.Hour), now.Add(-15 * time.Minute), true},
		{"inside both bounds", "rt", now.Add(time.Hour), now.Add(-14 * time.Minute), false},
		{"no refresh token, expired", "", now.Add(-time.Hour), now.Add(-time.Hour), false},
		{"no refresh token, interval passed", "", now.Add(time.Hour), now.Add(-time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := &sesslib.Session{RefreshToken: tt.refresh, ExpiresAt: tt.expiresAt, LastRefreshedAt: tt.lastRefresh}
			assert.Equal(t, tt.due, srv.refreshDue(sess, now))
		})
	}
}

func TestSessionRecheckInterval_DefaultsWhenUnset(t *testing.T) {
	assert.Equal(t, defaultSessionRecheckInterval, (&Server{}).sessionRecheckInterval())
	srv := &Server{config: &config.Config{Auth: config.AuthConfig{SessionRecheckInterval: time.Minute}}}
	assert.Equal(t, time.Minute, srv.sessionRecheckInterval())
}

func TestSessionRefresh_IntervalRecheck(t *testing.T) {
	t.Run("a session past the interval is refreshed although its access token is valid", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		rows := withOwnSessionStore(f.srv)
		f.container.authService.On("RefreshTokens", mock.Anything, "oidc", "rt-1").
			Return(&idp.Tokens{AccessToken: "at-2", RefreshToken: "rt-2", ExpiresAt: time.Now().Add(time.Hour)},
				&idp.Claims{Subject: refreshSubject}, nil).Once()
		cookie := storedSessionCookie(t, f.srv, refreshableSession("rt-1", time.Now().Add(time.Hour)))
		rows.backdateRefresh(defaultSessionRecheckInterval + time.Minute)

		rec, ctx, called := f.requireAuth(t, cookie)

		require.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
		_, refresh := decryptedRowTokens(t, rows.only())
		assert.Equal(t, "rt-2", refresh)
		require.NotNil(t, rows.only().LastRefreshedAt)
		assert.WithinDuration(t, time.Now(), *rows.only().LastRefreshedAt, time.Minute)
		assert.Equal(t, "rt-2", authenticatedSession(ctx).RefreshToken)
		f.container.authService.AssertExpectations(t)
	})

	t.Run("a session inside both bounds makes no provider call", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		rows := withOwnSessionStore(f.srv)
		cookie := storedSessionCookie(t, f.srv, refreshableSession("rt-1", time.Now().Add(time.Hour)))
		rows.backdateRefresh(defaultSessionRecheckInterval - time.Minute)

		_, _, called := f.requireAuth(t, cookie)

		require.True(t, called)
		f.container.authService.AssertNotCalled(t, "RefreshTokens", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("the configured interval is the bound", func(t *testing.T) {
		f := newEpochFixture(t, 0)
		f.srv.config = &config.Config{Auth: config.AuthConfig{SessionRecheckInterval: time.Hour}}
		rows := withOwnSessionStore(f.srv)
		cookie := storedSessionCookie(t, f.srv, refreshableSession("rt-1", time.Now().Add(2*time.Hour)))
		rows.backdateRefresh(30 * time.Minute)

		_, _, called := f.requireAuth(t, cookie)

		require.True(t, called)
		f.container.authService.AssertNotCalled(t, "RefreshTokens", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestSessionRefresh_ProviderEndsTheSession(t *testing.T) {
	tests := []struct {
		name      string
		tokens    *idp.Tokens
		claims    *idp.Claims
		err       error
		wantClass string
	}{
		{
			name:      "invalid_grant",
			err:       &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusBadRequest}, ErrorCode: "invalid_grant"},
			wantClass: "invalid_grant",
		},
		{
			name:      "the refreshed identity names another subject",
			tokens:    &idp.Tokens{AccessToken: "at-2", RefreshToken: "rt-2", ExpiresAt: time.Now().Add(time.Hour)},
			claims:    &idp.Claims{Subject: "someone-else"},
			wantClass: "subject_mismatch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEpochFixture(t, 0)
			rows := withOwnSessionStore(f.srv)
			rec := withAudit(f.srv)
			f.container.authService.On("RefreshTokens", mock.Anything, "oidc", "rt-1").
				Return(tt.tokens, tt.claims, tt.err).Once()
			cookie := storedSessionCookie(t, f.srv, refreshableSession("rt-1", time.Now().Add(-time.Minute)))

			resp, _, called := f.requireAuth(t, cookie)

			assert.False(t, called)
			assert.Equal(t, http.StatusUnauthorized, resp.Code)
			assert.Zero(t, rows.count(), "the row is deleted")
			cleared := sessionCookieIn(resp)
			require.NotNil(t, cleared, "the cookie is cleared")
			assert.Negative(t, cleared.MaxAge)

			got := rec.only(t).event
			assert.Equal(t, models.AuditOpAuthSessionEndedByProvider, got.Operation)
			require.NotNil(t, got.Actor)
			require.NotNil(t, got.Actor.UserID)
			assert.Equal(t, epochUserID, *got.Actor.UserID)
			assert.Equal(t, models.AuditResourceTypeUser, got.ResourceType)
			assert.Equal(t, epochUserID, got.ResourceID)
			assert.Equal(t, "oidc", got.Metadata[auditMetaProvider])
			assert.Equal(t, tt.wantClass, got.Metadata["error_class"])
		})
	}
}

func TestSessionRefresh_TransientFailureKeepsTheSession(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := newEpochFixture(t, 0)
			rows := withOwnSessionStore(f.srv)
			rec := withAudit(f.srv)
			f.container.authService.On("RefreshTokens", mock.Anything, "oidc", "rt-1").
				Return(nil, nil, &oauth2.RetrieveError{Response: &http.Response{StatusCode: status}}).Once()
			cookie := storedSessionCookie(t, f.srv, refreshableSession("rt-1", time.Now().Add(-time.Minute)))

			resp, _, called := f.requireAuth(t, cookie)

			assert.False(t, called)
			assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
			assert.Equal(t, sessionRefreshRetryAfter, resp.Header().Get("Retry-After"))
			assert.Nil(t, sessionCookieIn(resp), "the cookie is kept")
			require.Equal(t, 1, rows.count(), "the row is kept")
			_, refresh := decryptedRowTokens(t, rows.only())
			assert.Equal(t, "rt-1", refresh)
			assert.Empty(t, rec.events())
		})
	}
}

func TestSessionRefresh_RowGoneWhileWaitingIs401(t *testing.T) {
	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	sess := refreshableSession("rt-1", time.Now().Add(-time.Minute))
	cookie := storedSessionCookie(t, f.srv, sess)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	req.AddCookie(cookie)
	loaded, err := f.srv.sessionManager.Read(req)
	require.NoError(t, err)
	require.NoError(t, f.srv.loadSession(context.Background(), loaded))
	require.NoError(t, f.srv.sessionStore().Delete(context.Background(), sess.ID))

	resp := httptest.NewRecorder()
	ok := f.srv.refreshIfDue(resp, req, loaded, f.srv.logger)

	assert.False(t, ok)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
	assert.Zero(t, rows.count())
	f.container.authService.AssertNotCalled(t, "RefreshTokens", mock.Anything, mock.Anything, mock.Anything)
}

// TestSessionRefresh_ParallelRequestsRefreshOnce drives parallel requests on
// one due session through a real OIDC client against a fake issuer that
// rotates refresh tokens and revokes the whole chain when a spent one is
// presented again. Exactly one refresh may reach the issuer; a second would
// replay the spent token and sign the user out.
func TestSessionRefresh_ParallelRequestsRefreshOnce(t *testing.T) {
	const parallel = 8
	iss := fakeoidc.New(t, "client-1", "secret-1", refreshSubject, "user@example.com")
	client, err := oidc.New(context.Background(), oidc.Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL, ClientID: "client-1", ClientSecret: "secret-1",
	})
	require.NoError(t, err)

	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	f.container.authService.On("RefreshTokens", mock.Anything, "oidc", mock.Anything).
		Return(func(ctx context.Context, _, rt string) (*idp.Tokens, *idp.Claims, error) {
			return client.Refresh(ctx, rt)
		})
	cookie := storedSessionCookie(t, f.srv, refreshableSession(iss.IssueRefreshToken(), time.Now().Add(-time.Minute)))

	var wg sync.WaitGroup
	codes := make([]int, parallel)
	start := make(chan struct{})
	for i := range parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec, _, _ := f.requireAuth(t, cookie)
			codes[i] = rec.Code
		}()
	}
	close(start)
	wg.Wait()

	for i, code := range codes {
		assert.Equal(t, http.StatusOK, code, "request %d", i)
	}
	assert.Equal(t, int64(1), iss.RefreshCalls(), "exactly one refresh reached the issuer")
	require.Equal(t, 1, rows.count())

	// The stored token is the live successor: the next due refresh succeeds.
	rows.backdateRefresh(time.Hour)
	rec, _, called := f.requireAuth(t, cookie)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int64(2), iss.RefreshCalls())
}

// TestSessionRefresh_IssuerKeysOutageKeepsTheSession: the token endpoint
// answers, but the keys needed to verify the refreshed ID token cannot be
// fetched. That is the issuer failing, not a refusal: 503, session kept.
func TestSessionRefresh_IssuerKeysOutageKeepsTheSession(t *testing.T) {
	iss := fakeoidc.New(t, "client-1", "secret-1", refreshSubject, "user@example.com")
	client, err := oidc.New(context.Background(), oidc.Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL, ClientID: "client-1", ClientSecret: "secret-1",
	})
	require.NoError(t, err)
	iss.SetJWKSStatus(http.StatusServiceUnavailable)

	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	rec := withAudit(f.srv)
	f.container.authService.On("RefreshTokens", mock.Anything, "oidc", mock.Anything).
		Return(func(ctx context.Context, _, rt string) (*idp.Tokens, *idp.Claims, error) {
			return client.Refresh(ctx, rt)
		})
	cookie := storedSessionCookie(t, f.srv, refreshableSession(iss.IssueRefreshToken(), time.Now().Add(-time.Minute)))

	resp, _, called := f.requireAuth(t, cookie)

	assert.False(t, called)
	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
	assert.Equal(t, 1, rows.count(), "the row is kept")
	assert.Nil(t, sessionCookieIn(resp), "the cookie is kept")
	assert.Empty(t, rec.events(), "nothing was ended by the provider")
}
