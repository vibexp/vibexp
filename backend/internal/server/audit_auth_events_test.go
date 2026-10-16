package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/audit"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// Auth & security events in the audit log (#1514).

// auditingContainer adds an audit recorder to any test container.
type auditingContainer struct {
	container.Container
	rec audit.Recorder
}

func (c auditingContainer) AuditRecorder() audit.Recorder { return c.rec }

// withAudit installs a capturing recorder on srv and returns it.
func withAudit(srv *Server) *auditCapture {
	rec := &auditCapture{}
	srv.container = auditingContainer{Container: srv.container, rec: rec}
	return rec
}

// panickyRecorder panics: it must not change an auth response. A recorder
// that blocks is ruled out by the Recorder contract, which AsyncRecorder's own
// tests pin (TestRecord_NeverBlocksTheCaller).
type panickyRecorder struct{}

func (panickyRecorder) Record(context.Context, audit.Event) { panic("audit down") }

// auditSecrets are values a test sends as credentials; none may reach an
// audit event.
var auditSecrets = []string{pwLoginPassword, "wrong-password-SECRET", "SECRET-LINK-TOKEN", "vx_SECRETKEY"}

func assertNoSecrets(t *testing.T, got []capturedAudit) {
	t.Helper()
	for _, g := range got {
		doc, err := json.Marshal(g.event)
		require.NoError(t, err)
		for _, secret := range auditSecrets {
			assert.NotContains(t, string(doc), secret, "a credential reached the audit log")
		}
	}
}

// ---------------------------------------------------------------------------
// Password sign-in
// ---------------------------------------------------------------------------

func TestAuditPasswordLogin_Success(t *testing.T) {
	f := newPwLoginFixture(t)
	f.expectActivity(activities.ActivityTypeAuthLogin)
	rec := withAudit(f.srv)

	res := f.post(t, pwLoginEmail, pwLoginPassword)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLogin, got.event.Operation)
	require.NotNil(t, got.event.Actor)
	require.NotNil(t, got.event.Actor.UserID)
	assert.Equal(t, pwLoginUserID, *got.event.Actor.UserID)
	assert.Equal(t, pwLoginEmail, got.event.Actor.Email)
	assert.Equal(t, map[string]any{
		auditMetaMethod: auditAuthMethodLocalCredential, auditMetaProvider: services.LocalSignInProvider, auditMetaNewUser: false,
	}, got.event.Metadata)
	assertNoSecrets(t, rec.events())
}

func TestAuditPasswordLogin_UnknownAccount(t *testing.T) {
	f := newPwLoginFixture(t)
	rec := withAudit(f.srv)

	res := f.post(t, "nobody@example.com", "wrong-password-SECRET")
	require.Equal(t, http.StatusUnauthorized, res.Code, res.Body.String())

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLoginFailed, got.event.Operation)
	assert.Equal(t, models.AuditOutcomeFailure, got.event.Outcome)
	require.NotNil(t, got.event.Actor)
	assert.Nil(t, got.event.Actor.UserID, "an unknown account has no user id")
	assert.Equal(t, models.AuditActorAnonymous, got.event.Actor.Type)
	assert.Equal(t, "nobody@example.com", got.event.Actor.Email, "the attempted address is kept")
	assert.Equal(t, auditAuthMethodLocalCredential, got.event.Metadata[auditMetaMethod])
	assert.NotEmpty(t, got.event.Metadata[auditMetaReason])
	assertNoSecrets(t, rec.events())
}

func TestAuditPasswordLogin_KnownAccountWrongPassword(t *testing.T) {
	f := newPwLoginFixture(t)
	f.expectActivity(activities.ActivityTypeAuthFailure)
	rec := withAudit(f.srv)

	res := f.post(t, pwLoginEmail, "wrong-password-SECRET")
	require.Equal(t, http.StatusUnauthorized, res.Code, res.Body.String())

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLoginFailed, got.event.Operation)
	require.NotNil(t, got.event.Actor)
	require.NotNil(t, got.event.Actor.UserID, "a refusal that identified the account names it")
	assert.Equal(t, pwLoginUserID, *got.event.Actor.UserID)
	assertNoSecrets(t, rec.events())
}

