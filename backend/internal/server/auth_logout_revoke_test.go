package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// Revoking the identity provider's refresh token at sign-out (#1649). The
// session's provider is a real OIDC client against a fake issuer, so every
// test sees exactly what reached the issuer's revocation endpoint.

const revokeClientID, revokeClientSecret = "client-1", "secret-1"

type revokeFixture struct {
	*epochFixture
	rows *memSessionRepo
	iss  *fakeoidc.Issuer
	auth *svcmocks.MockAuthServiceInterface
}

// newRevokeFixture builds an epoch fixture whose "oidc" provider is a real
// client of a fake issuer, advertising a revocation endpoint when endpoint is
// set. RevokeRefreshToken routes to that client, as AuthService does.
func newRevokeFixture(t *testing.T, endpoint bool) *revokeFixture {
	t.Helper()
	iss := fakeoidc.New(t, revokeClientID, revokeClientSecret, refreshSubject, "user@example.com")
	iss.SetRevocationEndpoint(endpoint)
	client, err := oidc.New(context.Background(), oidc.Config{
		Name: idp.ProviderOIDC, IssuerURL: iss.URL, ClientID: revokeClientID, ClientSecret: revokeClientSecret,
	})
	require.NoError(t, err)

	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	// A fresh mock: the container's default RevokeRefreshToken answer would
	// shadow this one.
	auth := svcmocks.NewMockAuthServiceInterface(t)
	auth.On("RevokeRefreshToken", mock.Anything, "oidc", mock.Anything).
		Return(func(ctx context.Context, _, rt string) error { return client.RevokeRefreshToken(ctx, rt) }).Maybe()
	f.container.authService = auth
	f.container.activityService.On("RecordAuthActivity", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	return &revokeFixture{epochFixture: f, rows: rows, iss: iss, auth: auth}
}

// providerSession is a provider session of the fixture user holding
// refreshToken.
func providerSession(refreshToken string) *sesslib.Session {
	return &sesslib.Session{
		UserID: epochUserID, AccessToken: "at-1", RefreshToken: refreshToken, Provider: "oidc",
		ExpiresAt: time.Now().Add(time.Hour), AuthMethod: sesslib.AuthMethodOAuth, IDPSubject: refreshSubject,
		AuthenticatedAt: time.Now(),
	}
}

// logout signs out with cookie and asserts the sign-out itself succeeded: 200
// with the confirmation, the cookie cleared and every row gone.
func (f *revokeFixture) logout(t *testing.T, cookie *http.Cookie) time.Duration {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	start := time.Now()
	f.srv.handleLogout(rec, req)
	elapsed := time.Since(start)

	require.Equal(t, http.StatusOK, rec.Code)
	var body LogoutResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "logged out", body.Message)
	cleared := sessionCookieIn(rec)
	require.NotNil(t, cleared, "the cookie is cleared")
	assert.Negative(t, cleared.MaxAge)
	assert.Zero(t, f.rows.count(), "the row is deleted")
	return elapsed
}

func TestLogoutRevoke_RevokesTheRefreshTokenAtTheIssuer(t *testing.T) {
	f := newRevokeFixture(t, true)
	token := f.iss.IssueRefreshToken()
	cookie := storedSessionCookie(t, f.srv, providerSession(token))

	f.logout(t, cookie)

	assert.Equal(t, []fakeoidc.Revocation{
		{Token: token, TokenTypeHint: "refresh_token", BasicAuth: true, Authenticated: true},
	}, f.iss.Revocations(), "exactly one authenticated revocation of the session's refresh token")
}

func TestLogoutRevoke_NoCallWithoutARevocationEndpoint(t *testing.T) {
	f := newRevokeFixture(t, false)
	cookie := storedSessionCookie(t, f.srv, providerSession(f.iss.IssueRefreshToken()))

	f.logout(t, cookie)

	assert.Empty(t, f.iss.Revocations())
}

func TestLogoutRevoke_NoCallForASessionWithoutARefreshToken(t *testing.T) {
	f := newRevokeFixture(t, true)
	cookie := storedSessionCookie(t, f.srv, slidingSession(0, time.Hour))

	f.logout(t, cookie)

	assert.Empty(t, f.iss.Revocations())
	f.auth.AssertNotCalled(t, "RevokeRefreshToken", mock.Anything, mock.Anything, mock.Anything)
}

func TestLogoutRevoke_NoCallWhenTheRowIsAlreadyGone(t *testing.T) {
	f := newRevokeFixture(t, true)
	sess := providerSession(f.iss.IssueRefreshToken())
	cookie := storedSessionCookie(t, f.srv, sess)
	require.NoError(t, f.srv.sessionStore().Delete(context.Background(), sess.ID))

	f.logout(t, cookie)

	assert.Empty(t, f.iss.Revocations())
}

func TestLogoutRevoke_AFailingEndpointDoesNotPreventSignOut(t *testing.T) {
	f := newRevokeFixture(t, true)
	f.iss.SetRevocationStatus(http.StatusServiceUnavailable)
	cookie := storedSessionCookie(t, f.srv, providerSession(f.iss.IssueRefreshToken()))

	f.logout(t, cookie)

	assert.Len(t, f.iss.Revocations(), 1, "the revocation was attempted")
}

