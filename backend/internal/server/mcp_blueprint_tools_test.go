package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// Test fixture for a valid UUID project_id used by blueprint MCP tool tests.
const (
	testBlueprintProjectID = "550e8400-e29b-41d4-a716-446655440030"
)

// newBlueprintTestServer builds a server whose blueprint + team services are mocked,
// with the member user belonging to testTeamUUID/testTeamSlug so resolveTeam succeeds.
func newBlueprintTestServer(t *testing.T) (*Server, *mocks.MockBlueprintServiceInterface) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	mockBlueprintService := mocks.NewMockBlueprintServiceInterface(t)
	mockTeamService := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
	}
	stubUserTeams(mockTeamService, []models.Team{memberTeam()})
	return srv, mockBlueprintService
}

func verifyBlueprintWriteResult(
	t *testing.T,
	result *mcp.CallToolResult,
	structuredResult interface{},
	err error,
) *blueprintWriteResponse {
	t.Helper()
	if err != nil {
		t.Errorf("blueprint write returned error: %v", err)
		return nil
	}
	if result == nil {
		t.Error("blueprint write returned nil result")
		return nil
	}
	if result.IsError {
		t.Errorf("blueprint write returned error result: %s", extractText(t, result))
		return nil
	}
	if len(result.Content) == 0 {
		t.Error("blueprint write returned no content")
		return nil
	}
	resp, ok := structuredResult.(*blueprintWriteResponse)
	if !ok {
		t.Error("blueprint write returned wrong structured result type")
		return nil
	}
	return resp
}

func TestCreateBlueprint_Success(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	expectedBlueprint := &models.Blueprint{
		ID:        "test-id",
		ProjectID: testBlueprintProjectID,
		Slug:      "test-slug",
		Title:     "Test Blueprint",
		Content:   "Test content",
	}
	mockBlueprintService.On(
		"CreateBlueprint", testMemberUserID, testTeamUUID,
		mock.MatchedBy(func(req *models.CreateBlueprintRequest) bool {
			return req.ProjectID == testBlueprintProjectID && req.Slug == "test-slug" &&
				req.Title == "Test Blueprint" && req.Content == "Test content"
		}),
	).Return(expectedBlueprint, nil)

	params := &CreateBlueprintParams{
		TeamID:    testTeamSlug, // exercise slug resolution on the create path
		ProjectID: testBlueprintProjectID,
		Slug:      "test-slug",
		Title:     "Test Blueprint",
		Content:   "Test content",
	}

	result, structuredResult, err := srv.createBlueprint(context.Background(), nil, params, testMemberUserID)

	resp := verifyBlueprintWriteResult(t, result, structuredResult, err)
	if resp == nil {
		return
	}
	if resp.ID != expectedBlueprint.ID {
		t.Errorf("got ID %v want %v", resp.ID, expectedBlueprint.ID)
	}
	if !strings.HasSuffix(resp.FullURL, "/blueprints/"+testBlueprintProjectID+"/test-slug") {
		t.Errorf("unexpected full_url %q", resp.FullURL)
	}
}

