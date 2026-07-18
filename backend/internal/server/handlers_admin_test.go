package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// adminMockContainer exposes only the AuthService and AdminService mocks needed
// by the admin middleware + handler.
type adminMockContainer struct {
	BaseMockContainer
	authService  services.AuthServiceInterface
	adminService services.AdminServiceInterface
}

func (c *adminMockContainer) AuthService() services.AuthServiceInterface   { return c.authService }
func (c *adminMockContainer) AdminService() services.AdminServiceInterface { return c.adminService }

func newAdminTestServer(cfg *config.Config, container *adminMockContainer) *Server {
	srv := New("8080", nil, "test-api-key", cfg, slog.New(slog.DiscardHandler))
	srv.container = container
	return srv
}

// TestInstanceAdminMiddleware verifies the 404-not-403 non-advertisement gate:
// only an authenticated instance admin passes through; everyone else (non-admin,
// unauthenticated, lookup failure) gets 404.
func TestInstanceAdminMiddleware(t *testing.T) {
	adminUser := &models.User{ID: "user-1", Email: "Admin@Example.com", Name: "Admin"}
	nonAdminUser := &models.User{ID: "user-2", Email: "user@example.com", Name: "User"}
	cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: config.EnvStringSlice{"admin@example.com"}}}

	tests := []struct {
		name           string
		userID         string // "" => unauthenticated (no context user)
		setupAuth      func(m *servicesmocks.MockAuthServiceInterface)
		wantStatus     int
		wantNextCalled bool
	}{
		{
			name:   "admin passes through (case-insensitive)",
			userID: "user-1",
			setupAuth: func(m *servicesmocks.MockAuthServiceInterface) {
				m.On("GetUserByID", mock.Anything, "user-1").Return(adminUser, nil)
			},
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:   "non-admin gets 404",
			userID: "user-2",
			setupAuth: func(m *servicesmocks.MockAuthServiceInterface) {
				m.On("GetUserByID", mock.Anything, "user-2").Return(nonAdminUser, nil)
			},
			wantStatus:     http.StatusNotFound,
			wantNextCalled: false,
		},
		{
			name:           "unauthenticated gets 404 without a user lookup",
			userID:         "",
			setupAuth:      func(m *servicesmocks.MockAuthServiceInterface) {},
			wantStatus:     http.StatusNotFound,
			wantNextCalled: false,
		},
		{
			name:   "user lookup failure gets 404",
			userID: "user-1",
			setupAuth: func(m *servicesmocks.MockAuthServiceInterface) {
				m.On("GetUserByID", mock.Anything, "user-1").Return(nil, assert.AnError)
			},
			wantStatus:     http.StatusNotFound,
			wantNextCalled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockAuth := servicesmocks.NewMockAuthServiceInterface(t)
			tc.setupAuth(mockAuth)
			srv := newAdminTestServer(cfg, &adminMockContainer{authService: mockAuth})

			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
			if tc.userID != "" {
				req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, tc.userID))
			}
			rr := httptest.NewRecorder()

			srv.instanceAdminMiddleware(next).ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			assert.Equal(t, tc.wantNextCalled, nextCalled)
		})
	}
}

// TestGetAdminStats_Success verifies the stats handler returns the repository
// counts plus the app version, and that the response conforms to the spec.
func TestGetAdminStats_Success(t *testing.T) {
	counts := models.InstanceCounts{Users: 42, Teams: 12, Prompts: 340, Artifacts: 128, Memories: 512}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetInstanceCounts", mock.Anything).Return(counts, nil)

	cfg := &config.Config{Server: config.ServerConfig{ServiceVersion: "1.2.3"}}
	srv := newAdminTestServer(cfg, &adminMockContainer{adminService: mockAdmin})

	// Mount the generated admin handler directly (auth middleware exercised
	// separately in TestInstanceAdminMiddleware).
	strict := admingen.NewStrictHandlerWithOptions(
		&adminStrictServer{s: srv},
		nil,
		admingen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.adminBindErrorHandler,
			ResponseErrorHandlerFunc: srv.adminResponseErrorHandler,
		},
	)
	router := chi.NewRouter()
	admingen.HandlerWithOptions(strict, admingen.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: srv.adminBindErrorHandler,
	})

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp admingen.AdminStatsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, int64(42), resp.Counts.Users)
	assert.Equal(t, int64(12), resp.Counts.Teams)
	assert.Equal(t, int64(340), resp.Counts.Prompts)
	assert.Equal(t, int64(128), resp.Counts.Artifacts)
	assert.Equal(t, int64(512), resp.Counts.Memories)
	assert.Equal(t, "1.2.3", resp.Version)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockAdmin.AssertExpectations(t)
}

// TestGetAdminStats_VersionFallback verifies the "dev" fallback when the
// configured service version is empty.
func TestGetAdminStats_VersionFallback(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetInstanceCounts", mock.Anything).Return(models.InstanceCounts{}, nil)

	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	resp, err := (&adminStrictServer{s: srv}).GetAdminStats(context.Background(), admingen.GetAdminStatsRequestObject{})
	require.NoError(t, err)
	stats, ok := resp.(admingen.GetAdminStats200JSONResponse)
	require.True(t, ok)
	assert.Equal(t, "dev", stats.Version)
}
