package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	setupTestRootEmail = "root@instance.test"
	setupTestBaseURL   = "https://vibexp.instance.test"
)

// memSetupState is an in-memory InstanceAuthSetupRepository with the stored
// repository's semantics, so the handler tests run the REAL SetupModeService.
type memSetupState struct {
	mu         sync.Mutex
	row        *models.InstanceAuthSetup
	getErr     error
	consumeErr error
}

func (m *memSetupState) Get(context.Context) (*models.InstanceAuthSetup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.row == nil {
		return nil, repositories.ErrInstanceAuthSetupNotFound
	}
	cp := *m.row
	return &cp, nil
}

func (m *memSetupState) store(hash []byte, expiresAt time.Time, rearm bool) *models.InstanceAuthSetup {
	next := &models.InstanceAuthSetup{TokenHash: hash, ExpiresAt: &expiresAt, Rearmed: rearm, Generation: 1}
	if m.row != nil {
		next.Generation, next.Rearmed = m.row.Generation+1, m.row.Rearmed || rearm
	}
	m.row = next
	cp := *next
	return &cp
}

func (m *memSetupState) MintIfAbsentOrExpired(
	_ context.Context, hash []byte, expiresAt, now time.Time,
) (*models.InstanceAuthSetup, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.row != nil && m.row.HasLiveToken(now) {
		cp := *m.row
		return &cp, false, nil
	}
	return m.store(hash, expiresAt, false), true, nil
}

func (m *memSetupState) ForceMint(
	_ context.Context, hash []byte, expiresAt time.Time,
) (*models.InstanceAuthSetup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store(hash, expiresAt, true), nil
}

func (m *memSetupState) MintReplacing(
	_ context.Context, hash []byte, expiresAt time.Time,
) (*models.InstanceAuthSetup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store(hash, expiresAt, false), nil
}

func (m *memSetupState) Consume(_ context.Context, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.consumeErr != nil {
		return false, m.consumeErr
	}
	if m.row == nil || m.row.IsConsumed() {
		return false, nil
	}
	now := time.Now()
	m.row.TokenHash, m.row.ConsumedAt, m.row.ConsumedBy = nil, &now, &userID
	m.row.Rearmed = false
	m.row.Generation++
	return true, nil
}

func (m *memSetupState) consumed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.row != nil && m.row.IsConsumed()
}

// setupTestContainer serves the real setup-mode service over the admin mock
// container.
type setupTestContainer struct {
	*adminMockContainer
	setup services.SetupModeService
}

func (c *setupTestContainer) SetupModeService() services.SetupModeService { return c.setup }

type setupHarness struct {
	srv       *Server
	state     *memSetupState
	providers *memAuthProviders
	setup     services.SetupModeService
	authSvc   *servicesmocks.MockAuthServiceInterface
	keySvc    *servicesmocks.MockAPIKeyServiceInterface
}

func newSetupModeService(state *memSetupState, providers *memAuthProviders) services.SetupModeService {
	return services.NewSetupModeService(services.SetupModeDeps{
		Setup:       state,
		Providers:   providers,
		IsRootAdmin: func(email string) bool { return strings.EqualFold(email, setupTestRootEmail) },
		BaseURL:     setupTestBaseURL,
		Logger:      slog.New(slog.DiscardHandler),
	})
}

// newSetupHarness builds the full router over a fresh instance in setup mode:
// no provider rows, and a production-like (non-localhost) base URL so the
// cookie is Secure and the rate limiters are live when a limit is configured.
func newSetupHarness(t *testing.T, limits config.RateLimitConfig) *setupHarness {
	t.Helper()
	h := &setupHarness{
		state: &memSetupState{}, providers: &memAuthProviders{},
		authSvc: servicesmocks.NewMockAuthServiceInterface(t),
		keySvc:  servicesmocks.NewMockAPIKeyServiceInterface(t),
	}
	h.setup = newSetupModeService(h.state, h.providers)
	cfg := &config.Config{
		Frontend:  config.FrontendConfig{BaseURL: setupTestBaseURL},
		Auth:      config.AuthConfig{SessionEncryptionKey: testCookiePassword, InstanceAdmins: config.EnvStringSlice{setupTestRootEmail}},
		RateLimit: limits,
	}
	admin := &adminMockContainer{authService: h.authSvc, apiKeyService: h.keySvc}
	h.srv = newAdminTestServer(cfg, admin)
	h.srv.container = &setupTestContainer{adminMockContainer: admin, setup: h.setup}
	return h
}

