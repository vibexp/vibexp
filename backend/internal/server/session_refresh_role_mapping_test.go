package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// The role mapping re-applied on every token refresh (#1648).

const (
	roleRefreshSlug    = "keycloak"
	roleRefreshSubject = "sub-1"
	roleRefreshAdmin   = "platform-admins"
)

// roleRefreshFixture is the admin API over a real identity provider resolver
// and a real auth service, with one OIDC provider served by a fake issuer and
// mapping the "groups" claim to instance admin.
type roleRefreshFixture struct {
	*authSettingsFixture
	iss      *fakeoidc.Issuer
	provider *models.InstanceAuthProvider
}

func newRoleRefreshFixture(t *testing.T) *roleRefreshFixture {
	t.Helper()
	f := newAuthSettingsFixture(t)
	iss := fakeoidc.New(t, "client-"+roleRefreshSlug, authTestSecretSentinel, roleRefreshSubject, authTestMemberEmail)
	resolver := services.NewIdentityProviderResolver(services.IdentityProviderResolverDeps{
		Providers: f.store.Providers(), Versions: f.store.Versions(), Enc: f.enc,
		CallbackURL: authTestCallbackURL, Logger: slog.New(slog.DiscardHandler),
	})
	f.srv.container.(*authSettingsContainer).auth = services.NewAuthService(
		f.users, nil, resolver, nil, slog.New(slog.DiscardHandler), nil,
		services.WithProviderAdminSync(f.admins))

	body := oidcProviderBody(roleRefreshSlug)
	body["issuer_url"] = iss.URL
	body["roles_claim"] = "groups"
	body["admin_values"] = []string{roleRefreshAdmin}
	rr := f.serve(t, asRoot, http.MethodPost, authProvidersPath, body)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	provider, err := f.store.Providers().GetBySlug(context.Background(), roleRefreshSlug)
	require.NoError(t, err)
	return &roleRefreshFixture{authSettingsFixture: f, iss: iss, provider: provider}
}

// asRefreshingSession is a cookie session of userID through the fixture's
// provider whose access token has expired, so its next request refreshes.
func (f *roleRefreshFixture) asRefreshingSession(userID string) authCaller {
	return func(t *testing.T, _ *authSettingsFixture, r *http.Request) {
		t.Helper()
		r.AddCookie(storedSessionCookie(t, f.srv, &sesslib.Session{
			UserID: userID, AccessToken: "at-1", RefreshToken: f.iss.IssueRefreshToken(), Provider: roleRefreshSlug,
			ExpiresAt: time.Now().Add(-time.Minute), AuthMethod: sesslib.AuthMethodOAuth, IDPSubject: roleRefreshSubject,
		}))
	}
}

func (f *roleRefreshFixture) syncSignIn(t *testing.T, userID string, matched *string) {
	t.Helper()
	user, err := f.users.GetByID(context.Background(), userID)
	require.NoError(t, err)
	require.NoError(t, f.admins.SyncProviderAdmin(context.Background(), user, f.provider.ID, f.provider.Slug, matched,
		services.ProviderAdminTriggerSignIn))
}

