package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// deleteResourceMocks bundles the service mocks a delete_resource test may need.
type deleteResourceMocks struct {
	memory    *mocks.MockMemoryServiceInterface
	prompt    *mocks.MockPromptServiceInterface
	artifact  *mocks.MockArtifactServiceInterface
	blueprint *mocks.MockBlueprintServiceInterface
	embedding *mocks.MockEmbeddingServiceInterface
	team      *mocks.MockTeamServiceInterface
}

// newDeleteResourceTestServer builds a server whose resource + team + embedding
// services are mocked, with the member user belonging to testTeamUUID/testTeamSlug
// so resolveTeam succeeds. Attachment service is left nil (delete is nil-safe).
func newDeleteResourceTestServer(t *testing.T) (*Server, *deleteResourceMocks) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	m := &deleteResourceMocks{
		memory:    mocks.NewMockMemoryServiceInterface(t),
		prompt:    mocks.NewMockPromptServiceInterface(t),
		artifact:  mocks.NewMockArtifactServiceInterface(t),
		blueprint: mocks.NewMockBlueprintServiceInterface(t),
		embedding: mocks.NewMockEmbeddingServiceInterface(t),
		team:      mocks.NewMockTeamServiceInterface(t),
	}
	srv.container = &TestContainer{
		MemoryServiceMock:    m.memory,
		PromptServiceMock:    m.prompt,
		ArtifactServiceMock:  m.artifact,
		BlueprintServiceMock: m.blueprint,
		EmbeddingServiceMock: m.embedding,
		TeamServiceMock:      m.team,
	}
	stubUserTeams(m.team, []models.Team{memberTeam()})
	return srv, m
}

// assertDeleteSuccess asserts the result is a non-error delete with a structured
// payload whose ResourceType matches and Deleted=true, plus matching JSON text.
func assertDeleteSuccess(
	t *testing.T, result *mcp.CallToolResult, structured any, err error, wantType string,
) *deleteResourceResponse {
	t.Helper()
	if err != nil {
		t.Fatalf("deleteResource returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	resp, ok := structured.(*deleteResourceResponse)
	if !ok {
		t.Fatalf("deleteResource returned wrong structured result type: %T", structured)
	}
	if !resp.Deleted || resp.ResourceType != wantType {
		t.Errorf("got %+v, want Deleted=true ResourceType=%q", resp, wantType)
	}
	var fromJSON deleteResourceResponse
	if err := json.Unmarshal([]byte(extractText(t, result)), &fromJSON); err != nil {
		t.Errorf("deleteResource returned invalid JSON: %v", err)
	}
	if fromJSON.ResourceType != wantType || !fromJSON.Deleted {
		t.Errorf("JSON payload mismatch: got %+v", fromJSON)
	}
	return resp
}

// assertDeleteIsError asserts the result is an IsError result with no structured payload.
func assertDeleteIsError(t *testing.T, result *mcp.CallToolResult, structured any, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected function error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("expected IsError=true, got %v", result)
	}
	if structured != nil {
		t.Error("expected nil structured result on error")
	}
}

func TestDeleteResource_MemorySuccess(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.memory.On("DeleteMemory", testMemberUserID, testTeamUUID, "mem-1").Return(nil)
	m.embedding.On("DeleteEmbeddingsByEntity", resourceTypeMemory, "mem-1").Return(nil)

	params := &DeleteResourceParams{TeamID: testTeamSlug, ResourceType: "memory", ID: "mem-1"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	resp := assertDeleteSuccess(t, result, structured, err, resourceTypeMemory)
	if resp.ID != "mem-1" {
		t.Errorf("got ID %q want mem-1", resp.ID)
	}
	m.memory.AssertExpectations(t)
	m.embedding.AssertExpectations(t)
}

func TestDeleteResource_PromptSuccess(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.prompt.On("GetPromptBySlug", testMemberUserID, testTeamUUID, "my-prompt").
		Return(&models.Prompt{ID: "prompt-1"}, nil)
	m.prompt.On("DeletePromptBySlug", testMemberUserID, testTeamUUID, "my-prompt").Return(nil)
	m.embedding.On("DeleteEmbeddingsByEntity", resourceTypePrompt, "prompt-1").Return(nil)

	params := &DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "prompt", Slug: "my-prompt"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	resp := assertDeleteSuccess(t, result, structured, err, resourceTypePrompt)
	if resp.ID != "prompt-1" || resp.Slug != "my-prompt" {
		t.Errorf("got %+v want ID=prompt-1 Slug=my-prompt", resp)
	}
	m.prompt.AssertExpectations(t)
	m.embedding.AssertExpectations(t)
}

func TestDeleteResource_ArtifactSuccess(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.artifact.On("GetArtifactByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testProjectID, "art").
		Return(&models.Artifact{ID: "artifact-1"}, nil)
	m.artifact.On("DeleteArtifactByProjectIDAndSlug", testMemberUserID, testProjectID, "art").Return(nil)
	m.embedding.On("DeleteEmbeddingsByEntity", resourceTypeArtifact, "artifact-1").Return(nil)

	params := &DeleteResourceParams{
		TeamID: testTeamUUID, ResourceType: "artifact", ProjectID: testProjectID, Slug: "art",
	}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	resp := assertDeleteSuccess(t, result, structured, err, resourceTypeArtifact)
	if resp.ID != "artifact-1" || resp.ProjectID != testProjectID || resp.Slug != "art" {
		t.Errorf("got %+v", resp)
	}
	m.artifact.AssertExpectations(t)
	m.embedding.AssertExpectations(t)
}

func TestDeleteResource_BlueprintSuccess(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.blueprint.On("GetBlueprintByProjectIDAndSlug", testMemberUserID, testProjectID, "bp").
		Return(&models.Blueprint{ID: "blueprint-1"}, nil)
	m.blueprint.On("DeleteBlueprintByProjectIDAndSlug", testMemberUserID, testProjectID, "bp").Return(nil)
	m.embedding.On("DeleteEmbeddingsByEntity", resourceTypeBlueprint, "blueprint-1").Return(nil)

	params := &DeleteResourceParams{
		TeamID: testTeamUUID, ResourceType: "blueprint", ProjectID: testProjectID, Slug: "bp",
	}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	resp := assertDeleteSuccess(t, result, structured, err, resourceTypeBlueprint)
	if resp.ID != "blueprint-1" {
		t.Errorf("got ID %q want blueprint-1", resp.ID)
	}
	m.blueprint.AssertExpectations(t)
	m.embedding.AssertExpectations(t)
}

// TestDeleteResource_ResourceTypeCaseInsensitive proves the discriminator is
// normalized (trimmed + lowercased) before dispatch.
func TestDeleteResource_ResourceTypeCaseInsensitive(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.memory.On("DeleteMemory", testMemberUserID, testTeamUUID, "mem-1").Return(nil)
	m.embedding.On("DeleteEmbeddingsByEntity", resourceTypeMemory, "mem-1").Return(nil)

	params := &DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "  Memory  ", ID: "mem-1"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	assertDeleteSuccess(t, result, structured, err, resourceTypeMemory)
}

func TestDeleteResource_UnknownType(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)

	params := &DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "team", ID: "x"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	assertDeleteIsError(t, result, structured, err)
	assertValidationFailure(t, result, []string{"memory", "artifact", "blueprint", "prompt"})
}

// TestDeleteResource_MissingIdentifiers covers the per-type required-identifier
// validation: each type rejects a call missing its identifier before any service
// is touched.
func TestDeleteResource_MissingIdentifiers(t *testing.T) {
	cases := []struct {
		name       string
		params     *DeleteResourceParams
		substrMust []string
	}{
		{
			"memory without id",
			&DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "memory"},
			[]string{"id is required"},
		},
		{
			"prompt without slug",
			&DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "prompt"},
			[]string{"slug is required"},
		},
		{
			"artifact without slug",
			&DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "artifact", ProjectID: testProjectID},
			[]string{"slug is required"},
		},
		{
			"blueprint without slug",
			&DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "blueprint", ProjectID: testProjectID},
			[]string{"slug is required"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newDeleteResourceTestServer(t)
			result, structured, err := srv.deleteResource(context.Background(), nil, tc.params, testMemberUserID)
			assertDeleteIsError(t, result, structured, err)
			assertValidationFailure(t, result, tc.substrMust)
		})
	}
}

