package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

const testProjectID = "550e8400-e29b-41d4-a716-446655440001"

// newMemoryTestServer builds a server whose memory + team services are mocked,
// with the member user belonging to testTeamUUID/testTeamSlug so resolveTeam succeeds.
func newMemoryTestServer(t *testing.T) (*Server, *mocks.MockMemoryServiceInterface) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	mockMemoryService := mocks.NewMockMemoryServiceInterface(t)
	mockTeamService := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{
		MemoryServiceMock: mockMemoryService,
		TeamServiceMock:   mockTeamService,
	}
	stubUserTeams(mockTeamService, []models.Team{memberTeam()})
	return srv, mockMemoryService
}

func buildTestMemory() *models.Memory {
	return &models.Memory{
		ID:        "test-memory-id",
		ProjectID: testProjectID,
		Text:      "Test memory content",
		Metadata:  map[string]interface{}{"key": "value"},
	}
}

func TestStoreMemory_Success(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	expectedMemory := buildTestMemory()
	mockMemoryService.On(
		"CreateMemory", testMemberUserID, testTeamUUID,
		mock.MatchedBy(func(req *models.CreateMemoryRequest) bool {
			return req.Text == "Test memory content" && req.ProjectID == testProjectID
		}),
	).Return(expectedMemory, nil)

	params := &StoreMemoryParams{
		TeamID:    testTeamSlug, // exercise slug resolution
		ProjectID: testProjectID,
		Text:      "Test memory content",
		Metadata:  map[string]interface{}{"key": "value"},
	}

	result, structuredResult, err := srv.storeMemory(context.Background(), nil, params, testMemberUserID)
	assertStoreMemoryResult(t, result, structuredResult, err, expectedMemory)
}

