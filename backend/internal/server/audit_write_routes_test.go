package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/audit"
	"github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// Resource writes in the audit log, through the real route registrations and
// MCP tool handlers (#1513).

const (
	auditTestFeedID    = "550e8400-e29b-41d4-a716-446655440040"
	auditTestItemID    = "550e8400-e29b-41d4-a716-446655440050"
	auditTestOtherProj = "550e8400-e29b-41d4-a716-446655440060"
	auditTestPromptID  = "550e8400-e29b-41d4-a716-446655440070"
	auditSecretContent = "SECRET-CONTENT-NEVER-AUDITED"
)

// auditTestContainer is TestContainer plus everything the audited write paths
// reach: the recorder, locks, migration, the GitHub import and stubs for the
// best-effort side effects (embeddings, activity, the write service).
type auditTestContainer struct {
	TestContainer
	recorder   audit.Recorder
	locks      services.ResourceLockServiceInterface
	migrations services.ProjectMigrationServiceInterface
	github     services.GitHubAppServiceInterface
	types      services.TypeServiceInterface
}

func (c *auditTestContainer) AuditRecorder() audit.Recorder { return c.recorder }
func (c *auditTestContainer) ResourceLockService() services.ResourceLockServiceInterface {
	return c.locks
}
func (c *auditTestContainer) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return c.migrations
}
func (c *auditTestContainer) GitHubAppService() services.GitHubAppServiceInterface { return c.github }
func (c *auditTestContainer) TypeService() services.TypeServiceInterface           { return c.types }
func (c *auditTestContainer) ActivityService() activities.ActivityService {
	return &mockActivityService{}
}

type auditRouteFixture struct {
	srv        *Server
	router     chi.Router
	rec        *auditCapture
	prompts    *mocks.MockPromptServiceInterface
	artifacts  *mocks.MockArtifactServiceInterface
	blueprints *mocks.MockBlueprintServiceInterface
	memories   *mocks.MockMemoryServiceInterface
	feeds      *mocks.MockFeedServiceInterface
	items      *mocks.MockFeedItemServiceInterface
	projects   *mocks.MockProjectServiceInterface
	locks      *mocks.MockResourceLockServiceInterface
	migrations *mocks.MockProjectMigrationServiceInterface
	github     *mocks.MockGitHubAppServiceInterface
}

