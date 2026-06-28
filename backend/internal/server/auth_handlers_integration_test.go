package server

import (
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
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	ometrics "github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// MockAuthContainer implements Container interface for auth handler tests
type MockAuthContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	authService        *svcmocks.MockAuthServiceInterface
	activityService    *MockActivityServiceForAuthHandlers
	environmentService *services.EnvironmentService
}

func (m *MockAuthContainer) AuthService() services.AuthServiceInterface {
	return m.authService
}

func (m *MockAuthContainer) ActivityService() activities.ActivityService {
	return m.activityService
}

func (m *MockAuthContainer) EnvironmentService() *services.EnvironmentService {
	return m.environmentService
}

// MockActivityServiceForAuthHandlers is a mock implementation for ActivityService
type MockActivityServiceForAuthHandlers struct {
	mock.Mock
}

func (m *MockActivityServiceForAuthHandlers) DeleteActivity(ctx context.Context, activityID string) error {
	args := m.Called(ctx, activityID)
	return args.Error(0)
}

func (m *MockActivityServiceForAuthHandlers) GetActivityTypes() []string {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]string)
}

func (m *MockActivityServiceForAuthHandlers) GetEntityTypes() []string {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]string)
}

func (m *MockActivityServiceForAuthHandlers) GetAllTypes() *activities.ActivityTypesResponse {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*activities.ActivityTypesResponse)
}

func (m *MockActivityServiceForAuthHandlers) GetActivities(
	ctx context.Context, filters activities.ActivityFilters,
) (*activities.ActivityListResponse, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.ActivityListResponse), args.Error(1)
}

func (m *MockActivityServiceForAuthHandlers) GetActivityStats(
	ctx context.Context, userID string,
) (*activities.ActivityStatsResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.ActivityStatsResponse), args.Error(1)
}

func (m *MockActivityServiceForAuthHandlers) GetActivityByID(
	ctx context.Context, userID string, activityID string,
) (*activities.Activity, error) {
	args := m.Called(ctx, userID, activityID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.Activity), args.Error(1)
}

func (m *MockActivityServiceForAuthHandlers) RecordActivity(
	ctx context.Context, userID string, req activities.CreateActivityRequest,
) (*activities.Activity, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.Activity), args.Error(1)
}

func (m *MockActivityServiceForAuthHandlers) RecordAuthActivity(
	ctx context.Context, userID string, activityType string, sessionID *string,
	metadata map[string]interface{}, sourceIP *string, userAgent *string,
) error {
	args := m.Called(ctx, userID, activityType, sessionID, metadata, sourceIP, userAgent)
	return args.Error(0)
}

func (m *MockActivityServiceForAuthHandlers) RecordResourceActivity(
	ctx context.Context, userID string, activityType string, entityType string,
	entityID *string, description string, metadata map[string]interface{},
) error {
	args := m.Called(ctx, userID, activityType, entityType, entityID, description, metadata)
	return args.Error(0)
}

func (m *MockActivityServiceForAuthHandlers) RecordClaudeCodeActivity(
	ctx context.Context, userID string, sessionID string, toolName *string,
	hookEventName string, metadata map[string]interface{},
) error {
	args := m.Called(ctx, userID, sessionID, toolName, hookEventName, metadata)
	return args.Error(0)
}

