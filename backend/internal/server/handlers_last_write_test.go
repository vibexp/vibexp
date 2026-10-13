package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The list and detail reads of the four resource types report the channel of
// the resource's most recent write (#1418). These are the spec-validated
// assertions for that: both keys are on the wire in every case, with the value
// the repository resolved, and null (never a guessed channel) for a resource
// that has no recorded write.

// lastWriteRead serves one list read and one detail read of a resource type
// whose single resource carries lw, and names the key the list items sit
// under.
type lastWriteRead struct {
	listKey string
	// wrapped is true for the one list response that sits inside a
	// {status, message, data} envelope.
	wrapped bool
	list    func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder)
	detail  func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder)
}

func lastWriteReads() map[string]lastWriteRead {
	return map[string]lastWriteRead{
		"prompts": {
			listKey: "prompts", wrapped: true,
			list: func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder) {
				prompt := samplePrompt()
				prompt.LastWrite = lw
				srv, container := strictPromptServer(t)
				container.promptService.On("ListPrompts", strictPrUserID, mock.Anything).
					Return(&models.PromptListResponse{
						Prompts:    models.JSONArray[models.Prompt]{*prompt},
						TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
					}, nil)
				return strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts")
			},
			detail: func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder) {
				prompt := samplePrompt()
				prompt.LastWrite = lw
				srv, container := strictPromptServer(t)
				container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
					Return(prompt, nil)
				return strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)
			},
		},
		"artifacts": {
			listKey: "artifacts",
			list: func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder) {
				artifact := sampleStrictArtifact()
				artifact.LastWrite = lw
				svc := servicesmocks.NewMockArtifactServiceInterface(t)
				svc.On("ListArtifacts", strictArtUserID, mock.Anything).
					Return(&models.ArtifactListResponse{
						Artifacts:  models.JSONArray[models.Artifact]{*artifact},
						TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
					}, nil)
				return strictArtifactRequest(t, strictArtifactServer(t, svc), "/api/v1/"+strictArtTeamID+"/artifacts")
			},
			detail: func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder) {
				artifact := sampleStrictArtifact()
				artifact.LastWrite = lw
				svc := servicesmocks.NewMockArtifactServiceInterface(t)
				svc.On("GetArtifactByProjectIDAndSlugInTeam",
					strictArtUserID, strictArtTeamID, strictArtProject, strictArtSlug).
					Return(artifact, nil)
				return strictArtifactRequest(t, strictArtifactServer(t, svc),
					"/api/v1/"+strictArtTeamID+"/artifacts/"+strictArtProject+"/"+strictArtSlug)
			},
		},
		"blueprints": {
			listKey: "blueprints",
			list: func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder) {
				blueprint := sampleStrictBlueprint()
				blueprint.LastWrite = lw
				svc := servicesmocks.NewMockBlueprintServiceInterface(t)
				svc.On("ListBlueprints", strictBpUserID, mock.Anything).
					Return(&models.BlueprintListResponse{
						Blueprints: models.JSONArray[models.Blueprint]{*blueprint},
						TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
					}, nil)
				return strictBlueprintRequest(t, strictBlueprintServer(t, svc), "/api/v1/"+strictBpTeamID+"/blueprints")
			},
			detail: func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder) {
				blueprint := sampleStrictBlueprint()
				blueprint.LastWrite = lw
				svc := servicesmocks.NewMockBlueprintServiceInterface(t)
				svc.On("GetBlueprintByProjectIDAndSlugInTeam",
					strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
					Return(blueprint, nil)
				return strictBlueprintRequest(t, strictBlueprintServer(t, svc),
					"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)
			},
		},
		"memories": {
			listKey: "memories",
			list: func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder) {
				memory := sampleStrictMemory()
				memory.LastWrite = lw
				container := newMockMemoryContainer(t)
				container.memoryService.On("ListMemories", memoriesTestUserID, mock.Anything).
					Return(&models.MemoryListResponse{
						Memories:   models.JSONArray[models.Memory]{*memory},
						TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
					}, nil)
				srv := createMemoryTestServer(container)
				req := makeMemoryAuthenticatedRequest(
					"GET", "/api/v1/"+memoriesTestTeamID+"/memories", nil, memoriesTestUserID)
				w := httptest.NewRecorder()
				srv.router.ServeHTTP(w, req)
				return req, w
			},
			detail: func(t *testing.T, lw models.LastWrite) (*http.Request, *httptest.ResponseRecorder) {
				memory := sampleStrictMemory()
				memory.LastWrite = lw
				container := newMockMemoryContainer(t)
				container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
					Return(memory, nil)
				return getMemoryRequest(t, createMemoryTestServer(container), memoriesTestMemoryID)
			},
		},
	}
}

