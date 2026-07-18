package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// The memory/artifact/blueprint handlers must turn services.ErrPermissionDenied
// into a 403.
//
// This is the bug class that shipped green twice in this epic (#222/PR #233,
// #235/PR #239) and was caught again in #236: these handlers map errors by
// substring, and ErrPermissionDenied's text matches none of their branches. The
// artifact and blueprint DELETE paths are worse than that — they had no branching
// at all and wrote an unconditional 500 — so without an explicit guard a denied
// delete is indistinguishable from a server fault. Service tests cannot see any
// of this; only a request through the handler can.

const (
	resHTeam = "team-123"
	resHUser = "user-caller"
	// The artifact/blueprint handlers validate project_id as a UUID before
	// reaching the service.
	resHProject = "6f1e7a2c-9b3d-4e5f-8a01-2b3c4d5e6f70"
)

type MockResourceRBACContainer struct {
	BaseMockContainer
	memoryService        services.MemoryServiceInterface
	artifactService      services.ArtifactServiceInterface
	blueprintService     services.BlueprintServiceInterface
	resourceUsageService services.ResourceUsageServiceInterface
	projectRepository    repositories.ProjectRepository
}

func (m *MockResourceRBACContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

func (m *MockResourceRBACContainer) ProjectRepository() repositories.ProjectRepository {
	return m.projectRepository
}

func (m *MockResourceRBACContainer) MemoryService() services.MemoryServiceInterface {
	return m.memoryService
}

func (m *MockResourceRBACContainer) ArtifactService() services.ArtifactServiceInterface {
	return m.artifactService
}

func (m *MockResourceRBACContainer) BlueprintService() services.BlueprintServiceInterface {
	return m.blueprintService
}

// createTestResourceRBACServer mounts the delete routes for the three domains.
// The team validation middleware is deliberately not mounted: it enforces
// tenancy, while the role decision under test happens in the service.
func createTestResourceRBACServer(t *testing.T, c *MockResourceRBACContainer) *Server {
	t.Helper()

	// The create paths check a resource limit and validate the project before
	// reaching the service.
	usage := svcmocks.NewMockResourceUsageServiceInterface(t)
	usage.EXPECT().CheckResourceLimit(mock.Anything, mock.Anything, mock.Anything).
		Return(true, nil).Maybe()
	c.resourceUsageService = usage

	projectRepo := repomocks.NewMockProjectRepository(t)
	projectRepo.EXPECT().GetByID(mock.Anything, mock.Anything, mock.Anything).
		Return(&models.Project{ID: resHProject, UserID: resHUser, TeamID: resHTeam}, nil).Maybe()
	c.projectRepository = projectRepo

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: c,
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}

	r.Route("/api/v1/{team_id}", func(r chi.Router) {
		r.Post("/memories", srv.handleCreateMemory)
		r.Delete("/memories/{id}", srv.handleDeleteMemory)
		r.Post("/projects/{project_id}/artifacts", srv.handleCreateArtifact)
		r.Delete("/projects/{project_id}/artifacts/{slug}", srv.handleDeleteArtifact)
		r.Post("/projects/{project_id}/blueprints", srv.handleCreateBlueprint)
		r.Delete("/projects/{project_id}/blueprints/{slug}", srv.handleDeleteBlueprint)
	})
	return srv
}

func resHRequest(method, path string) *http.Request {
	return resHRequestBody(method, path, "")
}

func resHRequestBody(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, resHUser))
}

