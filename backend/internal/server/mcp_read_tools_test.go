package server

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
)

// The unified read dispatchers reuse newDeleteResourceTestServer (it mocks the
// memory/artifact/blueprint/team services and stubs team membership). Artifact
// and memory get/list behavior is covered in the artifact/memory tool tests via
// getResource/listResources; these tests focus on the blueprint paths (a new
// capability), the resource_type discriminator, and identifier validation.

func TestGetResource_BlueprintSuccess(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.blueprint.On("GetBlueprintByProjectIDAndSlug", testMemberUserID, testProjectID, "bp").
		Return(&models.Blueprint{ID: "bp-1", Slug: "bp", Title: "My BP", Content: "full body"}, nil)

	params := &GetResourceParams{
		TeamID: testTeamUUID, ResourceType: "blueprint", ProjectID: testProjectID, Slug: "bp",
	}
	result, structured, err := srv.getResource(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Fatalf("getResource returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	bp, ok := structured.(*models.Blueprint)
	if !ok || bp.ID != "bp-1" {
		t.Fatalf("expected *models.Blueprint bp-1, got %T %+v", structured, structured)
	}
	// get_resource returns the full resource, so content is present.
	if !strings.Contains(extractText(t, result), "full body") {
		t.Error("get_resource(blueprint) should return full content")
	}
	m.blueprint.AssertExpectations(t)
}

func TestListResources_BlueprintSuccessIsSlim(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.blueprint.On("ListBlueprintsByProject", testMemberUserID, testProjectID, mock.Anything).
		Return(&models.BlueprintListResponse{
			Blueprints: models.JSONArray[models.Blueprint]{
				{ID: "bp-1", Slug: "bp", Title: "My BP", Content: "SECRET BODY"},
			},
			TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
		}, nil)

	params := &ListResourcesParams{
		TeamID: testTeamUUID, ResourceType: "blueprint", ProjectID: testProjectID, Page: 1, Limit: 10,
	}
	result, structured, err := srv.listResources(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Fatalf("listResources returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	if _, ok := structured.(*blueprintListResponse); !ok {
		t.Fatalf("expected *blueprintListResponse, got %T", structured)
	}
	text := extractText(t, result)
	if strings.Contains(text, "SECRET BODY") || strings.Contains(text, "\"content\"") {
		t.Error("list_resources(blueprint) must be slim — no content field/value")
	}
	if !strings.Contains(text, "bp-1") {
		t.Error("list_resources(blueprint) should include the blueprint id")
	}
	m.blueprint.AssertExpectations(t)
}

// TestListResources_BlueprintCaseInsensitiveType proves the discriminator is
// trimmed + lowercased before dispatch, matching delete_resource.
func TestListResources_BlueprintCaseInsensitiveType(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.blueprint.On("ListBlueprintsByProject", testMemberUserID, testProjectID, mock.Anything).
		Return(&models.BlueprintListResponse{Blueprints: models.JSONArray[models.Blueprint]{}, Page: 1, PerPage: 10}, nil)

	params := &ListResourcesParams{
		TeamID: testTeamUUID, ResourceType: "  Blueprint  ", ProjectID: testProjectID, Page: 1, Limit: 10,
	}
	result, _, err := srv.listResources(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Fatalf("listResources returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success for normalized type, got %v", result)
	}
	m.blueprint.AssertExpectations(t)
}

func TestGetResource_UnknownType(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)
	params := &GetResourceParams{TeamID: testTeamUUID, ResourceType: "team", ID: "x"}
	result, structured, err := srv.getResource(context.Background(), nil, params, testMemberUserID)
	assertReadIsError(t, result, structured, err)
}

// list_resources does not support prompt (reads flow through prompt primitives),
// so a prompt resource_type must be rejected.
func TestListResources_UnsupportedType(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)
	params := &ListResourcesParams{TeamID: testTeamUUID, ResourceType: "prompt", ProjectID: testProjectID}
	result, structured, err := srv.listResources(context.Background(), nil, params, testMemberUserID)
	assertReadIsError(t, result, structured, err)
}

func TestGetResource_MemoryRequiresID(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)
	params := &GetResourceParams{TeamID: testTeamUUID, ResourceType: "memory"}
	result, structured, err := srv.getResource(context.Background(), nil, params, testMemberUserID)
	assertReadIsError(t, result, structured, err)
}

func TestGetResource_ArtifactRequiresSlug(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)
	params := &GetResourceParams{TeamID: testTeamUUID, ResourceType: "artifact", ProjectID: testProjectID}
	result, structured, err := srv.getResource(context.Background(), nil, params, testMemberUserID)
	assertReadIsError(t, result, structured, err)
}

// assertReadIsError asserts an IsError result with no structured payload and no function error.
func assertReadIsError(t *testing.T, result *mcp.CallToolResult, structured any, err error) {
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