// lastWriteCase is one last-write state and the two JSON values it must
// produce. A nil want is JSON null.
type lastWriteCase struct {
	name          string
	lastWrite     models.LastWrite
	wantSource    any
	wantAssistant any
}

func lastWriteCases() []lastWriteCase {
	origin := func(source, assistant string) models.LastWrite {
		return models.LastWriteFromOrigin(&models.WriteOrigin{
			Source: source, AssistantName: models.NormalizeAssistantName(assistant),
		})
	}
	return []lastWriteCase{
		{name: "web", lastWrite: origin("web", ""), wantSource: "web"},
		{name: "cli", lastWrite: origin("cli", ""), wantSource: "cli"},
		{name: "api", lastWrite: origin("api", ""), wantSource: "api"},
		{name: "mcp with a client name", lastWrite: origin("mcp", "claude-code"),
			wantSource: "mcp", wantAssistant: "claude-code"},
		{name: "mcp without a client name", lastWrite: origin("mcp", ""), wantSource: "mcp"},
		// No recorded write: written before write channels were recorded.
		{name: "no recorded write", lastWrite: models.LastWrite{}},
	}
}

// assertLastWriteKeys checks that both keys are present on the resource and
// carry the expected values; a missing key would pass an Equal-with-nil check.
func assertLastWriteKeys(t *testing.T, resource map[string]any, tc lastWriteCase) {
	t.Helper()
	require.Contains(t, resource, "last_write_source")
	require.Contains(t, resource, "last_write_assistant_name")
	assert.Equal(t, tc.wantSource, resource["last_write_source"])
	assert.Equal(t, tc.wantAssistant, resource["last_write_assistant_name"])
}

func TestResourceLists_ReportLastWriteSource(t *testing.T) {
	for domain, read := range lastWriteReads() {
		for _, tc := range lastWriteCases() {
			t.Run(domain+"/"+tc.name, func(t *testing.T) {
				req, w := read.list(t, tc.lastWrite)

				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				specconformance.AssertConformsToSpec(t, req, w)

				var body map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				if read.wrapped {
					body = body["data"].(map[string]any)
				}
				items, ok := body[read.listKey].([]any)
				require.True(t, ok, "list key %q", read.listKey)
				require.Len(t, items, 1)
				assertLastWriteKeys(t, items[0].(map[string]any), tc)
			})
		}
	}
}

func TestResourceDetails_ReportLastWriteSource(t *testing.T) {
	for domain, read := range lastWriteReads() {
		for _, tc := range lastWriteCases() {
			t.Run(domain+"/"+tc.name, func(t *testing.T) {
				req, w := read.detail(t, tc.lastWrite)

				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				specconformance.AssertConformsToSpec(t, req, w)

				var body map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				assertLastWriteKeys(t, body, tc)
			})
		}
	}
}

// nil stays nil, so the field serializes as null, and a value is copied rather
// than aliased.
func TestToGenWriteSource(t *testing.T) {
	assert.Nil(t, toGenWriteSource[string](nil))

	source := "mcp"
	got := toGenWriteSource[string](&source)
	require.NotNil(t, got)
	assert.Equal(t, "mcp", *got)
	// A copy: mutating the model afterwards must not reach the response.
	source = "web"
	assert.Equal(t, "mcp", *got)
}
