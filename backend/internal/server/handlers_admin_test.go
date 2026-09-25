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
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// adminMockContainer exposes only the AuthService and AdminService mocks needed
// by the admin middleware + handler.
type adminMockContainer struct {
	BaseMockContainer
	authService     services.AuthServiceInterface
	adminService    services.AdminServiceInterface
	activityService activities.ActivityService
	prefsService    services.UserPreferencesServiceInterface

	// Team configuration reads (#1140). Nil unless a suite installs them.
	teamRepo              repositories.TeamRepository
	userRepo              repositories.UserRepository
	modelProviderRepo     repositories.ModelProviderRepository
	settingsAuditRepo     repositories.TeamSettingsAuditRepository
	searchSettingsService services.TeamSearchSettingsServiceInterface
	aiSummaryService      services.TeamAISummarySettingsServiceInterface
	freshnessService      services.FreshnessServiceInterface
	typeService           services.TypeServiceInterface

	// Credential-bearing team configuration reads (#1141). Nil unless a suite
	// installs them.
	modelProviderService     services.ModelProviderServiceInterface
	embeddingProviderService services.EmbeddingProviderServiceInterface
	embeddingStatusService   services.EmbeddingCoverageGetter
	emailProviderService     services.TeamEmailProviderServiceInterface
	githubAppConfigService   services.GitHubAppConfigServiceInterface
	githubAppService         services.GitHubAppServiceInterface
}

func (c *adminMockContainer) TeamRepository() repositories.TeamRepository { return c.teamRepo }

// UserRepository falls back to the base's always-active default, which every
// authenticated request's suspension check relies on.
func (c *adminMockContainer) UserRepository() repositories.UserRepository {
	if c.userRepo == nil {
		return c.BaseMockContainer.UserRepository()
	}
	return c.userRepo
}
func (c *adminMockContainer) ModelProviderRepository() repositories.ModelProviderRepository {
	return c.modelProviderRepo
}
func (c *adminMockContainer) TeamSettingsAuditRepository() repositories.TeamSettingsAuditRepository {
	return c.settingsAuditRepo
}
func (c *adminMockContainer) TeamSearchSettingsService() services.TeamSearchSettingsServiceInterface {
	return c.searchSettingsService
}
func (c *adminMockContainer) TeamAISummarySettingsService() services.TeamAISummarySettingsServiceInterface {
	return c.aiSummaryService
}
func (c *adminMockContainer) FreshnessService() services.FreshnessServiceInterface {
	return c.freshnessService
}
func (c *adminMockContainer) TypeService() services.TypeServiceInterface { return c.typeService }
func (c *adminMockContainer) ModelProviderService() services.ModelProviderServiceInterface {
	return c.modelProviderService
}
func (c *adminMockContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return c.embeddingProviderService
}
func (c *adminMockContainer) EmbeddingStatusService() services.EmbeddingCoverageGetter {
	return c.embeddingStatusService
}
func (c *adminMockContainer) TeamEmailProviderService() services.TeamEmailProviderServiceInterface {
	return c.emailProviderService
}
func (c *adminMockContainer) GitHubAppConfigService() services.GitHubAppConfigServiceInterface {
	return c.githubAppConfigService
}
func (c *adminMockContainer) GitHubAppService() services.GitHubAppServiceInterface {
	return c.githubAppService
}

func (c *adminMockContainer) AuthService() services.AuthServiceInterface   { return c.authService }
func (c *adminMockContainer) AdminService() services.AdminServiceInterface { return c.adminService }
func (c *adminMockContainer) UserPreferencesService() services.UserPreferencesServiceInterface {
	return c.prefsService
}