// TestCreateBlueprint_ForwardsOptionalFields verifies optional create fields reach the service.
func TestCreateBlueprint_ForwardsOptionalFields(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	subtype := "sub-agents"
	mockBlueprintService.On(
		"CreateBlueprint", testMemberUserID, testTeamUUID,
		mock.MatchedBy(func(req *models.CreateBlueprintRequest) bool {
			return req.Type == "agent" && req.Status == "active" &&
				req.Subtype != nil && *req.Subtype == "sub-agents" &&
				req.Metadata != nil && req.Metadata["model"] == "claude-opus-4-8"
		}),
	).Return(&models.Blueprint{ID: "id", ProjectID: testBlueprintProjectID, Slug: "s"}, nil)

	params := &CreateBlueprintParams{
		TeamID:    testTeamUUID,
		ProjectID: testBlueprintProjectID,
		Slug:      "s",
		Title:     "t",
		Content:   "c",
		Type:      "agent",
		Subtype:   &subtype,
		Status:    "active",
		Metadata:  map[string]interface{}{"model": "claude-opus-4-8"},
	}
	result, _, err := srv.createBlueprint(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	mockBlueprintService.AssertExpectations(t)
}

func TestCreateBlueprint_ServiceError(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	mockBlueprintService.On(
		"CreateBlueprint", testMemberUserID, testTeamUUID, mock.AnythingOfType("*models.CreateBlueprintRequest"),
	).Return(nil, errors.New("service error"))

	params := &CreateBlueprintParams{
		TeamID:    testTeamUUID,
		ProjectID: testBlueprintProjectID,
		Slug:      "test-slug",
		Title:     "Test Blueprint",
		Content:   "Test content",
	}

	result, structuredResult, err := srv.createBlueprint(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true result")
	}
	if structuredResult != nil {
		t.Error("expected nil structured result on error")
	}
}

// TestCreateBlueprint_SubAgentsRuleErrorSurfaced verifies the sub-agents business-rule
// error from the service (subtype "sub-agents" requires metadata.model) is surfaced
// cleanly as an IsError result rather than panicking.
func TestCreateBlueprint_SubAgentsRuleErrorSurfaced(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	subtype := "sub-agents"
	mockBlueprintService.On(
		"CreateBlueprint", testMemberUserID, testTeamUUID, mock.AnythingOfType("*models.CreateBlueprintRequest"),
	).Return(nil, errors.New("sub-agents blueprints require a model in metadata"))

	params := &CreateBlueprintParams{
		TeamID:    testTeamUUID,
		ProjectID: testBlueprintProjectID,
		Slug:      "agent",
		Title:     "Agent",
		Content:   "c",
		Subtype:   &subtype,
	}
	result, structured, err := srv.createBlueprint(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true result")
	}
	if structured != nil {
		t.Error("expected nil structured result on error")
	}
	if text := extractText(t, result); !strings.Contains(text, "model") {
		t.Errorf("expected surfaced business-rule error, got %q", text)
	}
}

// TestCreateBlueprint_NonMemberTeamDenied verifies a team the user does not belong to
// is rejected with a generic access-denied message and the blueprint service is never called.
func TestCreateBlueprint_NonMemberTeamDenied(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	params := &CreateBlueprintParams{
		TeamID:    testOtherTeamUUID,
		ProjectID: testBlueprintProjectID,
		Slug:      "s",
		Title:     "t",
		Content:   "c",
	}

	result, structured, err := srv.createBlueprint(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result on access denied")
	}
	assertGenericAccessDenied(t, result)
	mockBlueprintService.AssertNotCalled(t, "CreateBlueprint")
}

// TestCreateBlueprint_MissingTeamID verifies missing team_id yields a model-actionable error.
func TestCreateBlueprint_MissingTeamID(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	params := &CreateBlueprintParams{ProjectID: testBlueprintProjectID, Slug: "s", Title: "t", Content: "c"}

	result, structured, err := srv.createBlueprint(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	text := extractText(t, result)
	if !strings.Contains(text, "team_id is required") || !strings.Contains(text, "vibexp_io_list_teams") {
		t.Errorf("expected model-actionable missing team_id error, got %q", text)
	}
	mockBlueprintService.AssertNotCalled(t, "CreateBlueprint")
}

func TestCreateBlueprint_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockBlueprintService := newBlueprintTestServer(t)

			params := &CreateBlueprintParams{
				TeamID:    testTeamUUID,
				ProjectID: tc.projectID,
				Slug:      "any",
				Title:     "any",
				Content:   "any",
			}
			result, structured, err := srv.createBlueprint(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on validation rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockBlueprintService.AssertNotCalled(t, "CreateBlueprint")
		})
	}
}

