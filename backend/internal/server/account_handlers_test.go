package server

import (
	"context"
	"encoding/json"
	"errors"
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
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Self-service account API (#1437). The service's rules are tested in
// services/account_test.go; these tests are about what only the HTTP layer
// knows: which credential authenticated the request, how recent the sign-in
// is, the session cookie that is re-issued, the documented status codes, and
// that every response conforms to the spec.

const (
	accountUUID  = "7c1f6f0e-6d0f-4d8e-9b6a-1b6f0c6f3a11"
	identityUUID = "2a6b9a53-3f0a-4c58-8f7f-6b0a7f3f9d22"
)

// accountContainer serves a mock AccountService over the auth test container.
type accountContainer struct {
	*MockAuthContainer
	account *servicesmocks.MockAccountServiceInterface
}

func (c *accountContainer) AccountService() services.AccountServiceInterface { return c.account }

// accountFixture is the account routes behind the real cookie middleware, over
// the session-epoch fixture: its user row carries an epoch that a revocation
// moves, so "the other sessions end" is the real mechanism, not a mock of it.
type accountFixture struct {
	*epochFixture
	account *servicesmocks.MockAccountServiceInterface
	router  chi.Router
}

func newAccountFixture(t *testing.T) *accountFixture {
	t.Helper()
	epoch := newEpochFixture(t, 3)
	account := servicesmocks.NewMockAccountServiceInterface(t)
	epoch.srv.container = &accountContainer{MockAuthContainer: epoch.container, account: account}
	// A revocation bumps the epoch the user row reports from then on.
	epoch.container.authService.On("RevokeUserSessions", mock.Anything, epochUserID).
		Return(func(context.Context, string) (int64, error) { return epoch.epoch.Add(1), nil }).Maybe()
	epoch.container.activityService.On("RecordAuthActivity",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			epoch.srv.authenticateWithSession(w, r, next)
		})
	})
	epoch.srv.setupAccountRoutes(router)
	return &accountFixture{epochFixture: epoch, account: account, router: router}
}

// session returns a cookie for the fixture user that signed in `age` ago.
func (f *accountFixture) session(t *testing.T, age time.Duration) *http.Cookie {
	t.Helper()
	return f.cookieFor(t, slidingSession(f.epoch.Load(), age))
}

// do sends a request with the cookie through the account routes.
func (f *accountFixture) do(method, path, body string, cookie *http.Cookie) (*http.Request, *httptest.ResponseRecorder) {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return req, rec
}

// stillSignedIn reports whether the cookie authenticates a request now.
func (f *accountFixture) stillSignedIn(t *testing.T, cookie *http.Cookie) bool {
	t.Helper()
	_, _, called := f.requireAuth(t, cookie)
	return called
}

func accountView(identities ...services.AccountIdentityView) *services.AccountView {
	return &services.AccountView{
		User:       &models.User{ID: accountUUID, Email: "ada@example.com", Name: "Ada"},
		Password:   services.AccountPasswordView{LoginEnabled: true},
		Identities: identities,
	}
}

// problem is the part of an RFC 9457 body these tests read.
type problem struct {
	Code             string `json:"code"`
	Detail           string `json:"detail"`
	ValidationErrors []struct {
		Field, Message, Code string
	} `json:"validation_errors"`
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) problem {
	t.Helper()
	var p problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p), rec.Body.String())
	return p
}

