package server

import (
	"bytes"
	"context"
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

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
	"github.com/vibexp/vibexp/internal/testutils/memauthsettings"
)

const (
	authSettingsPath      = "/api/v1/admin/settings/auth"
	authProvidersPath     = authSettingsPath + "/providers"
	authAllowlistPath     = authSettingsPath + "/allowlist"
	authAdminsPath        = authSettingsPath + "/admins"
	authAuditPath         = authSettingsPath + "/audit"
	authTestBaseURL       = "https://vibexp.test"
	authTestCallbackURL   = authTestBaseURL + "/api/v1/auth/callback"
	authTestIssuer        = "https://issuer.example.com"
	authTestRootID        = "11111111-1111-4111-8111-111111111111"
	authTestRootEmail     = "root@instance.test"
	authTestDBAdminID     = "22222222-2222-4222-8222-222222222222"
	authTestMemberID      = "33333333-3333-4333-8333-333333333333"
	authTestMemberEmail   = "member@instance.test"
	authTestSuspendedID   = "44444444-4444-4444-8444-444444444444"
	authTestUnknownUserID = "55555555-5555-4555-8555-555555555555"
)

// authTestSecretSentinel is the plaintext client secret the fixtures store. It
// must never appear in any response body. Built at run time so the fixture does
// not read as a committed credential.
var authTestSecretSentinel = strings.Repeat("z", 6) + "-auth-secret-sentinel"

// authProviderKeys is the complete key set of a provider in a response.
// Anything else appearing is a leak.
var authProviderKeys = []string{
	"id", "type", "slug", "display_name", "enabled", "sort_order", "client_id", "has_client_secret",
	"issuer_url", "redirect_uri", "health", "health.status", "health.last_error", "health.checked_at",
	"created_at", "updated_at", "updated_by_user_id",
}

func prefixedKeys(prefix string, keys []string, extra ...string) []string {
	out := append([]string{}, extra...)
	for _, k := range keys {
		out = append(out, prefix+k)
	}
	return out
}

// stubIDPResolver is an IdentityProviderResolver whose test outcome a test sets
// directly; every enabled provider reads as healthy.
type stubIDPResolver struct {
	store   *memauthsettings.Store
	testErr error
	tested  int
}

func (r *stubIDPResolver) Snapshot(context.Context) (*idp.Registry, error) { return nil, nil }

func (r *stubIDPResolver) Provider(context.Context, string) (idp.IdentityProvider, bool, error) {
	return nil, false, nil
}

func (r *stubIDPResolver) Health() map[string]services.ProviderHealth {
	out := map[string]services.ProviderHealth{}
	rows, err := r.store.Providers().List(context.Background())
	if err != nil {
		return out
	}
	for _, row := range rows {
		if row.Enabled {
			out[row.Slug] = services.ProviderHealth{Healthy: true, CheckedAt: row.UpdatedAt}
		}
	}
	return out
}

func (r *stubIDPResolver) TestProvider(context.Context, models.InstanceAuthProvider, *string) error {
	r.tested++
	return r.testErr
}

// authTestUsers is the user store behind the admin resolver, the grant lookups
// and the audit actor names.
type authTestUsers struct {
	repositories.UserRepository
	users []*models.User
}

func newAuthTestUsers() *authTestUsers {
	return &authTestUsers{users: []*models.User{
		{ID: authTestRootID, Email: authTestRootEmail, Name: "Root Admin", Status: models.UserStatusActive},
		{ID: authTestDBAdminID, Email: "delegate@instance.test", Name: "Delegate", Status: models.UserStatusActive},
		{ID: authTestMemberID, Email: authTestMemberEmail, Status: models.UserStatusActive},
		{ID: authTestSuspendedID, Email: "gone@instance.test", Status: models.UserStatusSuspended},
	}}
}

func (u *authTestUsers) GetByID(_ context.Context, id string) (*models.User, error) {
	for _, user := range u.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, repositories.ErrUserNotFound
}

func (u *authTestUsers) GetByEmail(_ context.Context, email string) (*models.User, error) {
	for _, user := range u.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, repositories.ErrUserNotFound
}

func (u *authTestUsers) GetNamesByIDs(_ context.Context, ids []string) (map[string]string, error) {
	names := map[string]string{}
	for _, id := range ids {
		if user, err := u.GetByID(context.Background(), id); err == nil && user.Name != "" {
			names[id] = user.Name
		}
	}
	return names, nil
}

// authSettingsFixture serves the REAL InstanceAuthSettingsService, admin
// resolver, allowlist resolver and setup-mode service over in-memory storage,
// through the FULL router: adminRouteGuard, the body guard and the generated
// handlers are all exercised as deployed.
type authSettingsFixture struct {
	srv      *Server
	store    *memauthsettings.Store
	users    *authTestUsers
	resolver *stubIDPResolver
	setup    services.SetupModeService
	enc      *services.EncryptionService
	// setupCookie is the setup session's cookie, once primeSetupSession minted it.
	setupCookie *http.Cookie
}

// authSettingsContainer adds the setup-mode service to the admin mock
// container.
type authSettingsContainer struct {
	*adminMockContainer
	setup services.SetupModeService
}

func (c *authSettingsContainer) SetupModeService() services.SetupModeService { return c.setup }

func newAuthSettingsFixture(t *testing.T) *authSettingsFixture {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	enc, err := services.NewEncryptionService(strings.Repeat("k", 32))
	require.NoError(t, err)

	f := &authSettingsFixture{store: memauthsettings.New(), users: newAuthTestUsers(), enc: enc}
	f.resolver = &stubIDPResolver{store: f.store}
	admins := services.NewInstanceAdminService([]string{authTestRootEmail}, f.store.Grants(), f.users, logger)
	f.setup = services.NewSetupModeService(services.SetupModeDeps{
		Setup:       &memSetupState{},
		Providers:   f.store.Providers(),
		IsRootAdmin: admins.IsRootAdmin,
		BaseURL:     authTestBaseURL,
		Logger:      logger,
	})

	authSvc := servicesmocks.NewMockAuthServiceInterface(t)
	keySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
	for _, user := range f.users.users {
		keySvc.On("ValidateAPIKey", mock.Anything, "vxk_"+user.ID).
			Return(&models.APIKey{ID: "key-" + user.ID, UserID: user.ID}, nil).Maybe()
		authSvc.On("GetUserByID", mock.Anything, user.ID).Return(user, nil).Maybe()
	}

	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: authTestBaseURL},
		Auth: config.AuthConfig{
			SessionEncryptionKey: testCookiePassword,
			InstanceAdmins:       config.EnvStringSlice{authTestRootEmail},
		},
	}
	admin := &adminMockContainer{
		authService:           authSvc,
		apiKeyService:         keySvc,
		userRepo:              f.users,
		instanceAuditRepo:     f.store.AuditLog(),
		instanceAdminResolver: admins,
		instanceAuthSettingsService: services.NewInstanceAuthSettingsService(services.InstanceAuthSettingsDeps{
			Providers:  f.store.Providers(),
			Allowlists: f.store.Allowlist(),
			Versions:   f.store.Versions(),
			Grants:     f.store.Grants(),
			Users:      f.users,
			Resolver:   f.resolver,
			AllowlistResolver: services.NewAccessAllowlistResolver(services.AccessAllowlistResolverDeps{
				Allowlists: f.store.Allowlist(), Versions: f.store.Versions(), RootAdmins: admins, Logger: logger,
			}),
			Admins:      admins,
			Enc:         enc,
			CallbackURL: authTestCallbackURL,
			Logger:      logger,
		}),
	}
	f.srv = newAdminTestServer(cfg, admin)
	f.srv.container = &authSettingsContainer{adminMockContainer: admin, setup: f.setup}
	return f
}

