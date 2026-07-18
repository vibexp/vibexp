package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	apierrors "github.com/vibexp/vibexp/internal/errors"
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

// TestAdminRoutes_Unauthenticated_Returns404 exercises the FULL router wiring
// (setupAdminRoutes: optionalAuthMiddleware + instanceAdminMiddleware): an
// unauthenticated request to a mounted /api/v1/admin route must get 404, proving
// the surface is not advertised and the middleware is actually chained onto the
// route. This guards against a wiring regression the isolated middleware/handler
// tests would miss.
func TestAdminRoutes_Unauthenticated_Returns404(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{InstanceAdmins: config.EnvStringSlice{"admin@example.com"}},
	}
	srv := New("8080", nil, "test-api-key", cfg, slog.New(slog.DiscardHandler))

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
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

// TestGetAdminStats_ServiceError verifies a repository/service failure maps to a
// 500-class *apierrors.APIError returned to the strict response-error handler.
func TestGetAdminStats_ServiceError(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetInstanceCounts", mock.Anything).Return(models.InstanceCounts{}, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	resp, err := (&adminStrictServer{s: srv}).GetAdminStats(context.Background(), admingen.GetAdminStatsRequestObject{})
	require.Error(t, err)
	assert.Nil(t, resp)
	var apiErr *apierrors.APIError
	require.True(t, errors.As(err, &apiErr))
}

// TestAdminResponseErrorHandler verifies APIErrors pass through with their status
// and other errors map to 500.
func TestAdminResponseErrorHandler(t *testing.T) {
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{})

	t.Run("api error keeps its status", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
		srv.adminResponseErrorHandler(rr, req, apierrors.NewBadRequestError("bad"))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("generic error maps to 500", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
		srv.adminResponseErrorHandler(rr, req, errors.New("boom"))
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

// TestAdminBindErrorHandler verifies binding failures map to 400.
func TestAdminBindErrorHandler(t *testing.T) {
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	srv.adminBindErrorHandler(rr, req, errors.New("bad param"))
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// mountAdminStrictRouter builds the generated admin router around srv (without
// the auth middleware, which is exercised separately).
func mountAdminStrictRouter(srv *Server) *chi.Mux {
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
	return router
}

// TestListAdminUsers verifies the paginated user listing returns the service's
// page (including an empty page as `[]`) and conforms to the spec.
func TestListAdminUsers(t *testing.T) {
	idp := "oidc"
	populated := models.AdminUserList{
		Users: []models.AdminUserListItem{
			{ID: uuid.NewString(), Email: "a@example.com", Name: "A", IDPProvider: &idp, CreatedAt: time.Now(), TeamCount: 2},
			{ID: uuid.NewString(), Email: "b@example.com", Name: "B", CreatedAt: time.Now(), TeamCount: 0},
		},
		TotalCount: 2, Page: 1, PerPage: 20, TotalPages: 1,
	}
	empty := models.AdminUserList{Users: []models.AdminUserListItem{}, TotalCount: 0, Page: 1, PerPage: 20, TotalPages: 0}

	tests := []struct {
		name      string
		list      models.AdminUserList
		wantUsers int
	}{
		{"populated page", populated, 2},
		{"empty page serializes as []", empty, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On("ListUsers", mock.Anything, 0, 0).Return(tc.list, nil)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			var resp admingen.AdminUserListResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assert.Len(t, resp.Users, tc.wantUsers)
			// The required array must never be null.
			assert.Contains(t, rr.Body.String(), `"users":`)
			assert.NotContains(t, rr.Body.String(), `"users":null`)

			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestGetAdminUser_Found verifies a user detail with memberships conforms to spec.
func TestGetAdminUser_Found(t *testing.T) {
	id := uuid.NewString()
	teamID := uuid.NewString()
	idp := "google"
	detail := &models.AdminUserDetail{
		ID: id, Email: "admin@example.com", Name: "Admin", IDPProvider: &idp, CreatedAt: time.Now(),
		Memberships: []models.AdminTeamMembership{{TeamID: teamID, TeamName: "Acme", Role: "owner"}},
	}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(detail, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminUserDetail
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, id, resp.Id.String())
	require.Len(t, resp.Memberships, 1)
	assert.Equal(t, "owner", resp.Memberships[0].Role)

	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestListAdminUsers_ServiceError maps a service failure to 500.
func TestListAdminUsers_ServiceError(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListUsers", mock.Anything, 0, 0).Return(models.AdminUserList{}, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestListAdminUsers_ConversionError maps a non-UUID id from the store to 500.
func TestListAdminUsers_ConversionError(t *testing.T) {
	bad := models.AdminUserList{
		Users:   []models.AdminUserListItem{{ID: "not-a-uuid", Email: "a@example.com", Name: "A"}},
		Page:    1,
		PerPage: 20,
	}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListUsers", mock.Anything, 0, 0).Return(bad, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestGetAdminUser_ServiceError maps a service failure to 500.
func TestGetAdminUser_ServiceError(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(nil, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestGetAdminUser_ConversionError maps a non-UUID stored id to 500.
func TestGetAdminUser_ConversionError(t *testing.T) {
	id := uuid.NewString()
	// Valid user id but a membership with a non-UUID team id → conversion fails.
	bad := &models.AdminUserDetail{
		ID: id, Email: "a@example.com", Name: "A",
		Memberships: []models.AdminTeamMembership{{TeamID: "not-a-uuid", TeamName: "X", Role: "member"}},
	}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(bad, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestGetAdminUser_NotFound verifies an unknown id 404s (service returns nil).
func TestGetAdminUser_NotFound(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(nil, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestGetAdminUser_InvalidUUID verifies a non-UUID id is rejected (400) by the
// generated binding layer before reaching the service.
func TestGetAdminUser_InvalidUUID(t *testing.T) {
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminService: servicesmocks.NewMockAdminServiceInterface(t),
	})

	req := httptest.NewRequest("GET", "/api/v1/admin/users/not-a-uuid", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
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