func TestGetAccount(t *testing.T) {
	t.Run("an account with no identities serializes an empty array", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("Get", mock.Anything, epochUserID).Return(accountView(), nil)

		req, rec := f.do(http.MethodGet, "/api/v1/account", "", f.session(t, time.Minute))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"identities":[]`, "a required array is never null")
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "ada@example.com", body["email"])
		assert.Equal(t, true, body["recent_sign_in"])
		specconformance.AssertConformsToSpec(t, req, rec)
	})

	t.Run("password and identities", func(t *testing.T) {
		f := newAccountFixture(t)
		changedAt := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
		lastUsed := changedAt.Add(time.Hour)
		view := accountView(
			services.AccountIdentityView{
				ID: identityUUID, Provider: "google", DisplayName: "Google", Available: true,
				ConnectedAt: changedAt, LastUsedAt: &lastUsed,
			},
			services.AccountIdentityView{
				ID: "9d3c1c0a-8f5e-4b0b-9a53-0d6a2e1b7c33", Provider: "old-sso", DisplayName: "old-sso",
				ConnectedAt: changedAt,
			},
		)
		view.Password = services.AccountPasswordView{Set: true, ChangedAt: &changedAt, LoginEnabled: false}
		f.account.On("Get", mock.Anything, epochUserID).Return(view, nil)

		req, rec := f.do(http.MethodGet, "/api/v1/account", "", f.session(t, time.Hour))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var body struct {
			Password struct {
				Set          bool    `json:"set"`
				ChangedAt    *string `json:"changed_at"`
				LoginEnabled bool    `json:"login_enabled"`
			} `json:"password"`
			Identities []struct {
				ID         string  `json:"id"`
				Available  bool    `json:"available"`
				LastUsedAt *string `json:"last_used_at"`
			} `json:"identities"`
			RecentSignIn bool `json:"recent_sign_in"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.True(t, body.Password.Set)
		require.NotNil(t, body.Password.ChangedAt)
		assert.False(t, body.Password.LoginEnabled)
		require.Len(t, body.Identities, 2)
		assert.Equal(t, identityUUID, body.Identities[0].ID)
		assert.NotNil(t, body.Identities[0].LastUsedAt)
		assert.False(t, body.Identities[1].Available)
		assert.Nil(t, body.Identities[1].LastUsedAt)
		assert.False(t, body.RecentSignIn, "a session that signed in an hour ago is not recent")
		specconformance.AssertConformsToSpec(t, req, rec)
	})

	t.Run("a failure is a 500 without detail", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("Get", mock.Anything, epochUserID).Return(nil, errors.New("pq: connection refused"))

		req, rec := f.do(http.MethodGet, "/api/v1/account", "", f.session(t, time.Minute))

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.NotContains(t, rec.Body.String(), "connection refused")
		specconformance.AssertConformsToSpec(t, req, rec)
	})
}