// bootToken mints the setup token as boot does and returns it.
func (h *setupHarness) bootToken(t *testing.T) string {
	t.Helper()
	setupURL, minted, err := h.setup.EnsureTokenAtBoot(context.Background())
	require.NoError(t, err)
	require.True(t, minted)
	return strings.TrimPrefix(setupURL, setupTestBaseURL+"/setup?token=")
}

func (h *setupHarness) exchange(token string) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/session",
		strings.NewReader(`{"token":`+strconv.Quote(token)+`}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.srv.router.ServeHTTP(rr, req)
	return req, rr
}

// setupCookie exchanges token and returns the setup cookie.
func (h *setupHarness) setupCookie(t *testing.T, token string) *http.Cookie {
	t.Helper()
	_, rr := h.exchange(token)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	for _, c := range rr.Result().Cookies() {
		if c.Name == sesslib.SetupCookieName {
			return c
		}
	}
	t.Fatal("no setup cookie")
	return nil
}

func (h *setupHarness) enableProvider() {
	h.providers.replace(&models.InstanceAuthProvider{ID: "p-1", Slug: "sso", Enabled: true})
}

func problemOf(t *testing.T, rr *httptest.ResponseRecorder) (code, detail string) {
	t.Helper()
	var p struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &p), rr.Body.String())
	return p.Code, p.Detail
}

func TestGetSetupStatus(t *testing.T) {
	status := func(h *setupHarness) (*http.Request, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
		rr := httptest.NewRecorder()
		h.srv.router.ServeHTTP(rr, req)
		return req, rr
	}

	t.Run("setup required: the body is the one boolean and nothing else", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		h.bootToken(t) // an outstanding token must not show up in the body
		req, rr := status(h)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.JSONEq(t, `{"setup_required":true}`, rr.Body.String())
		assert.Empty(t, rr.Result().Cookies(), "the public status sets no cookie")
	})

	t.Run("not required once a provider is enabled", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		h.enableProvider()
		req, rr := status(h)

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.JSONEq(t, `{"setup_required":false}`, rr.Body.String())
	})

	t.Run("an unreadable state is a 500 that names nothing", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		h.state.getErr = errors.New("pq: connection refused to db.internal:5432")
		req, rr := status(h)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.NotContains(t, rr.Body.String(), "db.internal")
		assert.NotContains(t, rr.Body.String(), "setup_required")
	})
}

func TestCreateSetupSession(t *testing.T) {
	t.Run("a valid token starts a setup session", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		token := h.bootToken(t)
		before := time.Now()
		req, rr := h.exchange(token)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)

		cookies := rr.Result().Cookies()
		require.Len(t, cookies, 1, "only the setup cookie is written, never a user session")
		c := cookies[0]
		assert.Equal(t, sesslib.SetupCookieName, c.Name)
		assert.True(t, c.HttpOnly)
		assert.True(t, c.Secure)
		assert.Equal(t, http.SameSiteStrictMode, c.SameSite)
		assert.Equal(t, 3600, c.MaxAge)
		assert.NotContains(t, c.Value, token)

		var body struct {
			ExpiresAt time.Time `json:"expires_at"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.WithinDuration(t, before.Add(time.Hour), body.ExpiresAt, 5*time.Second)
		assert.NotContains(t, rr.Body.String(), token)

		// The token stays exchangeable until consumed, so a lost cookie is
		// recoverable from the same URL.
		_, again := h.exchange(token)
		assert.Equal(t, http.StatusOK, again.Code)
	})

	t.Run("unknown, expired and consumed tokens get one identical 401", func(t *testing.T) {
		rejected := map[string]*httptest.ResponseRecorder{}

		h := newSetupHarness(t, config.RateLimitConfig{})
		h.bootToken(t)
		req, rr := h.exchange("not-the-token")
		require.Equal(t, http.StatusUnauthorized, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		rejected["unknown"] = rr

		h = newSetupHarness(t, config.RateLimitConfig{})
		token := h.bootToken(t)
		past := time.Now().Add(-time.Minute)
		h.state.row.ExpiresAt = &past
		_, rejected["expired"] = h.exchange(token)

		h = newSetupHarness(t, config.RateLimitConfig{})
		token = h.bootToken(t)
		require.NoError(t, h.setup.ConsumeOnRootLogin(context.Background(),
			&models.User{ID: "u-root", Email: setupTestRootEmail}))
		_, rejected["consumed"] = h.exchange(token)

		for name, rr := range rejected {
			require.Equal(t, http.StatusUnauthorized, rr.Code, name)
			code, detail := problemOf(t, rr)
			assert.Equal(t, "AUTH_INVALID", code, name)
			assert.Equal(t, setupMsgRejected, detail, name)
			assert.Empty(t, rr.Result().Cookies(), name)
		}
	})

	t.Run("outside setup mode the endpoint is a 404, even for the right token", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		token := h.bootToken(t)
		h.enableProvider()
		req, rr := h.exchange(token)

		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Empty(t, rr.Result().Cookies())
	})

	t.Run("a malformed or token-less body is a 400", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		h.bootToken(t)
		for _, body := range []string{`{not json`, `{}`, `{"token":""}`} {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/session", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			h.srv.router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, body)
			specconformance.AssertConformsToSpec(t, req, rr)
		}
	})

	t.Run("an unreadable state is a 500, not a 401", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		token := h.bootToken(t)
		h.state.getErr = errors.New("db down")
		req, rr := h.exchange(token)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("no session manager configured is a 500", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		token := h.bootToken(t)
		h.srv.sessionManager = nil
		_, rr := h.exchange(token)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

// The token exchange takes the strict per-IP auth limit; the status, which the
// web app fetches on every load, does not.
func TestCreateSetupSession_IsRateLimitedPerIP(t *testing.T) {
	h := newSetupHarness(t, config.RateLimitConfig{AuthPerMinute: 2, APIPerMinute: 1000})
	h.bootToken(t)

	exchangeFrom := func(ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/session", strings.NewReader(`{"token":"guess"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":40000"
		rr := httptest.NewRecorder()
		h.srv.router.ServeHTTP(rr, req)
		return rr.Code
	}

	assert.Equal(t, http.StatusUnauthorized, exchangeFrom("203.0.113.7"))
	assert.Equal(t, http.StatusUnauthorized, exchangeFrom("203.0.113.7"))
	assert.Equal(t, http.StatusTooManyRequests, exchangeFrom("203.0.113.7"), "the third guess from one IP is throttled")
	assert.Equal(t, http.StatusUnauthorized, exchangeFrom("198.51.100.9"), "another IP has its own budget")

	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
		req.RemoteAddr = "203.0.113.7:40000"
		rr := httptest.NewRecorder()
		h.srv.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, "the status is not on the auth limit")
	}
}