func TestAuditPasswordLogin_PanickingRecorderNeverChangesTheResponse(t *testing.T) {
	f := newPwLoginFixture(t)
	f.expectActivity(activities.ActivityTypeAuthLogin)
	f.srv.container = auditingContainer{Container: f.srv.container, rec: panickyRecorder{}}

	res := f.post(t, pwLoginEmail, pwLoginPassword)

	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.NotNil(t, sessionCookieIn(res), "the session is still issued")
}

// ---------------------------------------------------------------------------
// One-time links (setup / reset completion) and forgot-password
// ---------------------------------------------------------------------------

func TestAuditAccountLink_InvalidTokenIsAFailedSignIn(t *testing.T) {
	f := newLinkFixture(t)
	rec := withAudit(f.srv)

	res := f.send(t, linkResetPath, map[string]any{"token": "SECRET-LINK-TOKEN", "password": "wrong-password-SECRET"})
	require.Equal(t, http.StatusUnauthorized, res.Code, res.Body.String())

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLoginFailed, got.event.Operation)
	assert.Equal(t, "account_link_invalid", got.event.Metadata[auditMetaReason])
	assert.Nil(t, got.event.Actor.UserID)
	assertNoSecrets(t, rec.events())
}

func TestAuditAccountLink_CompletionIsASignInViaTheLink(t *testing.T) {
	f := newLinkFixture(t)
	f.expectActivity(activities.ActivityTypeAuthLogin)
	token := f.copied(t, models.AccountTokenPurposeReset)
	rec := withAudit(f.srv)

	res := f.send(t, linkResetPath, map[string]any{"token": token, "password": linkNewPassword})
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLogin, got.event.Operation)
	assert.Equal(t, "reset", got.event.Metadata[auditMetaVia])
	assert.Equal(t, pwLoginUserID, *got.event.Actor.UserID)
	doc, err := json.Marshal(got.event)
	require.NoError(t, err)
	assert.NotContains(t, string(doc), token, "the link token never reaches the log")
	assert.NotContains(t, string(doc), linkNewPassword)
}

func TestAuditForgotPassword_RecordsTheRequest(t *testing.T) {
	f := newForgotFixture(t)
	rec := withAudit(f.srv)

	res := f.send(t, `{"email":"someone@example.com"}`)
	require.Equal(t, http.StatusAccepted, res.Code, res.Body.String())

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLocalCredentialResetRequested, got.event.Operation)
	require.NotNil(t, got.event.Actor)
	assert.Nil(t, got.event.Actor.UserID, "whether the address matched an account is never recorded")
	assert.Equal(t, "someone@example.com", got.event.Actor.Email)
}

// ---------------------------------------------------------------------------
// OIDC callback, dev login, logout
// ---------------------------------------------------------------------------

func newOIDCAuditServer(t *testing.T) (*Server, *auditCapture) {
	t.Helper()
	c := newMockAuthContainer(t)
	c.activityService.On("RecordAuthActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	srv := createTestAuthServer(c)
	return srv, withAudit(srv)
}

func TestAuditOIDC_Success(t *testing.T) {
	srv, rec := newOIDCAuditServer(t)
	user := &models.User{ID: "u-1", Email: "ada@example.com", Name: "Ada", Status: models.UserStatusActive}

	w := httptest.NewRecorder()
	srv.handleCallbackSuccess(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil),
		user, &idp.Tokens{AccessToken: "SECRET-ACCESS"}, "state-1", "github", true)
	require.Equal(t, http.StatusFound, w.Code)

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLogin, got.event.Operation)
	assert.Equal(t, "u-1", *got.event.Actor.UserID)
	assert.Equal(t, map[string]any{
		auditMetaMethod: auditAuthMethodOIDC, auditMetaProvider: "github", auditMetaNewUser: true,
	}, got.event.Metadata)
	doc, err := json.Marshal(got.event)
	require.NoError(t, err)
	assert.NotContains(t, string(doc), "SECRET-ACCESS", "provider tokens never reach the log")
}

