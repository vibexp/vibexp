package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/password"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Public invitation operations (#1435).

const (
	invRegTestToken  = "tok-invite-123"
	invRegTestTeamID = "0b7c3f2e-6a4d-4c1e-9a55-1f2e3d4c5b6a"
	invRegTestPhrase = "a long invited passphrase"
	invRegPreviewURL = "/api/v1/invitations/" + invRegTestToken + "/preview"
	invRegRegister   = "/api/v1/invitations/" + invRegTestToken + "/register"
)

// invRegContainer adds the invitation registration service to the auth mock
// container.
type invRegContainer struct {
	*MockAuthContainer
	registration services.InvitationRegistrationServiceInterface
}

func (c *invRegContainer) InvitationRegistrationService() services.InvitationRegistrationServiceInterface {
	return c.registration
}

type invRegHandlerFixture struct {
	srv          *Server
	container    *MockAuthContainer
	registration *servicesmocks.MockInvitationRegistrationServiceInterface
}

// newInvRegHandlerFixture mounts the two operations with password login on or off.
func newInvRegHandlerFixture(t *testing.T, passwordLogin bool) *invRegHandlerFixture {
	t.Helper()
	f := &invRegHandlerFixture{
		container:    newMockAuthContainer(t),
		registration: servicesmocks.NewMockInvitationRegistrationServiceInterface(t),
	}
	f.container.authPolicy = staticAuthPolicy{
		policy: models.InstanceAuthPolicy{PasswordLoginEnabled: passwordLogin, RegistrationMode: models.RegistrationModeOpen},
		reset:  services.PasswordResetByAdmin,
	}
	f.srv = createTestAuthServer(f.container)
	f.srv.container = &invRegContainer{MockAuthContainer: f.container, registration: f.registration}
	f.srv.setupInvitationRegistrationRoutes()
	return f
}

// send serves one request and checks the response against the OpenAPI spec.
func (f *invRegHandlerFixture) send(t *testing.T, method, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" && contentType == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	specconformance.AssertConformsToSpec(t, req, rec)
	return rec
}

func (f *invRegHandlerFixture) register(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.send(t, http.MethodPost, invRegRegister, "application/json", body)
}

func invRegProblemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var p struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p), rec.Body.String())
	return p.Code
}