// A setup session is not a user session: everything except the setup guard
// treats its holder as anonymous.
func TestSetupSession_IsAnonymousEverywhereElse(t *testing.T) {
	h := newSetupHarness(t, config.RateLimitConfig{})
	cookie := h.setupCookie(t, h.bootToken(t))

	// Present the setup cookie under both names: its own, and replayed as the
	// user session cookie.
	as := func(name string) *http.Cookie { return &http.Cookie{Name: name, Value: cookie.Value} }

	routes := []struct {
		method, path string
		want         int
	}{
		// Instance-admin routes do not advertise themselves: 404.
		{http.MethodGet, "/api/v1/admin/users", http.StatusNotFound},
		{http.MethodGet, "/api/v1/admin/teams", http.StatusNotFound},
		{http.MethodGet, "/api/v1/admin/projects", http.StatusNotFound},
		{http.MethodGet, "/api/v1/admin/dashboard/stats", http.StatusNotFound},
		{http.MethodGet, "/api/v1/admin/settings/email", http.StatusNotFound},
		{http.MethodPut, "/api/v1/admin/settings/search", http.StatusNotFound},
		// User and team routes require a user: 401.
		{http.MethodGet, "/api/v1/auth/me", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/teams", http.StatusUnauthorized},
		{http.MethodPost, "/api/v1/teams", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/api-keys", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/11111111-1111-4111-8111-111111111111/prompts", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/11111111-1111-4111-8111-111111111111/memories", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/11111111-1111-4111-8111-111111111111/settings/search", http.StatusUnauthorized},
	}
	for _, route := range routes {
		for _, name := range []string{sesslib.SetupCookieName, sesslib.CookieName} {
			t.Run(route.method+" "+route.path+" as "+name, func(t *testing.T) {
				req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(as(name))
				rr := httptest.NewRecorder()
				h.srv.router.ServeHTTP(rr, req)
				assert.Equal(t, route.want, rr.Code, rr.Body.String())
			})
		}
	}
}

// guardedRouter mounts a stand-in for an authentication-settings route behind
// the setup guard alone, the way the admin API will mount it: the guard
// authenticates the request itself. The real routes arrive with #1238; this
// pins the guard they will sit behind.
func guardedRouter(h *setupHarness, reached *context.Context) http.Handler {
	r := chi.NewRouter()
	r.Use(h.srv.setupSessionOrInstanceAdmin)
	r.Post("/api/v1/admin/settings/auth/providers", func(w http.ResponseWriter, req *http.Request) {
		*reached = req.Context()
		w.WriteHeader(http.StatusCreated)
	})
	return r
}

func TestSetupSessionOrInstanceAdmin(t *testing.T) {
	call := func(h *setupHarness, decorate func(*http.Request)) (int, context.Context) {
		var reached context.Context
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/auth/providers", nil)
		if decorate != nil {
			decorate(req)
		}
		rr := httptest.NewRecorder()
		guardedRouter(h, &reached).ServeHTTP(rr, req)
		return rr.Code, reached
	}
	asAPIKeyUser := func(h *setupHarness, user *models.User) func(*http.Request) {
		h.keySvc.On("ValidateAPIKey", mock.Anything, "vxk_"+user.ID).
			Return(&models.APIKey{ID: "key-" + user.ID, UserID: user.ID}, nil)
		h.authSvc.On("GetUserByID", mock.Anything, user.ID).Return(user, nil)
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer vxk_"+user.ID) }
	}

	t.Run("a setup session is admitted, userless", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		cookie := h.setupCookie(t, h.bootToken(t))
		code, ctx := call(h, func(r *http.Request) { r.AddCookie(cookie) })

		require.Equal(t, http.StatusCreated, code)
		assert.True(t, isSetupSessionRequest(ctx))
		assert.Nil(t, ctx.Value(contextkeys.UserID), "a setup session names no user")
		assert.Equal(t, authTypeSetup, ctx.Value(contextkeys.AuthType))
	})

	t.Run("it stays admitted after the first provider is enabled", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		cookie := h.setupCookie(t, h.bootToken(t))
		h.enableProvider()
		code, _ := call(h, func(r *http.Request) { r.AddCookie(cookie) })
		assert.Equal(t, http.StatusCreated, code)
	})

	t.Run("an instance admin is admitted as themselves, without a setup session", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		code, ctx := call(h, asAPIKeyUser(h, &models.User{ID: "u-root", Email: setupTestRootEmail}))

		require.Equal(t, http.StatusCreated, code)
		assert.False(t, isSetupSessionRequest(ctx))
		assert.Equal(t, "u-root", ctx.Value(contextkeys.UserID))
	})

	t.Run("an instance admin who also holds a setup cookie still acts as themselves", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		cookie := h.setupCookie(t, h.bootToken(t))
		asRoot := asAPIKeyUser(h, &models.User{ID: "u-root", Email: setupTestRootEmail})
		code, ctx := call(h, func(r *http.Request) { asRoot(r); r.AddCookie(cookie) })

		require.Equal(t, http.StatusCreated, code)
		assert.False(t, isSetupSessionRequest(ctx))
		assert.Equal(t, "u-root", ctx.Value(contextkeys.UserID))
	})

	t.Run("a signed-in non-admin holding the setup cookie acts as the setup session, not as the user", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		cookie := h.setupCookie(t, h.bootToken(t))
		asUser := asAPIKeyUser(h, &models.User{ID: "u-member", Email: "member@instance.test"})
		code, ctx := call(h, func(r *http.Request) { asUser(r); r.AddCookie(cookie) })

		require.Equal(t, http.StatusCreated, code)
		assert.True(t, isSetupSessionRequest(ctx))
		assert.Nil(t, ctx.Value(contextkeys.UserID), "the member's identity never reaches the handler")
		assert.Nil(t, ctx.Value(contextkeys.APIKeyID))
	})

	t.Run("everyone else is a 404", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		cookie := h.setupCookie(t, h.bootToken(t))

		code, _ := call(h, nil)
		assert.Equal(t, http.StatusNotFound, code, "anonymous")

		code, _ = call(h, asAPIKeyUser(h, &models.User{ID: "u-member", Email: "member@instance.test"}))
		assert.Equal(t, http.StatusNotFound, code, "a signed-in non-admin")

		code, _ = call(h, func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: sesslib.SetupCookieName, Value: "tampered" + cookie.Value})
		})
		assert.Equal(t, http.StatusNotFound, code, "a tampered cookie")

		// A cookie issued for an older generation: a re-arm replaced the token.
		_, err := h.setup.ForceRearm(context.Background())
		require.NoError(t, err)
		code, _ = call(h, func(r *http.Request) { r.AddCookie(cookie) })
		assert.Equal(t, http.StatusNotFound, code, "a session from before the re-arm")
	})

	t.Run("an expired setup session is a 404", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		h.bootToken(t)
		expired, err := h.srv.sessionManager.SetupCookie(&sesslib.SetupSession{
			Generation: h.state.row.Generation, ExpiresAt: time.Now().Add(-time.Second),
		})
		require.NoError(t, err)
		code, _ := call(h, func(r *http.Request) { r.AddCookie(expired) })
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("a state read failure fails closed", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		cookie := h.setupCookie(t, h.bootToken(t))
		h.state.getErr = errors.New("db down")
		code, _ := call(h, func(r *http.Request) { r.AddCookie(cookie) })
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("no session manager: only an instance admin passes", func(t *testing.T) {
		h := newSetupHarness(t, config.RateLimitConfig{})
		cookie := h.setupCookie(t, h.bootToken(t))
		h.srv.sessionManager = nil
		code, _ := call(h, func(r *http.Request) { r.AddCookie(cookie) })
		assert.Equal(t, http.StatusNotFound, code)
	})
}

// A root admin's sign-in consumes the setup: the cookie issued before it stops
// working on its very next request, and setup_required flips to false.
func TestSetupSession_ConsumptionInvalidatesOutstandingCookies(t *testing.T) {
	h := newSetupHarness(t, config.RateLimitConfig{})
	cookie := h.setupCookie(t, h.bootToken(t))
	h.enableProvider() // the provider the admin configured and signs in through

	var reached context.Context
	guarded := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/auth/providers", nil)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		guardedRouter(h, &reached).ServeHTTP(rr, req)
		return rr.Code
	}
	require.Equal(t, http.StatusCreated, guarded())

	h.srv.consumeSetupOnRootLogin(context.Background(), &models.User{ID: "u-root", Email: setupTestRootEmail})

	assert.True(t, h.state.consumed())
	assert.Equal(t, http.StatusNotFound, guarded(), "the outstanding setup cookie is dead")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rr := httptest.NewRecorder()
	h.srv.router.ServeHTTP(rr, req)
	assert.JSONEq(t, `{"setup_required":false}`, rr.Body.String())
}
