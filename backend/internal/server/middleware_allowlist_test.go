package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// Per-request access allowlist enforcement (#1235).
//
// Tightening the allowlist is an access revocation, so it must reach
// credentials issued while the user still matched. Every test below
// authenticates once with no allowlist stored, tightens the allowlist, and then
// presents the SAME credential again.

const (
	allowlistUserID    = "allowlist-user"
	allowlistUserEmail = "guest@other.com"
	allowlistRootEmail = "root@corp.example"
	allowlistAPIToken  = "the-token"
)

// allowlistContainer supplies the user repository, the allowlist resolver and
// (for the API-key legs) the API-key service the auth paths touch.
type allowlistContainer struct {
	*BaseMockContainer
	users     repositories.UserRepository
	allowlist services.AccessAllowlistResolver
	apiKeySvc services.APIKeyServiceInterface
	admins    services.InstanceAdminResolver
}

func (c allowlistContainer) UserRepository() repositories.UserRepository { return c.users }
func (c allowlistContainer) AccessAllowlistResolver() services.AccessAllowlistResolver {
	return c.allowlist
}
func (c allowlistContainer) APIKeyService() services.APIKeyServiceInterface { return c.apiKeySvc }
func (c allowlistContainer) InstanceAdminResolver() services.InstanceAdminResolver {
	if c.admins != nil {
		return c.admins
	}
	return c.BaseMockContainer.InstanceAdminResolver()
}

// allowlistFixture is a Server over the real allowlist resolver and an
// in-memory allowlist, with allowlistRootEmail as the only root admin.
type allowlistFixture struct {
	srv   *Server
	store *memAccessAllowlist
	users memUsers
	logs  *logtest.Recorder
}

// newAllowlistFixture registers users, every one of them active unless its
// Status says otherwise. The API-key service resolves allowlistAPIToken to
// allowlistUserID.
func newAllowlistFixture(t *testing.T, users ...*models.User) *allowlistFixture {
	t.Helper()
	logger, logs := logtest.New()
	sessMgr, err := sesslib.NewManager(testCookiePassword, true)
	require.NoError(t, err)

	f := &allowlistFixture{
		store: &memAccessAllowlist{},
		users: memUsers{users: map[string]*models.User{}},
		logs:  logs,
	}
	for _, u := range users {
		f.users.users[u.ID] = u
	}
	keySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
	keySvc.On("ValidateAPIKey", mock.Anything, allowlistAPIToken).
		Return(&models.APIKey{ID: "key-1", UserID: allowlistUserID}, nil).Maybe()

	f.srv = &Server{
		container: allowlistContainer{
			BaseMockContainer: &BaseMockContainer{},
			users:             f.users,
			allowlist:         newMemAllowlistResolver(logger, f.store, allowlistRootEmail),
			apiKeySvc:         keySvc,
		},
		sessionManager: sessMgr,
		logger:         slog.New(slog.DiscardHandler),
	}
	return f
}

func allowlistGuest() *models.User {
	return &models.User{ID: allowlistUserID, Email: allowlistUserEmail, Status: models.UserStatusActive}
}

// tighten stores an allowlist that does not admit allowlistUserEmail.
func (f *allowlistFixture) tighten() { f.store.set([]string{"example.com"}, nil) }

func (f *allowlistFixture) sessionCookie(t *testing.T) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	require.NoError(t, f.srv.sessionManager.Write(rec, &sesslib.Session{
		UserID: allowlistUserID, AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour),
	}))
	for _, c := range rec.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			return c
		}
	}
	t.Fatal("session cookie not written")
	return nil
}

