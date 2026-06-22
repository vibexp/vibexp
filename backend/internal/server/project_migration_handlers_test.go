package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
)

// mockProjectMigrationService is a hand-rolled mock for ProjectMigrationServiceInterface.
// Mockery-generated mocks live in internal/services/mocks/; since the interface is newly added
// in this PR and mockery is run as a separate make target, we write a minimal mock here.
type mockProjectMigrationService struct {
	mock.Mock
}

func (m *mockProjectMigrationService) GetInventory(
	ctx context.Context, userID, teamID, projectID string,
) (*projectmigration.MigrationInventory, error) {
	args := m.Called(ctx, userID, teamID, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*projectmigration.MigrationInventory), args.Error(1)
}

func (m *mockProjectMigrationService) Migrate(
	ctx context.Context, userID, teamID, sourceProjectID string,
	req *projectmigration.MigrationRequest,
) (*projectmigration.MigrationResult, error) {
	args := m.Called(ctx, userID, teamID, sourceProjectID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*projectmigration.MigrationResult), args.Error(1)
}

// migrationMockContainer embeds BaseMockContainer and overrides ProjectMigrationService.
type migrationMockContainer struct {
	BaseMockContainer
	migrationSvc services.ProjectMigrationServiceInterface
}

func (m *migrationMockContainer) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return m.migrationSvc
}

func newMigrationServer(_ *testing.T, svc services.ProjectMigrationServiceInterface) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()
	c := &migrationMockContainer{migrationSvc: svc}

	srv := &Server{
		port:      "8080",
		container: c,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register only the migration routes (avoids nil-service panics from full setupRoutes).
	r.Route("/api/v1/{team_id}/projects", func(r chi.Router) {
		r.Use(srv.flexibleAuthMiddleware)
		r.Get("/{project_id}/migration/inventory", srv.handleGetMigrationInventory)
		r.Post("/{project_id}/migration", srv.handleMigrateProject)
	})

	return srv
}

// --- Route registration tests (no auth required, just 401 vs 404) ---

func TestMigrationRoutes_Registered(t *testing.T) {
	t.Parallel()

	srv := newMigrationServer(t, nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "GET inventory route registered",
			method: http.MethodGet,
			path:   "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/proj-123/migration/inventory",
		},
		{
			name:   "POST migration route registered",
			method: http.MethodPost,
			path:   "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/proj-123/migration",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequest(tc.method, tc.path, nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)
			// Auth middleware runs first → 401, not 404
			assert.NotEqual(t, http.StatusNotFound, rr.Code, "route should be registered, got 404")
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
		})
	}
}

// --- Handler unit tests using injected context (bypasses auth middleware) ---

// withMigrationChiParams adds chi URL route parameters for migration tests.
// Required when calling handlers directly (not through the chi router).
func withMigrationChiParams(req *http.Request, teamID, projectID string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("team_id", teamID)
	rctx.URLParams.Add("project_id", projectID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// buildMigrationRequest creates an authenticated HTTP request with optional JSON body and chi params.
// teamID is always migrationTestTeamID in these tests; projectID selects the source project.
func buildMigrationRequest(t *testing.T, method, projectID string, body interface{}) *http.Request {
	t.Helper()

	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req, err = http.NewRequest(method, "/migration", bytes.NewReader(b))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
	} else {
		var err error
		req, err = http.NewRequest(method, "/migration", nil)
		require.NoError(t, err)
	}

	// Inject user ID directly into the context to bypass the auth middleware.
	ctx := context.WithValue(req.Context(), contextKeyUserID, "user-test-123")
	req = req.WithContext(ctx)
	return withMigrationChiParams(req, migrationTestTeamID, projectID)
}

const (
	migrationTestTeamID    = "550e8400-e29b-41d4-a716-446655440000"
	migrationTestSrcProjID = "550e8400-e29b-41d4-a716-446655440002"
	migrationTestDstProjID = "550e8400-e29b-41d4-a716-446655440003"
)

func TestHandleGetMigrationInventory_Success(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	inv := &projectmigration.MigrationInventory{
		Prompts:    projectmigration.ResourceInventory{Count: 2},
		Artifacts:  projectmigration.ResourceInventory{Count: 1},
		Blueprints: projectmigration.ResourceInventory{Count: 0},
		FeedItems:  projectmigration.ResourceInventory{Count: 3},
	}
	svc.On("GetInventory", mock.Anything, "user-test-123", mock.AnythingOfType("string"), migrationTestSrcProjID).
		Return(inv, nil)

	srv := newMigrationServer(t, svc)
	req := buildMigrationRequest(t, http.MethodGet, migrationTestSrcProjID, nil)
	rr := httptest.NewRecorder()
	srv.handleGetMigrationInventory(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var got projectmigration.MigrationInventory
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))
	assert.Equal(t, 2, got.Prompts.Count)
	assert.Equal(t, 1, got.Artifacts.Count)
	assert.Equal(t, 3, got.FeedItems.Count)
	svc.AssertExpectations(t)
}