func newAuditRouteFixture(t *testing.T) *auditRouteFixture {
	t.Helper()
	f := &auditRouteFixture{
		rec:        &auditCapture{},
		prompts:    mocks.NewMockPromptServiceInterface(t),
		artifacts:  mocks.NewMockArtifactServiceInterface(t),
		blueprints: mocks.NewMockBlueprintServiceInterface(t),
		memories:   mocks.NewMockMemoryServiceInterface(t),
		feeds:      mocks.NewMockFeedServiceInterface(t),
		items:      mocks.NewMockFeedItemServiceInterface(t),
		projects:   mocks.NewMockProjectServiceInterface(t),
		locks:      mocks.NewMockResourceLockServiceInterface(t),
		migrations: mocks.NewMockProjectMigrationServiceInterface(t),
		github:     mocks.NewMockGitHubAppServiceInterface(t),
	}

	teams := mocks.NewMockTeamServiceInterface(t)
	teams.EXPECT().IsUserMemberOfTeam(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()
	embeddings := mocks.NewMockEmbeddingServiceInterface(t)
	embeddings.EXPECT().DeleteEmbeddingsByEntity(mock.Anything, mock.Anything).Return(nil).Maybe()
	replies := mocks.NewMockFeedItemReplyServiceInterface(t)
	replies.EXPECT().ListReplyPosters(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil).Maybe()
	projectRepo := repomocks.NewMockProjectRepository(t)
	projectRepo.EXPECT().GetByID(mock.Anything, mock.Anything, mock.Anything).
		Return(&models.Project{ID: writeTestProjectID, TeamID: writeTestTeamID}, nil).Maybe()
	types := mocks.NewMockTypeServiceInterface(t)
	types.EXPECT().ValidateType(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()

	f.srv = New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	f.srv.container = &auditTestContainer{
		TestContainer: TestContainer{
			PromptServiceMock: f.prompts, ArtifactServiceMock: f.artifacts,
			BlueprintServiceMock: f.blueprints, MemoryServiceMock: f.memories,
			FeedServiceMock: f.feeds, FeedItemServiceMock: f.items, FeedItemReplyServiceMock: replies,
			ProjectServiceMock: f.projects, TeamServiceMock: teams, EmbeddingServiceMock: embeddings,
			ProjectRepositoryMock: projectRepo,
			TeamRepositoryMock:    stubTeamResolution(t, []models.Team{memberTeam()}),
		},
		recorder: f.rec, locks: f.locks, migrations: f.migrations, github: f.github, types: types,
	}

	f.router = chi.NewRouter()
	f.srv.mountPromptsHandlers(f.router)
	f.srv.mountArtifactsHandlers(f.router)
	f.srv.mountBlueprintsHandlers(f.router)
	f.srv.mountMemoriesHandlers(f.router)
	f.srv.setupFeedsRoutes(f.router)
	f.srv.setupProjectsRoutes(f.router)
	f.srv.setupGitHubIntegrationRoutes(f.router)
	return f
}

// auditRequest is a CLI request on an API key, so the channel the audit log
// sees ("cli") can only come from the route's write-source annotation.
func auditRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", writeTestCLIAgent)
	ctx := context.WithValue(req.Context(), contextKeyUserID, writeTestUserID)
	ctx = context.WithValue(ctx, contextkeys.AuthType, "api_key")
	ctx = context.WithValue(ctx, contextkeys.APIKeyID, writeTestCallerRef)
	return req.WithContext(ctx)
}

type auditRouteCase struct {
	name, method, path, body string
	wantStatus               int
	arrange                  func(f *auditRouteFixture)
	op                       models.AuditOperation
	resourceID, resourceName string
	projectID                string
	// check asserts the case's metadata, when it has any worth asserting.
	check func(t *testing.T, meta map[string]any)
}

func auditPrompt(projectID, status string) *models.Prompt {
	return &models.Prompt{
		ID: auditTestPromptID, TeamID: writeTestTeamID, Slug: "p", Name: "Release checklist",
		Body: auditSecretContent, Status: status,
		ProjectID: projectID, Project: &models.ProjectSummary{ID: projectID, Name: "Alpha"},
	}
}

func auditArtifact() *models.Artifact {
	return &models.Artifact{
		ID: "art-1", Slug: "a", Title: "Design doc", Content: auditSecretContent,
		ProjectID: writeTestProjectID, Type: "general", Status: "active",
	}
}

func auditBlueprint() *models.Blueprint {
	return &models.Blueprint{
		ID: "bp-1", Slug: "b", Title: "CLAUDE.md", Content: auditSecretContent,
		ProjectID: writeTestProjectID, Type: "general", Status: "active",
	}
}

func auditMemory(projectID string) *models.Memory {
	title := "Deploy notes"
	return &models.Memory{ID: writeTestMemoryID, Title: &title, Text: auditSecretContent, ProjectID: projectID}
}

//nolint:funlen // one table row per audited route
func auditResourceRouteCases() []auditRouteCase {
	base := "/api/v1/" + writeTestTeamID
	artifactPath := base + "/artifacts/" + writeTestProjectID + "/a"
	blueprintPath := base + "/blueprints/" + writeTestProjectID + "/b"
	memoryPath := base + "/memories/" + writeTestMemoryID

	return []auditRouteCase{
		{
			name: "create prompt", method: http.MethodPost, path: base + "/prompts", wantStatus: http.StatusCreated,
			body: `{"name":"Release checklist","slug":"p","body":"x","project_id":"` + writeTestProjectID + `"}`,
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().CreatePrompt(mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything).
					Return(auditPrompt(writeTestProjectID, "active"), nil)
			},
			op: models.AuditOpPromptCreate, resourceID: auditTestPromptID, resourceName: "Release checklist",
			projectID: writeTestProjectID,
		},
		{
			name: "update prompt status", method: http.MethodPut, path: base + "/prompts/p", wantStatus: http.StatusOK,
			body: `{"status":"archived"}`,
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().GetPromptBySlug(writeTestUserID, writeTestTeamID, "p").
					Return(auditPrompt(writeTestProjectID, "active"), nil)
				f.prompts.EXPECT().UpdatePromptBySlug(mock.Anything, writeTestUserID, writeTestTeamID, "p", mock.Anything).
					Return(auditPrompt(writeTestProjectID, "archived"), nil)
			},
			op: models.AuditOpPromptUpdate, resourceID: auditTestPromptID, resourceName: "Release checklist",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, []string{"status"}, meta[auditMetaChangedFields])
				assert.Equal(t, map[string]any{"old": "active", "new": "archived"}, meta["status"])
			},
		},
		{
			name: "move prompt", method: http.MethodPut, path: base + "/prompts/p", wantStatus: http.StatusOK,
			body: `{"project_id":"` + auditTestOtherProj + `"}`,
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().GetPromptBySlug(writeTestUserID, writeTestTeamID, "p").
					Return(auditPrompt(writeTestProjectID, "active"), nil)
				moved := auditPrompt(auditTestOtherProj, "active")
				moved.Project = nil // the service clears the stale summary on a move
				f.prompts.EXPECT().UpdatePromptBySlug(mock.Anything, writeTestUserID, writeTestTeamID, "p", mock.Anything).
					Return(moved, nil)
				f.projects.EXPECT().GetProjectBySlugOrID(writeTestTeamID, writeTestUserID, auditTestOtherProj).
					Return(&models.Project{ID: auditTestOtherProj, Name: "Beta"}, nil)
			},
			op: models.AuditOpPromptMove, resourceID: auditTestPromptID, resourceName: "Release checklist",
			projectID: auditTestOtherProj,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, writeTestProjectID, meta[auditMetaFromProjectID])
				assert.Equal(t, "Alpha", meta[auditMetaFromProjectName])
				assert.Equal(t, auditTestOtherProj, meta[auditMetaToProjectID])
				assert.Equal(t, "Beta", meta[auditMetaToProjectName], "the destination is looked up")
			},
		},
		{
			name: "restore prompt", method: http.MethodPost, path: base + "/prompts/p/versions/2/restore",
			wantStatus: http.StatusOK,
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().RestorePromptVersion(mock.Anything, writeTestUserID, writeTestTeamID, "p", 2, mock.Anything).
					Return(auditPrompt(writeTestProjectID, "active"), nil)
			},
			op: models.AuditOpPromptUpdate, resourceID: auditTestPromptID, resourceName: "Release checklist",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, map[string]any{auditMetaRestoredVersion: 2}, meta)
			},
		},
		{
			name: "delete prompt", method: http.MethodDelete, path: base + "/prompts/p", wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().GetPromptBySlug(writeTestUserID, writeTestTeamID, "p").
					Return(auditPrompt(writeTestProjectID, "active"), nil)
				f.prompts.EXPECT().DeletePromptBySlug(mock.Anything, writeTestUserID, writeTestTeamID, "p").Return(nil)
			},
			op: models.AuditOpPromptDelete, resourceID: auditTestPromptID, resourceName: "Release checklist",
			projectID: writeTestProjectID,
		},
		{
			name: "lock prompt", method: http.MethodPost, path: base + "/prompts/p/lock", wantStatus: http.StatusOK,
			body: `{"comment":"release freeze"}`,
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().GetPromptBySlug(writeTestUserID, writeTestTeamID, "p").
					Return(auditPrompt(writeTestProjectID, "active"), nil)
				f.locks.EXPECT().Lock(mock.Anything, writeTestUserID, writeTestTeamID, "prompt", auditTestPromptID,
					"release freeze").Return(nil)
			},
			op: models.AuditOpPromptLock, resourceID: auditTestPromptID, resourceName: "Release checklist",
			projectID: writeTestProjectID,
		},
		{
			name: "unlock prompt", method: http.MethodDelete, path: base + "/prompts/p/lock", wantStatus: http.StatusOK,
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().GetPromptBySlug(writeTestUserID, writeTestTeamID, "p").
					Return(auditPrompt(writeTestProjectID, "active"), nil)
				f.locks.EXPECT().Unlock(mock.Anything, writeTestUserID, writeTestTeamID, "prompt", auditTestPromptID).
					Return(nil)
			},
			op: models.AuditOpPromptUnlock, resourceID: auditTestPromptID, resourceName: "Release checklist",
			projectID: writeTestProjectID,
		},
		{
			name: "create artifact", method: http.MethodPost, path: base + "/artifacts", wantStatus: http.StatusCreated,
			body: `{"project_id":"` + writeTestProjectID +
				`","slug":"a","title":"Design doc","content":"c","type":"general","status":"active"}`,
			arrange: func(f *auditRouteFixture) {
				f.artifacts.EXPECT().CreateArtifact(mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything).
					Return(auditArtifact(), nil)
			},
			op: models.AuditOpArtifactCreate, resourceID: "art-1", resourceName: "Design doc",
			projectID: writeTestProjectID,
		},
		{
			name: "update artifact content", method: http.MethodPut, path: artifactPath, wantStatus: http.StatusOK,
			body: `{"content":"new"}`,
			arrange: func(f *auditRouteFixture) {
				f.artifacts.EXPECT().GetArtifactByProjectIDAndSlugInTeam(
					writeTestUserID, writeTestTeamID, writeTestProjectID, "a").Return(auditArtifact(), nil)
				updated := auditArtifact()
				updated.Content = "SECRET-NEW-CONTENT"
				f.artifacts.EXPECT().UpdateArtifactByProjectIDAndSlugInTeam(
					mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "a", mock.Anything).Return(updated, nil)
			},
			op: models.AuditOpArtifactUpdate, resourceID: "art-1", resourceName: "Design doc",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, []string{"content"}, meta[auditMetaChangedFields])
			},
		},
		{
			name: "restore artifact", method: http.MethodPost, path: artifactPath + "/versions/2/restore",
			wantStatus: http.StatusOK,
			arrange: func(f *auditRouteFixture) {
				f.artifacts.EXPECT().RestoreArtifactVersionInTeam(
					mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "a", 2, mock.Anything).Return(auditArtifact(), nil)
			},
			op: models.AuditOpArtifactUpdate, resourceID: "art-1", resourceName: "Design doc",
			projectID: writeTestProjectID,
		},
		{
			name: "delete artifact", method: http.MethodDelete, path: artifactPath, wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.artifacts.EXPECT().GetArtifactByProjectIDAndSlugInTeam(
					writeTestUserID, writeTestTeamID, writeTestProjectID, "a").Return(auditArtifact(), nil)
				f.artifacts.EXPECT().DeleteArtifactByProjectIDAndSlug(
					mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "a").Return(nil)
			},
			op: models.AuditOpArtifactDelete, resourceID: "art-1", resourceName: "Design doc",
			projectID: writeTestProjectID,
		},
		{
			name: "lock artifact", method: http.MethodPost, path: artifactPath + "/lock", wantStatus: http.StatusOK,
			body: `{"comment":"final"}`,
			arrange: func(f *auditRouteFixture) {
				f.artifacts.EXPECT().GetArtifactByProjectIDAndSlugInTeam(
					writeTestUserID, writeTestTeamID, writeTestProjectID, "a").Return(auditArtifact(), nil)
				f.locks.EXPECT().Lock(mock.Anything, writeTestUserID, writeTestTeamID, "artifact", "art-1", "final").
					Return(nil)
			},
			op: models.AuditOpArtifactLock, resourceID: "art-1", resourceName: "Design doc",
			projectID: writeTestProjectID,
		},
		{
			name: "create blueprint", method: http.MethodPost, path: base + "/blueprints", wantStatus: http.StatusCreated,
			body: `{"project_id":"` + writeTestProjectID +
				`","slug":"b","title":"CLAUDE.md","content":"c","type":"general","status":"active"}`,
			arrange: func(f *auditRouteFixture) {
				f.blueprints.EXPECT().CreateBlueprint(mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything).
					Return(auditBlueprint(), nil)
			},
			op: models.AuditOpBlueprintCreate, resourceID: "bp-1", resourceName: "CLAUDE.md",
			projectID: writeTestProjectID,
		},
		{
			name: "update blueprint title", method: http.MethodPut, path: blueprintPath, wantStatus: http.StatusOK,
			body: `{"title":"AGENTS.md"}`,
			arrange: func(f *auditRouteFixture) {
				f.blueprints.EXPECT().GetBlueprintByProjectIDAndSlugInTeam(
					writeTestUserID, writeTestTeamID, writeTestProjectID, "b").Return(auditBlueprint(), nil)
				updated := auditBlueprint()
				updated.Title = "AGENTS.md"
				f.blueprints.EXPECT().UpdateBlueprintByProjectIDAndSlugInTeam(
					mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "b", mock.Anything).Return(updated, nil)
			},
			op: models.AuditOpBlueprintUpdate, resourceID: "bp-1", resourceName: "AGENTS.md",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, map[string]any{"old": "CLAUDE.md", "new": "AGENTS.md"}, meta["title"])
			},
		},
		{
			name: "restore blueprint", method: http.MethodPost, path: blueprintPath + "/versions/2/restore",
			wantStatus: http.StatusOK,
			arrange: func(f *auditRouteFixture) {
				f.blueprints.EXPECT().RestoreBlueprintVersionInTeam(
					mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "b", 2, mock.Anything).Return(auditBlueprint(), nil)
			},
			op: models.AuditOpBlueprintUpdate, resourceID: "bp-1", resourceName: "CLAUDE.md",
			projectID: writeTestProjectID,
		},
		{
			name: "delete blueprint", method: http.MethodDelete, path: blueprintPath, wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.blueprints.EXPECT().GetBlueprintByProjectIDAndSlugInTeam(
					writeTestUserID, writeTestTeamID, writeTestProjectID, "b").Return(auditBlueprint(), nil)
				f.blueprints.EXPECT().DeleteBlueprintByProjectIDAndSlug(
					mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "b").Return(nil)
			},
			op: models.AuditOpBlueprintDelete, resourceID: "bp-1", resourceName: "CLAUDE.md",
			projectID: writeTestProjectID,
		},
		{
			name: "unlock blueprint", method: http.MethodDelete, path: blueprintPath + "/lock", wantStatus: http.StatusOK,
			arrange: func(f *auditRouteFixture) {
				f.blueprints.EXPECT().GetBlueprintByProjectIDAndSlugInTeam(
					writeTestUserID, writeTestTeamID, writeTestProjectID, "b").Return(auditBlueprint(), nil)
				f.locks.EXPECT().Unlock(mock.Anything, writeTestUserID, writeTestTeamID, "blueprint", "bp-1").
					Return(nil)
			},
			op: models.AuditOpBlueprintUnlock, resourceID: "bp-1", resourceName: "CLAUDE.md",
			projectID: writeTestProjectID,
		},
		{
			name: "create memory", method: http.MethodPost, path: base + "/memories", wantStatus: http.StatusCreated,
			body: `{"project_id":"` + writeTestProjectID + `","text":"remember"}`,
			arrange: func(f *auditRouteFixture) {
				f.memories.EXPECT().CreateMemory(mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything).
					Return(auditMemory(writeTestProjectID), nil)
			},
			op: models.AuditOpMemoryCreate, resourceID: writeTestMemoryID, resourceName: "Deploy notes",
			projectID: writeTestProjectID,
		},
		{
			name: "move memory", method: http.MethodPut, path: memoryPath, wantStatus: http.StatusOK,
			body: `{"project_id":"` + auditTestOtherProj + `"}`,
			arrange: func(f *auditRouteFixture) {
				f.memories.EXPECT().GetMemory(writeTestUserID, writeTestTeamID, writeTestMemoryID).
					Return(auditMemory(writeTestProjectID), nil)
				f.memories.EXPECT().UpdateMemory(mock.Anything, writeTestUserID, writeTestTeamID, writeTestMemoryID, mock.Anything).
					Return(auditMemory(auditTestOtherProj), nil)
				f.projects.EXPECT().GetProjectBySlugOrID(writeTestTeamID, writeTestUserID, auditTestOtherProj).
					Return(nil, errors.New("gone"))
			},
			op: models.AuditOpMemoryMove, resourceID: writeTestMemoryID, resourceName: "Deploy notes",
			projectID: auditTestOtherProj,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, writeTestProjectID, meta[auditMetaFromProjectID])
				assert.Equal(t, auditTestOtherProj, meta[auditMetaToProjectID])
				assert.NotContains(t, meta, auditMetaToProjectName, "a failed lookup leaves the name out")
			},
		},
		{
			name: "restore memory", method: http.MethodPost, path: memoryPath + "/versions/2/restore",
			wantStatus: http.StatusOK,
			arrange: func(f *auditRouteFixture) {
				f.memories.EXPECT().RestoreMemoryVersion(mock.Anything, writeTestUserID, writeTestTeamID, writeTestMemoryID, 2, mock.Anything).
					Return(auditMemory(writeTestProjectID), nil)
			},
			op: models.AuditOpMemoryUpdate, resourceID: writeTestMemoryID, resourceName: "Deploy notes",
			projectID: writeTestProjectID,
		},
		{
			name: "delete memory", method: http.MethodDelete, path: memoryPath, wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.memories.EXPECT().GetMemory(writeTestUserID, writeTestTeamID, writeTestMemoryID).
					Return(auditMemory(writeTestProjectID), nil)
				f.memories.EXPECT().DeleteMemory(mock.Anything, writeTestUserID, writeTestTeamID, writeTestMemoryID).Return(nil)
			},
			op: models.AuditOpMemoryDelete, resourceID: writeTestMemoryID, resourceName: "Deploy notes",
			projectID: writeTestProjectID,
		},
	}
}