// ActivityService backs the audit rows the suspension handlers write (#454). A
// nil service is a supported no-op, so tests that do not care simply omit it.
func (c *adminMockContainer) ActivityService() activities.ActivityService {
	if c.activityService == nil {
		return nil
	}
	return c.activityService
}

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

	// Every mounted admin route must be behind the middleware — not just /stats.
	paths := []string{
		"/api/v1/admin/stats",
		"/api/v1/admin/users",
		"/api/v1/admin/users/" + uuid.NewString(),
		"/api/v1/admin/teams",
		"/api/v1/admin/teams/" + uuid.NewString(),
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusNotFound, rr.Code)
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
	// Mirror setupAdminRoutes' middleware chain (minus auth, which these tests
	// bypass deliberately) so the body guard is exercised here too — otherwise
	// the unknown-field rejection would be untested against the real routing.
	router.Use(srv.rejectUnknownAdminBodyFields)
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
			{
				ID: uuid.NewString(), Email: "a@example.com", Name: "A", IDPProvider: &idp,
				Status: models.UserStatusActive, CreatedAt: time.Now(), TeamCount: 2, ProjectCount: 3,
				ResourceCounts: models.AdminResourceCounts{
					Prompts: 1, Memories: 2, Artifacts: 3, Blueprints: 4, Agents: 5,
					Feeds: 6, FeedItems: 7, Comments: 8, Attachments: 9, Total: 45,
				},
				LastResourceCreatedAt: &adminFilterQueryTime,
			},
			{
				ID: uuid.NewString(), Email: "b@example.com", Name: "B",
				Status: models.UserStatusSuspended, CreatedAt: time.Now(), TeamCount: 0,
			},
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
			mockAdmin.On("ListUsers", mock.Anything, repositories.AdminUserFilters{}).Return(tc.list, nil)
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
			// #454's status must actually be mapped, both values.
			if tc.wantUsers > 0 {
				assert.Equal(t, admingen.AdminUserListItemStatus("active"), resp.Users[0].Status)
				assert.Equal(t, admingen.AdminUserListItemStatus("suspended"), resp.Users[1].Status)
				// #1133's aggregates are mapped; a user with no resources has no
				// last_resource_created_at and all-zero counts.
				assert.Equal(t, int64(3), resp.Users[0].ProjectCount)
				assert.Equal(t, admingen.AdminResourceCounts{
					Prompts: 1, Memories: 2, Artifacts: 3, Blueprints: 4, Agents: 5,
					Feeds: 6, FeedItems: 7, Comments: 8, Attachments: 9, Total: 45,
				}, resp.Users[0].ResourceCounts)
				require.NotNil(t, resp.Users[0].LastResourceCreatedAt)
				assert.True(t, adminFilterQueryTime.Equal(*resp.Users[0].LastResourceCreatedAt))
				assert.Nil(t, resp.Users[1].LastResourceCreatedAt)
				assert.Equal(t, admingen.AdminResourceCounts{}, resp.Users[1].ResourceCounts)
			}

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
		ID: id, Email: "admin@example.com", Name: "Admin", IDPProvider: &idp,
		Status: models.UserStatusSuspended, CreatedAt: time.Now(),
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
	assert.Equal(t, admingen.AdminUserDetailStatus("suspended"), resp.Status)
	require.Len(t, resp.Memberships, 1)
	assert.Equal(t, "owner", resp.Memberships[0].Role)

	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestListAdminUsers_ServiceError maps a service failure to 500.