// "Recent" is the sign-in itself. A session that is kept alive by sliding does
// not become recent again, however lately it slid.
func TestAccount_RecentSignInIsTheSignInTime(t *testing.T) {
	now := time.Now()
	recent := func(sess *sesslib.Session) bool {
		return accountCaller{userID: "u", cookieSession: sess}.recentSignIn(now)
	}

	assert.True(t, recent(&sesslib.Session{AuthenticatedAt: now.Add(-9 * time.Minute)}))
	assert.False(t, recent(&sesslib.Session{AuthenticatedAt: now.Add(-11 * time.Minute)}))
	assert.False(t, recent(&sesslib.Session{}), "a session with no sign-in time, minted before #1428")
	assert.False(t, recent(&sesslib.Session{AuthenticatedAt: now.Add(time.Hour)}), "a sign-in time in the future")
	assert.False(t, recent(nil), "an API key or a bearer token")

	// A session that signed in two days ago and is extended by today's activity.
	slid := slidingSession(3, 48*time.Hour)
	signedInAt := slid.AuthenticatedAt
	require.True(t, slid.Slide(now), "fixture: the session must actually slide")
	assert.True(t, signedInAt.Equal(slid.AuthenticatedAt), "sliding must not move the sign-in time")
	assert.False(t, recent(slid), "a session that just slid is not recent because of it")

	t.Run("end to end: the slid cookie is not recent", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("Get", mock.Anything, epochUserID).Return(accountView(), nil)
		old := slidingSession(f.epoch.Load(), 48*time.Hour)
		require.True(t, old.Slide(time.Now()), "fixture: the session must actually slide")

		_, rec := f.do(http.MethodGet, "/api/v1/account", "", f.cookieFor(t, old))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"recent_sign_in":false`)
	})
}

func TestUpdateAccount(t *testing.T) {
	t.Run("renames and returns the account", func(t *testing.T) {
		f := newAccountFixture(t)
		view := accountView()
		view.User.Name = "Ada Lovelace"
		f.account.On("UpdateName", mock.Anything, epochUserID, "Ada Lovelace").Return(view, nil)

		req, rec := f.do(http.MethodPatch, "/api/v1/account", `{"name":"Ada Lovelace"}`, f.session(t, time.Hour))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"name":"Ada Lovelace"`)
		specconformance.AssertConformsToSpec(t, req, rec)
	})

	t.Run("an invalid name is a field error", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("UpdateName", mock.Anything, epochUserID, "   ").Return(nil, services.ErrAccountNameInvalid)

		req, rec := f.do(http.MethodPatch, "/api/v1/account", `{"name":"   "}`, f.session(t, time.Hour))

		require.Equal(t, http.StatusBadRequest, rec.Code)
		p := decodeProblem(t, rec)
		require.Len(t, p.ValidationErrors, 1)
		assert.Equal(t, "name", p.ValidationErrors[0].Field)
		specconformance.AssertConformsToSpec(t, req, rec)
	})

	t.Run("a malformed body is a 400 that quotes nothing", func(t *testing.T) {
		f := newAccountFixture(t)

		req, rec := f.do(http.MethodPatch, "/api/v1/account", `{"name":`, f.session(t, time.Hour))

		require.Equal(t, http.StatusBadRequest, rec.Code)
		specconformance.AssertConformsToSpec(t, req, rec)
	})
}

// Setting or changing a password ends the user's OTHER browser sessions and
// keeps the caller signed in, whether it is a first password or a change.
func TestSetAccountPassword_EndsOtherSessionsAndKeepsTheCaller(t *testing.T) {
	for name, first := range map[string]bool{"a first password": true, "a change": false} {
		t.Run(name, func(t *testing.T) {
			f := newAccountFixture(t)
			caller := f.session(t, time.Minute)
			otherDevice := f.session(t, 2*time.Hour)
			require.True(t, f.stillSignedIn(t, otherDevice), "fixture: the second session works before the change")
			f.account.On("SetPassword", mock.Anything, epochUserID, mock.MatchedBy(func(c services.AccountPasswordChange) bool {
				return c.New == "a brand new long passphrase" && c.Current == "the old one" && c.RecentSignIn
			})).Return(first, nil).Once()

			req, rec := f.do(http.MethodPut, "/api/v1/account/password",
				`{"current_password":"the old one","new_password":"a brand new long passphrase"}`, caller)

			require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
			assert.Empty(t, rec.Body.String())
			specconformance.AssertConformsToSpec(t, req, rec)

			reissued := sessionCookieIn(rec)
			require.NotNil(t, reissued, "the caller's cookie is re-issued under the new epoch")
			assert.True(t, f.stillSignedIn(t, reissued), "the calling session survives")
			assert.False(t, f.stillSignedIn(t, otherDevice), "the session on the other device is ended")
			assert.False(t, f.stillSignedIn(t, caller), "and so is the caller's own pre-change cookie")

			wantActivity := activities.ActivityTypeAuthCredentialChanged
			if first {
				wantActivity = activities.ActivityTypeAuthCredentialSet
			}
			f.container.activityService.AssertCalled(t, "RecordAuthActivity", mock.Anything, epochUserID, wantActivity,
				mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.account.AssertNotCalled(t, "RevokeAPIAccess", mock.Anything, mock.Anything)
		})
	}
}

func TestSetAccountPassword_RevokeAPIAccess(t *testing.T) {
	t.Run("revokes when asked", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("SetPassword", mock.Anything, epochUserID, mock.Anything).Return(false, nil)
		f.account.On("RevokeAPIAccess", mock.Anything, epochUserID).Return(nil).Once()

		_, rec := f.do(http.MethodPut, "/api/v1/account/password",
			`{"current_password":"old","new_password":"a brand new long passphrase","revoke_api_access":true}`,
			f.session(t, time.Hour))

		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})

	t.Run("a failed revocation says the password was changed, and the sessions are still ended", func(t *testing.T) {
		f := newAccountFixture(t)
		otherDevice := f.session(t, 2*time.Hour)
		f.account.On("SetPassword", mock.Anything, epochUserID, mock.Anything).Return(false, nil)
		f.account.On("RevokeAPIAccess", mock.Anything, epochUserID).Return(errors.New("pq: timeout"))

		req, rec := f.do(http.MethodPut, "/api/v1/account/password",
			`{"current_password":"old","new_password":"a brand new long passphrase","revoke_api_access":true}`,
			f.session(t, time.Hour))

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		p := decodeProblem(t, rec)
		assert.Contains(t, p.Detail, "password was changed")
		assert.NotContains(t, rec.Body.String(), "timeout")
		assert.False(t, f.stillSignedIn(t, otherDevice))
		specconformance.AssertConformsToSpec(t, req, rec)
	})
}