// authCaller decorates a request with one caller's credentials.
type authCaller func(t *testing.T, f *authSettingsFixture, r *http.Request)

func asAnonymous(*testing.T, *authSettingsFixture, *http.Request) {}

func asAPIKey(userID string) authCaller {
	return func(_ *testing.T, _ *authSettingsFixture, r *http.Request) {
		r.Header.Set("Authorization", "Bearer vxk_"+userID)
	}
}

var (
	asRoot    = asAPIKey(authTestRootID)
	asDBAdmin = asAPIKey(authTestDBAdminID)
	asMember  = asAPIKey(authTestMemberID)
)

// asSetupSession attaches the setup session's cookie.
func asSetupSession(t *testing.T, f *authSettingsFixture, r *http.Request) {
	t.Helper()
	f.primeSetupSession(t)
	r.AddCookie(f.setupCookie)
}

// primeSetupSession mints the setup token as boot does and exchanges it for the
// setup cookie, once. A token is only minted while no provider is enabled, so a
// test that seeds providers calls this first; the session it starts stays valid
// after a provider is enabled.
func (f *authSettingsFixture) primeSetupSession(t *testing.T) {
	t.Helper()
	if f.setupCookie != nil {
		return
	}
	setupURL, _, err := f.setup.EnsureTokenAtBoot(context.Background())
	require.NoError(t, err)
	token := strings.TrimPrefix(setupURL, authTestBaseURL+"/setup?token=")
	require.NotEqual(t, setupURL, token, "the setup URL carries the token")

	raw, err := json.Marshal(map[string]string{"token": token})
	require.NoError(t, err)
	exchange := httptest.NewRequest(http.MethodPost, "/api/v1/setup/session", bytes.NewReader(raw))
	exchange.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	f.srv.router.ServeHTTP(rr, exchange)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	for _, c := range rr.Result().Cookies() {
		if c.Name == sesslib.SetupCookieName {
			f.setupCookie = c
			return
		}
	}
	t.Fatal("no setup cookie")
}

// asSessionOf signs userID in with a cookie session issued by provider.
func asSessionOf(userID, provider string) authCaller {
	return func(t *testing.T, f *authSettingsFixture, r *http.Request) {
		t.Helper()
		rr := httptest.NewRecorder()
		require.NoError(t, f.srv.sessionManager.Write(rr, &sesslib.Session{
			UserID: userID, Provider: provider, ExpiresAt: time.Now().Add(time.Hour),
		}))
		for _, c := range rr.Result().Cookies() {
			r.AddCookie(c)
		}
	}
}

// serve sends one request as caller through the full router, asserts the
// response conforms to the spec and never carries the stored client secret,
// and returns it.
func (f *authSettingsFixture) serve(
	t *testing.T, caller authCaller, method, path string, body any,
) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
	}
	caller(t, f, req)
	rr := httptest.NewRecorder()
	f.srv.router.ServeHTTP(rr, req)
	specconformance.AssertConformsToSpec(t, req, rr)
	assertBodyExcludes(t, rr.Body.Bytes(), authTestSecretSentinel)
	return rr
}

func oidcProviderBody(slug string) map[string]any {
	return map[string]any{
		"type": "oidc", "slug": slug, "display_name": "Provider " + slug, "client_id": "client-" + slug,
		"client_secret": authTestSecretSentinel, "issuer_url": authTestIssuer,
	}
}

// createProvider stores a provider as the root admin and returns its id.
func (f *authSettingsFixture) createProvider(t *testing.T, body map[string]any) string {
	t.Helper()
	rr := f.serve(t, asRoot, http.MethodPost, authProvidersPath, body)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var saved admingen.AdminAuthProviderSaved
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &saved))
	return saved.Provider.Id.String()
}

// updateBody is a whole-row PUT that changes nothing about the stored provider.
func (f *authSettingsFixture) updateBody(t *testing.T, slug string) map[string]any {
	t.Helper()
	row := f.store.Provider(slug)
	require.NotNil(t, row)
	body := map[string]any{
		"display_name": row.DisplayName, "enabled": row.Enabled, "sort_order": row.SortOrder,
		"client_id": row.ClientID, "expected_version": f.store.Version(),
	}
	if row.IssuerURL != nil {
		body["issuer_url"] = *row.IssuerURL
	}
	return body
}

func (f *authSettingsFixture) storedSecret(t *testing.T, slug string) string {
	t.Helper()
	row := f.store.Provider(slug)
	require.NotNil(t, row)
	require.NotNil(t, row.ClientSecretEncrypted)
	plain, err := f.enc.Decrypt(*row.ClientSecretEncrypted)
	require.NoError(t, err)
	return plain
}

func problemCodeAndReason(t *testing.T, rr *httptest.ResponseRecorder) (code string, reason any) {
	t.Helper()
	var p struct {
		Code     string         `json:"code"`
		Metadata map[string]any `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &p), rr.Body.String())
	return p.Code, p.Metadata["reason"]
}

func validationFields(t *testing.T, rr *httptest.ResponseRecorder) []string {
	t.Helper()
	var p struct {
		Code   string `json:"code"`
		Errors []struct {
			Field string `json:"field"`
		} `json:"validation_errors"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &p), rr.Body.String())
	require.Equal(t, "INSTANCE_SETTINGS_VALIDATION_FAILED", p.Code, rr.Body.String())
	fields := make([]string, 0, len(p.Errors))
	for _, e := range p.Errors {
		fields = append(fields, e.Field)
	}
	return fields
}

// --- providers ----------------------------------------------------------------