//nolint:funlen // one table row per audited route
func auditFeedProjectRouteCases() []auditRouteCase {
	base := "/api/v1/" + writeTestTeamID
	desc := "Release status"
	feed := &models.Feed{ID: auditTestFeedID, Name: "General", Description: &desc}
	projectID := writeTestProjectID
	item := &models.FeedItem{
		ID: auditTestItemID, FeedID: auditTestFeedID, Title: "Deploy done", Content: auditSecretContent,
		ProjectID: &projectID,
	}
	project := &models.Project{ID: writeTestProjectID, Name: "Alpha", Slug: "alpha", Description: auditSecretContent}

	return []auditRouteCase{
		{
			name: "create feed", method: http.MethodPost, path: base + "/feeds/", wantStatus: http.StatusCreated,
			body: `{"name":"General"}`,
			arrange: func(f *auditRouteFixture) {
				f.feeds.EXPECT().CreateFeed(mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything).Return(feed, nil)
			},
			op: models.AuditOpFeedCreate, resourceID: auditTestFeedID, resourceName: "General",
		},
		{
			name: "rename feed", method: http.MethodPut, path: base + "/feeds/" + auditTestFeedID,
			wantStatus: http.StatusOK, body: `{"name":"Releases"}`,
			arrange: func(f *auditRouteFixture) {
				f.feeds.EXPECT().GetFeed(mock.Anything, writeTestUserID, writeTestTeamID, auditTestFeedID).
					Return(feed, nil)
				renamed := *feed
				renamed.Name = "Releases"
				f.feeds.EXPECT().UpdateFeed(mock.Anything, writeTestUserID, writeTestTeamID, auditTestFeedID, mock.Anything).
					Return(&renamed, nil)
			},
			op: models.AuditOpFeedUpdate, resourceID: auditTestFeedID, resourceName: "Releases",
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, map[string]any{"old": "General", "new": "Releases"}, meta["name"])
			},
		},
		{
			name: "delete feed", method: http.MethodDelete, path: base + "/feeds/" + auditTestFeedID,
			wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.feeds.EXPECT().GetFeed(mock.Anything, writeTestUserID, writeTestTeamID, auditTestFeedID).
					Return(feed, nil)
				f.feeds.EXPECT().DeleteFeed(mock.Anything, writeTestUserID, writeTestTeamID, auditTestFeedID).Return(nil)
			},
			op: models.AuditOpFeedDelete, resourceID: auditTestFeedID, resourceName: "General",
		},
		{
			name: "create feed item", method: http.MethodPost, path: base + "/feeds/" + auditTestFeedID + "/items",
			wantStatus: http.StatusCreated, body: `{"title":"Deploy done","content":"x","ai_assistant_name":"Claude"}`,
			arrange: func(f *auditRouteFixture) {
				f.items.EXPECT().CreateFeedItem(mock.Anything, writeTestUserID, writeTestTeamID, auditTestFeedID,
					mock.Anything).Return(item, nil)
			},
			op: models.AuditOpFeedItemCreate, resourceID: auditTestItemID, resourceName: "Deploy done",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, map[string]any{"feed_id": auditTestFeedID}, meta)
			},
		},
		{
			name: "archive feed item", method: http.MethodPost, path: base + "/feed-items/" + auditTestItemID + "/archive",
			wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.items.EXPECT().GetFeedItem(mock.Anything, writeTestUserID, writeTestTeamID, auditTestItemID).
					Return(item, nil)
				f.items.EXPECT().ArchiveFeedItem(mock.Anything, writeTestUserID, writeTestTeamID, auditTestItemID).
					Return(nil)
			},
			op: models.AuditOpFeedItemArchive, resourceID: auditTestItemID, resourceName: "Deploy done",
			projectID: writeTestProjectID,
		},
		{
			name: "unarchive feed item", method: http.MethodPost,
			path: base + "/feed-items/" + auditTestItemID + "/unarchive", wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.items.EXPECT().GetFeedItem(mock.Anything, writeTestUserID, writeTestTeamID, auditTestItemID).
					Return(item, nil)
				f.items.EXPECT().UnarchiveFeedItem(mock.Anything, writeTestUserID, writeTestTeamID, auditTestItemID).
					Return(nil)
			},
			op: models.AuditOpFeedItemUnarchive, resourceID: auditTestItemID, resourceName: "Deploy done",
			projectID: writeTestProjectID,
		},
		{
			name: "delete feed item", method: http.MethodDelete, path: base + "/feed-items/" + auditTestItemID,
			wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.items.EXPECT().GetFeedItem(mock.Anything, writeTestUserID, writeTestTeamID, auditTestItemID).
					Return(item, nil)
				f.items.EXPECT().DeleteFeedItem(mock.Anything, writeTestUserID, writeTestTeamID, auditTestItemID).
					Return(nil)
			},
			op: models.AuditOpFeedItemDelete, resourceID: auditTestItemID, resourceName: "Deploy done",
			projectID: writeTestProjectID,
		},
		{
			name: "create project", method: http.MethodPost, path: base + "/projects/", wantStatus: http.StatusCreated,
			body: `{"name":"Alpha","slug":"alpha"}`,
			arrange: func(f *auditRouteFixture) {
				f.projects.EXPECT().CreateProject(mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything).Return(project, nil)
			},
			op: models.AuditOpProjectCreate, resourceID: writeTestProjectID, resourceName: "Alpha",
			projectID: writeTestProjectID,
		},
		{
			name: "update project", method: http.MethodPut, path: base + "/projects/alpha", wantStatus: http.StatusOK,
			body: `{"name":"Alpha 2"}`,
			arrange: func(f *auditRouteFixture) {
				f.projects.EXPECT().GetProjectBySlugOrID(writeTestTeamID, writeTestUserID, "alpha").Return(project, nil)
				renamed := *project
				renamed.Name = "Alpha 2"
				f.projects.EXPECT().UpdateProject(mock.Anything, writeTestTeamID, writeTestUserID, "alpha", mock.Anything).
					Return(&renamed, nil)
			},
			op: models.AuditOpProjectUpdate, resourceID: writeTestProjectID, resourceName: "Alpha 2",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, []string{"name"}, meta[auditMetaChangedFields])
			},
		},
		{
			name: "delete project", method: http.MethodDelete, path: base + "/projects/alpha",
			wantStatus: http.StatusNoContent,
			arrange: func(f *auditRouteFixture) {
				f.projects.EXPECT().GetProjectBySlugOrID(writeTestTeamID, writeTestUserID, "alpha").Return(project, nil)
				f.projects.EXPECT().DeleteProject(mock.Anything, writeTestTeamID, writeTestUserID, "alpha").Return(nil)
			},
			op: models.AuditOpProjectDelete, resourceID: writeTestProjectID, resourceName: "Alpha",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, map[string]any{auditMetaCascade: true}, meta)
			},
		},
		{
			name: "migrate project", method: http.MethodPost, path: base + "/projects/" + writeTestProjectID + "/migration",
			wantStatus: http.StatusOK, body: `{"destination_project_id":"` + auditTestOtherProj + `"}`,
			arrange: func(f *auditRouteFixture) {
				f.migrations.EXPECT().Migrate(mock.Anything, writeTestUserID, writeTestTeamID, writeTestProjectID,
					mock.Anything).Return(&projectmigration.MigrationResult{
					SourceProjectName: "Alpha", DestinationProjectName: "Beta",
					Migrated: projectmigration.ResourceMigrationCounts{Prompts: 2},
				}, nil)
			},
			op: models.AuditOpProjectMigrated, resourceID: writeTestProjectID, resourceName: "Alpha",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, map[string]any{"id": auditTestOtherProj, "name": "Beta"}, meta["target_project"])
			},
		},
		{
			name: "import a new project from GitHub", method: http.MethodPost,
			path:       base + "/integrations/github/repositories/42/import-project",
			wantStatus: http.StatusCreated,
			arrange: func(f *auditRouteFixture) {
				f.github.EXPECT().ImportProjectFromRepository(mock.Anything, writeTestUserID, writeTestTeamID, int64(42)).
					Return(project, true, nil)
			},
			op: models.AuditOpProjectCreate, resourceID: writeTestProjectID, resourceName: "Alpha",
			projectID: writeTestProjectID,
			check: func(t *testing.T, meta map[string]any) {
				assert.Equal(t, map[string]any{auditMetaVia: auditViaGitHubImport}, meta)
			},
		},
	}
}