// Every refusal has its documented status and stable code, changes nothing,
// and leaves the user's sessions alone.
func TestSetAccountPassword_Refusals(t *testing.T) {
	policyErr := password.Validate("short", "ada@example.com")
	require.Error(t, policyErr)

	tests := []struct {
		name       string
		err        error
		status     int
		code       string
		field      string
		fieldCode  string
		retryAfter bool
	}{
		{name: "no password yet and the sign-in is not recent", err: services.ErrReauthenticationRequired,
			status: http.StatusForbidden, code: apierrors.CodeReauthenticationRequired},
		{name: "password sign-in is off", err: services.ErrPasswordLoginDisabled,
			status: http.StatusConflict, code: apierrors.CodeLocalSignInDisabled},
		{name: "wrong current password", err: services.ErrCurrentPasswordInvalid,
			status: http.StatusBadRequest, code: apierrors.CodeValidationFailed,
			field: "current_password", fieldCode: "invalid_current_password"},
		{name: "too many wrong attempts", err: &services.PasswordLockedError{Until: time.Now().Add(90 * time.Second)},
			status: http.StatusTooManyRequests, code: apierrors.CodeTooManyAttempts, retryAfter: true},
		{name: "the new password breaks the policy", err: policyErr,
			status: http.StatusBadRequest, code: apierrors.CodeValidationFailed,
			field: "new_password", fieldCode: "too_short"},
		{name: "an unexpected failure", err: errors.New("pq: connection refused"),
			status: http.StatusInternalServerError, code: apierrors.CodeInternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAccountFixture(t)
			caller := f.session(t, time.Hour)
			f.account.On("SetPassword", mock.Anything, epochUserID, mock.Anything).Return(false, tt.err)

			req, rec := f.do(http.MethodPut, "/api/v1/account/password",
				`{"current_password":"guess","new_password":"short"}`, caller)

			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			p := decodeProblem(t, rec)
			assert.Equal(t, tt.code, p.Code)
			if tt.field != "" {
				require.NotEmpty(t, p.ValidationErrors)
				assert.Equal(t, tt.field, p.ValidationErrors[0].Field)
				assert.Equal(t, tt.fieldCode, p.ValidationErrors[0].Code)
			}
			if tt.retryAfter {
				assert.Regexp(t, `^[0-9]+$`, rec.Header().Get("Retry-After"))
				assert.NotEqual(t, "0", rec.Header().Get("Retry-After"))
			} else {
				assert.Empty(t, rec.Header().Get("Retry-After"))
			}
			assert.NotContains(t, rec.Body.String(), "connection refused")
			assert.NotContains(t, rec.Body.String(), "guess", "no password is ever echoed")
			assert.Nil(t, sessionCookieIn(rec), "no cookie is re-issued")
			assert.True(t, f.stillSignedIn(t, caller), "a refused change ends no session")
			f.container.authService.AssertNotCalled(t, "RevokeUserSessions", mock.Anything, mock.Anything)
			specconformance.AssertConformsToSpec(t, req, rec)
		})
	}

	t.Run("a missing new password is refused before the service is asked", func(t *testing.T) {
		f := newAccountFixture(t)

		req, rec := f.do(http.MethodPut, "/api/v1/account/password", `{"new_password":""}`, f.session(t, time.Minute))

		require.Equal(t, http.StatusBadRequest, rec.Code)
		specconformance.AssertConformsToSpec(t, req, rec)
	})
}