func assertResForbidden(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusForbidden, w.Code, "denial must be 403, not 500: %s", w.Body.String())
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"code":"FORBIDDEN"`)
}

func TestMemoryHandler_DeletePermissionDeniedIsForbidden(t *testing.T) {
	mem := svcmocks.NewMockMemoryServiceInterface(t)
	mem.EXPECT().DeleteMemory(resHUser, resHTeam, "memory-1").
		Return(services.ErrPermissionDenied).Once()

	srv := createTestResourceRBACServer(t, &MockResourceRBACContainer{memoryService: mem})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, resHRequest(http.MethodDelete, "/api/v1/"+resHTeam+"/memories/memory-1"))

	assertResForbidden(t, w)
}

// TestArtifactHandler_DeletePermissionDeniedIsForbidden guards the silent-500
// path: logHandlerError writes 500 unconditionally, so the guard is the only
// thing making a denial a 403.
func TestArtifactHandler_DeletePermissionDeniedIsForbidden(t *testing.T) {
	art := svcmocks.NewMockArtifactServiceInterface(t)
	art.EXPECT().GetArtifactByProjectIDAndSlugInTeam(resHUser, resHTeam, resHProject, "a").
		Return(&models.Artifact{ID: "artifact-1", Slug: "a"}, nil).Once()
	art.EXPECT().DeleteArtifactByProjectIDAndSlug(resHUser, resHTeam, resHProject, "a").
		Return(services.ErrPermissionDenied).Once()

	srv := createTestResourceRBACServer(t, &MockResourceRBACContainer{artifactService: art})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, resHRequest(http.MethodDelete, "/api/v1/"+resHTeam+"/projects/"+resHProject+"/artifacts/a"))

	assertResForbidden(t, w)
}

// TestBlueprintHandler_DeletePermissionDeniedIsForbidden — same silent-500 path
// as artifacts.
func TestBlueprintHandler_DeletePermissionDeniedIsForbidden(t *testing.T) {
	bp := svcmocks.NewMockBlueprintServiceInterface(t)
	bp.EXPECT().GetBlueprintByProjectIDAndSlugInTeam(resHUser, resHTeam, resHProject, "b").
		Return(&models.Blueprint{ID: "blueprint-1", Slug: "b"}, nil).Once()
	bp.EXPECT().DeleteBlueprintByProjectIDAndSlug(resHUser, resHTeam, resHProject, "b").
		Return(services.ErrPermissionDenied).Once()

	srv := createTestResourceRBACServer(t, &MockResourceRBACContainer{blueprintService: bp})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, resHRequest(http.MethodDelete, "/api/v1/"+resHTeam+"/projects/"+resHProject+"/blueprints/b"))

	assertResForbidden(t, w)
}

// TestResourceHandlers_CreatePermissionDeniedIsForbidden covers the create
// branches. Their error mappers match on substrings that ErrPermissionDenied's
// text hits nowhere, so without the guard each would render a 500.
func TestResourceHandlers_CreatePermissionDeniedIsForbidden(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		mem := svcmocks.NewMockMemoryServiceInterface(t)
		mem.EXPECT().CreateMemory(resHUser, resHTeam, mock.Anything).
			Return(nil, services.ErrPermissionDenied).Once()

		srv := createTestResourceRBACServer(t, &MockResourceRBACContainer{memoryService: mem})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, resHRequestBody(http.MethodPost, "/api/v1/"+resHTeam+"/memories",
			`{"text":"t","project_id":"`+resHProject+`"}`))

		assertResForbidden(t, w)
	})

	t.Run("artifact", func(t *testing.T) {
		art := svcmocks.NewMockArtifactServiceInterface(t)
		art.EXPECT().CreateArtifact(resHUser, resHTeam, mock.Anything).
			Return(nil, services.ErrPermissionDenied).Once()

		srv := createTestResourceRBACServer(t, &MockResourceRBACContainer{artifactService: art})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, resHRequestBody(http.MethodPost,
			"/api/v1/"+resHTeam+"/projects/"+resHProject+"/artifacts",
			`{"slug":"a","title":"A","content":"c","project_id":"`+resHProject+`"}`))

		assertResForbidden(t, w)
	})

	t.Run("blueprint", func(t *testing.T) {
		bp := svcmocks.NewMockBlueprintServiceInterface(t)
		bp.EXPECT().CreateBlueprint(resHUser, resHTeam, mock.Anything).
			Return(nil, services.ErrPermissionDenied).Once()

		srv := createTestResourceRBACServer(t, &MockResourceRBACContainer{blueprintService: bp})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, resHRequestBody(http.MethodPost,
			"/api/v1/"+resHTeam+"/projects/"+resHProject+"/blueprints",
			`{"slug":"b","title":"B","content":"c","project_id":"`+resHProject+`"}`))

		assertResForbidden(t, w)
	})
}
