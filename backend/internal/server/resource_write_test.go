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

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	ramocks "github.com/vibexp/vibexp/internal/services/resourceaccess/mocks"
)

const (
	writeTestTeamID    = "550e8400-e29b-41d4-a716-446655440000"
	writeTestProjectID = "550e8400-e29b-41d4-a716-446655440010"
	writeTestUserID    = "user-write"
	writeTestMemoryID  = "550e8400-e29b-41d4-a716-446655440020"
	writeTestCallerRef = "550e8400-e29b-41d4-a716-446655440030"
	writeTestCLIAgent  = "VibeXP-CLI/1.4.0"
)

// writeTestContainer wires the four resource services, the project lookup the
// memory handlers validate against, and the write service under test.
type writeTestContainer struct {
	BaseMockContainer
	prompts    services.PromptServiceInterface
	artifacts  services.ArtifactServiceInterface
	blueprints services.BlueprintServiceInterface
	memories   services.MemoryServiceInterface
	projects   repositories.ProjectRepository
	types      services.TypeServiceInterface
	writes     resourceaccess.ResourceWriteService
}

func (c *writeTestContainer) PromptService() services.PromptServiceInterface     { return c.prompts }
func (c *writeTestContainer) ArtifactService() services.ArtifactServiceInterface { return c.artifacts }
func (c *writeTestContainer) BlueprintService() services.BlueprintServiceInterface {
	return c.blueprints
}
func (c *writeTestContainer) MemoryService() services.MemoryServiceInterface    { return c.memories }
func (c *writeTestContainer) ProjectRepository() repositories.ProjectRepository { return c.projects }
func (c *writeTestContainer) TypeService() services.TypeServiceInterface        { return c.types }
func (c *writeTestContainer) ResourceWriteService() resourceaccess.ResourceWriteService {
	return c.writes
}

// ActivityService is the no-op stub the memory handler tests use: the memory
// handlers record an activity unconditionally.
func (c *writeTestContainer) ActivityService() activities.ActivityService {
	return &mockActivityService{}
}

// captureWrites returns a write-service mock that accepts any number of
// RecordWrite calls, and a pointer to the events it received.
func captureWrites(t *testing.T) (*ramocks.MockResourceWriteService, *[]*models.ResourceWriteEvent) {
	t.Helper()
	svc := ramocks.NewMockResourceWriteService(t)
	var events []*models.ResourceWriteEvent
	svc.EXPECT().RecordWrite(mock.Anything).
		Run(func(event *models.ResourceWriteEvent) { events = append(events, event) }).
		Maybe()
	return svc, &events
}

// ---------------------------------------------------------------------------
// annotateWriteSource
// ---------------------------------------------------------------------------