func TestAdminAuthProviders_EmptyListIsAnEmptyArray(t *testing.T) {
	f := newAuthSettingsFixture(t)

	rr := f.serve(t, asRoot, http.MethodGet, authProvidersPath, nil)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"providers":[],"version":1}`, rr.Body.String())
}

func TestAdminAuthProviders_CreateAndList(t *testing.T) {
	f := newAuthSettingsFixture(t)

	rr := f.serve(t, asRoot, http.MethodPost, authProvidersPath, oidcProviderBody("okta"))

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), prefixedKeys("provider.", authProviderKeys, "provider", "version")...)
	var saved admingen.AdminAuthProviderSaved
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &saved))
	assert.Equal(t, int64(2), saved.Version)
	assert.Equal(t, f.store.Version(), saved.Version)
	p := saved.Provider
	assert.Equal(t, admingen.AdminAuthProviderTypeOidc, p.Type)
	assert.True(t, p.Enabled, "enabled defaults to true")
	assert.Zero(t, p.SortOrder)
	assert.True(t, p.HasClientSecret)
	assert.Equal(t, authTestCallbackURL, p.RedirectUri)
	assert.Equal(t, admingen.AdminAuthProviderHealthStatusHealthy, p.Health.Status)
	require.NotNil(t, p.UpdatedByUserId)
	assert.Equal(t, authTestRootID, p.UpdatedByUserId.String())
	assert.Equal(t, authTestSecretSentinel, f.storedSecret(t, "okta"))
	assertBodyExcludes(t, rr.Body.Bytes(), *f.store.Provider("okta").ClientSecretEncrypted)

	disabled := oidcProviderBody("zeta")
	disabled["enabled"] = false
	disabled["sort_order"] = 5
	f.createProvider(t, disabled)

	rr = f.serve(t, asRoot, http.MethodGet, authProvidersPath, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), prefixedKeys("providers[].", authProviderKeys, "providers", "version")...)
	var list admingen.AdminAuthProviderList
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
	require.Len(t, list.Providers, 2)
	assert.Equal(t, int64(3), list.Version)
	assert.Equal(t, "okta", list.Providers[0].Slug, "in sign-in page order")
	assert.Equal(t, admingen.AdminAuthProviderHealthStatusDisabled, list.Providers[1].Health.Status)
	assert.Nil(t, list.Providers[1].Health.CheckedAt)
	for _, row := range []string{"okta", "zeta"} {
		assertBodyExcludes(t, rr.Body.Bytes(), *f.store.Provider(row).ClientSecretEncrypted)
	}
}

func TestAdminAuthProviders_CreateRejections(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(map[string]any)
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{name: "unknown field", mutate: func(b map[string]any) { b["redirect_uri"] = "https://evil.example" },
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "empty secret", mutate: func(b map[string]any) { b["client_secret"] = "" },
			wantStatus: http.StatusBadRequest, wantField: "client_secret"},
		{name: "missing secret", mutate: func(b map[string]any) { delete(b, "client_secret") },
			wantStatus: http.StatusBadRequest, wantField: "client_secret"},
		{name: "invalid slug", mutate: func(b map[string]any) { b["slug"] = "Not A Slug" },
			wantStatus: http.StatusBadRequest, wantField: "slug"},
		{name: "unknown type", mutate: func(b map[string]any) { b["type"] = "saml" },
			wantStatus: http.StatusBadRequest, wantField: "type"},
		{name: "oidc without an issuer", mutate: func(b map[string]any) { delete(b, "issuer_url") },
			wantStatus: http.StatusBadRequest, wantField: "issuer_url"},
		{name: "plain http issuer", mutate: func(b map[string]any) { b["issuer_url"] = "http://issuer.example.com" },
			wantStatus: http.StatusBadRequest, wantField: "issuer_url"},
		{name: "stale expected version", mutate: func(b map[string]any) { b["expected_version"] = 99 },
			wantStatus: http.StatusConflict, wantCode: "INSTANCE_SETTINGS_VERSION_CONFLICT"},
		{name: "taken slug", mutate: func(b map[string]any) { b["slug"] = "taken" },
			wantStatus: http.StatusConflict, wantCode: "INSTANCE_AUTH_PROVIDER_CONFLICT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			f.createProvider(t, oidcProviderBody("taken"))
			version := f.store.Version()
			body := oidcProviderBody("okta")
			tc.mutate(body)

			rr := f.serve(t, asRoot, http.MethodPost, authProvidersPath, body)

			require.Equal(t, tc.wantStatus, rr.Code, rr.Body.String())
			if tc.wantField != "" {
				assert.Equal(t, []string{tc.wantField}, validationFields(t, rr))
			} else {
				code, _ := problemCodeAndReason(t, rr)
				assert.Equal(t, tc.wantCode, code)
			}
			assert.Equal(t, version, f.store.Version(), "nothing was written")
		})
	}
}

// An operator-entered issuer URL is trusted (epic #1230): there is no SSRF
// filter on it, so an issuer on a private address is stored like any other
// https URL. Only instance admins and the setup session can reach this.
func TestAdminAuthProviders_PrivateIssuerIsTrusted(t *testing.T) {
	f := newAuthSettingsFixture(t)
	body := oidcProviderBody("internal")
	body["issuer_url"] = "https://10.0.0.5/realms/main"

	rr := f.serve(t, asRoot, http.MethodPost, authProvidersPath, body)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	assert.Equal(t, "https://10.0.0.5/realms/main", *f.store.Provider("internal").IssuerURL)
}

func TestAdminAuthProviders_UpdateKeepsOrReplacesTheSecret(t *testing.T) {
	f := newAuthSettingsFixture(t)
	id := f.createProvider(t, oidcProviderBody("okta"))
	path := authProvidersPath + "/" + id

	// Omitted secret: the stored one is kept.
	body := f.updateBody(t, "okta")
	body["display_name"] = "Renamed"
	rr := f.serve(t, asRoot, http.MethodPut, path, body)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), prefixedKeys("provider.", authProviderKeys, "provider", "version")...)
	assert.Equal(t, "Renamed", f.store.Provider("okta").DisplayName)
	assert.Equal(t, authTestSecretSentinel, f.storedSecret(t, "okta"))

	// Empty secret: rejected.
	body = f.updateBody(t, "okta")
	body["client_secret"] = ""
	rr = f.serve(t, asRoot, http.MethodPut, path, body)
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Equal(t, []string{"client_secret"}, validationFields(t, rr))

	// An issuer change without its own secret: rejected, the stored secret
	// stays with the issuer it was saved for.
	body = f.updateBody(t, "okta")
	body["issuer_url"] = "https://attacker.example.com"
	rr = f.serve(t, asRoot, http.MethodPut, path, body)
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Equal(t, []string{"client_secret"}, validationFields(t, rr))
	assert.Equal(t, authTestIssuer, *f.store.Provider("okta").IssuerURL)

	// A submitted secret replaces the stored one.
	replacement := "replacement-" + authTestSecretSentinel
	body = f.updateBody(t, "okta")
	body["client_secret"] = replacement
	rr = f.serve(t, asRoot, http.MethodPut, path, body)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, replacement, f.storedSecret(t, "okta"))
	assertBodyExcludes(t, rr.Body.Bytes(), replacement)
}

func TestAdminAuthProviders_UpdateRejections(t *testing.T) {
	cases := []struct {
		name       string
		path       func(id string) string
		mutate     func(map[string]any)
		wantStatus int
		wantCode   string
	}{
		{name: "stale expected version", mutate: func(b map[string]any) { b["expected_version"] = 1 },
			wantStatus: http.StatusConflict, wantCode: "INSTANCE_SETTINGS_VERSION_CONFLICT"},
		{name: "missing expected version", mutate: func(b map[string]any) { delete(b, "expected_version") },
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "missing enabled", mutate: func(b map[string]any) { delete(b, "enabled") },
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "slug is immutable", mutate: func(b map[string]any) { b["slug"] = "renamed" },
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "type is immutable", mutate: func(b map[string]any) { b["type"] = "github" },
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "blank display name", mutate: func(b map[string]any) { b["display_name"] = " " },
			wantStatus: http.StatusBadRequest, wantCode: "INSTANCE_SETTINGS_VALIDATION_FAILED"},
		{name: "unknown provider", path: func(string) string { return authProvidersPath + "/" + authTestUnknownUserID },
			mutate: func(map[string]any) {}, wantStatus: http.StatusNotFound, wantCode: "RESOURCE_NOT_FOUND"},
		{name: "malformed provider id", path: func(string) string { return authProvidersPath + "/not-a-uuid" },
			mutate: func(map[string]any) {}, wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			id := f.createProvider(t, oidcProviderBody("okta"))
			version := f.store.Version()
			body := f.updateBody(t, "okta")
			tc.mutate(body)
			path := authProvidersPath + "/" + id
			if tc.path != nil {
				path = tc.path(id)
			}

			rr := f.serve(t, asRoot, http.MethodPut, path, body)

			require.Equal(t, tc.wantStatus, rr.Code, rr.Body.String())
			code, _ := problemCodeAndReason(t, rr)
			assert.Equal(t, tc.wantCode, code)
			assert.Equal(t, version, f.store.Version(), "nothing was written")
		})
	}
}

func TestAdminAuthProviders_LockoutGuard(t *testing.T) {
	t.Run("disabling the last enabled provider needs confirmation", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		id := f.createProvider(t, oidcProviderBody("okta"))
		path := authProvidersPath + "/" + id
		body := f.updateBody(t, "okta")
		body["enabled"] = false

		rr := f.serve(t, asRoot, http.MethodPut, path, body)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		code, reason := problemCodeAndReason(t, rr)
		assert.Equal(t, "lockout_risk", code)
		assert.Equal(t, services.AuthLockoutNoEnabledProvider, reason)
		assert.True(t, f.store.Provider("okta").Enabled, "nothing was written")

		body["confirm_lockout_risk"] = true
		rr = f.serve(t, asRoot, http.MethodPut, path, body)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.False(t, f.store.Provider("okta").Enabled)
	})

	t.Run("deleting the last enabled provider needs confirmation", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		path := authProvidersPath + "/" + f.createProvider(t, oidcProviderBody("okta"))

		rr := f.serve(t, asRoot, http.MethodDelete, path, nil)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		code, reason := problemCodeAndReason(t, rr)
		assert.Equal(t, "lockout_risk", code)
		assert.Equal(t, services.AuthLockoutNoEnabledProvider, reason)
		assert.NotNil(t, f.store.Provider("okta"))

		rr = f.serve(t, asRoot, http.MethodDelete, path+"?confirm_lockout_risk=true", nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		assert.Nil(t, f.store.Provider("okta"))
	})

	t.Run("the provider the caller's session was issued by needs confirmation", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		oktaPath := authProvidersPath + "/" + f.createProvider(t, oidcProviderBody("okta"))
		backupPath := authProvidersPath + "/" + f.createProvider(t, oidcProviderBody("backup"))
		viaOkta := asSessionOf(authTestRootID, "okta")

		rr := f.serve(t, viaOkta, http.MethodDelete, oktaPath, nil)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		code, reason := problemCodeAndReason(t, rr)
		assert.Equal(t, "lockout_risk", code)
		assert.Equal(t, services.AuthLockoutOwnProvider, reason)

		// Another provider than the caller's own is not at risk, and neither is
		// the caller's own for an API key, which no provider issued.
		rr = f.serve(t, viaOkta, http.MethodDelete, backupPath, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		f.createProvider(t, oidcProviderBody("backup"))
		rr = f.serve(t, asRoot, http.MethodDelete, oktaPath, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	})

	t.Run("a change that takes no enabled provider away is never at risk", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		disabled := oidcProviderBody("spare")
		disabled["enabled"] = false
		sparePath := authProvidersPath + "/" + f.createProvider(t, disabled)

		rr := f.serve(t, asRoot, http.MethodPut, sparePath, f.updateBody(t, "spare"))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		rr = f.serve(t, asRoot, http.MethodDelete, sparePath, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	})
}

func TestAdminAuthProviders_DeleteRejections(t *testing.T) {
	f := newAuthSettingsFixture(t)
	id := f.createProvider(t, oidcProviderBody("okta"))
	f.createProvider(t, oidcProviderBody("backup"))

	rr := f.serve(t, asRoot, http.MethodDelete, authProvidersPath+"/"+id+"?expected_version=1", nil)
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	code, _ := problemCodeAndReason(t, rr)
	assert.Equal(t, "INSTANCE_SETTINGS_VERSION_CONFLICT", code)

	rr = f.serve(t, asRoot, http.MethodDelete, authProvidersPath+"/"+authTestUnknownUserID, nil)
	require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())

	rr = f.serve(t, asRoot, http.MethodDelete, authProvidersPath+"/not-a-uuid", nil)
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.NotNil(t, f.store.Provider("okta"))
}

func TestAdminAuthProviders_Test(t *testing.T) {
	f := newAuthSettingsFixture(t)
	id := f.createProvider(t, oidcProviderBody("okta"))
	testPath := authProvidersPath + "/test"
	version := f.store.Version()

	rr := f.serve(t, asRoot, http.MethodPost, testPath, map[string]any{"id": id})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"is_valid":true,"message":null}`, rr.Body.String())

	f.resolver.testErr = errors.New("oidc: discovery failed")
	rr = f.serve(t, asRoot, http.MethodPost, testPath, map[string]any{
		"type": "oidc", "client_id": "candidate", "client_secret": authTestSecretSentinel, "issuer_url": authTestIssuer,
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"is_valid":false,"message":"oidc: discovery failed"}`, rr.Body.String())
	assert.Equal(t, 2, f.resolver.tested)

	rr = f.serve(t, asRoot, http.MethodPost, testPath, map[string]any{"type": "github", "client_id": "candidate"})
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Equal(t, []string{"client_secret"}, validationFields(t, rr))

	rr = f.serve(t, asRoot, http.MethodPost, testPath, map[string]any{"id": authTestUnknownUserID})
	require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())

	rr = f.serve(t, asRoot, http.MethodPost, testPath, map[string]any{"id": id, "slug": "okta"})
	require.Equal(t, http.StatusBadRequest, rr.Code, "an unknown field is rejected: "+rr.Body.String())

	assert.Equal(t, 2, f.resolver.tested, "a rejected request is never tested")
	assert.Equal(t, version, f.store.Version(), "a test stores nothing")
	assert.Len(t, f.store.AuditEntries(models.InstanceSettingAuthProviders), 1, "a test audits nothing")
}

// --- allowlist ----------------------------------------------------------------

func TestAdminAuthAllowlist_RoundTrip(t *testing.T) {
	f := newAuthSettingsFixture(t)

	rr := f.serve(t, asRoot, http.MethodGet, authAllowlistPath, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"domains":[],"emails":[],"active":false,"version":null,
		"updated_at":null,"updated_by_user_id":null}`, rr.Body.String())

	// With nothing stored, any expected_version is a conflict.
	rr = f.serve(t, asRoot, http.MethodPut, authAllowlistPath,
		map[string]any{"domains": []string{"example.com"}, "emails": []string{}, "expected_version": 1})
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())

	rr = f.serve(t, asRoot, http.MethodPut, authAllowlistPath,
		map[string]any{"domains": []string{" Example.COM "}, "emails": []string{"Bob@Partner.example"}})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var stored admingen.AdminAuthAllowlist
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &stored))
	assert.Equal(t, []string{"example.com"}, stored.Domains)
	assert.Equal(t, []string{"bob@partner.example"}, stored.Emails)
	assert.True(t, stored.Active)
	require.NotNil(t, stored.Version)
	assert.Equal(t, int64(1), *stored.Version)
	require.NotNil(t, stored.UpdatedByUserId)
	assert.Equal(t, authTestRootID, stored.UpdatedByUserId.String())

	rr = f.serve(t, asRoot, http.MethodPut, authAllowlistPath,
		map[string]any{"domains": []string{}, "emails": []string{}, "expected_version": 7})
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	code, _ := problemCodeAndReason(t, rr)
	assert.Equal(t, "INSTANCE_SETTINGS_VERSION_CONFLICT", code)

	// Two empty lists are stored as open access, not as "nobody".
	rr = f.serve(t, asRoot, http.MethodPut, authAllowlistPath,
		map[string]any{"domains": []string{}, "emails": []string{}, "expected_version": 1})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &stored))
	assert.False(t, stored.Active)
	assert.Equal(t, int64(2), *stored.Version)

	rr = f.serve(t, asRoot, http.MethodDelete, authAllowlistPath, nil)
	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	rr = f.serve(t, asRoot, http.MethodDelete, authAllowlistPath, nil)
	require.Equal(t, http.StatusNoContent, rr.Code, "resetting nothing is still a 204")
	assert.Len(t, f.store.AuditEntries(models.InstanceSettingAuthAllowlist), 3, "two upserts and one delete")

	rr = f.serve(t, asRoot, http.MethodGet, authAllowlistPath, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &stored))
	assert.Nil(t, stored.Version)
}