func TestAuditOIDC_Failures(t *testing.T) {
	cases := map[string]struct {
		err    error
		reason string
	}{
		"access restricted": {services.ErrAccessRestricted, "access_restricted"},
		"unverified email":  {services.ErrEmailNotVerified, callbackErrorEmailNotVerified},
		"registration off":  {services.ErrRegistrationClosed, callbackErrorRegistrationClosed},
		"provider failure":  {assert.AnError, "idp_auth_failed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv, rec := newOIDCAuditServer(t)

			w := httptest.NewRecorder()
			srv.handleCallbackFailure(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil),
				"state-1", "google", tc.err)

			got := rec.only(t)
			assert.Equal(t, models.AuditOpAuthLoginFailed, got.event.Operation)
			assert.Equal(t, models.AuditOutcomeFailure, got.event.Outcome)
			assert.Equal(t, tc.reason, got.event.Metadata[auditMetaReason])
			assert.Equal(t, "google", got.event.Metadata[auditMetaProvider])
			assert.Nil(t, got.event.Actor.UserID)
		})
	}
}

func TestAuditOIDC_UnresolvableProvidersIsNotASignInAttempt(t *testing.T) {
	srv, rec := newOIDCAuditServer(t)

	srv.handleCallbackFailure(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil),
		"state-1", "google", services.ErrIdentityProvidersUnresolvable)

	assert.Empty(t, rec.events(), "no metric counts it, so neither does the audit log")
}

func TestAuditOIDC_SuspendedAccount(t *testing.T) {
	srv, rec := newOIDCAuditServer(t)
	user := &models.User{ID: "u-1", Email: "ada@example.com", Status: models.UserStatusSuspended}

	assert.True(t, srv.rejectSuspendedSignIn(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/", nil), user, "github"))

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLoginFailed, got.event.Operation)
	assert.Equal(t, "account_suspended", got.event.Metadata[auditMetaReason])
	require.NotNil(t, got.event.Actor.UserID)
	assert.Equal(t, "u-1", *got.event.Actor.UserID)
}

func TestAuditOIDC_PanickingRecorderNeverChangesTheResponse(t *testing.T) {
	srv, _ := newOIDCAuditServer(t)
	srv.container = auditingContainer{Container: srv.container.(auditingContainer).Container, rec: panickyRecorder{}}
	user := &models.User{ID: "u-1", Email: "ada@example.com", Status: models.UserStatusActive}

	w := httptest.NewRecorder()
	srv.handleCallbackSuccess(w, httptest.NewRequest(http.MethodGet, "/", nil),
		user, &idp.Tokens{AccessToken: "a"}, "state-1", "github", false)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))
	assert.NotNil(t, sessionCookieIn(w), "the session is still issued")
}

func TestAuditDevLogin(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, rec := newOIDCAuditServer(t)
		srv.handleDevLoginSuccess(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil),
			&models.User{ID: "u-1", Email: "dev@example.com"})

		got := rec.only(t)
		assert.Equal(t, models.AuditOpAuthLogin, got.event.Operation)
		assert.Equal(t, auditAuthMethodDev, got.event.Metadata[auditMetaMethod])
	})

	t.Run("access restricted", func(t *testing.T) {
		srv, rec := newOIDCAuditServer(t)
		srv.handleDevLoginFailure(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil),
			"dev@example.com", services.ErrAccessRestricted)

		got := rec.only(t)
		assert.Equal(t, models.AuditOpAuthLoginFailed, got.event.Operation)
		assert.Equal(t, "access_restricted", got.event.Metadata[auditMetaReason])
		assert.Equal(t, "dev@example.com", got.event.Actor.Email)
	})
}

func TestAuditLogout(t *testing.T) {
	srv, rec := newOIDCAuditServer(t)
	sess := &sesslib.Session{UserID: "u-1", AuthMethod: sesslib.AuthMethodPassword}
	sess.StartSliding(time.Now())
	cookie, err := srv.sessionManager.Cookie(sess)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	srv.recordLogout(req)

	got := rec.only(t)
	assert.Equal(t, models.AuditOpAuthLogout, got.event.Operation)
	require.NotNil(t, got.event.Actor)
	assert.Equal(t, map[string]any{auditMetaMethod: sesslib.AuthMethodPassword}, got.event.Metadata,
		"the sign-in method, never the HTTP verb or the endpoint the activity recorder adds")
}