func TestLogoutRevoke_AHangingEndpointDelaysSignOutByAtMostTheTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the revocation timeout")
	}
	f := newRevokeFixture(t, true)
	f.iss.SetRevocationHang(true)
	cookie := storedSessionCookie(t, f.srv, providerSession(f.iss.IssueRefreshToken()))

	elapsed := f.logout(t, cookie)

	assert.Less(t, elapsed, providerRevocationTimeout+time.Second)
	assert.Len(t, f.iss.Revocations(), 1)
}

func TestLogoutRevoke_ACancelledRequestStillRevokes(t *testing.T) {
	f := newRevokeFixture(t, true)
	token := f.iss.IssueRefreshToken()
	cookie := storedSessionCookie(t, f.srv, providerSession(token))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the browser navigated away

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil).WithContext(ctx)
	req.AddCookie(cookie)
	f.srv.handleLogout(httptest.NewRecorder(), req)

	got := f.iss.Revocations()
	require.Len(t, got, 1)
	assert.True(t, got[0].Authenticated)
	assert.Equal(t, token, got[0].Token)
}

// staleLoadStore is a session store whose Load hands back a refresh token a
// concurrent refresh has since rotated: the row holds a newer one.
type staleLoadStore struct {
	services.SessionStoreInterface
	stale string
}

func (s staleLoadStore) Load(ctx context.Context, sess *sesslib.Session) error {
	if err := s.SessionStoreInterface.Load(ctx, sess); err != nil {
		return err
	}
	sess.RefreshToken = s.stale
	return nil
}

func TestLogoutRevoke_RevokesTheTokenTheRowHeldWhenDeleted(t *testing.T) {
	f := newRevokeFixture(t, true)
	stale, latest := f.iss.IssueRefreshToken(), f.iss.IssueRefreshToken()
	cookie := storedSessionCookie(t, f.srv, providerSession(latest))
	f.srv.container = sessionStoreContainer{
		Container: f.srv.container, store: staleLoadStore{SessionStoreInterface: f.srv.sessionStore(), stale: stale},
	}

	f.logout(t, cookie)

	got := f.iss.Revocations()
	require.Len(t, got, 1)
	assert.Equal(t, latest, got[0].Token, "the rotated token the row held, never the one read before the delete")
}

func TestRevokeOtherSessions_DeletesTheOtherRowsAndRevokesTheirTokens(t *testing.T) {
	f := newRevokeFixture(t, true)
	f.auth.On("RevokeUserSessions", mock.Anything, epochUserID).
		Return(func(context.Context, string) (int64, error) { return f.epoch.Add(1), nil }).Once()
	mineToken, otherToken := f.iss.IssueRefreshToken(), f.iss.IssueRefreshToken()
	mine := storedSessionCookie(t, f.srv, providerSession(mineToken))
	storedSessionCookie(t, f.srv, providerSession(otherToken))
	storedSessionCookie(t, f.srv, slidingSession(0, time.Hour))
	require.Equal(t, 3, f.rows.count())

	rec, err := f.revokeVia(t, mine, epochUserID)
	require.NoError(t, err)

	assert.Equal(t, 1, f.rows.count(), "only the caller's row is left")
	_, kept := decryptedRowTokens(t, f.rows.only())
	assert.Equal(t, mineToken, kept)
	assert.Equal(t, []fakeoidc.Revocation{
		{Token: otherToken, TokenTypeHint: "refresh_token", BasicAuth: true, Authenticated: true},
	}, f.iss.Revocations(), "the other provider session's token, and never the caller's")

	_, _, called := f.requireAuth(t, sessionCookieIn(rec))
	assert.True(t, called, "the caller's own session keeps working")
}

func TestRevokeOtherSessions_ActingOnAnotherUserDeletesAllTheirRows(t *testing.T) {
	f := newRevokeFixture(t, true)
	f.auth.On("RevokeUserSessions", mock.Anything, epochUserID).Return(int64(1), nil).Once()
	token := f.iss.IssueRefreshToken()
	storedSessionCookie(t, f.srv, providerSession(token))
	// An API-key request: no cookie session of the user is the caller's.
	ctx, err := f.srv.authenticateUser(context.Background(), epochUserID, "api_key", nil, nil, time.Time{})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/anything", nil).WithContext(ctx)

	require.NoError(t, f.srv.revokeOtherSessions(httptest.NewRecorder(), req, epochUserID))

	assert.Zero(t, f.rows.count())
	assert.Len(t, f.iss.Revocations(), 1)
}

func TestRevokeOtherSessions_ARowDeletionFailureIsNotReported(t *testing.T) {
	f := newRevokeFixture(t, true)
	f.auth.On("RevokeUserSessions", mock.Anything, epochUserID).Return(int64(1), nil).Once()
	ctx, err := f.srv.authenticateUser(context.Background(), epochUserID, "api_key", nil, nil, time.Time{})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/anything", nil).WithContext(ctx)
	f.rows.setFail(assert.AnError)

	require.NoError(t, f.srv.revokeOtherSessions(httptest.NewRecorder(), req, epochUserID),
		"the epoch bump is what the caller is promised")
	assert.Empty(t, f.iss.Revocations())
}
