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
)

func buildProjectListResponse() *models.ProjectListResponse {
	return &models.ProjectListResponse{
		Projects: []models.ProjectResponse{
			{Project: models.Project{ID: "project-1", Name: "Project One", Slug: "project-one"}},
			{Project: models.Project{ID: "project-2", Name: "Project Two", Slug: "project-two"}},
		},
		TotalCount: 2, Page: 1, PerPage: 10, TotalPages: 1,
	}
}

// newProjectTestServer builds a server whose project + team services are mocked,
// with the member user belonging to testTeamUUID/testTeamSlug so resolveTeam succeeds.
func newProjectTestServer(t *testing.T) (*Server, *mocks.MockProjectServiceInterface) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	mockProjectService := mocks.NewMockProjectServiceInterface(t)
	mockTeamService := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{
		ProjectServiceMock: mockProjectService,
		TeamServiceMock:    mockTeamService,
	}
	stubUserTeams(mockTeamService, []models.Team{memberTeam()})
	return srv, mockProjectService
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestListProjects_Success(t *testing.T) {
	srv, mockProjectService := newProjectTestServer(t)

	expectedResponse := buildProjectListResponse()
	mockProjectService.On(
		"ListProjects", testMemberUserID,
		mock.MatchedBy(func(filters services.ProjectFilters) bool {
			return filters.TeamID == testTeamUUID && filters.Page == 1 && filters.Limit == 10
		}),
	).Return(expectedResponse, nil)

	params := &ListProjectsParams{TeamID: testTeamSlug, Page: 1, Limit: 10}
	result, structuredResult, err := srv.listProjects(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	response, ok := structuredResult.(*models.ProjectListResponse)
	if !ok {
		t.Fatal("listProjects returned wrong structured result type")
	}
	if len(response.Projects) != 2 {
		t.Errorf("got %d projects want 2", len(response.Projects))
	}
	textContent := result.Content[0].(*mcp.TextContent)
	var jsonResponse models.ProjectListResponse
	if err := json.Unmarshal([]byte(textContent.Text), &jsonResponse); err != nil {
		t.Errorf("invalid JSON: %v", err)
	}
	if jsonResponse.TotalCount != 2 {
		t.Errorf("got total_count %v want 2", jsonResponse.TotalCount)
	}
}

func TestListProjects_ServiceError(t *testing.T) {
	srv, mockProjectService := newProjectTestServer(t)

	mockProjectService.On(
		"ListProjects", testMemberUserID, mock.AnythingOfType("services.ProjectFilters"),
	).Return(nil, errors.New("service error"))

	params := &ListProjectsParams{TeamID: testTeamUUID}
	result, structuredResult, err := srv.listProjects(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected Go error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if structuredResult != nil {
		t.Error("expected nil structured result on error")
	}
}

func TestListProjects_NonMemberTeamDenied(t *testing.T) {
	srv, mockProjectService := newProjectTestServer(t)

	params := &ListProjectsParams{TeamID: testOtherTeamUUID}
	result, structured, err := srv.listProjects(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockProjectService.AssertNotCalled(t, "ListProjects")
}

func TestListProjects_MissingTeamID(t *testing.T) {
	srv, mockProjectService := newProjectTestServer(t)

	params := &ListProjectsParams{}
	result, _, err := srv.listProjects(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	text := extractText(t, result)
	if !strings.Contains(text, "team_id is required") || !strings.Contains(text, "vibexp_io_list_teams") {
		t.Errorf("expected model-actionable missing team_id error, got %q", text)
	}
	mockProjectService.AssertNotCalled(t, "ListProjects")
}

func TestListProjects_PaginationDefaultsAndCap(t *testing.T) {
	srv, mockProjectService := newProjectTestServer(t)

	expectedResponse := buildProjectListResponse()
	mockProjectService.On(
		"ListProjects", testMemberUserID,
		mock.MatchedBy(func(filters services.ProjectFilters) bool {
			// page=0 normalized to 1, limit=200 capped to 10
			return filters.TeamID == testTeamUUID && filters.Page == 1 && filters.Limit == 10
		}),
	).Return(expectedResponse, nil)

	params := &ListProjectsParams{TeamID: testTeamUUID, Page: 0, Limit: 200}
	result, _, err := srv.listProjects(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected success")
	}
	mockProjectService.AssertExpectations(t)
}