func TestHandleGetMigrationInventory_NotFound(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	svc.On("GetInventory", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("source project not accessible: project not found"))

	srv := newMigrationServer(t, svc)
	req := buildMigrationRequest(t, http.MethodGet, migrationTestSrcProjID, nil)
	rr := httptest.NewRecorder()
	srv.handleGetMigrationInventory(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	svc.AssertExpectations(t)
}

func TestHandleGetMigrationInventory_ServiceError(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	svc.On("GetInventory", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("db connection failed"))

	srv := newMigrationServer(t, svc)
	req := buildMigrationRequest(t, http.MethodGet, migrationTestSrcProjID, nil)
	rr := httptest.NewRecorder()
	srv.handleGetMigrationInventory(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleMigrateProject_Success(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	result := &projectmigration.MigrationResult{
		Migrated: projectmigration.ResourceMigrationCounts{Prompts: 2, Artifacts: 1},
	}
	svc.On("Migrate", mock.Anything, "user-test-123",
		mock.AnythingOfType("string"), migrationTestSrcProjID, mock.Anything).
		Return(result, nil)

	srv := newMigrationServer(t, svc)
	body := projectmigration.MigrationRequest{
		DestinationProjectID: migrationTestDstProjID,
		Resources:            projectmigration.ResourceSelections{Prompts: projectmigration.ResourceSelection{All: true}},
		ConflictPolicy:       projectmigration.ConflictPolicySkip,
	}
	req := buildMigrationRequest(t, http.MethodPost, migrationTestSrcProjID, body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var got projectmigration.MigrationResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))
	assert.Equal(t, 2, got.Migrated.Prompts)
	assert.Equal(t, 1, got.Migrated.Artifacts)
	svc.AssertExpectations(t)
}

func TestHandleMigrateProject_MissingDestinationProjectID(t *testing.T) {
	t.Parallel()

	srv := newMigrationServer(t, nil)
	body := projectmigration.MigrationRequest{ConflictPolicy: projectmigration.ConflictPolicySkip}
	req := buildMigrationRequest(t, http.MethodPost, migrationTestSrcProjID, body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleMigrateProject_InvalidConflictPolicy(t *testing.T) {
	t.Parallel()

	srv := newMigrationServer(t, nil)
	body := map[string]interface{}{
		"destination_project_id": migrationTestDstProjID,
		"conflict_policy":        "explode",
	}
	req := buildMigrationRequest(t, http.MethodPost, migrationTestSrcProjID, body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleMigrateProject_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newMigrationServer(t, nil)

	req, err := http.NewRequest(http.MethodPost, "/migration", bytes.NewBufferString("{not valid json}"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), contextKeyUserID, "user-test-123")
	req = req.WithContext(ctx)
	req = withMigrationChiParams(req, migrationTestTeamID, migrationTestSrcProjID)

	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleMigrateProject_CrossTeam(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	svc.On("Migrate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("cross-team migration not supported: source team a, destination team b"))

	srv := newMigrationServer(t, svc)
	body := projectmigration.MigrationRequest{
		DestinationProjectID: migrationTestDstProjID,
		ConflictPolicy:       projectmigration.ConflictPolicySkip,
	}
	req := buildMigrationRequest(t, http.MethodPost, migrationTestSrcProjID, body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleMigrateProject_DestinationNotFound(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	svc.On("Migrate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("Migrate: destination project not accessible: project not found"))

	srv := newMigrationServer(t, svc)
	body := projectmigration.MigrationRequest{
		DestinationProjectID: "550e8400-e29b-41d4-a716-446655440001",
		ConflictPolicy:       projectmigration.ConflictPolicySkip,
	}
	req := buildMigrationRequest(t, http.MethodPost, "550e8400-e29b-41d4-a716-446655440002", body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleGetMigrationInventory_InvalidProjectIDUUID(t *testing.T) {
	t.Parallel()

	srv := newMigrationServer(t, nil)
	req := buildMigrationRequest(t, http.MethodGet, "not-a-uuid", nil)
	rr := httptest.NewRecorder()
	srv.handleGetMigrationInventory(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleMigrateProject_InvalidProjectIDUUID(t *testing.T) {
	t.Parallel()

	srv := newMigrationServer(t, nil)
	body := projectmigration.MigrationRequest{
		DestinationProjectID: "550e8400-e29b-41d4-a716-446655440001",
		ConflictPolicy:       projectmigration.ConflictPolicySkip,
	}
	req := buildMigrationRequest(t, http.MethodPost, "not-a-uuid", body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleMigrateProject_InvalidDestinationProjectIDUUID(t *testing.T) {
	t.Parallel()

	srv := newMigrationServer(t, nil)
	body := projectmigration.MigrationRequest{
		DestinationProjectID: "not-a-uuid",
		ConflictPolicy:       projectmigration.ConflictPolicySkip,
	}
	req := buildMigrationRequest(t, http.MethodPost, "550e8400-e29b-41d4-a716-446655440002", body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleMigrateProject_TeamMismatch(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	svc.On("Migrate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("Migrate: %w", projectmigration.ErrTeamMismatch))

	srv := newMigrationServer(t, svc)
	body := projectmigration.MigrationRequest{
		DestinationProjectID: "550e8400-e29b-41d4-a716-446655440001",
		ConflictPolicy:       projectmigration.ConflictPolicySkip,
	}
	req := buildMigrationRequest(t, http.MethodPost, "550e8400-e29b-41d4-a716-446655440002", body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	svc.AssertExpectations(t)
}

func TestHandleGetMigrationInventory_TeamMismatch(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	svc.On("GetInventory", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("GetInventory: %w", projectmigration.ErrTeamMismatch))

	srv := newMigrationServer(t, svc)
	req := buildMigrationRequest(t, http.MethodGet, "550e8400-e29b-41d4-a716-446655440002", nil)
	rr := httptest.NewRecorder()
	srv.handleGetMigrationInventory(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	svc.AssertExpectations(t)
}

func TestHandleMigrateProject_ActivityUsesProjectNames(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	result := &projectmigration.MigrationResult{
		Migrated:               projectmigration.ResourceMigrationCounts{Prompts: 1},
		SourceProjectName:      "Source Project",
		DestinationProjectName: "Destination Project",
	}
	svc.On("Migrate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(result, nil)

	srv := newMigrationServer(t, svc)
	body := projectmigration.MigrationRequest{
		DestinationProjectID: "550e8400-e29b-41d4-a716-446655440001",
		Resources:            projectmigration.ResourceSelections{Prompts: projectmigration.ResourceSelection{All: true}},
		ConflictPolicy:       projectmigration.ConflictPolicySkip,
	}
	req := buildMigrationRequest(t, http.MethodPost, "550e8400-e29b-41d4-a716-446655440002", body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var got projectmigration.MigrationResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))
	assert.Equal(t, "Source Project", got.SourceProjectName)
	assert.Equal(t, "Destination Project", got.DestinationProjectName)
	svc.AssertExpectations(t)
}

func TestHandleMigrateProject_EmptyConflictPolicyDefaultsToSkip(t *testing.T) {
	t.Parallel()

	svc := new(mockProjectMigrationService)
	svc.On("Migrate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&projectmigration.MigrationResult{}, nil)

	srv := newMigrationServer(t, svc)
	// Send a request with no conflict_policy — it should default to "skip".
	body := map[string]interface{}{
		"destination_project_id": migrationTestDstProjID,
		"resources":              map[string]interface{}{"prompts": map[string]interface{}{"all": true}},
	}
	req := buildMigrationRequest(t, http.MethodPost, migrationTestSrcProjID, body)
	rr := httptest.NewRecorder()
	srv.handleMigrateProject(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	svc.AssertExpectations(t)
}

func TestValidateMigrationRequest_ValidRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		conflictPolicy projectmigration.ConflictPolicy
	}{
		{name: "skip policy", conflictPolicy: projectmigration.ConflictPolicySkip},
		{name: "rename policy", conflictPolicy: projectmigration.ConflictPolicyRename},
		{name: "overwrite policy", conflictPolicy: projectmigration.ConflictPolicyOverwrite},
		{name: "empty policy defaults to skip", conflictPolicy: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &projectmigration.MigrationRequest{
				DestinationProjectID: "dst-id",
				ConflictPolicy:       tc.conflictPolicy,
			}
			require.NoError(t, validateMigrationRequest(req))
		})
	}
}

func TestValidateMigrationRequest_InvalidRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		req    *projectmigration.MigrationRequest
		errMsg string
	}{
		{
			name:   "missing destination project id",
			req:    &projectmigration.MigrationRequest{ConflictPolicy: projectmigration.ConflictPolicySkip},
			errMsg: "destination_project_id is required",
		},
		{
			name: "invalid conflict policy",
			req: &projectmigration.MigrationRequest{
				DestinationProjectID: "dst-id",
				ConflictPolicy:       "destroy",
			},
			errMsg: "conflict_policy must be one of",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateMigrationRequest(tc.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
}