// A real activity recorder adds the request's method and endpoint to the map
// it is given; the audit event must not share that map.
func TestAuditLogout_DoesNotShareTheActivityMap(t *testing.T) {
	c := newMockAuthContainer(t)
	c.activityService.On("RecordAuthActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			m := args.Get(4).(map[string]interface{})
			m["method"], m["endpoint"] = http.MethodPost, "/api/v1/auth/logout"
		}).Return(nil)
	srv := createTestAuthServer(c)
	rec := withAudit(srv)
	sess := &sesslib.Session{UserID: "u-1", AuthMethod: sesslib.AuthMethodDev, Provider: "github"}
	sess.StartSliding(time.Now())
	cookie, err := srv.sessionManager.Cookie(sess)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	srv.recordLogout(req)

	assert.Equal(t, map[string]any{auditMetaMethod: sesslib.AuthMethodDev, auditMetaProvider: "github"},
		rec.only(t).event.Metadata)
}

func TestAuditLogout_UnusableCookieRecordsNothing(t *testing.T) {
	srv, rec := newOIDCAuditServer(t)
	srv.recordLogout(httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	assert.Empty(t, rec.events())
}

// ---------------------------------------------------------------------------
// Setup session
// ---------------------------------------------------------------------------

func TestAuditSetupSession(t *testing.T) {
	t.Run("started", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		rec := withAudit(h.srv)

		_, res := h.exchange(h.bootToken(t))
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())

		got := rec.only(t)
		assert.Equal(t, models.AuditOpAuthSetupSessionStarted, got.event.Operation)
		assert.Equal(t, models.AuditActorAnonymous, got.event.Actor.Type)
	})

	t.Run("a rejected token is a failed sign-in", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		h.bootToken(t)
		rec := withAudit(h.srv)

		_, res := h.exchange("SECRET-LINK-TOKEN")
		require.Equal(t, http.StatusUnauthorized, res.Code, res.Body.String())

		got := rec.only(t)
		assert.Equal(t, models.AuditOpAuthLoginFailed, got.event.Operation)
		assert.Equal(t, "setup_token_invalid", got.event.Metadata[auditMetaReason])
		assertNoSecrets(t, rec.events())
	})
}

// ---------------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------------

type apiKeyAuditContainer struct {
	BaseMockContainer
	keys *servicesmocks.MockAPIKeyServiceInterface
}

func (c *apiKeyAuditContainer) APIKeyService() services.APIKeyServiceInterface { return c.keys }
func (c *apiKeyAuditContainer) ActivityService() activities.ActivityService {
	return &mockActivityService{}
}

func newAPIKeyAuditServer(t *testing.T) (*Server, *servicesmocks.MockAPIKeyServiceInterface, *auditCapture) {
	t.Helper()
	keys := servicesmocks.NewMockAPIKeyServiceInterface(t)
	srv := newServerWithNullLogger(t)
	srv.container = &apiKeyAuditContainer{keys: keys}
	return srv, keys, withAudit(srv)
}

// apiKeyCreateBody creates a CLI key named "laptop".
const apiKeyCreateBody = `{"name":"laptop","integration_codes":["` + models.IntegrationCodeCLI + `"]}`

func apiKeyRequest(method, body string) *http.Request {
	req := httptest.NewRequest(method, "/api/v1/api-keys", strings.NewReader(body))
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "u-1"))
}