// Each audited REST route records exactly one event with the operation, the
// resource and its names, attributed to the channel and API key of the
// request, and with no resource content in its metadata.
func TestAuditRESTRoutes_RecordOneEventPerWrite(t *testing.T) {
	cases := append(auditResourceRouteCases(), auditFeedProjectRouteCases()...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuditRouteFixture(t)
			tc.arrange(f)

			w := httptest.NewRecorder()
			f.router.ServeHTTP(w, auditRequest(tc.method, tc.path, tc.body))
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())

			got := f.rec.only(t)
			assert.Equal(t, tc.op, got.event.Operation)
			assert.Equal(t, writeTestTeamID, got.event.TeamID)
			assert.Equal(t, tc.resourceID, got.event.ResourceID)
			assert.Equal(t, tc.resourceName, got.event.ResourceName)
			assert.Equal(t, tc.projectID, got.event.ProjectID)
			require.NotNil(t, got.origin, "the route must carry a write origin")
			assert.Equal(t, resourceaccess.SourceCLI, got.origin.Source)
			assert.Equal(t, writeTestCallerRef, got.apiKey)
			assertNoContentLeak(t, got.event.Metadata, auditSecretContent, "SECRET-NEW-CONTENT")
			if tc.check != nil {
				tc.check(t, got.event.Metadata)
			}
		})
	}
}