func TestDeleteAccountPassword(t *testing.T) {
	t.Run("removed: other sessions end, the caller stays", func(t *testing.T) {
		f := newAccountFixture(t)
		otherDevice := f.session(t, 2*time.Hour)
		f.account.On("RemovePassword", mock.Anything, epochUserID, true).Return(nil).Once()

		req, rec := f.do(http.MethodDelete, "/api/v1/account/password", "", f.session(t, time.Minute))

		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
		require.NotNil(t, sessionCookieIn(rec))
		assert.True(t, f.stillSignedIn(t, sessionCookieIn(rec)))
		assert.False(t, f.stillSignedIn(t, otherDevice))
		f.container.activityService.AssertCalled(t, "RecordAuthActivity", mock.Anything, epochUserID,
			activities.ActivityTypeAuthCredentialRemoved, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		specconformance.AssertConformsToSpec(t, req, rec)
	})

	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"not a recent sign-in":        {services.ErrReauthenticationRequired, http.StatusForbidden, apierrors.CodeReauthenticationRequired},
		"the last usable method":      {services.ErrLastSignInMethod, http.StatusConflict, apierrors.CodeLastSignInMethod},
		"the account has no password": {services.ErrAccountPasswordNotSet, http.StatusNotFound, apierrors.CodeResourceNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			f := newAccountFixture(t)
			caller := f.session(t, time.Hour)
			f.account.On("RemovePassword", mock.Anything, epochUserID, false).Return(tc.err)

			req, rec := f.do(http.MethodDelete, "/api/v1/account/password", "", caller)

			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, tc.code, decodeProblem(t, rec).Code)
			assert.True(t, f.stillSignedIn(t, caller))
			specconformance.AssertConformsToSpec(t, req, rec)
		})
	}
}

func TestDeleteAccountIdentity(t *testing.T) {
	path := "/api/v1/account/identities/" + identityUUID

	t.Run("disconnected: other sessions end, the caller stays", func(t *testing.T) {
		f := newAccountFixture(t)
		otherDevice := f.session(t, 2*time.Hour)
		f.account.On("DisconnectIdentity", mock.Anything, epochUserID, identityUUID, true).Return("google", nil).Once()

		req, rec := f.do(http.MethodDelete, path, "", f.session(t, time.Minute))

		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
		require.NotNil(t, sessionCookieIn(rec))
		assert.True(t, f.stillSignedIn(t, sessionCookieIn(rec)))
		assert.False(t, f.stillSignedIn(t, otherDevice))
		f.container.activityService.AssertCalled(t, "RecordAuthActivity", mock.Anything, epochUserID,
			activities.ActivityTypeAuthIdentityDisconnected, mock.Anything,
			mock.MatchedBy(func(metadata map[string]interface{}) bool { return metadata["provider"] == "google" }),
			mock.Anything, mock.Anything)
		specconformance.AssertConformsToSpec(t, req, rec)
	})

	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"not a recent sign-in":   {services.ErrReauthenticationRequired, http.StatusForbidden, apierrors.CodeReauthenticationRequired},
		"the last usable method": {services.ErrLastSignInMethod, http.StatusConflict, apierrors.CodeLastSignInMethod},
		"no such identity":       {services.ErrAccountIdentityNotFound, http.StatusNotFound, apierrors.CodeResourceNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			f := newAccountFixture(t)
			f.account.On("DisconnectIdentity", mock.Anything, epochUserID, identityUUID, false).Return("", tc.err)

			req, rec := f.do(http.MethodDelete, path, "", f.session(t, time.Hour))

			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, tc.code, decodeProblem(t, rec).Code)
			assert.Nil(t, sessionCookieIn(rec))
			specconformance.AssertConformsToSpec(t, req, rec)
		})
	}

	t.Run("an id that is not a UUID is a 400", func(t *testing.T) {
		f := newAccountFixture(t)

		req, rec := f.do(http.MethodDelete, "/api/v1/account/identities/not-a-uuid", "", f.session(t, time.Minute))

		require.Equal(t, http.StatusBadRequest, rec.Code)
		specconformance.AssertConformsToSpec(t, req, rec)
	})
}