func TestAuditAPIKeys(t *testing.T) {
	t.Run("created, never with the secret", func(t *testing.T) {
		srv, keys, rec := newAPIKeyAuditServer(t)
		keys.EXPECT().GenerateAPIKey(mock.Anything, "u-1", "laptop", mock.Anything).
			Return(&models.APIKey{ID: "key-1", Name: "laptop", KeyPrefix: "vx_"}, "vx_SECRETKEY", nil)

		w := httptest.NewRecorder()
		srv.handleCreateAPIKey(w, apiKeyRequest(http.MethodPost, apiKeyCreateBody))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		got := rec.only(t)
		assert.Equal(t, models.AuditOpAuthAPIKeyCreated, got.event.Operation)
		assert.Equal(t, models.AuditResourceTypeAPIKey, got.event.ResourceType)
		assert.Equal(t, "key-1", got.event.ResourceID)
		assert.Equal(t, "laptop", got.event.ResourceName)
		assertNoSecrets(t, rec.events())
	})

	t.Run("revoked, named as it was before the delete", func(t *testing.T) {
		srv, keys, rec := newAPIKeyAuditServer(t)
		keys.EXPECT().GetAPIKeysByUserID(mock.Anything, "u-1").
			Return([]models.APIKey{{ID: "other"}, {ID: "key-1", Name: "laptop"}}, nil)
		keys.EXPECT().DeleteAPIKey(mock.Anything, "u-1", "key-1").Return(nil)

		req := apiKeyRequest(http.MethodDelete, "")
		req = withChiURLParams(req, map[string]string{"id": "key-1"})
		w := httptest.NewRecorder()
		srv.handleDeleteAPIKey(w, req)
		require.Less(t, w.Code, 300, w.Body.String())

		got := rec.only(t)
		assert.Equal(t, models.AuditOpAuthAPIKeyRevoked, got.event.Operation)
		assert.Equal(t, "key-1", got.event.ResourceID)
		assert.Equal(t, "laptop", got.event.ResourceName)
	})

	t.Run("a failed delete records nothing", func(t *testing.T) {
		srv, keys, rec := newAPIKeyAuditServer(t)
		keys.EXPECT().GetAPIKeysByUserID(mock.Anything, "u-1").Return(nil, assert.AnError)
		keys.EXPECT().DeleteAPIKey(mock.Anything, "u-1", "key-1").Return(assert.AnError)

		req := withChiURLParams(apiKeyRequest(http.MethodDelete, ""), map[string]string{"id": "key-1"})
		srv.handleDeleteAPIKey(httptest.NewRecorder(), req)

		assert.Empty(t, rec.events())
	})
}

func TestAuditAPIKeys_PanickingRecorderNeverChangesTheResponse(t *testing.T) {
	srv, keys, _ := newAPIKeyAuditServer(t)
	srv.container = auditingContainer{Container: srv.container.(auditingContainer).Container, rec: panickyRecorder{}}
	keys.EXPECT().GenerateAPIKey(mock.Anything, "u-1", "laptop", mock.Anything).
		Return(&models.APIKey{ID: "key-1", Name: "laptop"}, "vx_SECRETKEY", nil)

	w := httptest.NewRecorder()
	srv.handleCreateAPIKey(w, apiKeyRequest(http.MethodPost, apiKeyCreateBody))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "vx_SECRETKEY", "the key is still returned to its owner")
}

// ---------------------------------------------------------------------------
// Account password and identity (self-service)
// ---------------------------------------------------------------------------

func TestAuditAccountCredentials(t *testing.T) {
	t.Run("first password is a set", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("SetPassword", mock.Anything, epochUserID, mock.Anything).Return(true, nil)
		rec := withAudit(f.srv)

		_, res := f.do(http.MethodPut, "/api/v1/account/password",
			`{"new_password":"a brand new long passphrase"}`, f.session(t, time.Minute))
		require.Equal(t, http.StatusNoContent, res.Code, res.Body.String())

		events := rec.events()
		ops := make([]models.AuditOperation, 0, len(events))
		for _, e := range events {
			ops = append(ops, e.event.Operation)
		}
		assert.Contains(t, ops, models.AuditOpAuthLocalCredentialSet)
		for _, e := range events {
			if e.event.Operation == models.AuditOpAuthLocalCredentialSet {
				assert.Equal(t, epochUserID, e.event.ResourceID)
				assert.Equal(t, auditBySelf, e.event.Metadata[auditMetaBy])
			}
		}
		assertNoSecrets(t, events)
		doc, err := json.Marshal(events)
		require.NoError(t, err)
		assert.NotContains(t, string(doc), "a brand new long passphrase")
	})

	t.Run("a later one is a change", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("SetPassword", mock.Anything, epochUserID, mock.Anything).Return(false, nil)
		rec := withAudit(f.srv)

		f.do(http.MethodPut, "/api/v1/account/password",
			`{"current_password":"old","new_password":"a brand new long passphrase"}`, f.session(t, time.Minute))

		assert.True(t, hasAuditOp(rec, models.AuditOpAuthLocalCredentialChanged))
	})

	t.Run("removed", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("RemovePassword", mock.Anything, epochUserID, true).Return(nil)
		rec := withAudit(f.srv)

		_, res := f.do(http.MethodDelete, "/api/v1/account/password", "", f.session(t, time.Minute))
		require.Equal(t, http.StatusNoContent, res.Code, res.Body.String())

		assert.True(t, hasAuditOp(rec, models.AuditOpAuthLocalCredentialRemoved))
	})

	t.Run("identity unlinked", func(t *testing.T) {
		f := newAccountFixture(t)
		f.account.On("DisconnectIdentity", mock.Anything, epochUserID, identityUUID, true).Return("google", nil)
		rec := withAudit(f.srv)

		_, res := f.do(http.MethodDelete, "/api/v1/account/identities/"+identityUUID, "", f.session(t, time.Minute))
		require.Equal(t, http.StatusNoContent, res.Code, res.Body.String())

		for _, e := range rec.events() {
			if e.event.Operation == models.AuditOpAuthIdentityUnlinked {
				assert.Equal(t, "google", e.event.Metadata[auditMetaProvider])
				return
			}
		}
		t.Fatal("no auth.identity.unlinked event")
	})
}