func TestUpdateBlueprint_Success(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	expectedBlueprint := &models.Blueprint{
		ID: "test-id", ProjectID: testBlueprintProjectID, Slug: "test-slug", Title: "Updated", Content: "Updated content",
	}
	mockBlueprintService.On(
		"UpdateBlueprintByProjectIDAndSlug",
		testMemberUserID, testBlueprintProjectID, "test-slug",
		mock.MatchedBy(func(req *models.UpdateBlueprintRequest) bool {
			return req.Title != nil && *req.Title == "Updated" &&
				req.Content != nil && *req.Content == "Updated content"
		}),
	).Return(expectedBlueprint, nil)

	params := &UpdateBlueprintParams{
		TeamID:    testTeamUUID,
		ProjectID: testBlueprintProjectID,
		Slug:      "test-slug",
		Title:     "Updated",
		Content:   "Updated content",
	}
	result, structuredResult, err := srv.updateBlueprint(context.Background(), nil, params, testMemberUserID)
	resp := verifyBlueprintWriteResult(t, result, structuredResult, err)
	if resp == nil {
		return
	}
	if resp.ID != expectedBlueprint.ID {
		t.Errorf("got ID %v want %v", resp.ID, expectedBlueprint.ID)
	}
	if !strings.HasSuffix(resp.FullURL, "/blueprints/"+testBlueprintProjectID+"/test-slug") {
		t.Errorf("unexpected full_url %q", resp.FullURL)
	}
}

// TestUpdateBlueprint_OptionalFields verifies only provided fields are forwarded as pointers,
// and unset string fields stay nil (pointer semantics matching models.UpdateBlueprintRequest).
func TestUpdateBlueprint_OptionalFields(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	mockBlueprintService.On(
		"UpdateBlueprintByProjectIDAndSlug",
		testMemberUserID, testBlueprintProjectID, "test-slug",
		mock.MatchedBy(func(req *models.UpdateBlueprintRequest) bool {
			// only status + metadata provided; title/content/description/type must remain nil
			return req.Status != nil && *req.Status == "expired" &&
				req.Metadata != nil && req.Metadata["k"] == "v" &&
				req.Title == nil && req.Content == nil && req.Description == nil && req.Type == nil
		}),
	).Return(&models.Blueprint{ID: "id", ProjectID: testBlueprintProjectID, Slug: "test-slug"}, nil)

	params := &UpdateBlueprintParams{
		TeamID:    testTeamUUID,
		ProjectID: testBlueprintProjectID,
		Slug:      "test-slug",
		Status:    "expired",
		Metadata:  map[string]interface{}{"k": "v"},
	}
	result, _, err := srv.updateBlueprint(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	mockBlueprintService.AssertExpectations(t)
}

func TestUpdateBlueprint_ServiceError(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	mockBlueprintService.On(
		"UpdateBlueprintByProjectIDAndSlug",
		testMemberUserID, testBlueprintProjectID, "nonexistent",
		mock.AnythingOfType("*models.UpdateBlueprintRequest"),
	).Return(nil, errors.New("blueprint not found"))

	params := &UpdateBlueprintParams{
		TeamID: testTeamUUID, ProjectID: testBlueprintProjectID, Slug: "nonexistent", Title: "x",
	}
	result, structured, err := srv.updateBlueprint(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true result")
	}
	if structured != nil {
		t.Error("expected nil structured result on error")
	}
}

func TestUpdateBlueprint_NonMemberTeamDenied(t *testing.T) {
	srv, mockBlueprintService := newBlueprintTestServer(t)

	params := &UpdateBlueprintParams{
		TeamID: testOtherTeamUUID, ProjectID: testBlueprintProjectID, Slug: "s", Title: "x",
	}
	result, structured, err := srv.updateBlueprint(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockBlueprintService.AssertNotCalled(t, "UpdateBlueprintByProjectIDAndSlug")
}

func TestUpdateBlueprint_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockBlueprintService := newBlueprintTestServer(t)

			params := &UpdateBlueprintParams{
				TeamID:    testTeamUUID,
				ProjectID: tc.projectID,
				Slug:      "any",
				Title:     "x",
			}
			result, structured, err := srv.updateBlueprint(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on validation rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockBlueprintService.AssertNotCalled(t, "UpdateBlueprintByProjectIDAndSlug")
		})
	}
}