// The channel of a REST write comes from how the request was authenticated and
// routed, never from the request itself, and a REST write never carries an
// assistant name.
func TestAnnotateWriteSource_DerivesChannelFromAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		authType  string
		userAgent string
		want      string
	}{
		{name: "session cookie is the web app", authType: "cookie", userAgent: "Mozilla/5.0", want: "web"},
		{name: "cookie wins over a CLI user agent", authType: "cookie", userAgent: writeTestCLIAgent, want: "web"},
		{name: "API key with the CLI user agent is the CLI", authType: "api_key", userAgent: writeTestCLIAgent, want: "cli"},
		{name: "bare API key is the API", authType: "api_key", userAgent: "curl/8.0", want: "api"},
		{name: "OAuth bearer over REST is the API", authType: "oauth", userAgent: "okhttp/4", want: "api"},
		{name: "no auth type falls back to the API", authType: "", userAgent: "", want: "api"},
	}

	srv := &Server{logger: slog.New(slog.DiscardHandler)}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got *models.WriteOrigin
			handler := srv.annotateWriteSource(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = resourceaccess.WriteOriginFromContext(r.Context())
			}))

			// A body that claims a channel and an assistant must change nothing.
			body := strings.NewReader(`{"source":"mcp","assistant_name":"claude-code","write_origin":{"source":"mcp"}}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/"+writeTestTeamID+"/memories", body)
			req.Header.Set("User-Agent", tt.userAgent)
			if tt.authType != "" {
				req = req.WithContext(context.WithValue(req.Context(), contextkeys.AuthType, tt.authType))
			}

			handler.ServeHTTP(httptest.NewRecorder(), req)

			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.Source)
			assert.Nil(t, got.AssistantName)
		})
	}
}

// ---------------------------------------------------------------------------
// MCP origin
// ---------------------------------------------------------------------------

func TestClientNameFromInitialize(t *testing.T) {
	t.Parallel()

	assert.Empty(t, clientNameFromInitialize(nil))
	assert.Empty(t, clientNameFromInitialize(&mcp.InitializeParams{}))
	assert.Equal(t, "claude-code", clientNameFromInitialize(&mcp.InitializeParams{
		ClientInfo: &mcp.Implementation{Name: "claude-code", Version: "2.0.0"},
	}))
}

// A tool call with no request or no session is still an MCP write; it just has
// no client name to report.
func TestWithMCPWriteOrigin_NilSafe(t *testing.T) {
	t.Parallel()

	for name, req := range map[string]*mcp.CallToolRequest{
		"nil request": nil,
		"nil session": {},
	} {
		origin := resourceaccess.WriteOriginFromContext(withMCPWriteOrigin(context.Background(), req))

		require.NotNil(t, origin, name)
		assert.Equal(t, resourceaccess.SourceMCP, origin.Source, name)
		assert.Nil(t, origin.AssistantName, name)
	}

	assert.Empty(t, mcpClientName(nil))
}

// ---------------------------------------------------------------------------
// recordResourceWrite
// ---------------------------------------------------------------------------

func TestRecordResourceWrite_BuildsEventFromContext(t *testing.T) {
	t.Parallel()

	writes, events := captureWrites(t)
	srv := &Server{container: &writeTestContainer{writes: writes}, logger: slog.New(slog.DiscardHandler)}

	ctx := resourceaccess.WithWriteOrigin(context.Background(), resourceaccess.SourceMCP, "claude-code")
	ctx = context.WithValue(ctx, contextkeys.APIKeyID, writeTestCallerRef)

	srv.recordResourceWrite(ctx, writeTestUserID, models.WriteActionUpdate,
		writtenMemory(writeTestTeamID, &models.Memory{ID: writeTestMemoryID, ProjectID: writeTestProjectID}))

	require.Len(t, *events, 1)
	event := (*events)[0]
	assert.Equal(t, writeTestTeamID, event.TeamID)
	assert.Equal(t, "memory", event.ResourceType)
	assert.Equal(t, writeTestMemoryID, event.ResourceID)
	assert.Equal(t, models.WriteActionUpdate, event.Action)
	assert.Equal(t, resourceaccess.SourceMCP, event.Source)
	require.NotNil(t, event.AssistantName)
	assert.Equal(t, "claude-code", *event.AssistantName)
	require.NotNil(t, event.ProjectID)
	assert.Equal(t, writeTestProjectID, *event.ProjectID)
	require.NotNil(t, event.UserID)
	assert.Equal(t, writeTestUserID, *event.UserID)
	require.NotNil(t, event.APIKeyID)
	assert.Equal(t, writeTestCallerRef, *event.APIKeyID)
}

// Optional attribution is left NULL rather than stored as an empty string: the
// columns are uuid, and "" would fail the insert and drop the event.
func TestRecordResourceWrite_OmitsAbsentAttribution(t *testing.T) {
	t.Parallel()

	writes, events := captureWrites(t)
	srv := &Server{container: &writeTestContainer{writes: writes}, logger: slog.New(slog.DiscardHandler)}

	ctx := resourceaccess.WithWriteOrigin(context.Background(), resourceaccess.SourceWeb, "")
	srv.recordResourceWrite(ctx, "", models.WriteActionCreate,
		writtenPrompt(writeTestTeamID, &models.Prompt{ID: "prompt-1"}))

	require.Len(t, *events, 1)
	event := (*events)[0]
	assert.Equal(t, "prompt", event.ResourceType)
	assert.Nil(t, event.ProjectID)
	assert.Nil(t, event.UserID)
	assert.Nil(t, event.APIKeyID)
	assert.Nil(t, event.AssistantName)
}

// A caller that is not a request has no write origin in its context (a
// scheduled job, a migration): its write records no event, and it leaves the
// model's last write as the repository resolved it rather than blanking it.
func TestRecordResourceWrite_NoOriginRecordsNothing(t *testing.T) {
	t.Parallel()

	// No RecordWrite expectation: a call would fail the test.
	writes := ramocks.NewMockResourceWriteService(t)
	srv := &Server{container: &writeTestContainer{writes: writes}, logger: slog.New(slog.DiscardHandler)}

	previous := resourceaccess.SourceWeb
	artifact := &models.Artifact{
		ID: "art-1", ProjectID: writeTestProjectID, LastWrite: models.LastWrite{LastWriteSource: &previous},
	}
	srv.recordResourceWrite(context.Background(), writeTestUserID, models.WriteActionCreate,
		writtenArtifact(writeTestTeamID, artifact))

	require.NotNil(t, artifact.LastWriteSource)
	assert.Equal(t, resourceaccess.SourceWeb, *artifact.LastWriteSource)
}

// The write being recorded is the resource's latest, so its origin replaces
// whatever the model carried (#1418): the previous write's channel when the
// service read the row back, or nothing for a create. The event lands off the
// request path, so the response cannot wait to read it.
func TestRecordResourceWrite_StampsLastWriteOnTheModel(t *testing.T) {
	t.Parallel()

	writes, _ := captureWrites(t)
	srv := &Server{container: &writeTestContainer{writes: writes}, logger: slog.New(slog.DiscardHandler)}

	previous, previousAssistant := resourceaccess.SourceMCP, "claude-code"
	prompt := &models.Prompt{ID: "prompt-1", LastWrite: models.LastWrite{
		LastWriteSource: &previous, LastWriteAssistantName: &previousAssistant,
	}}

	ctx := resourceaccess.WithWriteOrigin(context.Background(), resourceaccess.SourceWeb, "")
	srv.recordResourceWrite(ctx, writeTestUserID, models.WriteActionUpdate, writtenPrompt(writeTestTeamID, prompt))

	require.NotNil(t, prompt.LastWriteSource)
	assert.Equal(t, resourceaccess.SourceWeb, *prompt.LastWriteSource)
	// The MCP client's name belonged to the previous write, not this one.
	assert.Nil(t, prompt.LastWriteAssistantName)
}

// A GitHub import writes blueprints through the repository, so the handler
// records the events itself (#1418): a create per imported blueprint, an update
// per refreshed one, and nothing for a blueprint the import left alone.
func TestRecordImportedBlueprintWrites(t *testing.T) {
	t.Parallel()

	writes, events := captureWrites(t)
	srv := &Server{container: &writeTestContainer{writes: writes}, logger: slog.New(slog.DiscardHandler)}
	ctx := resourceaccess.WithWriteOrigin(context.Background(), resourceaccess.SourceWeb, "")

	srv.recordImportedBlueprintWrites(ctx, writeTestUserID, writeTestTeamID, &models.BlueprintImportReport{
		ProjectID:       writeTestProjectID,
		SuccessfulItems: []models.BlueprintImportSuccess{{BlueprintID: "bp-new-1"}, {BlueprintID: "bp-new-2"}},
		UpdatedItems:    []models.BlueprintImportUpdated{{BlueprintID: "bp-refreshed"}},
		UpToDateItems:   []models.BlueprintImportUpToDate{{}},
		ConflictItems:   []models.BlueprintImportConflict{{}},
		SkippedItems:    []models.BlueprintImportSkipped{{}},
		FailedItems:     []models.BlueprintImportFailed{{}},
	})

	require.Len(t, *events, 3)
	got := make(map[string]string, len(*events))
	for _, event := range *events {
		got[event.ResourceID] = event.Action
		assert.Equal(t, "blueprint", event.ResourceType)
		assert.Equal(t, resourceaccess.SourceWeb, event.Source)
		assert.Equal(t, writeTestTeamID, event.TeamID)
		require.NotNil(t, event.ProjectID)
		assert.Equal(t, writeTestProjectID, *event.ProjectID)
	}
	assert.Equal(t, map[string]string{
		"bp-new-1":     models.WriteActionCreate,
		"bp-new-2":     models.WriteActionCreate,
		"bp-refreshed": models.WriteActionUpdate,
	}, got)
}

// An import that failed before it produced a report has nothing to record.
func TestRecordImportedBlueprintWrites_NilReport(t *testing.T) {
	t.Parallel()

	writes := ramocks.NewMockResourceWriteService(t)
	srv := &Server{container: &writeTestContainer{writes: writes}, logger: slog.New(slog.DiscardHandler)}
	ctx := resourceaccess.WithWriteOrigin(context.Background(), resourceaccess.SourceWeb, "")

	assert.NotPanics(t, func() {
		srv.recordImportedBlueprintWrites(ctx, writeTestUserID, writeTestTeamID, nil)
	})
}

func TestRecordResourceWrite_NilServiceIsSafe(t *testing.T) {
	t.Parallel()

	srv := &Server{container: &writeTestContainer{}, logger: slog.New(slog.DiscardHandler)}
	ctx := resourceaccess.WithWriteOrigin(context.Background(), resourceaccess.SourceWeb, "")

	assert.NotPanics(t, func() {
		srv.recordResourceWrite(ctx, writeTestUserID, models.WriteActionCreate,
			writtenBlueprint(writeTestTeamID, &models.Blueprint{ID: "bp-1", ProjectID: writeTestProjectID}))
	})
}

// ---------------------------------------------------------------------------
// MCP write tools
// ---------------------------------------------------------------------------

// mcpWriteFixture is a server whose four resource services are mocks, with the
// member user resolving to testTeamUUID.
type mcpWriteFixture struct {
	srv        *Server
	prompts    *mocks.MockPromptServiceInterface
	artifacts  *mocks.MockArtifactServiceInterface
	blueprints *mocks.MockBlueprintServiceInterface
	memories   *mocks.MockMemoryServiceInterface
	events     *[]*models.ResourceWriteEvent
}

func newMCPWriteFixture(t *testing.T) *mcpWriteFixture {
	t.Helper()

	f := &mcpWriteFixture{
		srv:        newServerWithNullLogger(t),
		prompts:    mocks.NewMockPromptServiceInterface(t),
		artifacts:  mocks.NewMockArtifactServiceInterface(t),
		blueprints: mocks.NewMockBlueprintServiceInterface(t),
		memories:   mocks.NewMockMemoryServiceInterface(t),
	}
	writes, events := captureWrites(t)
	f.events = events
	f.srv.container = &mcpWriteContainer{
		TestContainer: TestContainer{
			PromptServiceMock:    f.prompts,
			ArtifactServiceMock:  f.artifacts,
			BlueprintServiceMock: f.blueprints,
			MemoryServiceMock:    f.memories,
			TeamRepositoryMock:   stubTeamResolution(t, []models.Team{memberTeam()}),
		},
		writes: writes,
	}
	return f
}

// mcpWriteContainer is the MCP TestContainer plus the write service.
type mcpWriteContainer struct {
	TestContainer
	writes resourceaccess.ResourceWriteService
}

func (c *mcpWriteContainer) ResourceWriteService() resourceaccess.ResourceWriteService {
	return c.writes
}

// isMCPOrigin reports whether an update request carries the MCP write origin
// the tool layer must set on it, so the version it produces is labelled too.
func isMCPOrigin(origin *models.WriteOrigin) bool {
	return origin != nil && origin.Source == resourceaccess.SourceMCP
}

// returnOrFail completes a write-service expectation: the resource and a nil
// error on success, or an untyped nil and err when fail is set.
func returnOrFail(call *mock.Call, fail bool, resource any, err error) {
	if fail {
		call.Return(nil, err)
		return
	}
	call.Return(resource, nil)
}

type mcpWriteCase struct {
	name         string
	resourceType string
	action       string
	resourceID   string
	// arrange sets the service expectation; fail makes the service return an
	// error instead of a resource.
	arrange func(f *mcpWriteFixture, fail bool)
	call    func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error)
}

func mcpWriteCases() []mcpWriteCase {
	errWrite := errors.New("write refused")
	prompt := &models.Prompt{ID: "prompt-1", ProjectID: writeTestProjectID, Slug: "p"}
	artifact := &models.Artifact{ID: "art-1", ProjectID: writeTestProjectID, Slug: "a"}
	blueprint := &models.Blueprint{ID: "bp-1", ProjectID: writeTestProjectID, Slug: "b"}
	memory := &models.Memory{ID: "mem-1", ProjectID: writeTestProjectID}

	return []mcpWriteCase{
		{
			name: "create_prompt", resourceType: "prompt", action: models.WriteActionCreate, resourceID: prompt.ID,
			arrange: func(f *mcpWriteFixture, fail bool) {
				call := f.prompts.On("CreatePrompt", mock.Anything, testMemberUserID, testTeamUUID, mock.Anything)
				returnOrFail(call, fail, prompt, errWrite)
			},
			call: func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.createPrompt(ctx, nil, &CreatePromptParams{
					TeamID: testTeamUUID, ProjectID: writeTestProjectID, Name: "P", Slug: "p", Body: "body",
				}, testMemberUserID)
				return res, err
			},
		},
		{
			name: "update_prompt", resourceType: "prompt", action: models.WriteActionUpdate, resourceID: prompt.ID,
			arrange: func(f *mcpWriteFixture, fail bool) {
				call := f.prompts.On("UpdatePromptBySlug", mock.Anything, testMemberUserID, testTeamUUID, "p",
					mock.MatchedBy(func(req *models.UpdatePromptRequest) bool { return isMCPOrigin(req.WriteOrigin) }))
				returnOrFail(call, fail, prompt, errWrite)
			},
			call: func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.updatePrompt(ctx, nil, &UpdatePromptParams{
					TeamID: testTeamUUID, Slug: "p", Body: "new body",
				}, testMemberUserID)
				return res, err
			},
		},
		{
			name: "create_artifact", resourceType: "artifact", action: models.WriteActionCreate, resourceID: artifact.ID,
			arrange: func(f *mcpWriteFixture, fail bool) {
				call := f.artifacts.On("CreateArtifact", mock.Anything, testMemberUserID, testTeamUUID, mock.Anything)
				returnOrFail(call, fail, artifact, errWrite)
			},
			call: func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.createArtifact(ctx, nil, &CreateArtifactParams{
					TeamID: testTeamUUID, ProjectID: writeTestProjectID, Slug: "a", Title: "A", Content: "c",
				}, testMemberUserID)
				return res, err
			},
		},
		{
			name: "update_artifact", resourceType: "artifact", action: models.WriteActionUpdate, resourceID: artifact.ID,
			arrange: func(f *mcpWriteFixture, fail bool) {
				call := f.artifacts.On("UpdateArtifactByProjectIDAndSlugInTeam", mock.Anything,
					testMemberUserID, testTeamUUID, writeTestProjectID, "a",
					mock.MatchedBy(func(req *models.UpdateArtifactRequest) bool { return isMCPOrigin(req.WriteOrigin) }))
				returnOrFail(call, fail, artifact, errWrite)
			},
			call: func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.updateArtifact(ctx, nil, &UpdateArtifactParams{
					TeamID: testTeamUUID, ProjectID: writeTestProjectID, Slug: "a", Content: "new",
				}, testMemberUserID)
				return res, err
			},
		},
		{
			name: "create_blueprint", resourceType: "blueprint", action: models.WriteActionCreate, resourceID: blueprint.ID,
			arrange: func(f *mcpWriteFixture, fail bool) {
				call := f.blueprints.On("CreateBlueprint", mock.Anything, testMemberUserID, testTeamUUID, mock.Anything)
				returnOrFail(call, fail, blueprint, errWrite)
			},
			call: func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.createBlueprint(ctx, nil, &CreateBlueprintParams{
					TeamID: testTeamUUID, ProjectID: writeTestProjectID, Slug: "b", Title: "B", Content: "c",
				}, testMemberUserID)
				return res, err
			},
		},
		{
			name: "update_blueprint", resourceType: "blueprint", action: models.WriteActionUpdate, resourceID: blueprint.ID,
			arrange: func(f *mcpWriteFixture, fail bool) {
				call := f.blueprints.On("UpdateBlueprintByProjectIDAndSlugInTeam", mock.Anything,
					testMemberUserID, testTeamUUID, writeTestProjectID, "b",
					mock.MatchedBy(func(req *models.UpdateBlueprintRequest) bool { return isMCPOrigin(req.WriteOrigin) }))
				returnOrFail(call, fail, blueprint, errWrite)
			},
			call: func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.updateBlueprint(ctx, nil, &UpdateBlueprintParams{
					TeamID: testTeamUUID, ProjectID: writeTestProjectID, Slug: "b", Content: "new",
				}, testMemberUserID)
				return res, err
			},
		},
		{
			name: "create_memory", resourceType: "memory", action: models.WriteActionCreate, resourceID: memory.ID,
			arrange: func(f *mcpWriteFixture, fail bool) {
				call := f.memories.On("CreateMemory", mock.Anything, testMemberUserID, testTeamUUID, mock.Anything)
				returnOrFail(call, fail, memory, errWrite)
			},
			call: func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.storeMemory(ctx, nil, &StoreMemoryParams{
					TeamID: testTeamUUID, ProjectID: writeTestProjectID, Text: "remember",
				}, testMemberUserID)
				return res, err
			},
		},
		{
			name: "update_memory", resourceType: "memory", action: models.WriteActionUpdate, resourceID: memory.ID,
			arrange: func(f *mcpWriteFixture, fail bool) {
				call := f.memories.On("UpdateMemory", testMemberUserID, testTeamUUID, memory.ID,
					mock.MatchedBy(func(req *models.UpdateMemoryRequest) bool { return isMCPOrigin(req.WriteOrigin) }))
				returnOrFail(call, fail, memory, errWrite)
			},
			call: func(f *mcpWriteFixture, ctx context.Context) (*mcp.CallToolResult, error) {
				res, _, err := f.srv.updateMemory(ctx, nil, &UpdateMemoryParams{
					TeamID: testTeamUUID, MemoryID: memory.ID, Text: "new",
				}, testMemberUserID)
				return res, err
			},
		},
	}
}

// Every MCP write tool records one event with source "mcp", whatever the
// request was authenticated with: an OAuth bearer token, which DeriveSource
// would classify as "api", is the case that must not leak through.
func TestMCPWriteTools_RecordWriteAsMCP(t *testing.T) {
	for _, authType := range []string{"api_key", "oauth"} {
		for _, tc := range mcpWriteCases() {
			t.Run(authType+"/"+tc.name, func(t *testing.T) {
				f := newMCPWriteFixture(t)
				tc.arrange(f, false)

				ctx := context.WithValue(context.Background(), contextkeys.AuthType, authType)
				result, err := tc.call(f, ctx)

				require.NoError(t, err)
				require.NotNil(t, result)
				require.False(t, result.IsError, extractText(t, result))

				require.Len(t, *f.events, 1)
				event := (*f.events)[0]
				assert.Equal(t, resourceaccess.SourceMCP, event.Source)
				assert.Equal(t, tc.action, event.Action)
				assert.Equal(t, tc.resourceType, event.ResourceType)
				assert.Equal(t, tc.resourceID, event.ResourceID)
				assert.Equal(t, testTeamUUID, event.TeamID)
				require.NotNil(t, event.ProjectID)
				assert.Equal(t, writeTestProjectID, *event.ProjectID)
				require.NotNil(t, event.UserID)
				assert.Equal(t, testMemberUserID, *event.UserID)
				// No session, so no client name to report.
				assert.Nil(t, event.AssistantName)
			})
		}
	}
}

// A write the service refused records no event.
func TestMCPWriteTools_FailedWriteRecordsNothing(t *testing.T) {
	for _, tc := range mcpWriteCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newMCPWriteFixture(t)
			tc.arrange(f, true)

			result, err := tc.call(f, context.Background())

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			assert.Empty(t, *f.events)
		})
	}
}

// A real MCP client session: the name the client sends in its initialize
// request reaches the write event, trimmed and capped. This is the only test
// that goes through the SDK, so it is the one that proves the session lookup.
func TestMCPWriteTools_RecordClientNameFromSession(t *testing.T) {
	tests := []struct {
		name       string
		clientName string
		want       *string
	}{
		{name: "reported name is stored", clientName: "claude-code", want: strPtr("claude-code")},
		{
			name:       "long name is capped at 30 characters",
			clientName: "an-assistant-with-a-very-long-product-name",
			want:       strPtr("an-assistant-with-a-very-long-"),
		},
		{name: "blank name is stored as not reported", clientName: "   ", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMCPWriteFixture(t)
			f.memories.On("CreateMemory", mock.Anything, testMemberUserID, testTeamUUID, mock.Anything).
				Return(&models.Memory{ID: "mem-1", ProjectID: writeTestProjectID}, nil)

			// The populated server lists the user's teams and prompts when the
			// session is created.
			teams := mocks.NewMockTeamServiceInterface(t)
			stubListTeams(teams, []models.Team{memberTeam()})
			f.srv.container.(*mcpWriteContainer).TeamServiceMock = teams
			f.prompts.On("ListPrompts", testMemberUserID, mock.Anything).
				Return(&models.PromptListResponse{}, nil).Maybe()

			mcpHandler := f.srv.createMCPHandlerCommon()
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), contextkeys.UserID, testMemberUserID)
				mcpHandler.ServeHTTP(w, r.WithContext(ctx))
			}))
			t.Cleanup(httpServer.Close)

			client := mcp.NewClient(&mcp.Implementation{Name: tt.clientName, Version: "1.0.0"}, nil)
			session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
				Endpoint:   httpServer.URL,
				HTTPClient: httpServer.Client(),
			}, nil)
			require.NoError(t, err)
			t.Cleanup(func() {
				if closeErr := session.Close(); closeErr != nil {
					t.Logf("failed to close MCP session: %v", closeErr)
				}
			})

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "vibexp_io_create_memory",
				Arguments: map[string]any{
					"team_id": testTeamUUID, "project_id": writeTestProjectID, "text": "remember this",
				},
			})
			require.NoError(t, err)
			require.False(t, result.IsError, extractText(t, result))

			require.Len(t, *f.events, 1)
			event := (*f.events)[0]
			assert.Equal(t, resourceaccess.SourceMCP, event.Source)
			if tt.want == nil {
				assert.Nil(t, event.AssistantName)
				return
			}
			require.NotNil(t, event.AssistantName)
			assert.Equal(t, *tt.want, *event.AssistantName)
		})
	}
}

// ---------------------------------------------------------------------------
// REST write handlers, through the real route registrations
// ---------------------------------------------------------------------------

type restWriteFixture struct {
	router     chi.Router
	prompts    *mocks.MockPromptServiceInterface
	artifacts  *mocks.MockArtifactServiceInterface
	blueprints *mocks.MockBlueprintServiceInterface
	memories   *mocks.MockMemoryServiceInterface
	events     *[]*models.ResourceWriteEvent
}

// newRESTWriteFixture mounts the four domains' real route registrations (the
// mount helpers server.go uses, minus the tenancy middleware), so a request
// goes through annotateWriteSource exactly where production mounts it.
func newRESTWriteFixture(t *testing.T) *restWriteFixture {
	t.Helper()

	f := &restWriteFixture{
		prompts:    mocks.NewMockPromptServiceInterface(t),
		artifacts:  mocks.NewMockArtifactServiceInterface(t),
		blueprints: mocks.NewMockBlueprintServiceInterface(t),
		memories:   mocks.NewMockMemoryServiceInterface(t),
	}
	writes, events := captureWrites(t)
	f.events = events

	projects := repomocks.NewMockProjectRepository(t)
	projects.EXPECT().GetByID(mock.Anything, writeTestUserID, writeTestProjectID).
		Return(&models.Project{ID: writeTestProjectID, TeamID: writeTestTeamID}, nil).Maybe()

	types := mocks.NewMockTypeServiceInterface(t)
	types.EXPECT().ValidateType(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()

	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	srv.container = &writeTestContainer{
		prompts: f.prompts, artifacts: f.artifacts, blueprints: f.blueprints, memories: f.memories,
		projects: projects, types: types, writes: writes,
	}

	f.router = chi.NewRouter()
	srv.mountPromptsHandlers(f.router)
	srv.mountArtifactsHandlers(f.router)
	srv.mountBlueprintsHandlers(f.router)
	srv.mountMemoriesHandlers(f.router)
	return f
}

type restWriteCase struct {
	name         string
	method       string
	path         string
	body         string
	wantStatus   int
	resourceType string
	action       string
	resourceID   string
	arrange      func(f *restWriteFixture, fail bool)
}

//nolint:funlen // one table row per write route
func restWriteCases() []restWriteCase {
	errWrite := errors.New("write refused")
	base := "/api/v1/" + writeTestTeamID
	prompt := &models.Prompt{ID: "prompt-1", ProjectID: writeTestProjectID, Slug: "p", Name: "P"}
	artifact := &models.Artifact{ID: "art-1", ProjectID: writeTestProjectID, Slug: "a", Title: "A"}
	blueprint := &models.Blueprint{ID: "bp-1", ProjectID: writeTestProjectID, Slug: "b", Title: "B"}
	memory := &models.Memory{ID: writeTestMemoryID, ProjectID: writeTestProjectID, Text: "t"}

	// isRESTOrigin: the update request must carry the channel the middleware
	// derived, with no assistant name.
	isRESTOrigin := func(origin *models.WriteOrigin) bool {
		return origin != nil && origin.Source == resourceaccess.SourceCLI && origin.AssistantName == nil
	}

	return []restWriteCase{
		{
			name: "create prompt", method: http.MethodPost, path: base + "/prompts",
			body:       `{"name":"P","slug":"p","body":"hello","project_id":"` + writeTestProjectID + `"}`,
			wantStatus: http.StatusCreated, resourceType: "prompt", action: models.WriteActionCreate, resourceID: prompt.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.prompts.On("CreatePrompt", mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything)
				returnOrFail(call, fail, prompt, errWrite)
			},
		},
		{
			name: "update prompt", method: http.MethodPut, path: base + "/prompts/p",
			body:       `{"body":"new body"}`,
			wantStatus: http.StatusOK, resourceType: "prompt", action: models.WriteActionUpdate, resourceID: prompt.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.prompts.On("UpdatePromptBySlug", mock.Anything, writeTestUserID, writeTestTeamID, "p",
					mock.MatchedBy(func(req *models.UpdatePromptRequest) bool { return isRESTOrigin(req.WriteOrigin) }))
				returnOrFail(call, fail, prompt, errWrite)
			},
		},
		{
			name: "create artifact", method: http.MethodPost, path: base + "/artifacts",
			body: `{"project_id":"` + writeTestProjectID +
				`","slug":"a","title":"A","content":"c","type":"general","status":"active"}`,
			wantStatus: http.StatusCreated, resourceType: "artifact", action: models.WriteActionCreate, resourceID: artifact.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.artifacts.On("CreateArtifact", mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything)
				returnOrFail(call, fail, artifact, errWrite)
			},
		},
		{
			name: "update artifact", method: http.MethodPut, path: base + "/artifacts/" + writeTestProjectID + "/a",
			body:       `{"content":"new"}`,
			wantStatus: http.StatusOK, resourceType: "artifact", action: models.WriteActionUpdate, resourceID: artifact.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.artifacts.On("UpdateArtifactByProjectIDAndSlugInTeam", mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "a",
					mock.MatchedBy(func(req *models.UpdateArtifactRequest) bool { return isRESTOrigin(req.WriteOrigin) }))
				returnOrFail(call, fail, artifact, errWrite)
			},
		},
		{
			name: "create blueprint", method: http.MethodPost, path: base + "/blueprints",
			body: `{"project_id":"` + writeTestProjectID +
				`","slug":"b","title":"B","content":"c","type":"general","status":"active"}`,
			wantStatus: http.StatusCreated, resourceType: "blueprint", action: models.WriteActionCreate, resourceID: blueprint.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.blueprints.On("CreateBlueprint", mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything)
				returnOrFail(call, fail, blueprint, errWrite)
			},
		},
		{
			name: "update blueprint", method: http.MethodPut, path: base + "/blueprints/" + writeTestProjectID + "/b",
			body:       `{"content":"new"}`,
			wantStatus: http.StatusOK, resourceType: "blueprint", action: models.WriteActionUpdate, resourceID: blueprint.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.blueprints.On("UpdateBlueprintByProjectIDAndSlugInTeam", mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "b",
					mock.MatchedBy(func(req *models.UpdateBlueprintRequest) bool { return isRESTOrigin(req.WriteOrigin) }))
				returnOrFail(call, fail, blueprint, errWrite)
			},
		},
		{
			name: "create memory", method: http.MethodPost, path: base + "/memories",
			body:       `{"project_id":"` + writeTestProjectID + `","text":"remember"}`,
			wantStatus: http.StatusCreated, resourceType: "memory", action: models.WriteActionCreate, resourceID: memory.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.memories.On("CreateMemory", mock.Anything, writeTestUserID, writeTestTeamID, mock.Anything)
				returnOrFail(call, fail, memory, errWrite)
			},
		},
		{
			name: "update memory", method: http.MethodPut, path: base + "/memories/" + writeTestMemoryID,
			body:       `{"text":"new"}`,
			wantStatus: http.StatusOK, resourceType: "memory", action: models.WriteActionUpdate, resourceID: memory.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.memories.On("UpdateMemory", writeTestUserID, writeTestTeamID, writeTestMemoryID,
					mock.MatchedBy(func(req *models.UpdateMemoryRequest) bool { return isRESTOrigin(req.WriteOrigin) }))
				returnOrFail(call, fail, memory, errWrite)
			},
		},
		// A restore is an update requested through a channel like any other
		// (#1418): it must reach the service with that channel, for the version
		// snapshot, and record one update event.
		{
			name: "restore prompt", method: http.MethodPost, path: base + "/prompts/p/versions/2/restore",
			wantStatus: http.StatusOK, resourceType: "prompt", action: models.WriteActionUpdate, resourceID: prompt.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.prompts.On("RestorePromptVersion", mock.Anything, writeTestUserID, writeTestTeamID, "p", 2,
					mock.MatchedBy(isRESTOrigin))
				returnOrFail(call, fail, prompt, errWrite)
			},
		},
		{
			name: "restore artifact", method: http.MethodPost,
			path:       base + "/artifacts/" + writeTestProjectID + "/a/versions/2/restore",
			wantStatus: http.StatusOK, resourceType: "artifact", action: models.WriteActionUpdate, resourceID: artifact.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.artifacts.On("RestoreArtifactVersionInTeam", mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "a", 2, mock.MatchedBy(isRESTOrigin))
				returnOrFail(call, fail, artifact, errWrite)
			},
		},
		{
			name: "restore blueprint", method: http.MethodPost,
			path:       base + "/blueprints/" + writeTestProjectID + "/b/versions/2/restore",
			wantStatus: http.StatusOK, resourceType: "blueprint", action: models.WriteActionUpdate, resourceID: blueprint.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.blueprints.On("RestoreBlueprintVersionInTeam", mock.Anything,
					writeTestUserID, writeTestTeamID, writeTestProjectID, "b", 2, mock.MatchedBy(isRESTOrigin))
				returnOrFail(call, fail, blueprint, errWrite)
			},
		},
		{
			name: "restore memory", method: http.MethodPost,
			path:       base + "/memories/" + writeTestMemoryID + "/versions/2/restore",
			wantStatus: http.StatusOK, resourceType: "memory", action: models.WriteActionUpdate, resourceID: memory.ID,
			arrange: func(f *restWriteFixture, fail bool) {
				call := f.memories.On("RestoreMemoryVersion",
					writeTestUserID, writeTestTeamID, writeTestMemoryID, 2, mock.MatchedBy(isRESTOrigin))
				returnOrFail(call, fail, memory, errWrite)
			},
		},
	}
}