func hasAuditOp(rec *auditCapture, op models.AuditOperation) bool {
	for _, e := range rec.events() {
		if e.event.Operation == op {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Admin: links and password removal
// ---------------------------------------------------------------------------

type adminLinkAuditContainer struct {
	BaseMockContainer
	accounts *servicesmocks.MockAdminAccountServiceInterface
}

func (c *adminLinkAuditContainer) AdminAccountService() services.AdminAccountServiceInterface {
	return c.accounts
}

func TestAuditAdminAccountLinks(t *testing.T) {
	userID := uuid.New()
	ctx := context.WithValue(context.Background(), contextKeyUserID, "admin-1")
	newAdmin := func(t *testing.T) (*adminStrictServer, *servicesmocks.MockAdminAccountServiceInterface, *auditCapture) {
		accounts := servicesmocks.NewMockAdminAccountServiceInterface(t)
		srv := newServerWithNullLogger(t)
		srv.container = &adminLinkAuditContainer{accounts: accounts}
		return &adminStrictServer{s: srv}, accounts, withAudit(srv)
	}

	t.Run("reset link", func(t *testing.T) {
		admin, accounts, rec := newAdmin(t)
		accounts.EXPECT().IssueResetLink(mock.Anything, "admin-1", userID.String(), mock.Anything).
			Return(&services.AdminResetLink{AdminSetupLink: services.AdminSetupLink{
				Email: "ada@example.com", Delivery: services.AdminLinkByCopy, LinkURL: "https://x/reset?token=SECRET-LINK-TOKEN",
			}, Invalidated: true}, nil)

		_, err := admin.IssueAdminUserResetLink(ctx, admingen.IssueAdminUserResetLinkRequestObject{
			Id: userID, Body: &admingen.IssueAdminUserResetLinkJSONRequestBody{Delivery: "link"},
		})
		require.NoError(t, err)

		got := rec.only(t)
		assert.Equal(t, models.AuditOpAuthLocalCredentialResetLinkIssued, got.event.Operation)
		assert.Equal(t, userID.String(), got.event.ResourceID)
		assert.Equal(t, auditLinkPurposeReset, got.event.Metadata["purpose"])
		assert.Equal(t, true, got.event.Metadata["invalidated"])
		assert.Equal(t, true, got.event.Metadata["issued"])
		assert.Equal(t, models.AuditOutcomeSuccess, got.event.Outcome)
		assert.Equal(t, auditByAdmin, got.event.Metadata[auditMetaBy])
		assertNoSecrets(t, rec.events())
	})

	t.Run("a reset that stopped the password but issued no link is a failure", func(t *testing.T) {
		admin, accounts, rec := newAdmin(t)
		accounts.EXPECT().IssueResetLink(mock.Anything, "admin-1", userID.String(), mock.Anything).
			Return(&services.AdminResetLink{AdminSetupLink: services.AdminSetupLink{
				Email: "ada@example.com", Delivery: services.AdminLinkByEmail,
			}, Invalidated: true}, assert.AnError)

		_, err := admin.IssueAdminUserResetLink(ctx, admingen.IssueAdminUserResetLinkRequestObject{
			Id: userID, Body: &admingen.IssueAdminUserResetLinkJSONRequestBody{Delivery: "email"},
		})
		require.Error(t, err)

		got := rec.only(t)
		assert.Equal(t, models.AuditOutcomeFailure, got.event.Outcome)
		assert.Equal(t, false, got.event.Metadata["issued"])
		assert.Equal(t, true, got.event.Metadata["invalidated"])
	})

	t.Run("setup link", func(t *testing.T) {
		admin, accounts, rec := newAdmin(t)
		accounts.EXPECT().IssueSetupLink(mock.Anything, "admin-1", userID.String(), services.AdminLinkByEmail).
			Return(&services.AdminSetupLink{Email: "ada@example.com", Delivery: services.AdminLinkByEmail}, nil)

		_, err := admin.IssueAdminUserSetupLink(ctx, admingen.IssueAdminUserSetupLinkRequestObject{
			Id: userID, Body: &admingen.IssueAdminUserSetupLinkJSONRequestBody{Delivery: "email"},
		})
		require.NoError(t, err)

		got := rec.only(t)
		assert.Equal(t, auditLinkPurposeSetup, got.event.Metadata["purpose"])
		assert.Equal(t, "email", got.event.Metadata["delivery"])
	})

	t.Run("password removed", func(t *testing.T) {
		admin, accounts, rec := newAdmin(t)
		accounts.EXPECT().RemovePassword(mock.Anything, "admin-1", userID.String(), false).
			Return("ada@example.com", nil)

		_, err := admin.DeleteAdminUserPassword(ctx, admingen.DeleteAdminUserPasswordRequestObject{Id: userID})
		require.NoError(t, err)

		got := rec.only(t)
		assert.Equal(t, models.AuditOpAuthLocalCredentialRemoved, got.event.Operation)
		assert.Equal(t, auditByAdmin, got.event.Metadata[auditMetaBy])
		assert.Equal(t, "ada@example.com", got.event.ResourceName)
	})

	t.Run("a refused action records nothing", func(t *testing.T) {
		admin, accounts, rec := newAdmin(t)
		accounts.EXPECT().RemovePassword(mock.Anything, "admin-1", userID.String(), false).
			Return("", services.ErrAdminOwnAccount)

		_, err := admin.DeleteAdminUserPassword(ctx, admingen.DeleteAdminUserPasswordRequestObject{Id: userID})
		require.Error(t, err)
		assert.Empty(t, rec.events())
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestAuditLoginFailed_ActorShapes(t *testing.T) {
	srv := newServerWithNullLogger(t)
	srv.container = &BaseMockContainer{}
	rec := withAudit(srv)

	srv.auditLoginFailed(context.Background(), auditLoginFailure{method: "password", reason: "x", email: "a@b.c"})
	srv.auditLoginFailed(context.Background(), auditLoginFailure{method: "password", reason: "x", userID: "u-1"})

	got := rec.events()
	require.Len(t, got, 2)
	assert.Equal(t, models.AuditActorAnonymous, got[0].event.Actor.Type)
	assert.Nil(t, got[0].event.Actor.UserID)
	assert.Equal(t, "u-1", *got[1].event.Actor.UserID)
	assert.Empty(t, got[1].event.Actor.Type, "a known user's type is left to the recorder (user)")
}

func TestAuditActorHelpers(t *testing.T) {
	assert.Nil(t, auditUserActor(nil))
}

func TestAPIKeyAuditName_SkippedWithoutRecorder(t *testing.T) {
	srv := newServerWithNullLogger(t)
	srv.container = &BaseMockContainer{}
	assert.Empty(t, srv.apiKeyAuditName(context.Background(), "u-1", "key-1"))
}

// withChiURLParams sets chi URL parameters on a request served without a
// router.
func withChiURLParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}