// The channel follows the request's authentication: a cookie session is the
// web app and a plain API key the API.
func TestAuditRESTRoutes_ChannelFollowsAuthentication(t *testing.T) {
	channels := map[string]string{"cookie": resourceaccess.SourceWeb, "api_key": resourceaccess.SourceAPI}
	for authType, want := range channels {
		t.Run(authType, func(t *testing.T) {
			f := newAuditRouteFixture(t)
			f.memories.EXPECT().GetMemory(writeTestUserID, writeTestTeamID, writeTestMemoryID).
				Return(auditMemory(writeTestProjectID), nil)
			f.memories.EXPECT().DeleteMemory(mock.Anything, writeTestUserID, writeTestTeamID, writeTestMemoryID).Return(nil)

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/"+writeTestTeamID+"/memories/"+writeTestMemoryID, nil)
			ctx := context.WithValue(req.Context(), contextKeyUserID, writeTestUserID)
			ctx = context.WithValue(ctx, contextkeys.AuthType, authType)
			w := httptest.NewRecorder()
			f.router.ServeHTTP(w, req.WithContext(ctx))
			require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

			got := f.rec.only(t)
			require.NotNil(t, got.origin)
			assert.Equal(t, want, got.origin.Source)
		})
	}
}

// A refused write records nothing (a denial is recorded by the authorization
// service, not here).
func TestAuditRESTRoutes_FailedWriteRecordsNothing(t *testing.T) {
	errRefused := errors.New("refused")
	base := "/api/v1/" + writeTestTeamID
	cases := []struct {
		name, method, path, body string
		arrange                  func(f *auditRouteFixture)
	}{
		{"update prompt", http.MethodPut, base + "/prompts/p", `{"status":"archived"}`, func(f *auditRouteFixture) {
			f.prompts.EXPECT().GetPromptBySlug(writeTestUserID, writeTestTeamID, "p").
				Return(auditPrompt(writeTestProjectID, "active"), nil)
			f.prompts.EXPECT().UpdatePromptBySlug(mock.Anything, writeTestUserID, writeTestTeamID, "p", mock.Anything).
				Return(nil, errRefused)
		}},
		{"delete memory", http.MethodDelete, base + "/memories/" + writeTestMemoryID, "", func(f *auditRouteFixture) {
			f.memories.EXPECT().GetMemory(writeTestUserID, writeTestTeamID, writeTestMemoryID).
				Return(auditMemory(writeTestProjectID), nil)
			f.memories.EXPECT().DeleteMemory(mock.Anything, writeTestUserID, writeTestTeamID, writeTestMemoryID).
				Return(services.ErrPermissionDenied)
		}},
		{"delete feed", http.MethodDelete, base + "/feeds/" + auditTestFeedID, "", func(f *auditRouteFixture) {
			f.feeds.EXPECT().GetFeed(mock.Anything, writeTestUserID, writeTestTeamID, auditTestFeedID).
				Return(nil, errRefused)
			f.feeds.EXPECT().DeleteFeed(mock.Anything, writeTestUserID, writeTestTeamID, auditTestFeedID).
				Return(errRefused)
		}},
		{"create project", http.MethodPost, base + "/projects/", `{"name":"A","slug":"a"}`, func(f *auditRouteFixture) {
			f.projects.EXPECT().CreateProject(mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything).Return(nil, errRefused)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuditRouteFixture(t)
			tc.arrange(f)

			w := httptest.NewRecorder()
			f.router.ServeHTTP(w, auditRequest(tc.method, tc.path, tc.body))

			assert.GreaterOrEqual(t, w.Code, http.StatusBadRequest, w.Body.String())
			assert.Empty(t, f.rec.events())
		})
	}
}