// meIsAdmin runs GET /auth/me as caller, which authenticates through the
// refreshing session chokepoint, and returns its is_instance_admin.
func (f *roleRefreshFixture) meIsAdmin(t *testing.T, caller authCaller) bool {
	t.Helper()
	rr := f.serve(t, caller, http.MethodGet, "/api/v1/auth/me", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var me struct {
		IsInstanceAdmin bool `json:"is_instance_admin"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &me))
	return me.IsInstanceAdmin
}

func (f *roleRefreshFixture) isProviderAdmin(t *testing.T, userID string) bool {
	t.Helper()
	ok, err := f.store.ProviderAdmins().IsProviderAdmin(context.Background(), userID)
	require.NoError(t, err)
	return ok
}

func TestSessionRefresh_RoleMapping(t *testing.T) {
	t.Run("a claim that lost the admin value demotes on the triggering request", func(t *testing.T) {
		f := newRoleRefreshFixture(t)
		matched := roleRefreshAdmin
		f.syncSignIn(t, authTestMemberID, &matched)
		require.True(t, f.isProviderAdmin(t, authTestMemberID), "a provider-sourced admin before the refresh")
		f.iss.SetClaim("groups", []string{"developers"})

		isAdmin := f.meIsAdmin(t, f.asRefreshingSession(authTestMemberID))

		assert.Equal(t, int64(1), f.iss.RefreshCalls(), "the request refreshed the session")
		assert.False(t, isAdmin, "demoted before the handler ran")
		assert.False(t, f.isProviderAdmin(t, authTestMemberID))
		assert.Equal(t, http.StatusNotFound, f.serve(t, asMember, http.MethodGet, authAdminsPath, nil).Code,
			"the admin routes no longer answer the user")
	})
	t.Run("a claim that gained the admin value promotes on the triggering request", func(t *testing.T) {
		f := newRoleRefreshFixture(t)
		f.iss.SetClaim("groups", []string{"developers", roleRefreshAdmin})

		isAdmin := f.meIsAdmin(t, f.asRefreshingSession(authTestMemberID))

		assert.Equal(t, int64(1), f.iss.RefreshCalls())
		assert.True(t, isAdmin, "promoted before the handler ran")
		assert.True(t, f.isProviderAdmin(t, authTestMemberID))
		assert.Equal(t, http.StatusOK, f.serve(t, asMember, http.MethodGet, authAdminsPath, nil).Code,
			"the admin routes answer the user")
	})
	t.Run("root and manual admins stay admins when the claim lacks the value", func(t *testing.T) {
		f := newRoleRefreshFixture(t)
		_, err := f.store.Grants().Grant(context.Background(), authTestDBAdminID, nil)
		require.NoError(t, err)
		matched := roleRefreshAdmin
		f.syncSignIn(t, authTestDBAdminID, &matched)
		f.iss.SetClaim("groups", []string{"developers"})
		for _, id := range []string{authTestRootID, authTestDBAdminID} {
			assert.True(t, f.meIsAdmin(t, f.asRefreshingSession(id)), "%s keeps its own source", id)
		}
		assert.Equal(t, int64(2), f.iss.RefreshCalls(), "both sessions refreshed")
		assert.False(t, f.isProviderAdmin(t, authTestDBAdminID), "only the provider-sourced status went")
		granted, err := f.store.Grants().IsGranted(context.Background(), authTestDBAdminID)
		require.NoError(t, err)
		assert.True(t, granted, "the refresh never touches a manual grant")
	})
}

func TestSessionRefresh_RoleMappingFailureKeepsTheSession(t *testing.T) {
	f := newEpochFixture(t, 0)
	rows := withOwnSessionStore(f.srv)
	auth := svcmocks.NewMockAuthServiceInterface(t)
	f.container.authService = auth
	claims := &idp.Claims{Subject: refreshSubject, Raw: map[string]any{"sub": refreshSubject}}
	auth.On("RefreshTokens", mock.Anything, "oidc", "rt-1").
		Return(&idp.Tokens{AccessToken: "at-2", RefreshToken: "rt-2", ExpiresAt: time.Now().Add(time.Hour)},
			claims, nil).Once()
	auth.On("ApplyRefreshedRoleMapping", mock.Anything, "oidc", epochUserID, claims).
		Return(errors.New("db down")).Once()
	cookie := storedSessionCookie(t, f.srv, refreshableSession("rt-1", time.Now().Add(-time.Minute)))

	rec, _, called := f.requireAuth(t, cookie)

	assert.False(t, called, "the handler never runs on stale admin status")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, sessionRefreshRetryAfter, rec.Header().Get("Retry-After"))
	_, refresh := decryptedRowTokens(t, rows.only())
	assert.Equal(t, "rt-2", refresh, "the rotated refresh token is kept, so it is never replayed")
}

func TestSessionRefresh_RoleMappingRunsOnlyWhenRefreshed(t *testing.T) {
	f := newEpochFixture(t, 0)
	withOwnSessionStore(f.srv)
	auth := svcmocks.NewMockAuthServiceInterface(t)
	f.container.authService = auth
	cookie := storedSessionCookie(t, f.srv, refreshableSession("rt-1", time.Now().Add(time.Hour)))

	rec, _, called := f.requireAuth(t, cookie)

	require.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
	auth.AssertNotCalled(t, "ApplyRefreshedRoleMapping", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