func assertStoreMemoryResult(
	t *testing.T,
	result *mcp.CallToolResult,
	structuredResult interface{},
	err error,
	expectedMemory *models.Memory,
) {
	if err != nil {
		t.Errorf("storeMemory returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	resp, ok := structuredResult.(*memoryWriteResponse)
	if !ok {
		t.Fatal("storeMemory returned wrong structured result type")
	}
	if resp.ID != expectedMemory.ID {
		t.Errorf("got ID %v want %v", resp.ID, expectedMemory.ID)
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("storeMemory returned non-text content")
	}
	var responseWrite memoryWriteResponse
	if err := json.Unmarshal([]byte(textContent.Text), &responseWrite); err != nil {
		t.Errorf("storeMemory returned invalid JSON: %v", err)
	}
	if responseWrite.ID != expectedMemory.ID {
		t.Errorf("JSON response has wrong ID: got %v want %v", responseWrite.ID, expectedMemory.ID)
	}
}

func TestStoreMemory_ServiceError(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	mockMemoryService.On(
		"CreateMemory", testMemberUserID, testTeamUUID, mock.AnythingOfType("*models.CreateMemoryRequest"),
	).Return(nil, errors.New("service error"))

	params := &StoreMemoryParams{TeamID: testTeamUUID, ProjectID: testProjectID, Text: "Test memory content"}
	result, structuredResult, err := srv.storeMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if structuredResult != nil {
		t.Error("expected nil structured result on error")
	}
}

func TestStoreMemory_NonMemberTeamDenied(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	params := &StoreMemoryParams{TeamID: testOtherTeamUUID, ProjectID: testProjectID, Text: "x"}
	result, structured, err := srv.storeMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockMemoryService.AssertNotCalled(t, "CreateMemory")
}

func TestStoreMemory_MissingTeamID(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	params := &StoreMemoryParams{ProjectID: testProjectID, Text: "x"}
	result, structured, err := srv.storeMemory(context.Background(), nil, params, testMemberUserID)
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
	mockMemoryService.AssertNotCalled(t, "CreateMemory")
}

func TestListMemoriesByProject_Success(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	expectedResponse := buildMemoryListResponse()
	mockMemoryService.On(
		"ListMemories", testMemberUserID,
		mock.MatchedBy(func(filters services.MemoryFilters) bool {
			return filters.TeamID == testTeamUUID && filters.Page == 1 && filters.Limit == 10 &&
				filters.ProjectID != nil && *filters.ProjectID == testProjectID
		}),
	).Return(expectedResponse, nil)

	params := &ListMemoriesByProjectParams{TeamID: testTeamUUID, ProjectID: testProjectID, Page: 1, Limit: 10}
	result, structuredResult, err := srv.listMemoriesByProject(context.Background(), nil, params, testMemberUserID)
	assertListMemoriesResult(t, result, structuredResult, err)
	mockMemoryService.AssertExpectations(t)
}

func buildMemoryListResponse() *models.MemoryListResponse {
	return &models.MemoryListResponse{
		Memories: []models.Memory{
			{ID: "test-memory-1", ProjectID: testProjectID, Text: "Test memory 1", Metadata: map[string]interface{}{}},
			{ID: "test-memory-2", ProjectID: testProjectID, Text: "Test memory 2", Metadata: map[string]interface{}{}},
		},
		TotalCount: 2, Page: 1, PerPage: 10, TotalPages: 1,
	}
}

func assertListMemoriesResult(t *testing.T, result *mcp.CallToolResult, structuredResult interface{}, err error) {
	if err != nil {
		t.Errorf("listMemoriesByProject returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	response, ok := structuredResult.(*memorySearchResponse)
	if !ok {
		t.Fatal("listMemoriesByProject returned wrong structured result type")
	}
	if len(response.Memories) != 2 {
		t.Errorf("got %d memories want 2", len(response.Memories))
	}
}

func TestListMemoriesByProject_NonMemberTeamDenied(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	params := &ListMemoriesByProjectParams{TeamID: testOtherTeamSlug, ProjectID: testProjectID, Page: 1, Limit: 10}
	result, structured, err := srv.listMemoriesByProject(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockMemoryService.AssertNotCalled(t, "ListMemories")
}

func TestGetMemory_Success(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	expectedMemory := &models.Memory{ID: "test-memory-id", Text: "Test memory content"}
	mockMemoryService.On("GetMemory", testMemberUserID, testTeamUUID, "test-memory-id").Return(expectedMemory, nil)

	params := &GetMemoryParams{TeamID: testTeamUUID, MemoryID: "test-memory-id"}
	result, structuredResult, err := srv.getMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	memory, ok := structuredResult.(*models.Memory)
	if !ok || memory.ID != expectedMemory.ID {
		t.Error("getMemory returned wrong structured result")
	}
}

func TestGetMemory_RecordsAccessEvent(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)
	spy := &spyResourceAccessService{}
	srv.container.(*TestContainer).ResourceAccessServiceMock = spy

	expectedMemory := &models.Memory{ID: "test-memory-id", Text: "Test memory content"}
	mockMemoryService.On("GetMemory", testMemberUserID, testTeamUUID, "test-memory-id").Return(expectedMemory, nil)

	params := &GetMemoryParams{TeamID: testTeamUUID, MemoryID: "test-memory-id"}
	_, _, err := srv.getMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 recorded access event, got %d", len(calls))
	}
	event := calls[0]
	if event.Source != resourceaccess.SourceMCP {
		t.Errorf("expected source %q, got %q", resourceaccess.SourceMCP, event.Source)
	}
	if event.ResourceType != resourceTypeMemory {
		t.Errorf("expected resource_type %q, got %q", resourceTypeMemory, event.ResourceType)
	}
	if event.ResourceID != expectedMemory.ID {
		t.Errorf("expected resource_id %q, got %q", expectedMemory.ID, event.ResourceID)
	}
}

func TestGetMemory_NonMemberTeamDenied(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	params := &GetMemoryParams{TeamID: testOtherTeamUUID, MemoryID: "m"}
	result, structured, err := srv.getMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockMemoryService.AssertNotCalled(t, "GetMemory")
}

func TestGetMemory_NotFound(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	mockMemoryService.On("GetMemory", testMemberUserID, testTeamUUID, "nonexistent").
		Return(nil, errors.New("not found"))

	params := &GetMemoryParams{TeamID: testTeamUUID, MemoryID: "nonexistent"}
	result, structuredResult, err := srv.getMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if structuredResult != nil {
		t.Error("expected nil structured result on error")
	}
}

func TestUpdateMemory_Success(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	expectedMemory := &models.Memory{ID: "test-memory-id", Text: "Updated memory content"}
	mockMemoryService.On(
		"UpdateMemory", testMemberUserID, testTeamUUID, "test-memory-id",
		mock.MatchedBy(func(req *models.UpdateMemoryRequest) bool {
			return req.Text != nil && *req.Text == "Updated memory content" && req.Metadata["updated"] == true
		}),
	).Return(expectedMemory, nil)

	params := &UpdateMemoryParams{
		TeamID:   testTeamUUID,
		MemoryID: "test-memory-id",
		Text:     "Updated memory content",
		Metadata: map[string]interface{}{"updated": true},
	}
	result, structuredResult, err := srv.updateMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	resp, ok := structuredResult.(*memoryWriteResponse)
	if !ok || resp.ID != expectedMemory.ID {
		t.Error("updateMemory returned wrong structured result")
	}
}

func TestUpdateMemory_NonMemberTeamDenied(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	params := &UpdateMemoryParams{TeamID: testOtherTeamUUID, MemoryID: "m", Text: "x"}
	result, structured, err := srv.updateMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockMemoryService.AssertNotCalled(t, "UpdateMemory")
}

func TestStoreMemory_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockMemoryService := newMemoryTestServer(t)

			params := &StoreMemoryParams{TeamID: testTeamUUID, ProjectID: tc.projectID, Text: "hello"}
			result, structured, err := srv.storeMemory(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockMemoryService.AssertNotCalled(t, "CreateMemory")
		})
	}
}