// restWriteRequest is an API-key request from the CLI, so the expected channel
// ("cli") can only come from the middleware reading the auth type and the user
// agent.
func restWriteRequest(tc restWriteCase) *http.Request {
	req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", writeTestCLIAgent)
	ctx := context.WithValue(req.Context(), contextKeyUserID, writeTestUserID)
	ctx = context.WithValue(ctx, contextkeys.AuthType, "api_key")
	ctx = context.WithValue(ctx, contextkeys.APIKeyID, writeTestCallerRef)
	return req.WithContext(ctx)
}

// Every REST create, update and restore route records one write event, labelled
// with the channel derived from the request's authentication, and answers with
// that channel as the resource's last write (#1418): the event is persisted off
// the request path, so the body cannot wait for it to be readable.
func TestRESTWriteRoutes_RecordWrite(t *testing.T) {
	for _, tc := range restWriteCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newRESTWriteFixture(t)
			tc.arrange(f, false)

			w := httptest.NewRecorder()
			f.router.ServeHTTP(w, restWriteRequest(tc))

			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())

			require.Len(t, *f.events, 1)
			event := (*f.events)[0]
			assert.Equal(t, resourceaccess.SourceCLI, event.Source)
			assert.Nil(t, event.AssistantName)
			assert.Equal(t, tc.action, event.Action)
			assert.Equal(t, tc.resourceType, event.ResourceType)
			assert.Equal(t, tc.resourceID, event.ResourceID)
			assert.Equal(t, writeTestTeamID, event.TeamID)
			require.NotNil(t, event.ProjectID)
			assert.Equal(t, writeTestProjectID, *event.ProjectID)
			require.NotNil(t, event.UserID)
			assert.Equal(t, writeTestUserID, *event.UserID)
			require.NotNil(t, event.APIKeyID)
			assert.Equal(t, writeTestCallerRef, *event.APIKeyID)

			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, resourceaccess.SourceCLI, body["last_write_source"])
			assert.Contains(t, body, "last_write_assistant_name")
			assert.Nil(t, body["last_write_assistant_name"])
		})
	}
}

// A write the service refused answers with an error and records no event.
func TestRESTWriteRoutes_FailedWriteRecordsNothing(t *testing.T) {
	for _, tc := range restWriteCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newRESTWriteFixture(t)
			tc.arrange(f, true)

			w := httptest.NewRecorder()
			f.router.ServeHTTP(w, restWriteRequest(tc))

			assert.GreaterOrEqual(t, w.Code, http.StatusBadRequest, w.Body.String())
			assert.Empty(t, *f.events)
		})
	}
}