// An update whose resource could not be read first still records the update,
// without a diff, and the update itself is unaffected.
func TestAuditRESTRoutes_UpdateWithoutBeforeReadRecordsBareUpdate(t *testing.T) {
	f := newAuditRouteFixture(t)
	f.prompts.EXPECT().GetPromptBySlug(writeTestUserID, writeTestTeamID, "p").Return(nil, errors.New("transient"))
	f.prompts.EXPECT().UpdatePromptBySlug(mock.Anything, writeTestUserID, writeTestTeamID, "p", mock.Anything).
		Return(auditPrompt(writeTestProjectID, "active"), nil)

	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, auditRequest(http.MethodPut, "/api/v1/"+writeTestTeamID+"/prompts/p", `{"body":"x"}`))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := f.rec.only(t)
	assert.Equal(t, models.AuditOpPromptUpdate, got.event.Operation)
	assert.Empty(t, got.event.Metadata)
}

// The GitHub blueprint import records one event per blueprint it wrote.
func TestRecordImportedBlueprintWrites_Audits(t *testing.T) {
	f := newAuditRouteFixture(t)
	f.srv.recordImportedBlueprintWrites(context.Background(), writeTestUserID, writeTestTeamID,
		&models.BlueprintImportReport{
			ProjectID:       writeTestProjectID,
			SuccessfulItems: []models.BlueprintImportSuccess{{BlueprintID: "bp-new", Title: "CLAUDE.md"}},
			UpdatedItems:    []models.BlueprintImportUpdated{{BlueprintID: "bp-old", Title: "AGENTS.md"}},
		})

	got := f.rec.events()
	require.Len(t, got, 2)
	assert.Equal(t, models.AuditOpBlueprintCreate, got[0].event.Operation)
	assert.Equal(t, "bp-new", got[0].event.ResourceID)
	assert.Equal(t, "CLAUDE.md", got[0].event.ResourceName)
	assert.Equal(t, writeTestProjectID, got[0].event.ProjectID)
	assert.Equal(t, map[string]any{auditMetaVia: auditViaGitHubImport}, got[0].event.Metadata)
	assert.Equal(t, models.AuditOpBlueprintUpdate, got[1].event.Operation)
	assert.Equal(t, "bp-old", got[1].event.ResourceID)
}

// ---------------------------------------------------------------------------
// MCP tools
// ---------------------------------------------------------------------------

type auditMCPCase struct {
	name                     string
	arrange                  func(f *auditRouteFixture)
	call                     func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error)
	op                       models.AuditOperation
	resourceID, resourceName string
}

