package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// Test fixtures for valid UUID project_id values used by prompt MCP tool tests.
const (
	testPromptProjectID = "550e8400-e29b-41d4-a716-446655440020"
)

// newPromptTestServer builds a server whose prompt + team services are mocked,
// with the member user belonging to testTeamUUID/testTeamSlug so resolveTeam succeeds.
func newPromptTestServer(t *testing.T) (*Server, *mocks.MockPromptServiceInterface) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	mockPromptService := mocks.NewMockPromptServiceInterface(t)
	mockTeamService := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{
		PromptServiceMock: mockPromptService,
		TeamServiceMock:   mockTeamService,
	}
	stubUserTeams(mockTeamService, []models.Team{memberTeam()})
	return srv, mockPromptService
}

func verifyPromptWriteResult(
	t *testing.T,
	result *mcp.CallToolResult,
	structuredResult interface{},
	err error,
) *promptWriteResponse {
	t.Helper()
	if err != nil {
		t.Errorf("prompt write returned error: %v", err)
		return nil
	}
	if result == nil {
		t.Error("prompt write returned nil result")
		return nil
	}
	if result.IsError {
		t.Errorf("prompt write returned error result: %s", extractText(t, result))
		return nil
	}
	if len(result.Content) == 0 {
		t.Error("prompt write returned no content")
		return nil
	}
	resp, ok := structuredResult.(*promptWriteResponse)
	if !ok {
		t.Error("prompt write returned wrong structured result type")
		return nil
	}
	return resp
}

func TestCreatePrompt_Success(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	expectedPrompt := &models.Prompt{
		ID:        "test-id",
		ProjectID: testPromptProjectID,
		Slug:      "test-slug",
		Name:      "Test Prompt",
		Body:      "Test body",
	}
	mockPromptService.On(
		"CreatePrompt", testMemberUserID, testTeamUUID,
		mock.MatchedBy(func(req *models.CreatePromptRequest) bool {
			return req.ProjectID == testPromptProjectID && req.Slug == "test-slug" &&
				req.Name == "Test Prompt" && req.Body == "Test body"
		}),
	).Return(expectedPrompt, nil)

	mcpExpose := true
	params := &CreatePromptParams{
		TeamID:    testTeamSlug, // exercise slug resolution on the create path
		ProjectID: testPromptProjectID,
		Slug:      "test-slug",
		Name:      "Test Prompt",
		Body:      "Test body",
		MCPExpose: &mcpExpose,
		Labels:    []string{"a", "b"},
	}

	result, structuredResult, err := srv.createPrompt(context.Background(), nil, params, testMemberUserID)

	resp := verifyPromptWriteResult(t, result, structuredResult, err)
	if resp == nil {
		return
	}
	if resp.ID != expectedPrompt.ID {
		t.Errorf("got ID %v want %v", resp.ID, expectedPrompt.ID)
	}
	if !strings.HasSuffix(resp.FullURL, "/prompts/test-slug") {
		t.Errorf("unexpected full_url %q", resp.FullURL)
	}
}

// TestCreatePrompt_ForwardsMCPExposeAndLabels verifies optional create fields reach the service.
func TestCreatePrompt_ForwardsMCPExposeAndLabels(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	expose := false
	mockPromptService.On(
		"CreatePrompt", testMemberUserID, testTeamUUID,
		mock.MatchedBy(func(req *models.CreatePromptRequest) bool {
			return req.MCPExpose != nil && !*req.MCPExpose &&
				len(req.Labels) == 1 && req.Labels[0] == "team"
		}),
	).Return(&models.Prompt{ID: "id", ProjectID: testPromptProjectID, Slug: "s"}, nil)

	params := &CreatePromptParams{
		TeamID:    testTeamUUID,
		ProjectID: testPromptProjectID,
		Slug:      "s",
		Name:      "n",
		Body:      "b",
		MCPExpose: &expose,
		Labels:    []string{"team"},
	}
	result, _, err := srv.createPrompt(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	mockPromptService.AssertExpectations(t)
}

func TestCreatePrompt_ServiceError(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	mockPromptService.On(
		"CreatePrompt", testMemberUserID, testTeamUUID, mock.AnythingOfType("*models.CreatePromptRequest"),
	).Return(nil, errors.New("service error"))

	params := &CreatePromptParams{
		TeamID:    testTeamUUID,
		ProjectID: testPromptProjectID,
		Slug:      "test-slug",
		Name:      "Test Prompt",
		Body:      "Test body",
	}

	result, structuredResult, err := srv.createPrompt(context.Background(), nil, params, testMemberUserID)
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

// TestCreatePrompt_NonMemberTeamDenied verifies a team the user does not belong to
// is rejected with a generic access-denied message and the prompt service is never called.
func TestCreatePrompt_NonMemberTeamDenied(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	params := &CreatePromptParams{
		TeamID:    testOtherTeamUUID,
		ProjectID: testPromptProjectID,
		Slug:      "s",
		Name:      "n",
		Body:      "b",
	}

	result, structured, err := srv.createPrompt(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result on access denied")
	}
	assertGenericAccessDenied(t, result)
	mockPromptService.AssertNotCalled(t, "CreatePrompt")
}

// TestCreatePrompt_MissingTeamID verifies missing team_id yields a model-actionable error.
func TestCreatePrompt_MissingTeamID(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	params := &CreatePromptParams{ProjectID: testPromptProjectID, Slug: "s", Name: "n", Body: "b"}

	result, structured, err := srv.createPrompt(context.Background(), nil, params, testMemberUserID)
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
	mockPromptService.AssertNotCalled(t, "CreatePrompt")
}

func TestCreatePrompt_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockPromptService := newPromptTestServer(t)

			params := &CreatePromptParams{
				TeamID:    testTeamUUID,
				ProjectID: tc.projectID,
				Slug:      "any",
				Name:      "any",
				Body:      "any",
			}
			result, structured, err := srv.createPrompt(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on validation rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockPromptService.AssertNotCalled(t, "CreatePrompt")
		})
	}
}

