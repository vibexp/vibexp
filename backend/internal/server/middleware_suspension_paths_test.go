package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// #454 AC: "one test per entry point proving a suspended user is rejected".
// middleware_suspension_test.go covers the shared chokepoint; this file drives
// each middleware end-to-end, because the value of the chokepoint depends on
// every path actually routing through it AND handling its error correctly.
//
// The OIDC and MCP legs live with their transports
// (auth_handlers_integration_test.go, mcp_oauth_test.go).

const suspendedPathUserID = "suspended-path-user"

// suspensionPathContainer wires the two dependencies these middlewares touch: a
// user repository reporting the account status, and (optionally) an API-key
// service resolving a token to that user.
type suspensionPathContainer struct {
	*BaseMockContainer
	users     repositories.UserRepository
	apiKeySvc services.APIKeyServiceInterface
}

func (c suspensionPathContainer) UserRepository() repositories.UserRepository { return c.users }
func (c suspensionPathContainer) APIKeyService() services.APIKeyServiceInterface {
	return c.apiKeySvc
}

// suspensionUserRepo reports the test user with the given status.
func suspensionUserRepo(t *testing.T, status string) repositories.UserRepository {
	t.Helper()
	users := repomocks.NewMockUserRepository(t)
	users.On("GetByID", mock.Anything, suspendedPathUserID).
		Return(&models.User{ID: suspendedPathUserID, Status: status}, nil).Maybe()
	return users
}

// newSuspensionPathServer builds a Server whose user lookup reports `status`.
func newSuspensionPathServer(t *testing.T, status string) *Server {
	t.Helper()

	sessMgr, err := sesslib.NewManager(testCookiePassword, true)
	require.NoError(t, err)

	return &Server{
		container: suspensionPathContainer{
			BaseMockContainer: &BaseMockContainer{},
			users:             suspensionUserRepo(t, status),
		},
		sessionManager: sessMgr,
		logger:         slog.New(slog.DiscardHandler),
	}
}

// suspensionContainerWithAPIKey adds an API-key service that resolves the fixed
// test token to the test user.
func suspensionContainerWithAPIKey(
	t *testing.T, status string, keySvc services.APIKeyServiceInterface,
) container.Container {
	t.Helper()
	return suspensionPathContainer{
		BaseMockContainer: &BaseMockContainer{},
		users:             suspensionUserRepo(t, status),
		apiKeySvc:         keySvc,
	}
}

// sessionCookieFor mints a valid, unexpired session cookie for the test user —
// i.e. a credential that was legitimately issued BEFORE the suspension.
func sessionCookieFor(t *testing.T, srv *Server) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	require.NoError(t, srv.sessionManager.Write(rec, &sesslib.Session{
		UserID:      suspendedPathUserID,
		AccessToken: "at",
		ExpiresAt:   time.Now().Add(time.Hour),
	}))
	for _, c := range rec.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			return c
		}
	}
	t.Fatal("session cookie not written")
	return nil
}

// TestAuthenticateWithSession_RejectsSuspended is the cookie-session entry
// point: an ALREADY-ISSUED session must stop working, which is the difference
// between suspension and merely blocking new sign-ins.
func TestAuthenticateWithSession_RejectsSuspended(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		wantCode   int
		wantCalled bool
	}{
		{"active session still works", models.UserStatusActive, http.StatusOK, true},
		{"suspended session is refused", models.UserStatusSuspended, http.StatusUnauthorized, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSuspensionPathServer(t, tc.status)

			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
			req.AddCookie(sessionCookieFor(t, srv))
			rr := httptest.NewRecorder()

			srv.authenticateWithSession(rr, req, next)

			assert.Equal(t, tc.wantCode, rr.Code)
			assert.Equal(t, tc.wantCalled, called)
			if !tc.wantCalled {
				assert.Contains(t, rr.Body.String(), suspendedAuthDetail)
			}
		})
	}
}

// TestOptionalSessionContext_SuspendedIsAnonymous covers the optional-auth cookie
// leg. Anonymous — not rejected — is what makes instanceAdminMiddleware 404 a
// suspended admin off the admin surface rather than serving it.
func TestOptionalSessionContext_SuspendedIsAnonymous(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		wantOK  bool
		wantUID string
	}{
		{"active attaches the user", models.UserStatusActive, true, suspendedPathUserID},
		{"suspended proceeds anonymous", models.UserStatusSuspended, false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSuspensionPathServer(t, tc.status)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
			req.AddCookie(sessionCookieFor(t, srv))

			ctx, ok := srv.optionalSessionContext(req)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.NotNil(t, ctx)
				assert.Equal(t, tc.wantUID, ctx.Value(contextkeys.UserID))
			} else {
				assert.Nil(t, ctx)
			}
		})
	}
}

// TestAuthenticateWithAPIKey_RejectsSuspended is the API-key entry point: a
// valid, unexpired key belonging to a suspended account must stop working
// immediately rather than at rotation.
func TestAuthenticateWithAPIKey_RejectsSuspended(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		wantCode   int
		wantCalled bool
	}{
		{"active key works", models.UserStatusActive, http.StatusOK, true},
		{"suspended key is refused", models.UserStatusSuspended, http.StatusUnauthorized, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
			keySvc.On("ValidateAPIKey", mock.Anything, "the-token").
				Return(&models.APIKey{ID: "key-1", UserID: suspendedPathUserID}, nil)
			srv := newSuspensionPathServer(t, tc.status)
			srv.container = suspensionContainerWithAPIKey(t, tc.status, keySvc)

			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
			rr := httptest.NewRecorder()

			srv.authenticateWithAPIKey(rr, req, next, "the-token")

			assert.Equal(t, tc.wantCode, rr.Code)
			assert.Equal(t, tc.wantCalled, called)
		})
	}
}

// TestOptionalAPIKeyContext_SuspendedIsAnonymous covers the optional-auth
// API-key leg.
func TestOptionalAPIKeyContext_SuspendedIsAnonymous(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		wantOK bool
	}{
		{"active attaches the user", models.UserStatusActive, true},
		{"suspended proceeds anonymous", models.UserStatusSuspended, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
			keySvc.On("ValidateAPIKey", mock.Anything, "the-token").
				Return(&models.APIKey{ID: "key-1", UserID: suspendedPathUserID}, nil)
			srv := newSuspensionPathServer(t, tc.status)
			srv.container = suspensionContainerWithAPIKey(t, tc.status, keySvc)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
			ctx, ok := srv.optionalAPIKeyContext(req, "the-token")

			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, suspendedPathUserID, ctx.Value(contextkeys.UserID))
			} else {
				assert.Nil(t, ctx)
			}
		})
	}
}