func TestListAdminUsers_ServiceError(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListUsers", mock.Anything, repositories.AdminUserFilters{}).Return(models.AdminUserList{}, errors.New("db down"))
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
	mockAdmin.On("ListUsers", mock.Anything, repositories.AdminUserFilters{}).Return(bad, nil)
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

// TestListAdminTeams verifies the paginated team listing returns the service's
// page (empty page as []) and conforms to the spec.
func TestListAdminTeams(t *testing.T) {
	owner := models.AdminTeamOwner{ID: uuid.NewString(), Email: "owner@example.com", Name: "Owner"}
	populated := models.AdminTeamList{
		Teams: []models.AdminTeamListItem{
			{
				ID: uuid.NewString(), Name: "Acme", Slug: "acme", IsPersonal: false,
				Owner: owner, MemberCount: 3, OwnerCount: 1, AdminCount: 2, ProjectCount: 4,
				ResourceCounts: models.AdminResourceCounts{
					Prompts: 1, Memories: 2, Artifacts: 3, Blueprints: 4, Agents: 5,
					Feeds: 6, FeedItems: 7, Comments: 8, Attachments: 9, Total: 45,
				},
				Configuration: models.AdminTeamConfiguration{
					EmbeddingConfigured: true, AISummaryEnabled: true, GitHubConfigured: true, FreshnessEnabled: true,
				},
				CreatedAt: time.Now(),
			},
			{
				ID: uuid.NewString(), Name: "Beta", Slug: "beta", IsPersonal: true,
				Owner: owner, MemberCount: 0, CreatedAt: time.Now(),
			},
		},
		TotalCount: 2, Page: 1, PerPage: 20, TotalPages: 1,
	}
	empty := models.AdminTeamList{Teams: []models.AdminTeamListItem{}, Page: 1, PerPage: 20}

	tests := []struct {
		name      string
		list      models.AdminTeamList
		wantTeams int
	}{
		{"populated page", populated, 2},
		{"empty page serializes as []", empty, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On("ListTeams", mock.Anything, repositories.AdminTeamFilters{}).Return(tc.list, nil)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", "/api/v1/admin/teams", nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			var resp admingen.AdminTeamListResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assert.Len(t, resp.Teams, tc.wantTeams)
			assert.NotContains(t, rr.Body.String(), `"teams":null`)
			// #452's additive required fields. Spec conformance alone cannot catch a
			// dropped mapping — the zero values ("" / false) are valid string/boolean —
			// so assert the values actually round-trip.
			if tc.wantTeams > 0 {
				assert.Equal(t, "acme", resp.Teams[0].Slug)
				assert.False(t, resp.Teams[0].IsPersonal)
				assert.Equal(t, "beta", resp.Teams[1].Slug)
				assert.True(t, resp.Teams[1].IsPersonal)
				// #1138's counts and configuration flags, for the same reason.
				assert.Equal(t, int64(1), resp.Teams[0].OwnerCount)
				assert.Equal(t, int64(2), resp.Teams[0].AdminCount)
				assert.Equal(t, int64(4), resp.Teams[0].ProjectCount)
				assert.Equal(t, int64(9), resp.Teams[0].ResourceCounts.Attachments)
				assert.Equal(t, int64(45), resp.Teams[0].ResourceCounts.Total)
				assert.Equal(t, admingen.AdminTeamConfiguration{
					EmbeddingConfigured: true, AiSummaryEnabled: true, GithubConfigured: true, FreshnessEnabled: true,
				}, resp.Teams[0].Configuration)
				assert.Equal(t, admingen.AdminTeamConfiguration{}, resp.Teams[1].Configuration)
				assert.Contains(t, rr.Body.String(), `"search_settings_customized":false`)
			}
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// adminFilterQueryTime is the instant used by the query-param mapping tests, in
// the RFC 3339 form the generated date-time binder accepts.
var adminFilterQueryTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// TestListAdminUsers_MapsQueryParams asserts every documented user query param
// reaches the service as the matching filter field, and that a filtered call
// still conforms to the spec.
func TestListAdminUsers_MapsQueryParams(t *testing.T) {
	search := "alice"
	idp := "google"
	want := repositories.AdminUserFilters{
		Search:                  &search,
		IDPProvider:             &idp,
		CreatedFrom:             &adminFilterQueryTime,
		CreatedTo:               &adminFilterQueryTime,
		TeamCount:               adminCountRange(0, 1),
		ProjectCount:            adminCountRange(1, 2),
		PromptCount:             adminCountRange(2, 3),
		MemoryCount:             adminCountRange(3, 4),
		ArtifactCount:           adminCountRange(4, 5),
		BlueprintCount:          adminCountRange(5, 6),
		AgentCount:              adminCountRange(6, 7),
		FeedCount:               adminCountRange(7, 8),
		FeedItemCount:           adminCountRange(8, 9),
		CommentCount:            adminCountRange(9, 10),
		AttachmentCount:         adminCountRange(10, 11),
		TotalResourceCount:      adminCountRange(11, 12),
		LastResourceCreatedFrom: &adminFilterQueryTime,
		LastResourceCreatedTo:   &adminFilterQueryTime,
		SortBy:                  "total_resource_count",
		SortOrder:               "asc",
		Page:                    2,
		Limit:                   50,
	}
	list := models.AdminUserList{
		Users: []models.AdminUserListItem{}, TotalCount: 0, Page: 2, PerPage: 50, TotalPages: 0,
	}

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListUsers", mock.Anything, want).Return(list, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	stamp := adminFilterQueryTime.Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/v1/admin/users?page=2&limit=50&search=alice"+
		"&idp_provider=google&created_from="+stamp+"&created_to="+stamp+
		"&team_count_min=0&team_count_max=1&project_count_min=1&project_count_max=2"+
		"&prompt_count_min=2&prompt_count_max=3&memory_count_min=3&memory_count_max=4"+
		"&artifact_count_min=4&artifact_count_max=5&blueprint_count_min=5&blueprint_count_max=6"+
		"&agent_count_min=6&agent_count_max=7&feed_count_min=7&feed_count_max=8"+
		"&feed_item_count_min=8&feed_item_count_max=9&comment_count_min=9&comment_count_max=10"+
		"&attachment_count_min=10&attachment_count_max=11"+
		"&total_resource_count_min=11&total_resource_count_max=12"+
		"&last_resource_created_from="+stamp+"&last_resource_created_to="+stamp+
		"&sort_by=total_resource_count&sort_order=asc", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// adminCountRange builds a closed repositories.AdminCountRange.
func adminCountRange(lower, upper int64) repositories.AdminCountRange {
	return repositories.AdminCountRange{Min: &lower, Max: &upper}
}

// TestListAdminUsers_InvalidAggregateFiltersReturn400 pins #1133's validation:
// the generated binder enforces neither `minimum` nor any cross-field rule, so a
// negative bound, an inverted min/max or from/to pair, or a malformed date must
// be rejected before the service is called.
func TestListAdminUsers_InvalidAggregateFiltersReturn400(t *testing.T) {
	later := adminFilterQueryTime.Add(time.Hour).Format(time.RFC3339)
	stamp := adminFilterQueryTime.Format(time.RFC3339)

	tests := []struct {
		name  string
		query string
	}{
		{"negative min", "team_count_min=-1"},
		{"negative max", "prompt_count_max=-3"},
		{"inverted count range", "memory_count_min=5&memory_count_max=4"},
		{"inverted total range", "total_resource_count_min=10&total_resource_count_max=1"},
		{"inverted attachment range", "attachment_count_min=2&attachment_count_max=1"},
		{"non-integer bound", "feed_count_min=many"},
		{"inverted last_resource_created range", "last_resource_created_from=" + later + "&last_resource_created_to=" + stamp},
		{"malformed last_resource_created date", "last_resource_created_from=last-week"},
		{"unknown sort_by", "sort_by=resource_title"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No service call is expected: the request must be rejected first.
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", "/api/v1/admin/users?"+tc.query, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Header().Get("Content-Type"), "application/problem+json")
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestListAdminUsers_EqualBoundsAccepted confirms min == max (and from == to) is
// a valid, inclusive range rather than an inverted one.
func TestListAdminUsers_EqualBoundsAccepted(t *testing.T) {
	want := repositories.AdminUserFilters{
		PromptCount:             adminCountRange(3, 3),
		LastResourceCreatedFrom: &adminFilterQueryTime,
		LastResourceCreatedTo:   &adminFilterQueryTime,
	}
	list := models.AdminUserList{Users: []models.AdminUserListItem{}, Page: 1, PerPage: 20}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListUsers", mock.Anything, want).Return(list, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	stamp := adminFilterQueryTime.Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/v1/admin/users?prompt_count_min=3&prompt_count_max=3"+
		"&last_resource_created_from="+stamp+"&last_resource_created_to="+stamp, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestListAdminTeams_MapsQueryParams is the team mirror of the user mapping test,
// covering every #1138 range, the owner-email match and each tri-state.
func TestListAdminTeams_MapsQueryParams(t *testing.T) {
	search := "acme"
	isPersonal := false
	ownerEmail := "Owner@Example.com"
	yes, no := true, false
	want := repositories.AdminTeamFilters{
		Search:                   &search,
		IsPersonal:               &isPersonal,
		CreatedFrom:              &adminFilterQueryTime,
		CreatedTo:                &adminFilterQueryTime,
		MemberCount:              adminCountRange(0, 1),
		OwnerCount:               adminCountRange(1, 1),
		AdminCount:               adminCountRange(1, 2),
		ProjectCount:             adminCountRange(2, 3),
		PromptCount:              adminCountRange(3, 4),
		MemoryCount:              adminCountRange(4, 5),
		ArtifactCount:            adminCountRange(5, 6),
		BlueprintCount:           adminCountRange(6, 7),
		AgentCount:               adminCountRange(7, 8),
		FeedCount:                adminCountRange(8, 9),
		FeedItemCount:            adminCountRange(9, 10),
		CommentCount:             adminCountRange(10, 11),
		AttachmentCount:          adminCountRange(11, 12),
		TotalResourceCount:       adminCountRange(12, 13),
		OwnerEmail:               &ownerEmail,
		EmbeddingConfigured:      &yes,
		LLMConfigured:            &no,
		AISummaryEnabled:         &yes,
		EmailConfigured:          &no,
		GitHubConfigured:         &yes,
		SearchSettingsCustomized: &no,
		FreshnessEnabled:         &yes,
		SortBy:                   "total_resource_count",
		SortOrder:                "desc",
		Page:                     1,
		Limit:                    10,
	}
	list := models.AdminTeamList{
		Teams: []models.AdminTeamListItem{}, TotalCount: 0, Page: 1, PerPage: 10, TotalPages: 0,
	}

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListTeams", mock.Anything, want).Return(list, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	stamp := adminFilterQueryTime.Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/v1/admin/teams?page=1&limit=10&search=acme"+
		"&is_personal=false&created_from="+stamp+"&created_to="+stamp+
		"&member_count_min=0&member_count_max=1&owner_count_min=1&owner_count_max=1"+
		"&admin_count_min=1&admin_count_max=2&project_count_min=2&project_count_max=3"+
		"&prompt_count_min=3&prompt_count_max=4&memory_count_min=4&memory_count_max=5"+
		"&artifact_count_min=5&artifact_count_max=6&blueprint_count_min=6&blueprint_count_max=7"+
		"&agent_count_min=7&agent_count_max=8&feed_count_min=8&feed_count_max=9"+
		"&feed_item_count_min=9&feed_item_count_max=10&comment_count_min=10&comment_count_max=11"+
		"&attachment_count_min=11&attachment_count_max=12"+
		"&total_resource_count_min=12&total_resource_count_max=13"+
		"&owner_email=%20Owner%40Example.com%20"+
		"&embedding_configured=true&llm_configured=false&ai_summary_enabled=true"+
		"&email_configured=false&github_configured=true&search_settings_customized=false"+
		"&freshness_enabled=true"+
		"&sort_by=total_resource_count&sort_order=desc", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestListAdminTeams_AbsentTriStatesAreAny pins "omit = any": with no tri-state
// in the query every configured filter reaches the service as nil.
func TestListAdminTeams_AbsentTriStatesAreAny(t *testing.T) {
	list := models.AdminTeamList{Teams: []models.AdminTeamListItem{}, Page: 1, PerPage: 20}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListTeams", mock.Anything, mock.MatchedBy(func(f repositories.AdminTeamFilters) bool {
		return f.EmbeddingConfigured == nil && f.LLMConfigured == nil && f.AISummaryEnabled == nil &&
			f.EmailConfigured == nil && f.GitHubConfigured == nil && f.SearchSettingsCustomized == nil &&
			f.FreshnessEnabled == nil && f.OwnerEmail == nil
	})).Return(list, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/teams?member_count_min=2", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

// TestListAdminTeams_InvalidAggregateFiltersReturn400 is the team mirror of the
// user validation test: bad bounds, a non-boolean tri-state and an unknown
// sort_by are rejected before the service is called.
func TestListAdminTeams_InvalidAggregateFiltersReturn400(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"negative min", "member_count_min=-1"},
		{"negative max", "admin_count_max=-2"},
		{"inverted owner range", "owner_count_min=2&owner_count_max=1"},
		{"inverted project range", "project_count_min=5&project_count_max=4"},
		{"inverted total range", "total_resource_count_min=10&total_resource_count_max=1"},
		{"non-integer bound", "feed_item_count_min=lots"},
		{"non-boolean tri-state", "embedding_configured=maybe"},
		{"non-boolean freshness tri-state", "freshness_enabled=2x"},
		{"unknown sort_by", "sort_by=embedding_configured"},
		{"malformed owner_email", "owner_email=foo"},
		{"owner_email with a display name", "owner_email=Owner%20%3Cowner%40example.com%3E"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No service call is expected: the request must be rejected first.
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", "/api/v1/admin/teams?"+tc.query, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Header().Get("Content-Type"), "application/problem+json")
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestListAdmin_InvalidSortEnumReturns400 pins the acceptance criterion that an
// out-of-enum sort_by/sort_order is a 400, not a silent fallback to the default
// ordering. The generated binder does NOT enforce the enum (it binds the raw
// string), so this is enforced explicitly in the handler — if that check is ever
// dropped these cases go green-to-200 and catch it.
func TestListAdmin_InvalidSortEnumReturns400(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"users unknown sort_by", "/api/v1/admin/users?sort_by=password"},
		{"users unknown sort_order", "/api/v1/admin/users?sort_order=sideways"},
		{"users injection-shaped sort_by", "/api/v1/admin/users?sort_by=id%3B+DROP+TABLE+users--"},
		{"teams unknown sort_by", "/api/v1/admin/teams?sort_by=owner"},
		{"teams unknown sort_order", "/api/v1/admin/teams?sort_order=random"},
		{"teams injection-shaped sort_by", "/api/v1/admin/teams?sort_by=id%3B+DROP+TABLE+teams--"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No service call is expected: the request must be rejected first.
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", tc.path, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestGetAdminTeam_Found(t *testing.T) {
	id := uuid.NewString()
	owner := models.AdminTeamOwner{ID: uuid.NewString(), Email: "owner@example.com", Name: "Owner"}
	detail := &models.AdminTeamDetail{
		ID: id, Name: "Acme", Slug: "acme", IsPersonal: true, Owner: owner, CreatedAt: time.Now(),
		Members: []models.AdminTeamMember{
			{UserID: uuid.NewString(), Email: "m@example.com", Name: "M", Role: "member", JoinedAt: time.Now()},
		},
	}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetTeamDetail", mock.Anything, id).Return(detail, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/teams/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminTeamDetail
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, id, resp.Id.String())
	assert.Equal(t, "Owner", resp.Owner.Name)
	// #452's additive required fields — asserted explicitly because their zero
	// values would still satisfy the spec (see TestListAdminTeams).
	assert.Equal(t, "acme", resp.Slug)
	assert.True(t, resp.IsPersonal)
	require.Len(t, resp.Members, 1)
	assert.Equal(t, "member", resp.Members[0].Role)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminTeam_NotFound(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetTeamDetail", mock.Anything, id).Return(nil, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/teams/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetAdminTeam_InvalidUUID(t *testing.T) {
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminService: servicesmocks.NewMockAdminServiceInterface(t),
	})
	req := httptest.NewRequest("GET", "/api/v1/admin/teams/not-a-uuid", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestListAdminTeams_ServiceError(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListTeams", mock.Anything, repositories.AdminTeamFilters{}).Return(models.AdminTeamList{}, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/teams", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestListAdminTeams_ConversionError(t *testing.T) {
	bad := models.AdminTeamList{
		Teams:   []models.AdminTeamListItem{{ID: "not-a-uuid", Name: "X", Owner: models.AdminTeamOwner{ID: uuid.NewString()}}},
		Page:    1,
		PerPage: 20,
	}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListTeams", mock.Anything, repositories.AdminTeamFilters{}).Return(bad, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/teams", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestGetAdminTeam_ServiceError(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetTeamDetail", mock.Anything, id).Return(nil, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/teams/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestGetAdminTeam_ConversionError(t *testing.T) {
	id := uuid.NewString()
	// Valid team id + owner, but a member with a non-UUID user id → conversion fails.
	bad := &models.AdminTeamDetail{
		ID: id, Name: "Acme", Owner: models.AdminTeamOwner{ID: uuid.NewString(), Email: "o@example.com", Name: "O"},
		Members: []models.AdminTeamMember{{UserID: "not-a-uuid", Email: "m@example.com", Name: "M", Role: "member", JoinedAt: time.Now()}},
	}
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetTeamDetail", mock.Anything, id).Return(bad, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/teams/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