func (m *MockActivityServiceForAuthHandlers) RunRetentionJob(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// testCookiePassword is a 64-char hex string → 32-byte AES-256-GCM key.
// #nosec G101 - Test credential, not a real secret
const testCookiePassword = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func newMockAuthContainer(t *testing.T) *MockAuthContainer {
	cfg := &config.Config{
		FrontendBaseURL: "http://localhost:5173",
		DevLoginEnabled: true,
	}
	return &MockAuthContainer{
		authService:        svcmocks.NewMockAuthServiceInterface(t),
		activityService:    &MockActivityServiceForAuthHandlers{},
		environmentService: services.NewEnvironmentService(cfg),
	}
}

func createTestAuthServer(container *MockAuthContainer) *Server {
	// Cookie password must be 32 hex-encoded bytes (64 chars)
	cfg := &config.Config{
		FrontendBaseURL:      "http://localhost:5173",
		SessionEncryptionKey: testCookiePassword,
	}
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()

	// Build session manager for test server
	sessMgr, err := sesslib.NewManager(testCookiePassword, true /* isLocal */)
	if err != nil {
		panic("test session manager: " + err.Error())
	}

	srv := &Server{
		port:           "8080",
		container:      container,
		logger:         logger,
		config:         cfg,
		router:         r,
		sessionManager: sessMgr,
	}

	// Register identity-provider auth routes
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Get("/login", srv.handleLogin)
		r.Get("/callback", srv.handleCallback)
		r.Post("/logout", srv.handleLogout)
		r.Post("/dev/login", srv.handleDevLogin)
	})

	r.Group(func(r chi.Router) {
		r.Use(srv.flexibleAuthMiddleware)
		r.Get("/api/v1/auth/me", srv.handleGetMe)
	})

	return srv
}

// TestHandleLogin_Success tests successful login URL generation
func TestHandleLogin_Success(t *testing.T) {
	mockContainer := newMockAuthContainer(t)

	expectedLoginURL := "https://idp.example.com/authorize?state=test-state&client_id=test"

	// A single enabled provider lets the no-param login default to it.
	mockContainer.authService.On("EnabledProviders").Return([]string{"oidc"})
	mockContainer.authService.On("GetLoginURL", mock.MatchedBy(func(state string) bool {
		return state != ""
	}), "oidc").Return(expectedLoginURL)

	srv := createTestAuthServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response LoginResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, expectedLoginURL, response.URL)

	// Verify state cookie was set
	var stateCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == stateCookieName {
			stateCookie = c
			break
		}
	}
	assert.NotNil(t, stateCookie, "State cookie should be set")
	assert.True(t, stateCookie.HttpOnly)

	mockContainer.authService.AssertExpectations(t)
}

// TestHandleCallback_Success tests successful callback with session cookie
func TestHandleCallback_Success(t *testing.T) {
	mockContainer, testMetrics, reader := setupAuthTestWithMetrics(t)

	expectedUser := &models.User{
		ID:          "user-123",
		Email:       "test@example.com",
		Name:        "Test User",
		IDPProvider: func() *string { s := "oidc"; return &s }(),
		IDPSubject:  func() *string { s := "oidc-sub-123"; return &s }(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	expectedTokens := &idp.Tokens{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	mockContainer.authService.On("HandleCallback", mock.Anything, "test-auth-code", "oidc").
		Return(expectedUser, expectedTokens, false, nil)
	mockAuthActivityOIDC(mockContainer, expectedUser.ID)

	srv := createTestAuthServer(mockContainer)
	srv.metrics = testMetrics

	// Build a request with valid state cookie
	stateCookie := buildStateCookie(t, srv, "test-state", "oidc")
	req := httptest.NewRequest("GET", "/api/v1/auth/callback?code=test-auth-code&state=test-state", nil)
	req.AddCookie(stateCookie)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Should redirect to frontend
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))

	// Should have set a session cookie
	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			sessionCookie = c
			break
		}
	}
	assert.NotNil(t, sessionCookie, "Session cookie should be set")
	assert.True(t, sessionCookie.HttpOnly)

	mockContainer.authService.AssertExpectations(t)
	mockContainer.activityService.AssertExpectations(t)

	rm := collectMetricsForAuthHandler(t, reader)
	assertCounterValueForAuthHandler(t, rm, "vx_user_login_successful", 1)
}

