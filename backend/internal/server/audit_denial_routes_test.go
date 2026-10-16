package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/audit"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// Permission denials carry the request they came from (#1541). The content
// services used to authorize on context.Background(), so the authz.denied row
// read source=system with no IP, user agent, request id or API key. These
// tests run the real routes, the real content services and the real
// AuthorizationService over mocked repositories, and read the row the real
// recorder writes.

const (
	denialTestIP        = "203.0.113.9"
	denialTestUserAgent = "Mozilla/5.0 (denial test)"
	denialTestRequestID = "req-denial-1541"
	denialTestOwnerID   = "user-someone-else"
	denialTestAPIKeyID  = "550e8400-e29b-41d4-a716-446655440099"
)

type denialFixture struct {
	srv         *Server
	router      chi.Router
	projectRepo *repomocks.MockProjectRepository
	memoryRepo  *repomocks.MockMemoryRepository

	mu   sync.Mutex
	rows []models.AuditEvent
}

func newDenialFixture(t *testing.T) *denialFixture {
	t.Helper()
	f := &denialFixture{
		projectRepo: repomocks.NewMockProjectRepository(t),
		memoryRepo:  repomocks.NewMockMemoryRepository(t),
	}
	logger := slog.New(slog.DiscardHandler)

	// The caller is a plain member: members may neither delete a project nor
	// another member's memory.
	members := repomocks.NewMockTeamMemberRepository(t)
	members.EXPECT().GetByTeamAndUser(mock.Anything, testTeamUUID, testMemberUserID).
		Return(&models.TeamMember{TeamID: testTeamUUID, UserID: testMemberUserID, Role: models.TeamMemberRoleMember}, nil)

	authzSvc := services.NewAuthorizationService(members, logger,
		services.WithAuthorizationAuditRecorder(audit.NewSyncRecorder(f.recorderDeps(t, logger))))

	teams := mocks.NewMockTeamServiceInterface(t)
	teams.EXPECT().IsUserMemberOfTeam(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()

	f.srv = New("8080", nil, "test-api-key", &config.Config{}, logger)
	f.srv.container = &auditTestContainer{
		TestContainer: TestContainer{
			ProjectServiceMock: services.NewProjectService(f.projectRepo, teams, authzSvc, nil, logger),
			MemoryServiceMock: services.NewMemoryService(services.MemoryServiceDeps{
				Repo: f.memoryRepo, TeamService: teams, Authz: authzSvc, Logger: logger,
			}),
			TeamServiceMock:    teams,
			TeamRepositoryMock: stubTeamResolution(t, []models.Team{memberTeam()}),
		},
		recorder: &auditCapture{},
	}

	// The middleware that puts the request id, client IP and user agent in
	// the context, as the server's root router does.
	f.router = chi.NewRouter()
	f.router.Use(RequestIDMiddleware(logger), clientIPMiddleware(nil), userAgentMiddleware)
	f.srv.setupProjectsRoutes(f.router)
	f.srv.mountMemoriesHandlers(f.router)
	return f
}

// recorderDeps is the production recorder over a repository that keeps every
// inserted row. Name lookups answer from mocks.
func (f *denialFixture) recorderDeps(t *testing.T, logger *slog.Logger) audit.RecorderDeps {
	t.Helper()
	repo := repomocks.NewMockAuditEventRepository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, e *models.AuditEvent) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.rows = append(f.rows, *e)
			return nil
		}).Maybe()
	users := repomocks.NewMockUserRepository(t)
	users.EXPECT().GetByID(mock.Anything, testMemberUserID).
		Return(&models.User{ID: testMemberUserID, Email: "member@example.com", Name: "Member"}, nil).Maybe()
	apiKeys := repomocks.NewMockAPIKeyRepository(t)
	apiKeys.EXPECT().GetNamesByIDs(mock.Anything, testMemberUserID, []string{denialTestAPIKeyID}).
		Return(map[string]string{denialTestAPIKeyID: "laptop key"}, nil).Maybe()
	teamRepo := repomocks.NewMockTeamRepository(t)
	teamRepo.EXPECT().GetNamesByIDs(mock.Anything, []string{testTeamUUID}).
		Return(map[string]string{testTeamUUID: "Acme Team"}, nil).Maybe()
	return audit.RecorderDeps{Repo: repo, Users: users, APIKeys: apiKeys, Teams: teamRepo, Logger: logger}
}

// onlyDenial returns the one authz.denied row.
func (f *denialFixture) onlyDenial(t *testing.T) models.AuditEvent {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.rows, 1, "exactly one audit row")
	row := f.rows[0]
	require.Equal(t, models.AuditOpAuthzDenied, row.Operation)
	require.Equal(t, models.AuditOutcomeDenied, row.Outcome)
	return row
}