func TestPreviewInvitation(t *testing.T) {
	t.Run("a signed-out visitor gets exactly the documented fields", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, false) // works while password login is off
		expires := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
		f.registration.EXPECT().Preview(mock.Anything, invRegTestToken).Return(&services.InvitationPreview{
			TeamName: "Acme", InviterName: "Ada Lovelace", Email: "grace@example.com",
			Role: models.TeamMemberRoleAdmin, ExpiresAt: expires, AccountExists: true,
		}, nil).Once()

		rec := f.send(t, http.MethodGet, invRegPreviewURL, "", "")

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, map[string]any{
			"team_name": "Acme", "inviter_name": "Ada Lovelace", "email": "grace@example.com", "role": "admin",
			"expires_at": "2026-10-21T09:00:00Z", "account_exists": true, "password_registration": false,
		}, got, "no id, no inviter email, no status")
	})

	t.Run("an inviter with no name is null", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, true)
		f.registration.EXPECT().Preview(mock.Anything, invRegTestToken).Return(&services.InvitationPreview{
			TeamName: "Acme", Email: "grace@example.com", Role: models.TeamMemberRoleMember,
			ExpiresAt: time.Now().Add(time.Hour), PasswordRegistration: true,
		}, nil).Once()

		rec := f.send(t, http.MethodGet, invRegPreviewURL, "", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"inviter_name":null`)
		assert.Contains(t, rec.Body.String(), `"password_registration":true`)
	})

	t.Run("an unusable link is one 404, whatever the reason", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, true)
		f.registration.EXPECT().Preview(mock.Anything, invRegTestToken).Return(nil, services.ErrInvitationUnusable).Once()

		rec := f.send(t, http.MethodGet, invRegPreviewURL, "", "")

		require.Equal(t, http.StatusNotFound, rec.Code)
		assert.Equal(t, "invitation_invalid", invRegProblemCode(t, rec))
		assert.NotContains(t, rec.Body.String(), invRegTestToken, "the token is not echoed, not even in `instance`")
	})

	t.Run("a token with reserved characters is percent-decoded as a path segment", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, true)
		// "+" must stay a plus (PathUnescape), "%2F" becomes a slash.
		f.registration.EXPECT().Preview(mock.Anything, "a+b/c").Return(nil, services.ErrInvitationUnusable).Once()

		rec := f.send(t, http.MethodGet, "/api/v1/invitations/a+b%2Fc/preview", "", "")

		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("an undecodable or oversized token never reaches the service", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, true) // a strict mock

		bad := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/x/preview", nil)
		bad.URL.RawPath = "/api/v1/invitations/%zz/preview"
		bad.URL.Path = "/api/v1/invitations/%zz/preview"
		rec := httptest.NewRecorder()
		f.srv.ServeHTTP(rec, bad)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

		long := f.send(t, http.MethodGet, "/api/v1/invitations/"+strings.Repeat("t", maxInvitationTokenBytes+1)+"/preview", "", "")
		require.Equal(t, http.StatusNotFound, long.Code)
		assert.Equal(t, "invitation_invalid", invRegProblemCode(t, long))
	})

	t.Run("a storage failure is a 500 that names nothing", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, true)
		f.registration.EXPECT().Preview(mock.Anything, invRegTestToken).Return(nil, errors.New("pq: connection refused")).Once()

		rec := f.send(t, http.MethodGet, invRegPreviewURL, "", "")

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.NotContains(t, rec.Body.String(), "pq:")
	})
}

func invRegNewUser() *models.User {
	return &models.User{
		ID: "9a1d2c3b-4e5f-4a6b-8c7d-0e1f2a3b4c5d", Email: "grace@example.com", Name: "Grace Hopper",
		Status: models.UserStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now(), Version: 1,
	}
}

func TestRegisterWithInvitation(t *testing.T) {
	body := `{"password":"` + invRegTestPhrase + `","name":"Grace Hopper"}`

	t.Run("creates the account and signs the person in", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, true)
		f.registration.EXPECT().Register(mock.Anything, invRegTestToken, invRegTestPhrase, "Grace Hopper").
			Return(&services.InvitationRegistration{User: invRegNewUser(), TeamID: invRegTestTeamID, InvitationAccepted: true}, nil).
			Once()
		var recorded map[string]interface{}
		f.container.activityService.On("RecordAuthActivity", mock.Anything, invRegNewUser().ID,
			activities.ActivityTypeAuthLogin, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { recorded = args.Get(4).(map[string]interface{}) }).Return(nil).Once()

		rec := f.register(t, body)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var got struct {
			User               struct{ ID, Email string } `json:"user"`
			TeamID             string                     `json:"team_id"`
			InvitationAccepted bool                       `json:"invitation_accepted"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, "grace@example.com", got.User.Email)
		assert.Equal(t, invRegTestTeamID, got.TeamID)
		assert.True(t, got.InvitationAccepted)
		assert.NotContains(t, rec.Body.String(), invRegTestPhrase)

		// The cookie is a password session for the new account.
		var cookie *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == sesslib.CookieName {
				cookie = c
			}
		}
		require.NotNil(t, cookie, "the session cookie is set")
		probe := httptest.NewRequest(http.MethodGet, "/", nil)
		probe.AddCookie(cookie)
		sess, err := f.srv.sessionManager.Read(probe)
		require.NoError(t, err)
		assert.Equal(t, invRegNewUser().ID, sess.UserID)
		assert.Equal(t, sesslib.AuthMethodPassword, sess.AuthMethod)
		assert.Equal(t, "invitation", recorded["via"], "the sign-in activity says how the person got in")
	})

	t.Run("a partial outcome is unmistakable: signed in, invitation not accepted", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, true)
		f.registration.EXPECT().Register(mock.Anything, invRegTestToken, invRegTestPhrase, "").
			Return(&services.InvitationRegistration{User: invRegNewUser(), TeamID: invRegTestTeamID}, nil).Once()
		f.container.activityService.On("RecordAuthActivity", mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		rec := f.register(t, `{"password":"`+invRegTestPhrase+`"}`)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"invitation_accepted":false`)
		assert.NotEmpty(t, rec.Result().Cookies(), "the person is signed in and accepts in the app")
	})

	t.Run("refusals keep their documented status and code, and set no cookie", func(t *testing.T) {
		for name, tc := range map[string]struct {
			err        error
			wantStatus int
			wantCode   string
		}{
			"unusable link":  {services.ErrInvitationUnusable, http.StatusNotFound, "invitation_invalid"},
			"account exists": {services.ErrInvitationAccountExists, http.StatusConflict, "account_exists"},
			"allowlist":      {services.ErrInvitationEmailNotAllowed, http.StatusForbidden, "access_restricted"},
			"weak password":  {password.Validate("short", "grace@example.com"), http.StatusBadRequest, "VALIDATION_FAILED"},
			"storage":        {errors.New("pq: connection refused"), http.StatusInternalServerError, "INTERNAL_ERROR"},
		} {
			f := newInvRegHandlerFixture(t, true)
			f.registration.EXPECT().Register(mock.Anything, invRegTestToken, mock.Anything, mock.Anything).
				Return(nil, tc.err).Once()

			rec := f.register(t, body)

			require.Equal(t, tc.wantStatus, rec.Code, name)
			assert.Equal(t, tc.wantCode, invRegProblemCode(t, rec), name)
			assert.Empty(t, rec.Result().Cookies(), name)
			assert.NotContains(t, rec.Body.String(), "pq:", name)
			assert.NotContains(t, rec.Body.String(), invRegTestPhrase, name)
		}
	})

	t.Run("a weak password names the rule on the password field", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, true)
		f.registration.EXPECT().Register(mock.Anything, invRegTestToken, "short", "").
			Return(nil, password.Validate("short", "grace@example.com")).Once()

		rec := f.register(t, `{"password":"short"}`)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), `"field":"password"`)
		assert.Contains(t, rec.Body.String(), `"code":"too_short"`)
	})

	t.Run("requests the service never sees", func(t *testing.T) {
		long := strings.Repeat("x", maxSignInPasswordBytes+1)
		for name, tc := range map[string]struct {
			contentType, body string
			wantStatus        int
		}{
			"a form post (login CSRF)": {"application/x-www-form-urlencoded", "password=" + invRegTestPhrase, http.StatusBadRequest},
			"text/plain JSON":          {"text/plain", body, http.StatusBadRequest},
			"no password":              {"application/json", `{"name":"Grace"}`, http.StatusBadRequest},
			"an unknown field":         {"application/json", `{"password":"` + invRegTestPhrase + `","email":"eve@example.com"}`, http.StatusBadRequest},
			"an overlong password":     {"application/json", `{"password":"` + long + `"}`, http.StatusBadRequest},
			"an overlong name":         {"application/json", `{"password":"` + invRegTestPhrase + `","name":"` + strings.Repeat("n", 256) + `"}`, http.StatusBadRequest},
			"malformed JSON":           {"application/json", `{"password":`, http.StatusBadRequest},
		} {
			f := newInvRegHandlerFixture(t, true) // a strict mock: Register is never expected

			rec := f.send(t, http.MethodPost, invRegRegister, tc.contentType, tc.body)

			require.Equal(t, tc.wantStatus, rec.Code, name+": "+rec.Body.String())
			assert.Empty(t, rec.Result().Cookies(), name)
			assert.NotContains(t, rec.Body.String(), invRegTestPhrase, name)
		}
	})

	t.Run("it does not exist while password login is off", func(t *testing.T) {
		f := newInvRegHandlerFixture(t, false) // a strict mock: Register is never expected

		rec := f.register(t, body)

		require.Equal(t, http.StatusNotFound, rec.Code)
		assert.NotEqual(t, "invitation_invalid", invRegProblemCode(t, rec), "not the unusable-link answer")
	})
}

// The public routes share the /api/v1/invitations/{token} prefix with the
// authenticated subtree (mounted with Route in server.go). Both must keep
// answering: chi tries the more specific public routes first and falls back to
// the mounted subtree for everything else.
func TestInvitationRegistrationRoutes_CoexistWithTheAuthenticatedSubtree(t *testing.T) {
	f := newInvRegHandlerFixture(t, true)
	reached := ""
	mark := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) { reached = name; w.WriteHeader(http.StatusNoContent) }
	}
	// As setupProtectedRoutes mounts them.
	f.srv.router.Route("/api/v1/invitations", func(r chi.Router) {
		r.Get("/pending", mark("pending"))
		r.Get("/{token}", mark("get"))
		r.Post("/{token}/accept", mark("accept"))
		r.Post("/{token}/reject", mark("reject"))
	})
	f.registration.EXPECT().Preview(mock.Anything, invRegTestToken).Return(nil, services.ErrInvitationUnusable).Once()

	for name, tc := range map[string]struct{ method, path string }{
		"pending": {http.MethodGet, "/api/v1/invitations/pending"},
		"get":     {http.MethodGet, "/api/v1/invitations/" + invRegTestToken},
		"accept":  {http.MethodPost, "/api/v1/invitations/" + invRegTestToken + "/accept"},
		"reject":  {http.MethodPost, "/api/v1/invitations/" + invRegTestToken + "/reject"},
	} {
		reached = ""
		rec := httptest.NewRecorder()
		f.srv.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		assert.Equal(t, name, reached, "the authenticated route still answers")
	}

	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, invRegPreviewURL, nil))
	assert.Equal(t, http.StatusNotFound, rec.Code, "the public preview answers (an unusable link here)")
	assert.Contains(t, rec.Body.String(), "invitation_invalid")
}

// Both operations are mounted on the real server, and both are public: the
// auth middleware of the protected routes is not in front of them.
func TestInvitationRegistrationRoutes_AreMountedOnTheRealServer(t *testing.T) {
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	var found []string
	require.NoError(t, chi.Walk(srv.router,
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if strings.HasPrefix(route, "/api/v1/invitations/{token}/") {
				found = append(found, method+" "+route)
			}
			return nil
		}))
	assert.Contains(t, found, "GET /api/v1/invitations/{token}/preview")
	assert.Contains(t, found, "POST /api/v1/invitations/{token}/register")
}