// TestHandleCallback_MissingCode tests callback without authorization code
func TestHandleCallback_MissingCode(t *testing.T) {
	mockContainer := newMockAuthContainer(t)
	srv := createTestAuthServer(mockContainer)

	req := httptest.NewRequest("GET", "/api/v1/auth/callback?state=test-state", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestHandleCallback_InvalidState tests callback with invalid state
func TestHandleCallback_InvalidState(t *testing.T) {
	mockContainer := newMockAuthContainer(t)
	srv := createTestAuthServer(mockContainer)

	// No state cookie set
	req := httptest.NewRequest("GET", "/api/v1/auth/callback?code=test-code&state=tampered-state", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestHandleCallback_OAuthFailure tests callback when OAuth exchange fails
func TestHandleCallback_OAuthFailure(t *testing.T) {
	mockContainer := newMockAuthContainer(t)

	mockContainer.authService.On("HandleCallback", mock.Anything, "invalid-code", "oidc").
		Return((*models.User)(nil), (*idp.Tokens)(nil), false, errors.New("exchange failed"))

	srv := createTestAuthServer(mockContainer)

	stateCookie := buildStateCookie(t, srv, "test-state", "oidc")
	req := httptest.NewRequest("GET", "/api/v1/auth/callback?code=invalid-code&state=test-state", nil)
	req.AddCookie(stateCookie)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	mockContainer.authService.AssertExpectations(t)
}

// TestHandleLogout_ClearsSessionCookie tests that logout clears the session cookie
func TestHandleLogout_ClearsSessionCookie(t *testing.T) {
	mockContainer := newMockAuthContainer(t)
	srv := createTestAuthServer(mockContainer)

	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response LogoutResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "logged out", response.Message)

	// Verify session cookie was cleared (MaxAge = -1 or 0)
	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			sessionCookie = c
			break
		}
	}
	assert.NotNil(t, sessionCookie, "Set-Cookie for session should be present after logout")
	assert.LessOrEqual(t, sessionCookie.MaxAge, 0, "Session cookie should have non-positive MaxAge after logout")
}

// TestHandleGetMe_Success tests successful retrieval of authenticated user info
func TestHandleGetMe_Success(t *testing.T) {
	mockContainer := newMockAuthContainer(t)

	expectedUser := &models.User{
		ID:        "user-789",
		Email:     "getme@example.com",
		Name:      "GetMe User",
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now(),
	}

	mockContainer.authService.On("GetUserByID", mock.Anything, "user-789").
		Return(expectedUser, nil)

	srv := createTestAuthServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-789"))
	w := httptest.NewRecorder()

	// Call handler directly to bypass middleware
	srv.handleGetMe(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.User
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedUser.ID, response.ID)
	assert.Equal(t, expectedUser.Email, response.Email)
	assert.Equal(t, expectedUser.Name, response.Name)

	mockContainer.authService.AssertExpectations(t)
}

// TestHandleGetMe_UserNotFound tests get me when user doesn't exist
func TestHandleGetMe_UserNotFound(t *testing.T) {
	mockContainer := newMockAuthContainer(t)

	mockContainer.authService.On("GetUserByID", mock.Anything, "non-existent-user").
		Return((*models.User)(nil), errors.New("user not found"))

	srv := createTestAuthServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "non-existent-user"))
	w := httptest.NewRecorder()

	srv.handleGetMe(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "User not found")

	mockContainer.authService.AssertExpectations(t)
}