func TestStoreMemory_NonExistentProjectUUID(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	mockMemoryService.On(
		"CreateMemory", testMemberUserID, testTeamUUID, mock.AnythingOfType("*models.CreateMemoryRequest"),
	).Return(nil, errors.New("project not found"))

	params := &StoreMemoryParams{TeamID: testTeamUUID, ProjectID: nonExistentProjectUUID, Text: "hello"}
	result, structured, err := srv.storeMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected function error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true result")
	}
	if structured != nil {
		t.Error("expected nil structured result on error")
	}
	text := extractText(t, result)
	if strings.Contains(text, "pq:") || strings.Contains(text, "22P02") {
		t.Errorf("response leaked driver prefix or SQLSTATE: %q", text)
	}
}

func TestListMemoriesByProject_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockMemoryService := newMemoryTestServer(t)

			params := &ListMemoriesByProjectParams{TeamID: testTeamUUID, ProjectID: tc.projectID, Page: 1, Limit: 10}
			result, structured, err := srv.listMemoriesByProject(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockMemoryService.AssertNotCalled(t, "ListMemories")
		})
	}
}

// TestStoreMemory_InvalidStatus rejects an out-of-enum status before the service
// is ever called.
func TestStoreMemory_InvalidStatus(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	params := &StoreMemoryParams{
		TeamID: testTeamUUID, ProjectID: testProjectID, Text: "hello", Status: "bogus",
	}
	result, structured, err := srv.storeMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected function error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result on rejection")
	}
	assertValidationFailure(t, result, []string{"active", "draft", "archived"})
	mockMemoryService.AssertNotCalled(t, "CreateMemory")
}

// TestStoreMemory_StatusThreaded passes a valid status through to the create request.
func TestStoreMemory_StatusThreaded(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	expectedMemory := buildTestMemory()
	mockMemoryService.On(
		"CreateMemory", testMemberUserID, testTeamUUID,
		mock.MatchedBy(func(req *models.CreateMemoryRequest) bool {
			return req.Status != nil && *req.Status == models.MemoryStatusDraft
		}),
	).Return(expectedMemory, nil)

	params := &StoreMemoryParams{
		TeamID: testTeamUUID, ProjectID: testProjectID, Text: "hello", Status: models.MemoryStatusDraft,
	}
	result, _, err := srv.storeMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected function error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	mockMemoryService.AssertExpectations(t)
}

// TestUpdateMemory_InvalidStatus rejects an out-of-enum status before the service
// is ever called.
func TestUpdateMemory_InvalidStatus(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	params := &UpdateMemoryParams{
		TeamID: testTeamUUID, MemoryID: "mem-1", Status: "bogus",
	}
	result, structured, err := srv.updateMemory(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected function error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result on rejection")
	}
	assertValidationFailure(t, result, []string{"active", "draft", "archived"})
	mockMemoryService.AssertNotCalled(t, "UpdateMemory")
}

// TestListMemoriesByProject_InvalidStatus rejects an out-of-enum status filter
// before the service is ever called.
func TestListMemoriesByProject_InvalidStatus(t *testing.T) {
	srv, mockMemoryService := newMemoryTestServer(t)

	params := &ListMemoriesByProjectParams{
		TeamID: testTeamUUID, ProjectID: testProjectID, Status: "bogus",
	}
	result, structured, err := srv.listMemoriesByProject(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected function error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result on rejection")
	}
	assertValidationFailure(t, result, []string{"active", "draft", "archived"})
	mockMemoryService.AssertNotCalled(t, "ListMemories")
}