// TestAuthenticateUser_AllowlistGate is the chokepoint contract every
// credential type inherits.
func TestAuthenticateUser_AllowlistGate(t *testing.T) {
	ctx := context.Background()
	root := &models.User{ID: "root-1", Email: "Root@Corp.example", Status: models.UserStatusActive}
	member := &models.User{ID: "member-1", Email: "dev@example.com", Status: models.UserStatusActive}
	delegate := &models.User{ID: "delegate-1", Email: "delegate@other.com", Status: models.UserStatusActive}
	suspended := &models.User{ID: "suspended-1", Email: "gone@other.com", Status: models.UserStatusSuspended}

	t.Run("with no allowlist stored everyone authenticates", func(t *testing.T) {
		f := newAllowlistFixture(t, allowlistGuest())
		got, err := f.srv.authenticateUser(ctx, allowlistUserID, "cookie", nil)
		require.NoError(t, err)
		assert.Equal(t, allowlistUserID, got.Value(contextkeys.UserID))
	})

	t.Run("an active allowlist admits a match and rejects everyone else", func(t *testing.T) {
		f := newAllowlistFixture(t, allowlistGuest(), member)
		f.tighten()

		got, err := f.srv.authenticateUser(ctx, member.ID, "cookie", nil)
		require.NoError(t, err)
		assert.Equal(t, member.ID, got.Value(contextkeys.UserID))

		got, err = f.srv.authenticateUser(ctx, allowlistUserID, "cookie", nil)
		require.ErrorIs(t, err, errUserNotOnAllowlist)
		assert.Nil(t, got, "no authenticated context may be built on rejection")
	})

	t.Run("a suspended account is reported as suspended, not as off the allowlist", func(t *testing.T) {
		f := newAllowlistFixture(t, suspended)
		f.tighten()
		_, err := f.srv.authenticateUser(ctx, suspended.ID, "cookie", nil)
		require.ErrorIs(t, err, errUserSuspended)
	})

	t.Run("a root admin is exempt; a DB-granted admin is not", func(t *testing.T) {
		f := newAllowlistFixture(t, root, delegate)
		admins := services.NewInstanceAdminService([]string{allowlistRootEmail},
			&memInstanceAdminGrants{granted: map[string]bool{delegate.ID: true}}, f.users,
			slog.New(slog.DiscardHandler))
		isAdmin, err := admins.IsInstanceAdmin(ctx, delegate)
		require.NoError(t, err)
		require.True(t, isAdmin, "the delegate really is an instance admin")
		f.tighten()

		got, err := f.srv.authenticateUser(ctx, root.ID, "cookie", nil)
		require.NoError(t, err, "an allowlist mistake must not lock out the trust root")
		assert.Equal(t, root.ID, got.Value(contextkeys.UserID))
		assert.Equal(t, 1, countLogs(f.logs,
			"Root instance admin is exempt from the access allowlist their email does not match"))

		_, err = f.srv.authenticateUser(ctx, delegate.ID, "cookie", nil)
		require.ErrorIs(t, err, errUserNotOnAllowlist)
	})

	t.Run("an unreadable allowlist fails CLOSED while one is known to be active", func(t *testing.T) {
		f := newAllowlistFixture(t, member)
		f.tighten()
		_, err := f.srv.authenticateUser(ctx, member.ID, "cookie", nil)
		require.NoError(t, err)

		dbDown := errors.New("db down")
		f.store.fail(dbDown)
		got, err := f.srv.authenticateUser(ctx, member.ID, "cookie", nil)
		require.ErrorIs(t, err, dbDown)
		assert.NotErrorIs(t, err, errUserNotOnAllowlist,
			"an infrastructure failure must not masquerade as an allowlist rejection")
		assert.Nil(t, got)
	})

	t.Run("an unreadable allowlist fails OPEN while none is known to be active", func(t *testing.T) {
		f := newAllowlistFixture(t, allowlistGuest())
		_, err := f.srv.authenticateUser(ctx, allowlistUserID, "cookie", nil)
		require.NoError(t, err)

		f.store.fail(errors.New("db down"))
		got, err := f.srv.authenticateUser(ctx, allowlistUserID, "cookie", nil)
		require.NoError(t, err, "an open instance must not refuse everyone over a database error")
		assert.NotNil(t, got)
	})

	t.Run("a missing resolver fails closed", func(t *testing.T) {
		f := newAllowlistFixture(t, allowlistGuest())
		f.srv.container = allowlistContainer{BaseMockContainer: &BaseMockContainer{}, users: f.users}
		got, err := f.srv.authenticateUser(ctx, allowlistUserID, "cookie", nil)
		require.Error(t, err)
		assert.NotErrorIs(t, err, errUserNotOnAllowlist)
		assert.Nil(t, got)
	})
}