func TestAdminAuthAllowlist_BadBodiesAre400(t *testing.T) {
	cases := map[string]map[string]any{
		"invalid domain": {"domains": []string{"not a domain"}, "emails": []string{}},
		"invalid email":  {"domains": []string{}, "emails": []string{"nope"}},
		"missing list":   {"domains": []string{"example.com"}},
		"null list":      {"domains": []string{"example.com"}, "emails": nil},
		"unknown field":  {"domains": []string{}, "emails": []string{}, "active": true},
	}
	for name, body := range cases {
		for _, path := range []string{authAllowlistPath, authAllowlistPath + "/preview"} {
			t.Run(name+" "+path, func(t *testing.T) {
				f := newAuthSettingsFixture(t)
				method := http.MethodPut
				if strings.HasSuffix(path, "/preview") {
					method = http.MethodPost
				}

				rr := f.serve(t, asRoot, method, path, body)

				require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
				assert.Empty(t, f.store.AuditEntries(models.InstanceSettingAuthAllowlist))
			})
		}
	}
}

func TestAdminAuthAllowlist_Preview(t *testing.T) {
	f := newAuthSettingsFixture(t)
	f.store.Outside = func(domains, _, exempt []string) (int, []string) {
		assert.Equal(t, []string{"example.com"}, domains)
		assert.Equal(t, []string{authTestRootEmail}, exempt)
		return 3, []string{"ada@other.example", "bob@other.example"}
	}

	rr := f.serve(t, asRoot, http.MethodPost, authAllowlistPath+"/preview",
		map[string]any{"domains": []string{"Example.com"}, "emails": []string{}})

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"count":3,"sample":["ada@other.example","bob@other.example"],"sample_truncated":true}`,
		rr.Body.String())

	// An open-access candidate affects nobody, with an empty (not null) sample.
	rr = f.serve(t, asRoot, http.MethodPost, authAllowlistPath+"/preview",
		map[string]any{"domains": []string{}, "emails": []string{}})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"count":0,"sample":[],"sample_truncated":false}`, rr.Body.String())

	assert.Equal(t, int64(1), f.store.Version(), "a preview writes nothing")
	assert.Empty(t, f.store.AuditEntries(models.InstanceSettingAuthAllowlist))
}