// denialRequest is a request from 203.0.113.9 with a request id and a user
// agent, authenticated as the member by authType (and an API key when set).
func denialRequest(method, path, authType, apiKeyID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = denialTestIP + ":51515"
	req.Header.Set("User-Agent", denialTestUserAgent)
	req.Header.Set("X-Request-ID", denialTestRequestID)
	ctx := context.WithValue(req.Context(), contextKeyUserID, testMemberUserID)
	ctx = context.WithValue(ctx, contextkeys.AuthType, authType)
	if apiKeyID != "" {
		ctx = context.WithValue(ctx, contextkeys.APIKeyID, apiKeyID)
	}
	return req.WithContext(ctx)
}

// assertRequestFacts checks the client facts every denial row must carry.
func assertRequestFacts(t *testing.T, row models.AuditEvent) {
	t.Helper()
	require.NotNil(t, row.IP, "the client IP")
	assert.Equal(t, denialTestIP, *row.IP)
	require.NotNil(t, row.UserAgent, "the user agent")
	assert.Equal(t, denialTestUserAgent, *row.UserAgent)
	require.NotNil(t, row.RequestID, "the request id")
	assert.Equal(t, denialTestRequestID, *row.RequestID)
	require.NotNil(t, row.ActorUserID)
	assert.Equal(t, testMemberUserID, *row.ActorUserID)
	require.NotNil(t, row.TeamID)
	assert.Equal(t, testTeamUUID, *row.TeamID)
}

// A member deleting a project from the web app: the denial is source=web
// with the request's client facts.
func TestAuditDenial_ProjectDeleteOverWeb(t *testing.T) {
	f := newDenialFixture(t)
	f.projectRepo.EXPECT().GetBySlug(mock.Anything, testTeamUUID, testMemberUserID, "alpha").
		Return(&models.Project{ID: writeTestProjectID, TeamID: testTeamUUID, Slug: "alpha"}, nil)

	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, denialRequest(http.MethodDelete,
		"/api/v1/"+testTeamUUID+"/projects/alpha", "cookie", ""))

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	row := f.onlyDenial(t)
	assert.Equal(t, models.AuditSourceWeb, row.Source)
	assert.Nil(t, row.APIKeyID)
	assertRequestFacts(t, row)
}

// A member deleting someone else's memory with an API key: the denial is
// source=api and names the key.
func TestAuditDenial_MemoryDeleteOverAPIKey(t *testing.T) {
	f := newDenialFixture(t)
	f.memoryRepo.EXPECT().GetByID(mock.Anything, testMemberUserID, testTeamUUID, writeTestMemoryID).
		Return(&models.Memory{
			ID: writeTestMemoryID, TeamID: testTeamUUID, UserID: denialTestOwnerID, ProjectID: writeTestProjectID,
		}, nil)

	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, denialRequest(http.MethodDelete,
		"/api/v1/"+testTeamUUID+"/memories/"+writeTestMemoryID, "api_key", denialTestAPIKeyID))

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	row := f.onlyDenial(t)
	assert.Equal(t, models.AuditSourceAPI, row.Source)
	require.NotNil(t, row.APIKeyID)
	assert.Equal(t, denialTestAPIKeyID, *row.APIKeyID)
	require.NotNil(t, row.APIKeyName)
	assert.Equal(t, "laptop key", *row.APIKeyName)
	assertRequestFacts(t, row)
}

// The same delete through the MCP delete tool: the denial is source=mcp. The
// client facts come from the HTTP request the MCP transport rides on.
func TestAuditDenial_MemoryDeleteOverMCP(t *testing.T) {
	f := newDenialFixture(t)
	f.memoryRepo.EXPECT().GetByID(mock.Anything, testMemberUserID, testTeamUUID, writeTestMemoryID).
		Return(&models.Memory{
			ID: writeTestMemoryID, TeamID: testTeamUUID, UserID: denialTestOwnerID, ProjectID: writeTestProjectID,
		}, nil)

	ctx := denialRequest(http.MethodPost, "/mcp", "oauth", "").Context()
	ctx = context.WithValue(ctx, contextkeys.RequestID, denialTestRequestID)
	ctx = context.WithValue(ctx, contextkeys.ClientIP, denialTestIP)
	ctx = context.WithValue(ctx, contextkeys.UserAgent, denialTestUserAgent)
	result, _, err := f.srv.deleteResource(ctx, nil, &DeleteResourceParams{
		TeamID: testTeamUUID, ResourceType: resourceTypeMemory, ID: writeTestMemoryID,
	}, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError, "the delete is refused")
	row := f.onlyDenial(t)
	assert.Equal(t, models.AuditSourceMCP, row.Source)
	assertRequestFacts(t, row)
}