func countLogs(logs *logtest.Recorder, message string) int {
	n := 0
	for _, e := range logs.AllEntries() {
		if e.Message == message {
			n++
		}
	}
	return n
}

// requireAuth drives one required-auth middleware and reports the status and
// whether the handler ran.
type requireAuth func(f *allowlistFixture, w http.ResponseWriter, r *http.Request, next http.Handler)

func (f *allowlistFixture) serve(t *testing.T, run requireAuth, decorate func(*http.Request)) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	if decorate != nil {
		decorate(req)
	}
	rr := httptest.NewRecorder()
	run(f, rr, req, next)
	return rr, called
}

// TestRequiredAuth_AllowlistTighteningBlocksTheNextRequest covers the three
// credential types on the required-auth paths: session cookie, API key and
// OAuth JWT (the /api/v1 bearer and the MCP bearer).
func TestRequiredAuth_AllowlistTighteningBlocksTheNextRequest(t *testing.T) {
	jwks := newOAuthJWKSServer(t)
	bearer := "Bearer " + jwks.sign(t, validOAuthClaims(jwks.issuer()))

	tests := []struct {
		name     string
		run      requireAuth
		decorate func(*allowlistFixture, *testing.T) func(*http.Request)
	}{
		{
			name: "session cookie",
			run: func(f *allowlistFixture, w http.ResponseWriter, r *http.Request, next http.Handler) {
				f.srv.authenticateWithSession(w, r, next)
			},
			decorate: func(f *allowlistFixture, t *testing.T) func(*http.Request) {
				cookie := f.sessionCookie(t)
				return func(r *http.Request) { r.AddCookie(cookie) }
			},
		},
		{
			name: "API key",
			run: func(f *allowlistFixture, w http.ResponseWriter, r *http.Request, next http.Handler) {
				f.srv.authenticateWithAPIKey(w, r, next, allowlistAPIToken)
			},
		},
		{
			name: "OAuth JWT on the API",
			run: func(f *allowlistFixture, w http.ResponseWriter, r *http.Request, next http.Handler) {
				attachVerifier(t, f.srv, jwks, oauthStubResolver{id: allowlistUserID})
				f.srv.flexibleAuthMiddleware(next).ServeHTTP(w, r)
			},
			decorate: func(*allowlistFixture, *testing.T) func(*http.Request) {
				return func(r *http.Request) { r.Header.Set("Authorization", bearer) }
			},
		},
		{
			name: "OAuth token on MCP",
			run: func(f *allowlistFixture, w http.ResponseWriter, r *http.Request, next http.Handler) {
				verifier := func(context.Context, string, *http.Request) (*mcpauth.TokenInfo, error) {
					return &mcpauth.TokenInfo{UserID: allowlistUserID, Expiration: time.Now().Add(time.Hour)}, nil
				}
				mcpauth.RequireBearerToken(verifier, nil)(f.srv.mcpTokenContextMiddleware(next)).ServeHTTP(w, r)
			},
			decorate: func(*allowlistFixture, *testing.T) func(*http.Request) {
				return func(r *http.Request) { r.Header.Set("Authorization", "Bearer valid-token") }
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newAllowlistFixture(t, allowlistGuest())
			var decorate func(*http.Request)
			if tc.decorate != nil {
				decorate = tc.decorate(f, t)
			}

			rr, called := f.serve(t, tc.run, decorate)
			require.Equal(t, http.StatusOK, rr.Code, "signed in before the allowlist was tightened")
			require.True(t, called)

			f.tighten()
			rr, called = f.serve(t, tc.run, decorate)
			assert.Equal(t, http.StatusUnauthorized, rr.Code, "the same credential is refused on the next request")
			assert.False(t, called)
			assert.Contains(t, rr.Body.String(), notOnAllowlistAuthDetail)

			// Loosening it again restores access with nothing else changed.
			f.store.set(nil, nil)
			rr, called = f.serve(t, tc.run, decorate)
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.True(t, called)
		})
	}
}