// --- instance admins ----------------------------------------------------------

func TestAdminInstanceAdmins_GrantListRevoke(t *testing.T) {
	f := newAuthSettingsFixture(t)

	rr := f.serve(t, asRoot, http.MethodGet, authAdminsPath, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"root_admins":["`+authTestRootEmail+`"],"admins":[]}`, rr.Body.String())

	rr = f.serve(t, asRoot, http.MethodPost, authAdminsPath, map[string]any{"user_id": authTestDBAdminID})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), "user_id", "email", "name", "granted_by_user_id", "granted_at")

	rr = f.serve(t, asRoot, http.MethodPost, authAdminsPath, map[string]any{"email": authTestMemberEmail})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var granted admingen.AdminInstanceAdmin
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &granted))
	assert.Equal(t, authTestMemberID, granted.UserId.String())
	assert.Nil(t, granted.Name, "a user with no display name")
	require.NotNil(t, granted.GrantedByUserId)
	assert.Equal(t, authTestRootID, granted.GrantedByUserId.String())

	rr = f.serve(t, asRoot, http.MethodGet, authAdminsPath, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var list admingen.AdminInstanceAdminList
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
	require.Len(t, list.Admins, 2)
	assert.Equal(t, authTestDBAdminID, list.Admins[0].UserId.String(), "oldest first")

	rr = f.serve(t, asRoot, http.MethodDelete, authAdminsPath+"/"+authTestMemberID, nil)
	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	rr = f.serve(t, asRoot, http.MethodDelete, authAdminsPath+"/"+authTestMemberID, nil)
	require.Equal(t, http.StatusNotFound, rr.Code, "no grant left to revoke: "+rr.Body.String())
	assert.Len(t, f.store.AuditEntries(models.InstanceSettingInstanceAdmins), 3, "two grants and one revoke")
}

func TestAdminInstanceAdmins_Rejections(t *testing.T) {
	cases := []struct {
		name       string
		caller     authCaller
		method     string
		path       string
		body       map[string]any
		wantStatus int
	}{
		{name: "a DB-granted admin cannot grant", caller: asDBAdmin, method: http.MethodPost, path: authAdminsPath,
			body: map[string]any{"user_id": authTestMemberID}, wantStatus: http.StatusForbidden},
		{name: "a DB-granted admin cannot probe emails", caller: asDBAdmin, method: http.MethodPost, path: authAdminsPath,
			body: map[string]any{"email": "nobody@instance.test"}, wantStatus: http.StatusForbidden},
		{name: "a DB-granted admin cannot revoke", caller: asDBAdmin, method: http.MethodDelete,
			path: authAdminsPath + "/" + authTestDBAdminID, wantStatus: http.StatusForbidden},
		{name: "neither user id nor email", caller: asRoot, method: http.MethodPost, path: authAdminsPath,
			body: map[string]any{}, wantStatus: http.StatusBadRequest},
		{name: "both user id and email", caller: asRoot, method: http.MethodPost, path: authAdminsPath,
			body:       map[string]any{"user_id": authTestMemberID, "email": authTestMemberEmail},
			wantStatus: http.StatusBadRequest},
		{name: "unknown field", caller: asRoot, method: http.MethodPost, path: authAdminsPath,
			body: map[string]any{"user_id": authTestMemberID, "role": "root"}, wantStatus: http.StatusBadRequest},
		{name: "unknown user id", caller: asRoot, method: http.MethodPost, path: authAdminsPath,
			body: map[string]any{"user_id": authTestUnknownUserID}, wantStatus: http.StatusNotFound},
		{name: "unknown email", caller: asRoot, method: http.MethodPost, path: authAdminsPath,
			body: map[string]any{"email": "nobody@instance.test"}, wantStatus: http.StatusNotFound},
		{name: "suspended user", caller: asRoot, method: http.MethodPost, path: authAdminsPath,
			body: map[string]any{"user_id": authTestSuspendedID}, wantStatus: http.StatusBadRequest},
		{name: "granting a root admin", caller: asRoot, method: http.MethodPost, path: authAdminsPath,
			body: map[string]any{"email": authTestRootEmail}, wantStatus: http.StatusConflict},
		{name: "revoking a root admin", caller: asRoot, method: http.MethodDelete,
			path: authAdminsPath + "/" + authTestRootID, wantStatus: http.StatusConflict},
		{name: "revoking a user with no grant", caller: asRoot, method: http.MethodDelete,
			path: authAdminsPath + "/" + authTestMemberID, wantStatus: http.StatusNotFound},
		{name: "malformed user id", caller: asRoot, method: http.MethodDelete,
			path: authAdminsPath + "/not-a-uuid", wantStatus: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			_, err := f.store.Grants().Grant(context.Background(), authTestDBAdminID, nil)
			require.NoError(t, err)
			var body any
			if tc.body != nil {
				body = tc.body
			}

			rr := f.serve(t, tc.caller, tc.method, tc.path, body)

			require.Equal(t, tc.wantStatus, rr.Code, rr.Body.String())
			assert.Len(t, f.store.AuditEntries(models.InstanceSettingInstanceAdmins), 1, "only the seeded grant")
		})
	}
}

// --- audit --------------------------------------------------------------------

func TestAdminAuthSettingsAudit_RedactsEverySetting(t *testing.T) {
	f := newAuthSettingsFixture(t)
	id := f.createProvider(t, oidcProviderBody("okta"))
	body := f.updateBody(t, "okta")
	body["display_name"] = "Renamed"
	require.Equal(t, http.StatusOK, f.serve(t, asRoot, http.MethodPut, authProvidersPath+"/"+id, body).Code)

	// A snapshot as a careless writer might have stored it: a raw credential
	// under a secret key, the ciphertext, and a key nobody allowlisted.
	leaked, err := json.Marshal(map[string]any{
		"slug": "leaky", "client_secret": authTestSecretSentinel,
		"client_secret_encrypted": authTestSecretSentinel, "token_hash": authTestSecretSentinel,
		"internal_note": authTestSecretSentinel,
	})
	require.NoError(t, err)
	for _, setting := range []string{
		models.InstanceSettingAuthProviders, models.InstanceSettingAuthAllowlist,
		models.InstanceSettingInstanceAdmins, models.InstanceSettingAuthSetup,
	} {
		f.store.AppendAudit(&models.InstanceSettingsAuditEntry{
			Setting: setting, Action: models.InstanceSettingsAuditActionUpsert, After: leaked,
		})
	}

	rr := f.serve(t, asRoot, http.MethodGet, authAuditPath+"?setting=auth_providers", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var page admingen.AdminInstanceSettingsAuditPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Entries, 3, "newest first: the leaky entry, the update, the create")
	assert.Equal(t, map[string]interface{}{"slug": "leaky"}, *page.Entries[0].After,
		"a non-marker client_secret and every unlisted key are dropped")

	update := page.Entries[1]
	require.NotNil(t, update.ActorName)
	assert.Equal(t, "Root Admin", *update.ActorName)
	assert.Equal(t, models.InstanceSettingsAuditSecretUnchanged, (*update.After)["client_secret"])
	assert.Equal(t, true, (*update.After)["has_client_secret"])
	assert.Equal(t, "Renamed", (*update.After)["display_name"])
	assert.NotContains(t, *update.Before, "client_secret", "only an after snapshot carries the marker")
	create := page.Entries[2]
	assert.Nil(t, create.Before)
	assert.Equal(t, models.InstanceSettingsAuditSecretChanged, (*create.After)["client_secret"])

	for _, setting := range []string{"auth_allowlist", "instance_admins", "auth_setup"} {
		rr = f.serve(t, asRoot, http.MethodGet, authAuditPath+"?setting="+setting, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var other admingen.AdminInstanceSettingsAuditPage
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &other))
		require.NotEmpty(t, other.Entries, setting)
		assert.Empty(t, *other.Entries[0].After, setting+": no key of the leaky snapshot is allowlisted")
		assert.Contains(t, authAuditSnapshotKeysOf(t, setting), authAuditSourceKey, setting+" surfaces the write's source")
	}
}

func TestAdminAuthSettingsAudit_SetupSessionWritesHaveNoActor(t *testing.T) {
	f := newAuthSettingsFixture(t)

	rr := f.serve(t, asSetupSession, http.MethodPost, authProvidersPath, oidcProviderBody("okta"))
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var saved admingen.AdminAuthProviderSaved
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &saved))
	assert.Nil(t, saved.Provider.UpdatedByUserId)

	rr = f.serve(t, asRoot, http.MethodGet, authAuditPath+"?setting=auth_providers", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var page admingen.AdminInstanceSettingsAuditPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Entries, 1)
	assert.Nil(t, page.Entries[0].ActorUserId)
	assert.Nil(t, page.Entries[0].ActorName)
}

// authAuditSnapshotKeysOf returns the allowlisted snapshot keys of a setting.
func authAuditSnapshotKeysOf(t *testing.T, setting string) []string {
	t.Helper()
	keys, ok := map[string][]string{
		models.InstanceSettingAuthProviders:  authProviderAuditSnapshotKeys,
		models.InstanceSettingAuthAllowlist:  authAllowlistAuditSnapshotKeys,
		models.InstanceSettingInstanceAdmins: instanceAdminAuditSnapshotKeys,
		models.InstanceSettingAuthSetup:      authSetupAuditSnapshotKeys,
	}[setting]
	require.True(t, ok, setting)
	return keys
}

// A setup audit entry surfaces its event, its state and where it came from,
// and never the token hash.
func TestAdminAuthSettingsAudit_SetupEntryOmitsTheTokenHash(t *testing.T) {
	f := newAuthSettingsFixture(t)
	after, err := json.Marshal(map[string]any{
		"event": "rearmed", "generation": 1, "rearmed": true, "token_hash": authTestSecretSentinel,
		"source": repositories.AuditSourceCLI,
	})
	require.NoError(t, err)
	f.store.AppendAudit(&models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingAuthSetup, Action: models.InstanceSettingsAuditActionUpsert, After: after,
	})

	rr := f.serve(t, asRoot, http.MethodGet, authAuditPath+"?setting=auth_setup", nil)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var page admingen.AdminInstanceSettingsAuditPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Entries, 1)
	assert.Equal(t, map[string]interface{}{
		"event": "rearmed", "generation": float64(1), "rearmed": true, "source": repositories.AuditSourceCLI,
	}, *page.Entries[0].After, "a write made by the CLI keeps its source")
}

func TestAdminAuthSettingsAudit_PagesAndBadParams(t *testing.T) {
	f := newAuthSettingsFixture(t)
	for _, slug := range []string{"one", "two", "three"} {
		f.createProvider(t, oidcProviderBody(slug))
	}

	rr := f.serve(t, asRoot, http.MethodGet, authAuditPath+"?setting=auth_providers&limit=2", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var page admingen.AdminInstanceSettingsAuditPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Entries, 2)
	require.NotNil(t, page.NextCursor)

	rr = f.serve(t, asRoot, http.MethodGet, authAuditPath+"?setting=auth_providers&cursor="+*page.NextCursor, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var last admingen.AdminInstanceSettingsAuditPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &last))
	require.Len(t, last.Entries, 1)
	assert.Nil(t, last.NextCursor)

	for _, query := range []string{
		"", "?setting=email_provider", "?setting=nope", "?setting=auth_providers&limit=0",
		"?setting=auth_providers&limit=101", "?setting=auth_providers&cursor=%21",
	} {
		rr = f.serve(t, asRoot, http.MethodGet, authAuditPath+query, nil)
		assert.Equal(t, http.StatusBadRequest, rr.Code, query+": "+rr.Body.String())
	}
}

// TestAuthSettingsAuditVocabularyMatchesSpec pins the query enum to the stored
// setting names (the handler passes the enum value straight to the repository)
// and the stored names to the audit entry's published enum.
func TestAuthSettingsAuditVocabularyMatchesSpec(t *testing.T) {
	settings := []string{
		models.InstanceSettingAuthProviders, models.InstanceSettingAuthAllowlist,
		models.InstanceSettingInstanceAdmins, models.InstanceSettingAuthSetup,
	}
	require.Len(t, authAuditSnapshotFilters, len(settings))
	for _, setting := range settings {
		assert.True(t, admingen.AdminAuthSettingsAuditSetting(setting).Valid(), setting)
		assert.True(t, admingen.AdminInstanceSettingsAuditEntrySetting(setting).Valid(), setting)
		assert.Contains(t, authAuditSnapshotFilters, admingen.AdminAuthSettingsAuditSetting(setting))
	}
	for _, status := range []string{
		models.InstanceAuthProviderHealthy, models.InstanceAuthProviderUnhealthy,
		models.InstanceAuthProviderDisabled, models.InstanceAuthProviderHealthUnknown,
	} {
		assert.True(t, admingen.AdminAuthProviderHealthStatus(status).Valid(), status)
	}
	for _, providerType := range []models.InstanceAuthProviderType{
		models.InstanceAuthProviderGoogle, models.InstanceAuthProviderGitHub, models.InstanceAuthProviderOIDC,
	} {
		assert.True(t, admingen.AdminAuthProviderType(providerType).Valid(), providerType)
	}
}

// --- authorization ------------------------------------------------------------

// authRoute is one instance authentication settings operation, with a request
// that succeeds for a caller who is admitted and allowed.
type authRoute struct {
	name   string
	method string
	// path and body receive the id of the seeded "okta" provider.
	path func(providerID string) string
	body func(f *authSettingsFixture, t *testing.T) any
	want int
}

func fixedPath(path string) func(string) string { return func(string) string { return path } }

func fixedBody(body any) func(*authSettingsFixture, *testing.T) any {
	return func(*authSettingsFixture, *testing.T) any { return body }
}

// authSetupRoutes are the operations a setup session may reach.
var authSetupRoutes = []authRoute{
	{name: "list providers", method: http.MethodGet, path: fixedPath(authProvidersPath), want: http.StatusOK},
	{name: "create provider", method: http.MethodPost, path: fixedPath(authProvidersPath),
		body: fixedBody(oidcProviderBody("fresh")), want: http.StatusCreated},
	{name: "test provider", method: http.MethodPost, path: fixedPath(authProvidersPath + "/test"),
		body: fixedBody(map[string]any{"type": "github", "client_id": "c", "client_secret": "candidate"}),
		want: http.StatusOK},
	{name: "update provider", method: http.MethodPut,
		path: func(id string) string { return authProvidersPath + "/" + id },
		body: func(f *authSettingsFixture, t *testing.T) any { return f.updateBody(t, "okta") }, want: http.StatusOK},
	{name: "delete provider", method: http.MethodDelete,
		path: func(id string) string { return authProvidersPath + "/" + id }, want: http.StatusNoContent},
	{name: "get allowlist", method: http.MethodGet, path: fixedPath(authAllowlistPath), want: http.StatusOK},
	{name: "update allowlist", method: http.MethodPut, path: fixedPath(authAllowlistPath),
		body: fixedBody(map[string]any{"domains": []string{"example.com"}, "emails": []string{}}), want: http.StatusOK},
	{name: "reset allowlist", method: http.MethodDelete, path: fixedPath(authAllowlistPath),
		want: http.StatusNoContent},
	{name: "preview allowlist", method: http.MethodPost, path: fixedPath(authAllowlistPath + "/preview"),
		body: fixedBody(map[string]any{"domains": []string{"example.com"}, "emails": []string{}}), want: http.StatusOK},
}

// authAdminOnlyRoutes are the operations only an instance admin may reach.
var authAdminOnlyRoutes = []authRoute{
	{name: "list admins", method: http.MethodGet, path: fixedPath(authAdminsPath), want: http.StatusOK},
	{name: "list audit", method: http.MethodGet, path: fixedPath(authAuditPath + "?setting=auth_providers"),
		want: http.StatusOK},
}

// authRootOnlyRoutes are the operations only a ROOT admin may perform.
var authRootOnlyRoutes = []authRoute{
	{name: "grant admin", method: http.MethodPost, path: fixedPath(authAdminsPath),
		body: fixedBody(map[string]any{"user_id": authTestMemberID}), want: http.StatusOK},
	{name: "revoke admin", method: http.MethodDelete, path: fixedPath(authAdminsPath + "/" + authTestDBAdminID),
		want: http.StatusNoContent},
}

// TestAdminAuthSettings_AuthorizationMatrix drives every operation through the
// full router as each kind of caller. Anonymous and non-admin callers get 404
// everywhere; a setup session reaches the provider and allowlist operations
// and nothing else; a DB-granted admin reaches everything but the root-only
// grant and revoke, which answer 403.
func TestAdminAuthSettings_AuthorizationMatrix(t *testing.T) {
	const notFound, forbidden = http.StatusNotFound, http.StatusForbidden
	callers := []struct {
		name   string
		caller authCaller
		// status maps the route's success status onto what this caller gets,
		// per route group.
		setup, adminOnly, rootOnly func(ok int) int
	}{
		{name: "anonymous", caller: asAnonymous,
			setup: always(notFound), adminOnly: always(notFound), rootOnly: always(notFound)},
		{name: "member", caller: asMember,
			setup: always(notFound), adminOnly: always(notFound), rootOnly: always(notFound)},
		{name: "setup session", caller: asSetupSession,
			setup: same, adminOnly: always(notFound), rootOnly: always(notFound)},
		{name: "db admin", caller: asDBAdmin, setup: same, adminOnly: same, rootOnly: always(forbidden)},
		{name: "root admin", caller: asRoot, setup: same, adminOnly: same, rootOnly: same},
	}
	groups := []struct {
		name   string
		routes []authRoute
	}{
		{"setup", authSetupRoutes}, {"admin-only", authAdminOnlyRoutes}, {"root-only", authRootOnlyRoutes},
	}

	for _, c := range callers {
		for _, group := range groups {
			for _, route := range group.routes {
				t.Run(c.name+"/"+route.name, func(t *testing.T) {
					f := newAuthSettingsFixture(t)
					f.primeSetupSession(t)
					// Two enabled providers, so no request trips the lockout guard.
					id := f.createProvider(t, oidcProviderBody("okta"))
					f.createProvider(t, oidcProviderBody("backup"))
					_, err := f.store.Grants().Grant(context.Background(), authTestDBAdminID, nil)
					require.NoError(t, err)
					var body any
					if route.body != nil {
						body = route.body(f, t)
					}
					want := map[string]func(int) int{
						"setup": c.setup, "admin-only": c.adminOnly, "root-only": c.rootOnly,
					}[group.name](route.want)

					rr := f.serve(t, c.caller, route.method, route.path(id), body)

					assert.Equal(t, want, rr.Code, rr.Body.String())
				})
			}
		}
	}
}

func always(status int) func(int) int { return func(int) int { return status } }

func same(status int) int { return status }

// TestAdminRouteGuard_SetupSessionReachesNothingElse pins the rest of the admin
// surface as instance-admin only: a valid setup session gets 404 on every
// other admin route, including the neighbouring settings.
func TestAdminRouteGuard_SetupSessionReachesNothingElse(t *testing.T) {
	routes := append([]struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/stats"},
		{http.MethodGet, "/api/v1/admin/users"},
		{http.MethodGet, authSettingsPath},
		{http.MethodGet, authSettingsPath + "/providers/x/y"},
		{http.MethodGet, authSettingsPath + "/allowlists"},
	}, adminInstanceSettingsRoutes...)

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			asSetupSession(t, f, req)
			rr := httptest.NewRecorder()

			f.srv.router.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
		})
	}
}

func TestAdminSetupSessionRoutesPattern(t *testing.T) {
	admitted := []string{
		authProvidersPath, authProvidersPath + "/test", authProvidersPath + "/" + authTestUnknownUserID,
		authAllowlistPath, authAllowlistPath + "/preview",
	}
	for _, path := range admitted {
		assert.True(t, adminSetupSessionRoutes.MatchString(path), path)
	}
	refused := []string{
		authAdminsPath, authAdminsPath + "/" + authTestMemberID, authAuditPath, authSettingsPath,
		authProvidersPath + "/a/b", authProvidersPath + "/", "/api/v1/admin/settings/email",
		"/x" + authProvidersPath, authProvidersPath + "x",
		// A route a later change might add under either prefix.
		authProvidersPath + "/reorder", authAllowlistPath + "/audit", authAllowlistPath + "/" + authTestMemberID,
	}
	for _, path := range refused {
		assert.False(t, adminSetupSessionRoutes.MatchString(path), path)
	}
}

// TestAdminSetupSessionRoutes_MatchExactlyTheMountedSetupRoutes walks the
// mounted route table: of every admin route, exactly the provider and allowlist
// operations match adminSetupSessionRoutes. A route added under either prefix
// fails here until it is deliberately admitted (or left admin-only).
func TestAdminSetupSessionRoutes_MatchExactlyTheMountedSetupRoutes(t *testing.T) {
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))

	var admitted []string
	walked := 0
	require.NoError(t, chi.Walk(srv.router,
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if !strings.HasPrefix(route, "/api/v1/admin/") {
				return nil
			}
			walked++
			// Every path parameter of these routes is a UUID.
			path := strings.NewReplacer("{id}", authTestUnknownUserID, "{user_id}", authTestUnknownUserID).Replace(route)
			if adminSetupSessionRoutes.MatchString(path) {
				admitted = append(admitted, method+" "+route)
			}
			return nil
		}))
	require.Greater(t, walked, 50, "the walk found the admin surface")

	assert.ElementsMatch(t, []string{
		"GET " + authProvidersPath, "POST " + authProvidersPath, "POST " + authProvidersPath + "/test",
		"PUT " + authProvidersPath + "/{id}", "DELETE " + authProvidersPath + "/{id}",
		"GET " + authAllowlistPath, "PUT " + authAllowlistPath, "DELETE " + authAllowlistPath,
		"POST " + authAllowlistPath + "/preview",
	}, admitted)
}

// --- failures -----------------------------------------------------------------

// TestAdminAuthSettings_ServiceFailuresAre500 covers each operation's
// unexpected-failure path with a mocked service, through the handlers alone.
func TestAdminAuthSettings_ServiceFailuresAre500(t *testing.T) {
	boom := errors.New("database unavailable")
	svc := servicesmocks.NewMockInstanceAuthSettingsServiceInterface(t)
	svc.On("ListProviders", mock.Anything).Return(nil, boom)
	svc.On("CreateProvider", mock.Anything, mock.Anything, mock.Anything).Return(nil, boom)
	svc.On("UpdateProvider", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, boom)
	svc.On("DeleteProvider", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(boom)
	svc.On("TestProvider", mock.Anything, mock.Anything).Return(nil, boom)
	svc.On("GetAllowlist", mock.Anything).Return(nil, boom)
	svc.On("UpdateAllowlist", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, boom)
	svc.On("ResetAllowlist", mock.Anything, mock.Anything).Return(boom)
	svc.On("PreviewAllowlist", mock.Anything, mock.Anything, mock.Anything).Return(nil, boom)
	svc.On("ListAdmins", mock.Anything).Return(nil, boom)
	svc.On("GrantAdmin", mock.Anything, mock.Anything, mock.Anything).Return(nil, boom)
	svc.On("RevokeAdmin", mock.Anything, mock.Anything, mock.Anything).Return(boom)
	router := mountAdminStrictRouter(newAdminTestServer(&config.Config{},
		&adminMockContainer{instanceAuthSettingsService: svc}))

	routes := append(append(append([]authRoute{}, authSetupRoutes...), authAdminOnlyRoutes[0]), authRootOnlyRoutes...)
	update := map[string]any{
		"display_name": "n", "enabled": true, "sort_order": 0, "client_id": "c", "expected_version": 1,
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			var body any
			switch {
			case route.name == "update provider":
				body = update
			case route.body != nil:
				body = route.body(nil, t)
			}
			req := instanceEmailRequest(t, route.method, route.path(authTestUnknownUserID), body)

			rr := serveInstanceEmail(t, router, req)

			require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
			assert.NotContains(t, rr.Body.String(), boom.Error(), "the cause is logged, not returned")
		})
	}
}

// TestAdminAuthSettings_DatabaseRejectedProviderIs400 covers a row the
// validator accepted but a database CHECK refused: a 400, not a 500.
func TestAdminAuthSettings_DatabaseRejectedProviderIs400(t *testing.T) {
	svc := servicesmocks.NewMockInstanceAuthSettingsServiceInterface(t)
	svc.On("CreateProvider", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, repositories.ErrInstanceAuthProviderInvalid)
	router := mountAdminStrictRouter(newAdminTestServer(&config.Config{},
		&adminMockContainer{instanceAuthSettingsService: svc}))
	req := instanceEmailRequest(t, http.MethodPost, authProvidersPath, oidcProviderBody("okta"))

	rr := serveInstanceEmail(t, router, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
	code, _ := problemCodeAndReason(t, rr)
	assert.Equal(t, "INSTANCE_SETTINGS_VALIDATION_FAILED", code)
}

// TestAdminAuthSettings_UnparseableStoredIDsAre500 covers the response mappers'
// refusal to render a row whose id is not a UUID.
func TestAdminAuthSettings_UnparseableStoredIDsAre500(t *testing.T) {
	notUUID := "not-a-uuid"
	svc := servicesmocks.NewMockInstanceAuthSettingsServiceInterface(t)
	svc.On("ListProviders", mock.Anything).Return(&models.InstanceAuthProviderList{
		Providers: []models.InstanceAuthProviderView{{Provider: models.InstanceAuthProvider{ID: notUUID}}},
	}, nil)
	svc.On("CreateProvider", mock.Anything, mock.Anything, mock.Anything).Return(&models.InstanceAuthProviderSaved{
		Provider: models.InstanceAuthProviderView{Provider: models.InstanceAuthProvider{
			ID: authTestUnknownUserID, UpdatedBy: &notUUID}},
	}, nil)
	svc.On("GetAllowlist", mock.Anything).Return(&models.InstanceAuthAllowlist{UpdatedBy: &notUUID}, nil)
	svc.On("ListAdmins", mock.Anything).Return(&models.InstanceAdminList{
		Admins: []models.InstanceAdminView{{UserID: notUUID}},
	}, nil)
	svc.On("GrantAdmin", mock.Anything, mock.Anything, mock.Anything).Return(
		&models.InstanceAdminView{UserID: authTestMemberID, GrantedBy: &notUUID}, nil)
	router := mountAdminStrictRouter(newAdminTestServer(&config.Config{},
		&adminMockContainer{instanceAuthSettingsService: svc}))

	for _, route := range []authRoute{
		authSetupRoutes[0], authSetupRoutes[1], authSetupRoutes[5], authAdminOnlyRoutes[0], authRootOnlyRoutes[0],
	} {
		t.Run(route.name, func(t *testing.T) {
			var body any
			if route.body != nil {
				body = route.body(nil, t)
			}
			req := instanceEmailRequest(t, route.method, route.path(""), body)

			rr := serveInstanceEmail(t, router, req)

			require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}