//nolint:funlen // one table row per audited tool
func auditMCPCases() []auditMCPCase {
	mcpTeam := testTeamUUID
	user := testMemberUserID
	return []auditMCPCase{
		{
			name: "create_prompt",
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().CreatePrompt(mock.Anything, user, mcpTeam, mock.Anything).
					Return(auditPrompt(writeTestProjectID, "active"), nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.createPrompt(ctx, nil, &CreatePromptParams{
					TeamID: mcpTeam, ProjectID: writeTestProjectID, Name: "Release checklist", Slug: "p", Body: "x",
				}, user)
				return res, err
			},
			op: models.AuditOpPromptCreate, resourceID: auditTestPromptID, resourceName: "Release checklist",
		},
		{
			name: "update_prompt",
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().GetPromptBySlug(user, mcpTeam, "p").Return(auditPrompt(writeTestProjectID, "active"), nil)
				f.prompts.EXPECT().UpdatePromptBySlug(mock.Anything, user, mcpTeam, "p", mock.Anything).
					Return(auditPrompt(writeTestProjectID, "archived"), nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.updatePrompt(ctx, nil, &UpdatePromptParams{
					TeamID: mcpTeam, Slug: "p", Status: "archived",
				}, user)
				return res, err
			},
			op: models.AuditOpPromptUpdate, resourceID: auditTestPromptID, resourceName: "Release checklist",
		},
		{
			name: "create_artifact",
			arrange: func(f *auditRouteFixture) {
				f.artifacts.EXPECT().CreateArtifact(mock.Anything, user, mcpTeam, mock.Anything).Return(auditArtifact(), nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.createArtifact(ctx, nil, &CreateArtifactParams{
					TeamID: mcpTeam, ProjectID: writeTestProjectID, Slug: "a", Title: "Design doc", Content: "c",
				}, user)
				return res, err
			},
			op: models.AuditOpArtifactCreate, resourceID: "art-1", resourceName: "Design doc",
		},
		{
			name: "update_artifact",
			arrange: func(f *auditRouteFixture) {
				f.artifacts.EXPECT().GetArtifactByProjectIDAndSlugInTeam(user, mcpTeam, writeTestProjectID, "a").
					Return(auditArtifact(), nil)
				f.artifacts.EXPECT().UpdateArtifactByProjectIDAndSlugInTeam(mock.Anything, user, mcpTeam, writeTestProjectID, "a",
					mock.Anything).Return(auditArtifact(), nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.updateArtifact(ctx, nil, &UpdateArtifactParams{
					TeamID: mcpTeam, ProjectID: writeTestProjectID, Slug: "a", Content: "new",
				}, user)
				return res, err
			},
			op: models.AuditOpArtifactUpdate, resourceID: "art-1", resourceName: "Design doc",
		},
		{
			name: "create_blueprint",
			arrange: func(f *auditRouteFixture) {
				f.blueprints.EXPECT().CreateBlueprint(mock.Anything, user, mcpTeam, mock.Anything).Return(auditBlueprint(), nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.createBlueprint(ctx, nil, &CreateBlueprintParams{
					TeamID: mcpTeam, ProjectID: writeTestProjectID, Slug: "b", Title: "CLAUDE.md", Content: "c",
				}, user)
				return res, err
			},
			op: models.AuditOpBlueprintCreate, resourceID: "bp-1", resourceName: "CLAUDE.md",
		},
		{
			name: "update_blueprint",
			arrange: func(f *auditRouteFixture) {
				f.blueprints.EXPECT().GetBlueprintByProjectIDAndSlugInTeam(user, mcpTeam, writeTestProjectID, "b").
					Return(auditBlueprint(), nil)
				f.blueprints.EXPECT().UpdateBlueprintByProjectIDAndSlugInTeam(mock.Anything, user, mcpTeam, writeTestProjectID, "b",
					mock.Anything).Return(auditBlueprint(), nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.updateBlueprint(ctx, nil, &UpdateBlueprintParams{
					TeamID: mcpTeam, ProjectID: writeTestProjectID, Slug: "b", Content: "new",
				}, user)
				return res, err
			},
			op: models.AuditOpBlueprintUpdate, resourceID: "bp-1", resourceName: "CLAUDE.md",
		},
		{
			name: "create_memory",
			arrange: func(f *auditRouteFixture) {
				f.memories.EXPECT().CreateMemory(mock.Anything, user, mcpTeam, mock.Anything).Return(auditMemory(writeTestProjectID), nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.storeMemory(ctx, nil, &StoreMemoryParams{
					TeamID: mcpTeam, ProjectID: writeTestProjectID, Text: "remember",
				}, user)
				return res, err
			},
			op: models.AuditOpMemoryCreate, resourceID: writeTestMemoryID, resourceName: "Deploy notes",
		},
		{
			name: "update_memory",
			arrange: func(f *auditRouteFixture) {
				f.memories.EXPECT().GetMemory(user, mcpTeam, writeTestMemoryID).Return(auditMemory(writeTestProjectID), nil)
				f.memories.EXPECT().UpdateMemory(mock.Anything, user, mcpTeam, writeTestMemoryID, mock.Anything).
					Return(auditMemory(writeTestProjectID), nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.updateMemory(ctx, nil, &UpdateMemoryParams{
					TeamID: mcpTeam, MemoryID: writeTestMemoryID, Text: "new",
				}, user)
				return res, err
			},
			op: models.AuditOpMemoryUpdate, resourceID: writeTestMemoryID, resourceName: "Deploy notes",
		},
		{
			name: "delete_resource memory",
			arrange: func(f *auditRouteFixture) {
				f.memories.EXPECT().GetMemory(user, mcpTeam, writeTestMemoryID).Return(auditMemory(writeTestProjectID), nil)
				f.memories.EXPECT().DeleteMemory(mock.Anything, user, mcpTeam, writeTestMemoryID).Return(nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.deleteResource(ctx, nil, &DeleteResourceParams{
					TeamID: mcpTeam, ResourceType: resourceTypeMemory, ID: writeTestMemoryID,
				}, user)
				return res, err
			},
			op: models.AuditOpMemoryDelete, resourceID: writeTestMemoryID, resourceName: "Deploy notes",
		},
		{
			name: "delete_resource prompt",
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().GetPromptBySlug(user, mcpTeam, "p").Return(auditPrompt(writeTestProjectID, "active"), nil)
				f.prompts.EXPECT().DeletePromptBySlug(mock.Anything, user, mcpTeam, "p").Return(nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.deleteResource(ctx, nil, &DeleteResourceParams{
					TeamID: mcpTeam, ResourceType: resourceTypePrompt, Slug: "p",
				}, user)
				return res, err
			},
			op: models.AuditOpPromptDelete, resourceID: auditTestPromptID, resourceName: "Release checklist",
		},
		{
			name: "delete_resource artifact",
			arrange: func(f *auditRouteFixture) {
				f.artifacts.EXPECT().GetArtifactByProjectIDAndSlugInTeam(user, mcpTeam, writeTestProjectID, "a").
					Return(auditArtifact(), nil)
				f.artifacts.EXPECT().DeleteArtifactByProjectIDAndSlug(mock.Anything, user, mcpTeam, writeTestProjectID, "a").Return(nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.deleteResource(ctx, nil, &DeleteResourceParams{
					TeamID: mcpTeam, ResourceType: resourceTypeArtifact, ProjectID: writeTestProjectID, Slug: "a",
				}, user)
				return res, err
			},
			op: models.AuditOpArtifactDelete, resourceID: "art-1", resourceName: "Design doc",
		},
		{
			name: "delete_resource blueprint",
			arrange: func(f *auditRouteFixture) {
				f.blueprints.EXPECT().GetBlueprintByProjectIDAndSlugInTeam(user, mcpTeam, writeTestProjectID, "b").
					Return(auditBlueprint(), nil)
				f.blueprints.EXPECT().DeleteBlueprintByProjectIDAndSlug(mock.Anything, user, mcpTeam, writeTestProjectID, "b").Return(nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.deleteResource(ctx, nil, &DeleteResourceParams{
					TeamID: mcpTeam, ResourceType: resourceTypeBlueprint, ProjectID: writeTestProjectID, Slug: "b",
				}, user)
				return res, err
			},
			op: models.AuditOpBlueprintDelete, resourceID: "bp-1", resourceName: "CLAUDE.md",
		},
		{
			name: "lock_resource",
			arrange: func(f *auditRouteFixture) {
				f.prompts.EXPECT().GetPromptBySlug(user, mcpTeam, "p").Return(auditPrompt(writeTestProjectID, "active"), nil)
				f.locks.EXPECT().Lock(mock.Anything, user, mcpTeam, "prompt", auditTestPromptID, "frozen").Return(nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.lockResourceTool(ctx, nil, &LockResourceParams{
					TeamID: mcpTeam, ResourceType: resourceTypePrompt, Slug: "p", Comment: "frozen",
				}, user)
				return res, err
			},
			op: models.AuditOpPromptLock, resourceID: auditTestPromptID, resourceName: "Release checklist",
		},
		{
			name: "post_to_feed",
			arrange: func(f *auditRouteFixture) {
				f.items.EXPECT().CreateFeedItem(mock.Anything, user, mcpTeam, auditTestFeedID, mock.Anything).
					Return(&models.FeedItem{ID: auditTestItemID, FeedID: auditTestFeedID, Title: "Deploy done"}, nil)
			},
			call: func(f *auditRouteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.postToFeed(ctx, nil, &PostToFeedParams{
					TeamID: mcpTeam, FeedID: auditTestFeedID, Title: "Deploy done", Content: "x", AIAssistantName: "Claude",
				}, user)
				return res, err
			},
			op: models.AuditOpFeedItemCreate, resourceID: auditTestItemID, resourceName: "Deploy done",
		},
	}
}

// Every MCP write tool records one event attributed to MCP, also when the
// session authenticated with an OAuth token, which the REST derivation would
// call "api".
func TestAuditMCPTools_RecordAsMCP(t *testing.T) {
	for _, tc := range auditMCPCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuditRouteFixture(t)
			tc.arrange(f)

			ctx := context.WithValue(context.Background(), contextkeys.AuthType, "oauth")
			result, err := tc.call(f, ctx)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.IsError, extractText(t, result))

			got := f.rec.only(t)
			assert.Equal(t, tc.op, got.event.Operation)
			assert.Equal(t, testTeamUUID, got.event.TeamID)
			assert.Equal(t, tc.resourceID, got.event.ResourceID)
			assert.Equal(t, tc.resourceName, got.event.ResourceName)
			require.NotNil(t, got.origin)
			assert.Equal(t, resourceaccess.SourceMCP, got.origin.Source)
			assertNoContentLeak(t, got.event.Metadata, auditSecretContent)
		})
	}
}

// The tools this issue wraps in withMCPWriteOrigin (delete, lock, post to
// feed) carry the client's self-reported name to the audit log. A real SDK
// session is the only way to prove the name comes from the initialize request.
func TestAuditMCPTools_RecordClientNameFromSession(t *testing.T) {
	tools := []struct {
		name    string
		args    map[string]any
		arrange func(f *auditRouteFixture)
	}{
		{"vibexp_io_delete_resource", map[string]any{
			"team_id": testTeamUUID, "resource_type": resourceTypeMemory, "id": writeTestMemoryID,
		}, func(f *auditRouteFixture) {
			f.memories.EXPECT().GetMemory(testMemberUserID, testTeamUUID, writeTestMemoryID).
				Return(auditMemory(writeTestProjectID), nil)
			f.memories.EXPECT().DeleteMemory(mock.Anything, testMemberUserID, testTeamUUID, writeTestMemoryID).Return(nil)
		}},
		{"vibexp_io_lock_resource", map[string]any{
			"team_id": testTeamUUID, "resource_type": resourceTypePrompt, "slug": "p", "comment": "frozen",
		}, func(f *auditRouteFixture) {
			f.prompts.EXPECT().GetPromptBySlug(testMemberUserID, testTeamUUID, "p").
				Return(auditPrompt(writeTestProjectID, "active"), nil)
			f.locks.EXPECT().Lock(mock.Anything, testMemberUserID, testTeamUUID, "prompt", auditTestPromptID, "frozen").
				Return(nil)
		}},
		{"vibexp_io_post_to_feed", map[string]any{
			"team_id": testTeamUUID, "feed_id": auditTestFeedID, "title": "Deploy done", "content": "x",
			"ai_assistant_name": "Claude Code",
		}, func(f *auditRouteFixture) {
			f.items.EXPECT().CreateFeedItem(mock.Anything, testMemberUserID, testTeamUUID, auditTestFeedID, mock.Anything).
				Return(&models.FeedItem{ID: auditTestItemID, FeedID: auditTestFeedID, Title: "Deploy done"}, nil)
		}},
	}
	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			f := newAuditRouteFixture(t)
			tool.arrange(f)
			teams := mocks.NewMockTeamServiceInterface(t)
			stubListTeams(teams, []models.Team{memberTeam()})
			f.srv.container.(*auditTestContainer).TeamServiceMock = teams
			f.prompts.EXPECT().ListPrompts(testMemberUserID, mock.Anything).Return(&models.PromptListResponse{}, nil).Maybe()

			mcpHandler := f.srv.createMCPHandlerCommon()
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), contextkeys.UserID, testMemberUserID)
				mcpHandler.ServeHTTP(w, r.WithContext(ctx))
			}))
			t.Cleanup(httpServer.Close)

			client := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1.0.0"}, nil)
			session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
				Endpoint: httpServer.URL, HTTPClient: httpServer.Client(),
			}, nil)
			require.NoError(t, err)
			t.Cleanup(func() {
				if closeErr := session.Close(); closeErr != nil {
					t.Logf("failed to close MCP session: %v", closeErr)
				}
			})

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool.name, Arguments: tool.args})
			require.NoError(t, err)
			require.False(t, result.IsError, extractText(t, result))

			got := f.rec.only(t)
			require.NotNil(t, got.origin)
			assert.Equal(t, resourceaccess.SourceMCP, got.origin.Source)
			require.NotNil(t, got.origin.AssistantName)
			assert.Equal(t, "claude-code", *got.origin.AssistantName)
		})
	}
}