// TestHandleDevLogin_Success tests successful dev login sets session cookie
func TestHandleDevLogin_Success(t *testing.T) {
	mockContainer := newMockAuthContainer(t)

	expectedUser := &models.User{
		ID:    "dev-user-123",
		Email: "dev@example.com",
		Name:  "Dev User",
	}

	mockContainer.authService.On("HandleDevLogin", mock.Anything, "dev@example.com", "Dev User").
		Return(expectedUser, nil)
	mockContainer.activityService.On(
		"RecordAuthActivity",
		mock.Anything,
		"dev-user-123",
		activities.ActivityTypeAuthLogin,
		(*string)(nil),
		mock.MatchedBy(func(metadata map[string]interface{}) bool {
			return metadata["provider"] == "dev"
		}),
		mock.AnythingOfType("*string"),
		mock.AnythingOfType("*string"),
	).Return(nil)

	srv := createTestAuthServer(mockContainer)

	body := strings.NewReader(`{"email":"dev@example.com","name":"Dev User"}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/dev/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Should have set session cookie
	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			sessionCookie = c
			break
		}
	}
	assert.NotNil(t, sessionCookie, "Session cookie should be set after dev login")
	assert.True(t, sessionCookie.HttpOnly)

	mockContainer.authService.AssertExpectations(t)
	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleDevLogin_NotAvailableInProduction tests dev login is blocked in production
func TestHandleDevLogin_NotAvailableInProduction(t *testing.T) {
	cfg := &config.Config{
		FrontendBaseURL:      "https://app.vibexp.io",
		SessionEncryptionKey: testCookiePassword,
	}
	prodEnvSvc := services.NewEnvironmentService(cfg)
	mockContainer := &MockAuthContainer{
		authService:        svcmocks.NewMockAuthServiceInterface(t),
		activityService:    &MockActivityServiceForAuthHandlers{},
		environmentService: prodEnvSvc,
	}

	srv := createTestAuthServer(mockContainer)
	srv.config = cfg
	srv.container = mockContainer

	body := strings.NewReader(`{"email":"dev@example.com"}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/dev/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Should return 404 (endpoint not found in production)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestFlexibleAuthMiddleware_WithSessionCookie tests cookie-based session auth
func TestFlexibleAuthMiddleware_WithSessionCookie(t *testing.T) {
	mockContainer := newMockAuthContainer(t)

	expectedUser := &models.User{
		ID:    "cookie-user-123",
		Email: "cookie@example.com",
		Name:  "Cookie User",
	}

	mockContainer.authService.On("GetUserByID", mock.Anything, "cookie-user-123").
		Return(expectedUser, nil)

	srv := createTestAuthServer(mockContainer)

	// Create a valid session cookie
	mgr, err := sesslib.NewManager(testCookiePassword, true)
	require.NoError(t, err)

	sess := &sesslib.Session{
		AccessToken:  "test-access-token",
		RefreshToken: "",
		ExpiresAt:    time.Now().Add(time.Hour),
		IDPSubject:   "oidc-sub-123",
		UserID:       "cookie-user-123",
	}
	rw := httptest.NewRecorder()
	require.NoError(t, mgr.Write(rw, sess))

	var sessionCookie *http.Cookie
	for _, c := range rw.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie)

	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.authService.AssertExpectations(t)
}

// TestFlexibleAuthMiddleware_WithAPIKey tests API key auth still works
func TestFlexibleAuthMiddleware_NoAuth_Returns401(t *testing.T) {
	mockContainer := newMockAuthContainer(t)
	srv := createTestAuthServer(mockContainer)

	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// Helper: builds a signed state cookie (state + provider) for the test server
func buildStateCookie(t *testing.T, srv *Server, state, provider string) *http.Cookie {
	t.Helper()
	signedState := srv.signState(state, provider)
	return &http.Cookie{
		Name:  stateCookieName,
		Value: signedState,
	}
}

// Helper functions for metrics verification

func setupAuthTestWithMetrics(t *testing.T) (*MockAuthContainer, *ometrics.Metrics, sdkmetric.Reader) {
	t.Helper()
	mockContainer := newMockAuthContainer(t)
	reader := sdkmetric.NewManualReader()
	testMetrics := setupTestMetricsForAuthHandler(t, reader)
	return mockContainer, testMetrics, reader
}

func mockAuthActivityOIDC(mockContainer *MockAuthContainer, userID string) {
	mockContainer.activityService.On(
		"RecordAuthActivity",
		mock.Anything,
		userID,
		activities.ActivityTypeAuthLogin,
		mock.AnythingOfType("*string"),
		mock.MatchedBy(func(metadata map[string]interface{}) bool {
			return metadata["provider"] == "oidc"
		}),
		mock.AnythingOfType("*string"),
		mock.AnythingOfType("*string"),
	).Return(nil)
}

func setupTestMetricsForAuthHandler(t *testing.T, reader sdkmetric.Reader) *ometrics.Metrics {
	t.Helper()
	res, err := resource.New(context.Background())
	require.NoError(t, err)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	meter := meterProvider.Meter("test")
	m := &ometrics.Metrics{}

	var err2 error
	m.UserCreated, err2 = meter.Int64Counter("vx_user_created")
	require.NoError(t, err2)
	m.UserLoginSuccessful, err2 = meter.Int64Counter("vx_user_login_successful")
	require.NoError(t, err2)
	m.UserLoginFailed, err2 = meter.Int64Counter("vx_user_login_failed")
	require.NoError(t, err2)

	return m
}

func collectMetricsForAuthHandler(t *testing.T, reader sdkmetric.Reader) *metricdata.ResourceMetrics {
	t.Helper()
	rm := &metricdata.ResourceMetrics{}
	err := reader.Collect(context.Background(), rm)
	require.NoError(t, err)
	return rm
}

func assertCounterValueForAuthHandler(t *testing.T, rm *metricdata.ResourceMetrics,
	metricName string, expectedValue int64) {
	t.Helper()
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == metricName {
				found = true
				sum, ok := m.Data.(metricdata.Sum[int64])
				require.True(t, ok, "metric %s should be Sum[int64]", metricName)
				require.Len(t, sum.DataPoints, 1, "metric %s should have exactly 1 data point", metricName)
				assert.Equal(t, expectedValue, sum.DataPoints[0].Value,
					"metric %s should have value %d", metricName, expectedValue)
			}
		}
	}
	assert.True(t, found, "metric %s not found - handler did not record it!", metricName)
}