// TestRequiredAuth_UnreadableActiveAllowlistIs500 pins that a database failure
// while an allowlist is active is a 500, never a 401: an outage must not look
// like a revoked credential.
func TestRequiredAuth_UnreadableActiveAllowlistIs500(t *testing.T) {
	member := &models.User{ID: allowlistUserID, Email: "dev@example.com", Status: models.UserStatusActive}
	f := newAllowlistFixture(t, member)
	f.tighten()
	run := func(f *allowlistFixture, w http.ResponseWriter, r *http.Request, next http.Handler) {
		f.srv.authenticateWithAPIKey(w, r, next, allowlistAPIToken)
	}
	rr, called := f.serve(t, run, nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.True(t, called)

	f.store.fail(errors.New("db down"))
	rr, called = f.serve(t, run, nil)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.False(t, called)
	assert.NotContains(t, rr.Body.String(), "db down")
}

// TestOptionalAuth_AllowlistTighteningMakesTheNextRequestAnonymous covers the
// same three credential types on the optional-auth paths, which serve the
// request anonymously instead of rejecting it, mirroring suspension.
func TestOptionalAuth_AllowlistTighteningMakesTheNextRequestAnonymous(t *testing.T) {
	jwks := newOAuthJWKSServer(t)
	token := jwks.sign(t, validOAuthClaims(jwks.issuer()))

	tests := []struct {
		name    string
		resolve func(f *allowlistFixture, t *testing.T) func() (context.Context, bool)
	}{
		{
			name: "session cookie",
			resolve: func(f *allowlistFixture, t *testing.T) func() (context.Context, bool) {
				cookie := f.sessionCookie(t)
				return func() (context.Context, bool) {
					req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
					req.AddCookie(cookie)
					return f.srv.optionalSessionContext(req)
				}
			},
		},
		{
			name: "API key",
			resolve: func(f *allowlistFixture, _ *testing.T) func() (context.Context, bool) {
				return func() (context.Context, bool) {
					req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
					return f.srv.optionalAPIKeyContext(req, allowlistAPIToken)
				}
			},
		},
		{
			name: "OAuth JWT",
			resolve: func(f *allowlistFixture, t *testing.T) func() (context.Context, bool) {
				attachVerifier(t, f.srv, jwks, oauthStubResolver{id: allowlistUserID})
				return func() (context.Context, bool) {
					req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
					return f.srv.optionalOAuthJWTContext(req, token)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newAllowlistFixture(t, allowlistGuest())
			resolve := tc.resolve(f, t)

			ctx, ok := resolve()
			require.True(t, ok)
			require.Equal(t, allowlistUserID, ctx.Value(contextkeys.UserID))

			f.tighten()
			ctx, ok = resolve()
			assert.False(t, ok, "the request proceeds anonymous")
			assert.Nil(t, ctx)

			// An unreadable active allowlist is anonymous too, never authenticated.
			f.store.fail(errors.New("db down"))
			ctx, ok = resolve()
			assert.False(t, ok)
			assert.Nil(t, ctx)
		})
	}
}

// TestLogSuspendedRejection_AllowlistReason pins the audit trail: an allowlist
// rejection is logged at Info with reason=not_on_allowlist, distinct from the
// suspension line and from an infrastructure failure.
func TestLogSuspendedRejection_AllowlistReason(t *testing.T) {
	entryFor := func(err error) *logtest.Entry {
		logger, logs := logtest.New()
		ctx := context.WithValue(context.Background(), contextkeys.Logger, logger)
		(&Server{}).logSuspendedRejection(ctx, "authenticateWithSession", "cookie", allowlistUserID, err)
		require.Len(t, logs.AllEntries(), 1)
		return logs.LastEntry()
	}

	allowlist := entryFor(errUserNotOnAllowlist)
	assert.Equal(t, slog.LevelInfo, allowlist.Level)
	assert.Equal(t, "Rejected request from account no longer on the access allowlist", allowlist.Message)
	assert.Equal(t, "not_on_allowlist", allowlist.Data["reason"])
	assert.Equal(t, allowlistUserID, allowlist.Data["user_id"])
	assert.Equal(t, "cookie", allowlist.Data["auth_type"])

	suspension := entryFor(errUserSuspended)
	assert.Equal(t, "Rejected request from suspended account", suspension.Message)
	assert.NotContains(t, suspension.Data, "reason")

	failure := entryFor(errors.New("db down"))
	assert.Equal(t, slog.LevelError, failure.Level)
	assert.NotContains(t, failure.Data, "reason")
}