// TestDeleteResource_InvalidProjectID covers project_id validation for the two
// project-scoped types.
func TestDeleteResource_InvalidProjectID(t *testing.T) {
	for _, rt := range []string{"artifact", "blueprint"} {
		t.Run(rt, func(t *testing.T) {
			srv, _ := newDeleteResourceTestServer(t)
			params := &DeleteResourceParams{
				TeamID: testTeamUUID, ResourceType: rt, ProjectID: "not-a-uuid", Slug: "x",
			}
			result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)
			assertDeleteIsError(t, result, structured, err)
		})
	}
}

func TestDeleteResource_NonMemberTeamDenied(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)

	params := &DeleteResourceParams{TeamID: testOtherTeamUUID, ResourceType: "memory", ID: "mem-1"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	m.memory.AssertNotCalled(t, "DeleteMemory")
}

func TestDeleteResource_MissingTeamID(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)

	params := &DeleteResourceParams{ResourceType: "memory", ID: "mem-1"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	assertDeleteIsError(t, result, structured, err)
	assertValidationFailure(t, result, []string{"team_id is required", "vibexp_io_list_teams"})
	m.memory.AssertNotCalled(t, "DeleteMemory")
}

// TestDeleteResource_ServiceError verifies a delete failure surfaces as an
// IsError result and no embeddings cleanup is attempted.
func TestDeleteResource_ServiceError(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.memory.On("DeleteMemory", testMemberUserID, testTeamUUID, "mem-1").
		Return(errors.New("boom"))

	params := &DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "memory", ID: "mem-1"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	assertDeleteIsError(t, result, structured, err)
	m.embedding.AssertNotCalled(t, "DeleteEmbeddingsByEntity")
}

// TestDeleteResource_PromptNotFound verifies the load-before-delete step
// propagates a not-found error without attempting the delete.
func TestDeleteResource_PromptNotFound(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.prompt.On("GetPromptBySlug", testMemberUserID, testTeamUUID, "missing").
		Return(nil, errors.New("not found"))

	params := &DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "prompt", Slug: "missing"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	assertDeleteIsError(t, result, structured, err)
	m.prompt.AssertNotCalled(t, "DeletePromptBySlug")
}

// TestDeleteResource_EmbeddingFailureNonFatal proves a best-effort embeddings
// cleanup failure does not fail the delete (the row is already gone).
func TestDeleteResource_EmbeddingFailureNonFatal(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.memory.On("DeleteMemory", testMemberUserID, testTeamUUID, "mem-1").Return(nil)
	m.embedding.On("DeleteEmbeddingsByEntity", resourceTypeMemory, "mem-1").
		Return(errors.New("embedding store down"))

	params := &DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "memory", ID: "mem-1"}
	result, structured, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	assertDeleteSuccess(t, result, structured, err, resourceTypeMemory)
}