// enabledForLoginTests is the multi-provider set used by the login
// query-param tests below.
var enabledForLoginTests = []string{"github", "google", "oidc"}

// TestHandleLogin_ProviderQueryParam_Allowed asserts that a requested provider
// in the enabled set is forwarded to GetLoginURL by its canonical name.
func TestHandleLogin_ProviderQueryParam_Allowed(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		provider string
	}{
		{"github selected", "/api/v1/auth/login?provider=github", "github"},
		{"google selected", "/api/v1/auth/login?provider=google", "google"},
		{"oidc selected", "/api/v1/auth/login?provider=oidc", "oidc"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockAuthContainer(t)
			expectedLoginURL := "https://idp.example.com/authorize?state=x"
			mockContainer.authService.On("EnabledProviders").Return(enabledForLoginTests)
			mockContainer.authService.On(
				"GetLoginURL",
				mock.MatchedBy(func(state string) bool { return state != "" }),
				tt.provider,
			).Return(expectedLoginURL)

			srv := createTestAuthServer(mockContainer)
			req := httptest.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			var response LoginResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, expectedLoginURL, response.URL)
			mockContainer.authService.AssertExpectations(t)
		})
	}
}

// TestHandleLogin_SingleProviderDefaults asserts that with exactly one enabled
// provider and no ?provider= hint, that provider is used.
func TestHandleLogin_SingleProviderDefaults(t *testing.T) {
	mockContainer := newMockAuthContainer(t)
	mockContainer.authService.On("EnabledProviders").Return([]string{"google"})
	mockContainer.authService.On(
		"GetLoginURL",
		mock.MatchedBy(func(state string) bool { return state != "" }),
		"google",
	).Return("https://idp.example.com/authorize?state=x")

	srv := createTestAuthServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.authService.AssertExpectations(t)
}

// TestHandleLogin_ProviderQueryParam_Rejected asserts that an unknown provider,
// or a missing/empty hint when multiple providers are enabled, is rejected with
// 400 and never reaches GetLoginURL (no silent default).
func TestHandleLogin_ProviderQueryParam_Rejected(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"unknown provider rejected", "/api/v1/auth/login?provider=okta-magic"},
		{"malicious value rejected", "/api/v1/auth/login?provider=DROP_TABLE"},
		{"missing hint with multiple providers", "/api/v1/auth/login"},
		{"empty hint with multiple providers", "/api/v1/auth/login?provider="},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockAuthContainer(t)
			mockContainer.authService.On("EnabledProviders").Return(enabledForLoginTests)

			srv := createTestAuthServer(mockContainer)
			req := httptest.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			mockContainer.authService.AssertNotCalled(t, "GetLoginURL", mock.Anything, mock.Anything)
		})
	}
}

// TestHandleLogin_NoProvidersEnabled asserts that with no providers enabled the
// login endpoint returns 503 (web login unavailable).
func TestHandleLogin_NoProvidersEnabled(t *testing.T) {
	mockContainer := newMockAuthContainer(t)
	mockContainer.authService.On("EnabledProviders").Return([]string{})

	srv := createTestAuthServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