func TestUpdatePrompt_Success(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	expectedPrompt := &models.Prompt{
		ID: "test-id", ProjectID: testPromptProjectID, Slug: "test-slug", Name: "Updated", Body: "Updated body",
	}
	mockPromptService.On(
		"UpdatePromptBySlug",
		testMemberUserID, testTeamUUID, "test-slug",
		mock.MatchedBy(func(req *models.UpdatePromptRequest) bool {
			return req.Name != nil && *req.Name == "Updated" &&
				req.Body != nil && *req.Body == "Updated body"
		}),
	).Return(expectedPrompt, nil)

	params := &UpdatePromptParams{
		TeamID: testTeamUUID,
		Slug:   "test-slug",
		Name:   "Updated",
		Body:   "Updated body",
	}
	result, structuredResult, err := srv.updatePrompt(context.Background(), nil, params, testMemberUserID)
	resp := verifyPromptWriteResult(t, result, structuredResult, err)
	if resp == nil {
		return
	}
	if resp.ID != expectedPrompt.ID {
		t.Errorf("got ID %v want %v", resp.ID, expectedPrompt.ID)
	}
}

// TestUpdatePrompt_OptionalFields verifies only provided fields are forwarded as pointers,
// and unset string fields stay nil (pointer semantics matching models.UpdatePromptRequest).
func TestUpdatePrompt_OptionalFields(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	expose := true
	mockPromptService.On(
		"UpdatePromptBySlug",
		testMemberUserID, testTeamUUID, "test-slug",
		mock.MatchedBy(func(req *models.UpdatePromptRequest) bool {
			// only status + mcp_expose provided; name/body/description must remain nil
			return req.Status != nil && *req.Status == "published" &&
				req.MCPExpose != nil && *req.MCPExpose &&
				req.Name == nil && req.Body == nil && req.Description == nil
		}),
	).Return(&models.Prompt{ID: "id", ProjectID: testPromptProjectID, Slug: "test-slug"}, nil)

	params := &UpdatePromptParams{
		TeamID:    testTeamUUID,
		Slug:      "test-slug",
		Status:    "published",
		MCPExpose: &expose,
	}
	result, _, err := srv.updatePrompt(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	mockPromptService.AssertExpectations(t)
}

func TestUpdatePrompt_ServiceError(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	mockPromptService.On(
		"UpdatePromptBySlug",
		testMemberUserID, testTeamUUID, "nonexistent",
		mock.AnythingOfType("*models.UpdatePromptRequest"),
	).Return(nil, errors.New("prompt not found"))

	params := &UpdatePromptParams{TeamID: testTeamUUID, Slug: "nonexistent", Name: "x"}
	result, structured, err := srv.updatePrompt(context.Background(), nil, params, testMemberUserID)
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

func TestUpdatePrompt_NonMemberTeamDenied(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	params := &UpdatePromptParams{TeamID: testOtherTeamUUID, Slug: "s", Name: "x"}
	result, structured, err := srv.updatePrompt(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockPromptService.AssertNotCalled(t, "UpdatePromptBySlug")
}

func TestRenderPrompt_Success(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)
	mockPromptService.On("RenderPrompt", testMemberUserID, testTeamUUID, "deploy", map[string]string{"env": "prod"}).
		Return(&models.RenderPromptResponse{RenderedBody: "deploy to prod"}, nil)

	params := &RenderPromptParams{TeamID: testTeamSlug, Slug: "deploy", Arguments: map[string]string{"env": "prod"}}
	result, structured, err := srv.renderPrompt(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	// The rendered body is the text content so a client can use it directly.
	assert.Equal(t, "deploy to prod", extractText(t, result))
	rendered, ok := structured.(*models.RenderPromptResponse)
	assert.True(t, ok)
	assert.Equal(t, "deploy to prod", rendered.RenderedBody)
}

func TestRenderPrompt_EmptySlug(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	params := &RenderPromptParams{TeamID: testTeamUUID, Slug: "  "}
	result, structured, err := srv.renderPrompt(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
	mockPromptService.AssertNotCalled(t, "RenderPrompt")
}

func TestRenderPrompt_NonMemberTeamDenied(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)

	params := &RenderPromptParams{TeamID: testOtherTeamUUID, Slug: "deploy"}
	result, structured, err := srv.renderPrompt(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
	mockPromptService.AssertNotCalled(t, "RenderPrompt")
}

func TestRenderPrompt_ServiceError(t *testing.T) {
	srv, mockPromptService := newPromptTestServer(t)
	mockPromptService.On("RenderPrompt", testMemberUserID, testTeamUUID, "deploy", mock.Anything).
		Return(nil, errors.New("boom"))

	params := &RenderPromptParams{TeamID: testTeamUUID, Slug: "deploy"}
	result, structured, err := srv.renderPrompt(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
}