// Changing how an account signs in is open to a browser session only. An API
// key or a bearer token that could set a password would turn a leaked key into
// a permanent way in. Reading and renaming stay open to every credential.
func TestAccount_CredentialChangesNeedABrowserSession(t *testing.T) {
	for _, authType := range []string{"api_key", authTypeOAuth} {
		t.Run(authType, func(t *testing.T) {
			f := newAccountFixture(t)
			router := chi.NewRouter()
			router.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r.WithContext(authenticatedContext(r.Context(), epochUserID, authType, nil, nil)))
				})
			})
			f.srv.setupAccountRoutes(router)
			send := func(method, path, body string) (*http.Request, *httptest.ResponseRecorder) {
				var req *http.Request
				if body != "" {
					req = httptest.NewRequest(method, path, strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
				} else {
					req = httptest.NewRequest(method, path, nil)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return req, rec
			}

			for _, op := range []struct{ method, path, body string }{
				{http.MethodPut, "/api/v1/account/password", `{"new_password":"a brand new long passphrase"}`},
				{http.MethodDelete, "/api/v1/account/password", ""},
				{http.MethodDelete, "/api/v1/account/identities/" + identityUUID, ""},
			} {
				req, rec := send(op.method, op.path, op.body)

				require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", op.method, op.path, rec.Body.String())
				assert.Equal(t, apierrors.CodeCookieSessionRequired, decodeProblem(t, rec).Code)
				specconformance.AssertConformsToSpec(t, req, rec)
			}
			// account is a strict mock with no expectations for those three: the
			// service was never asked.

			f.account.On("Get", mock.Anything, epochUserID).Return(accountView(), nil)
			_, rec := send(http.MethodGet, "/api/v1/account", "")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), `"recent_sign_in":false`, "never recent without a browser session")

			f.account.On("UpdateName", mock.Anything, epochUserID, "Ada").Return(accountView(), nil)
			_, rec = send(http.MethodPatch, "/api/v1/account", `{"name":"Ada"}`)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

// If the other sessions cannot be ended after a change was saved, the response
// says so instead of reporting success: the person may be relying on it.
func TestAccount_FailureToEndOtherSessionsIsNotSuccess(t *testing.T) {
	epoch := newEpochFixture(t, 3)
	account := servicesmocks.NewMockAccountServiceInterface(t)
	epoch.srv.container = &accountContainer{MockAuthContainer: epoch.container, account: account}
	epoch.container.authService.On("RevokeUserSessions", mock.Anything, epochUserID).
		Return(int64(0), errors.New("pq: connection refused"))
	epoch.container.activityService.On("RecordAuthActivity",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()
	account.On("RemovePassword", mock.Anything, epochUserID, true).Return(nil)
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			epoch.srv.authenticateWithSession(w, r, next)
		})
	})
	epoch.srv.setupAccountRoutes(router)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/password", nil)
	req.AddCookie(epoch.cookieFor(t, slidingSession(3, time.Minute)))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "could not be signed out")
	assert.NotContains(t, rec.Body.String(), "connection refused")
	specconformance.AssertConformsToSpec(t, req, rec)
}
