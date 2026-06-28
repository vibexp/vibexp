package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// Test fixtures for valid UUID project_id values used by artifact MCP tool tests.
const (
	testArtifactProjectID  = "550e8400-e29b-41d4-a716-446655440010"
	testArtifactProjectID2 = "550e8400-e29b-41d4-a716-446655440011"
	testArtifactProjectID3 = "550e8400-e29b-41d4-a716-446655440012"
)

// newArtifactTestServer builds a server whose artifact + team services are mocked,
// with the member user belonging to testTeamUUID/testTeamSlug so resolveTeam succeeds.
func newArtifactTestServer(t *testing.T) (*Server, *mocks.MockArtifactServiceInterface) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	mockArtifactService := mocks.NewMockArtifactServiceInterface(t)
	mockTeamService := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{
		ArtifactServiceMock: mockArtifactService,
		TeamServiceMock:     mockTeamService,
	}
	stubUserTeams(mockTeamService, []models.Team{memberTeam()})
	return srv, mockArtifactService
}

func verifyArtifactCallResult(
	t *testing.T,
	result *mcp.CallToolResult,
	structuredResult interface{},
	err error,
) *artifactWriteResponse {
	t.Helper()
	if err != nil {
		t.Errorf("createArtifact returned error: %v", err)
		return nil
	}
	if result == nil {
		t.Error("createArtifact returned nil result")
		return nil
	}
	if result.IsError {
		t.Errorf("createArtifact returned error result: %s", extractText(t, result))
		return nil
	}
	if len(result.Content) == 0 {
		t.Error("createArtifact returned no content")
		return nil
	}
	resp, ok := structuredResult.(*artifactWriteResponse)
	if !ok {
		t.Error("createArtifact returned wrong structured result type")
		return nil
	}
	return resp
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestCreateArtifact_Success(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	expectedArtifact := &models.Artifact{
		ID:        "test-id",
		ProjectID: testArtifactProjectID,
		Slug:      "test-slug",
		Title:     "Test Artifact",
		Content:   "Test content",
	}
	mockArtifactService.On(
		"CreateArtifact", testMemberUserID, testTeamUUID,
		mock.MatchedBy(func(req *models.CreateArtifactRequest) bool {
			return req.ProjectID == testArtifactProjectID && req.Slug == "test-slug"
		}),
	).Return(expectedArtifact, nil)

	params := &CreateArtifactParams{
		TeamID:    testTeamSlug, // exercise slug resolution on the create path
		ProjectID: testArtifactProjectID,
		Slug:      "test-slug",
		Title:     "Test Artifact",
		Content:   "Test content",
	}

	result, structuredResult, err := srv.createArtifact(context.Background(), nil, params, testMemberUserID)

	artifact := verifyArtifactCallResult(t, result, structuredResult, err)
	if artifact == nil {
		return
	}
	if artifact.ID != expectedArtifact.ID {
		t.Errorf("got ID %v want %v", artifact.ID, expectedArtifact.ID)
	}
}

func TestCreateArtifact_ServiceError(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	mockArtifactService.On(
		"CreateArtifact", testMemberUserID, testTeamUUID, mock.AnythingOfType("*models.CreateArtifactRequest"),
	).Return(nil, errors.New("service error"))

	params := &CreateArtifactParams{
		TeamID:    testTeamUUID,
		ProjectID: testArtifactProjectID,
		Slug:      "test-slug",
		Title:     "Test Artifact",
		Content:   "Test content",
	}

	result, structuredResult, err := srv.createArtifact(context.Background(), nil, params, testMemberUserID)
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

// TestCreateArtifact_NonMemberTeamDenied verifies that supplying a team the user
// does not belong to is rejected with a generic access-denied message and the
// artifact service is never called.
func TestCreateArtifact_NonMemberTeamDenied(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	params := &CreateArtifactParams{
		TeamID:    testOtherTeamUUID,
		ProjectID: testArtifactProjectID,
		Slug:      "s",
		Title:     "t",
		Content:   "c",
	}

	result, structured, err := srv.createArtifact(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result on access denied")
	}
	assertGenericAccessDenied(t, result)
	mockArtifactService.AssertNotCalled(t, "CreateArtifact")
}

// TestCreateArtifact_MissingTeamID verifies missing team_id yields a model-actionable error.
func TestCreateArtifact_MissingTeamID(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	params := &CreateArtifactParams{ProjectID: testArtifactProjectID, Slug: "s", Title: "t", Content: "c"}

	result, structured, err := srv.createArtifact(context.Background(), nil, params, testMemberUserID)
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
	mockArtifactService.AssertNotCalled(t, "CreateArtifact")
}

func TestGetArtifact_Success(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	expectedArtifact := &models.Artifact{
		ID:        "test-id",
		ProjectID: testArtifactProjectID,
		Slug:      "test-slug",
		Title:     "Test Artifact",
		Content:   "Test content",
	}
	mockArtifactService.On(
		"GetArtifactByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testArtifactProjectID, "test-slug",
	).Return(expectedArtifact, nil)

	params := &GetArtifactParams{TeamID: testTeamUUID, ProjectID: testArtifactProjectID, Slug: "test-slug"}
	result, structuredResult, err := srv.getArtifact(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	artifact, ok := structuredResult.(*models.Artifact)
	if !ok || artifact.ID != expectedArtifact.ID {
		t.Error("getArtifact returned wrong structured result")
	}
}

func TestGetArtifact_RecordsAccessEvent(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)
	spy := &spyResourceAccessService{}
	srv.container.(*TestContainer).ResourceAccessServiceMock = spy

	expectedArtifact := &models.Artifact{ID: "test-id", ProjectID: testArtifactProjectID, Slug: "test-slug"}
	mockArtifactService.On(
		"GetArtifactByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testArtifactProjectID, "test-slug",
	).Return(expectedArtifact, nil)

	params := &GetArtifactParams{TeamID: testTeamUUID, ProjectID: testArtifactProjectID, Slug: "test-slug"}
	_, _, err := srv.getArtifact(context.Background(), nil, params, testMemberUserID)
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
	if event.ResourceType != resourceTypeArtifact {
		t.Errorf("expected resource_type %q, got %q", resourceTypeArtifact, event.ResourceType)
	}
	if event.ResourceID != expectedArtifact.ID {
		t.Errorf("expected resource_id %q, got %q", expectedArtifact.ID, event.ResourceID)
	}
}

func TestGetArtifact_NonMemberTeamDenied(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	params := &GetArtifactParams{TeamID: testOtherTeamSlug, ProjectID: testArtifactProjectID, Slug: "s"}
	result, structured, err := srv.getArtifact(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockArtifactService.AssertNotCalled(t, "GetArtifactByProjectIDAndSlugInTeam")
}

func TestGetArtifact_NotFound(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	mockArtifactService.On(
		"GetArtifactByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testArtifactProjectID, "nonexistent",
	).Return(nil, errors.New("not found"))

	params := &GetArtifactParams{TeamID: testTeamUUID, ProjectID: testArtifactProjectID, Slug: "nonexistent"}
	result, structured, err := srv.getArtifact(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if structured != nil {
		t.Error("expected nil structured result on error")
	}
}

// TestGetArtifact_CrossTeamArtifactNotReturned verifies that when the caller IS a
// member of the supplied team but the target artifact lives in a DIFFERENT team, the
// team-scoped lookup returns not-found rather than the artifact. The handler must call
// the team-scoped method with the resolved team (never the cross-team lookup), so an
// artifact bound only by user_id in another team cannot be read by passing team_id.
func TestGetArtifact_CrossTeamArtifactNotReturned(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	// Team-scoped lookup for the resolved (member) team finds nothing because the
	// artifact actually lives in another team.
	mockArtifactService.On(
		"GetArtifactByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testArtifactProjectID, "in-other-team",
	).Return(nil, errors.New("artifact not found"))

	params := &GetArtifactParams{TeamID: testTeamUUID, ProjectID: testArtifactProjectID, Slug: "in-other-team"}
	result, structured, err := srv.getArtifact(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true result when artifact is in another team")
	}
	if structured != nil {
		t.Error("expected nil structured result on not-found")
	}
	// The cross-team lookup must never be used by this handler.
	mockArtifactService.AssertNotCalled(t, "GetArtifactByProjectIDAndSlug")
}

func TestListArtifactsByProject_Success(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	expectedResponse := buildListArtifactsResponse()
	mockArtifactService.On(
		"ListArtifactsByProject",
		testMemberUserID,
		testArtifactProjectID,
		mock.MatchedBy(func(filters services.ArtifactFilters) bool {
			return filters.ProjectID == testArtifactProjectID && filters.TeamID == testTeamUUID
		}),
	).Return(expectedResponse, nil)

	params := &ListArtifactsByProjectParams{TeamID: testTeamUUID, ProjectID: testArtifactProjectID, Page: 1, Limit: 10}
	result, structuredResult, err := srv.listArtifactsByProject(context.Background(), nil, params, testMemberUserID)
	assertListArtifactsResult(t, result, structuredResult, err)
	mockArtifactService.AssertExpectations(t)
}

func buildListArtifactsResponse() *models.ArtifactListResponse {
	return &models.ArtifactListResponse{
		Artifacts: []models.Artifact{
			{ID: "test-id-1", ProjectID: testArtifactProjectID, Slug: "test-slug-1", Title: "Test Artifact 1"},
			{ID: "test-id-2", ProjectID: testArtifactProjectID, Slug: "test-slug-2", Title: "Test Artifact 2"},
		},
		TotalCount: 2, Page: 1, PerPage: 10, TotalPages: 1,
	}
}

func assertListArtifactsResult(t *testing.T, result *mcp.CallToolResult, structuredResult interface{}, err error) {
	if err != nil {
		t.Errorf("listArtifactsByProject returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	response, ok := structuredResult.(*artifactSearchResponse)
	if !ok {
		t.Fatal("listArtifactsByProject returned wrong structured result type")
	}
	if len(response.Artifacts) != 2 {
		t.Errorf("got %d artifacts want 2", len(response.Artifacts))
	}
}

func TestListArtifactsByProject_NonMemberTeamDenied(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	params := &ListArtifactsByProjectParams{
		TeamID: testOtherTeamUUID, ProjectID: testArtifactProjectID, Page: 1, Limit: 10,
	}
	result, structured, err := srv.listArtifactsByProject(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockArtifactService.AssertNotCalled(t, "ListArtifactsByProject")
}

func TestListArtifactsByProject_FullDetailsRejected(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	params := &ListArtifactsByProjectParams{TeamID: testTeamUUID, ProjectID: testArtifactProjectID, FullDetails: true}
	result, structured, err := srv.listArtifactsByProject(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true when full_details=true")
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	mockArtifactService.AssertNotCalled(t, "ListArtifactsByProject")
}

// assertArtifactListJSONShape verifies content/version are always absent from search items.
func assertArtifactListJSONShape(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Error("expected TextContent in result")
		return
	}
	var rawResponse map[string]interface{}
	if err := json.Unmarshal([]byte(textContent.Text), &rawResponse); err != nil {
		t.Errorf("failed to unmarshal response JSON: %v", err)
		return
	}
	artifactsRaw, ok := rawResponse["artifacts"].([]interface{})
	if !ok || len(artifactsRaw) == 0 {
		t.Error("expected artifacts array in response")
		return
	}
	firstItem, ok := artifactsRaw[0].(map[string]interface{})
	if !ok {
		t.Error("expected artifact item to be a JSON object")
		return
	}
	if _, hasContent := firstItem["content"]; hasContent {
		t.Error("search_artifacts response item should NEVER contain 'content' key")
	}
	if _, hasVersion := firstItem["version"]; hasVersion {
		t.Error("search_artifacts response item should NEVER contain 'version' key")
	}
}

func TestListArtifactsByProject_JSONShapeOmitsContentAndVersion(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	listResponse := &models.ArtifactListResponse{
		Artifacts: []models.Artifact{
			{ID: "art-1", ProjectID: testArtifactProjectID3, Slug: "slug-1", Title: "Artifact One"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}
	mockArtifactService.On("ListArtifactsByProject", testMemberUserID, testArtifactProjectID3, mock.Anything).
		Return(listResponse, nil)

	params := &ListArtifactsByProjectParams{TeamID: testTeamUUID, ProjectID: testArtifactProjectID3, Page: 1, Limit: 10}
	result, _, err := srv.listArtifactsByProject(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}
	assertArtifactListJSONShape(t, result)
}

func TestUpdateArtifact_Success(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	expectedArtifact := buildUpdatedArtifact()
	setupUpdateArtifactMock(mockArtifactService, expectedArtifact)

	params := &UpdateArtifactParams{
		TeamID:    testTeamUUID,
		ProjectID: testArtifactProjectID,
		Slug:      "test-slug",
		Title:     "Updated Artifact",
		Content:   "Updated content",
	}
	result, structuredResult, err := srv.updateArtifact(context.Background(), nil, params, testMemberUserID)
	assertUpdateArtifactResult(t, result, structuredResult, err, expectedArtifact)
}

func buildUpdatedArtifact() *models.Artifact {
	return &models.Artifact{
		ID: "test-id", ProjectID: testArtifactProjectID, Slug: "test-slug",
		Title: "Updated Artifact", Content: "Updated content",
	}
}

func setupUpdateArtifactMock(
	mockArtifactService *mocks.MockArtifactServiceInterface,
	expectedArtifact *models.Artifact,
) {
	mockArtifactService.On(
		"UpdateArtifactByProjectIDAndSlugInTeam",
		testMemberUserID,
		testTeamUUID,
		testArtifactProjectID,
		"test-slug",
		mock.MatchedBy(func(req *models.UpdateArtifactRequest) bool {
			return req.Title != nil && *req.Title == "Updated Artifact" &&
				req.Content != nil && *req.Content == "Updated content"
		}),
	).Return(expectedArtifact, nil)
}

func assertUpdateArtifactResult(
	t *testing.T,
	result *mcp.CallToolResult,
	structuredResult interface{},
	err error,
	expectedArtifact *models.Artifact,
) {
	if err != nil {
		t.Errorf("updateArtifact returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %v", result)
	}
	resp, ok := structuredResult.(*artifactWriteResponse)
	if !ok {
		t.Fatal("updateArtifact returned wrong structured result type")
	}
	if resp.ID != expectedArtifact.ID {
		t.Errorf("got ID %v want %v", resp.ID, expectedArtifact.ID)
	}
}

func TestUpdateArtifact_NonMemberTeamDenied(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	params := &UpdateArtifactParams{TeamID: testOtherTeamUUID, ProjectID: testArtifactProjectID, Slug: "s", Title: "x"}
	result, structured, err := srv.updateArtifact(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Error("expected nil structured result")
	}
	assertGenericAccessDenied(t, result)
	mockArtifactService.AssertNotCalled(t, "UpdateArtifactByProjectIDAndSlugInTeam")
}

// TestUpdateArtifact_CrossTeamArtifactNotMutated verifies that when the caller IS a
// member of the supplied team but the target artifact lives in a DIFFERENT team, the
// team-scoped update returns not-found and never mutates an artifact bound only by
// user_id in another team. The handler must call the team-scoped method with the
// resolved team and never the cross-team update.
func TestUpdateArtifact_CrossTeamArtifactNotMutated(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	mockArtifactService.On(
		"UpdateArtifactByProjectIDAndSlugInTeam",
		testMemberUserID, testTeamUUID, testArtifactProjectID, "in-other-team",
		mock.AnythingOfType("*models.UpdateArtifactRequest"),
	).Return(nil, errors.New("artifact not found"))

	params := &UpdateArtifactParams{
		TeamID:    testTeamUUID,
		ProjectID: testArtifactProjectID,
		Slug:      "in-other-team",
		Title:     "Hijacked Title",
	}
	result, structured, err := srv.updateArtifact(context.Background(), nil, params, testMemberUserID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true result when artifact is in another team")
	}
	if structured != nil {
		t.Error("expected nil structured result on not-found")
	}
	// The cross-team mutating method must never be used by this handler.
	mockArtifactService.AssertNotCalled(t, "UpdateArtifactByProjectIDAndSlug")
}

// nonExistentProjectUUID is well-formed but matches no row in the DB.
const nonExistentProjectUUID = "00000000-0000-0000-0000-000000000000"

// TestCreateArtifact_InvalidProjectID covers empty and malformed project_id rejection.
func TestCreateArtifact_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockArtifactService := newArtifactTestServer(t)

			params := &CreateArtifactParams{
				TeamID:    testTeamUUID,
				ProjectID: tc.projectID,
				Slug:      "any",
				Title:     "any",
				Content:   "any",
			}
			result, structured, err := srv.createArtifact(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on validation rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockArtifactService.AssertNotCalled(t, "CreateArtifact")
		})
	}
}

// TestCreateArtifact_NonExistentProjectUUID verifies downstream not-found does not leak driver prefix.
func TestCreateArtifact_NonExistentProjectUUID(t *testing.T) {
	srv, mockArtifactService := newArtifactTestServer(t)

	mockArtifactService.On(
		"CreateArtifact", testMemberUserID, testTeamUUID, mock.AnythingOfType("*models.CreateArtifactRequest"),
	).Return(nil, errors.New("project not found"))

	params := &CreateArtifactParams{
		TeamID:    testTeamUUID,
		ProjectID: nonExistentProjectUUID,
		Slug:      "any",
		Title:     "any",
		Content:   "any",
	}
	result, structured, err := srv.createArtifact(context.Background(), nil, params, testMemberUserID)
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

func TestListArtifactsByProject_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockArtifactService := newArtifactTestServer(t)

			params := &ListArtifactsByProjectParams{TeamID: testTeamUUID, ProjectID: tc.projectID, Page: 1, Limit: 10}
			result, structured, err := srv.listArtifactsByProject(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockArtifactService.AssertNotCalled(t, "ListArtifactsByProject")
		})
	}
}

func TestGetArtifact_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockArtifactService := newArtifactTestServer(t)

			params := &GetArtifactParams{TeamID: testTeamUUID, ProjectID: tc.projectID, Slug: "any"}
			result, structured, err := srv.getArtifact(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockArtifactService.AssertNotCalled(t, "GetArtifactByProjectIDAndSlugInTeam")
		})
	}
}

func TestUpdateArtifact_InvalidProjectID(t *testing.T) {
	for _, tc := range invalidProjectIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockArtifactService := newArtifactTestServer(t)

			params := &UpdateArtifactParams{TeamID: testTeamUUID, ProjectID: tc.projectID, Slug: "any", Title: "x"}
			result, structured, err := srv.updateArtifact(context.Background(), nil, params, testMemberUserID)
			if err != nil {
				t.Errorf("unexpected function error: %v", err)
			}
			if structured != nil {
				t.Error("expected nil structured result on rejection")
			}
			assertValidationFailure(t, result, tc.substrMust)
			mockArtifactService.AssertNotCalled(t, "UpdateArtifactByProjectIDAndSlugInTeam")
		})
	}
}

// Test container for dependency injection in tests
type TestContainer struct {
	BaseMockContainer         // Embed base container for default nil implementations
	ArtifactServiceMock       services.ArtifactServiceInterface
	MemoryServiceMock         services.MemoryServiceInterface
	PromptServiceMock         services.PromptServiceInterface
	BlueprintServiceMock      services.BlueprintServiceInterface
	APIKeyServiceMock         services.APIKeyServiceInterface
	AuthServiceMock           services.AuthServiceInterface
	TeamServiceMock           services.TeamServiceInterface
	ProjectServiceMock        services.ProjectServiceInterface
	FeedServiceMock           services.FeedServiceInterface
	FeedItemServiceMock       services.FeedItemServiceInterface
	FeedItemReplyServiceMock  services.FeedItemReplyServiceInterface
	ResourceUsageServiceMock  services.ResourceUsageServiceInterface
	SearchServiceMock         services.SearchServiceInterface
	ResourceAccessServiceMock resourceaccess.ResourceAccessService
	AttachmentServiceMock     services.AttachmentServiceInterface
	EmbeddingServiceMock      services.EmbeddingServiceInterface
}

func (tc *TestContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return tc.ResourceAccessServiceMock
}

func (tc *TestContainer) ArtifactService() services.ArtifactServiceInterface {
	return tc.ArtifactServiceMock
}

func (tc *TestContainer) MemoryService() services.MemoryServiceInterface {
	return tc.MemoryServiceMock
}

func (tc *TestContainer) PromptService() services.PromptServiceInterface {
	if tc.PromptServiceMock != nil {
		return tc.PromptServiceMock
	}
	return nil
}

func (tc *TestContainer) BlueprintService() services.BlueprintServiceInterface {
	if tc.BlueprintServiceMock != nil {
		return tc.BlueprintServiceMock
	}
	return nil
}

func (tc *TestContainer) APIKeyService() services.APIKeyServiceInterface {
	if tc.APIKeyServiceMock != nil {
		return tc.APIKeyServiceMock
	}
	return nil
}

func (tc *TestContainer) AuthService() services.AuthServiceInterface {
	if tc.AuthServiceMock != nil {
		return tc.AuthServiceMock
	}
	return nil
}

func (tc *TestContainer) TeamService() services.TeamServiceInterface {
	if tc.TeamServiceMock != nil {
		return tc.TeamServiceMock
	}
	return nil
}

func (tc *TestContainer) ProjectService() services.ProjectServiceInterface {
	if tc.ProjectServiceMock != nil {
		return tc.ProjectServiceMock
	}
	return nil
}

func (tc *TestContainer) FeedService() services.FeedServiceInterface {
	if tc.FeedServiceMock != nil {
		return tc.FeedServiceMock
	}
	return nil
}

func (tc *TestContainer) FeedItemService() services.FeedItemServiceInterface {
	if tc.FeedItemServiceMock != nil {
		return tc.FeedItemServiceMock
	}
	return nil
}

func (tc *TestContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	if tc.FeedItemReplyServiceMock != nil {
		return tc.FeedItemReplyServiceMock
	}
	return nil
}

func (tc *TestContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	if tc.ResourceUsageServiceMock != nil {
		return tc.ResourceUsageServiceMock
	}
	return nil
}

func (tc *TestContainer) SearchService() services.SearchServiceInterface {
	if tc.SearchServiceMock != nil {
		return tc.SearchServiceMock
	}
	return nil
}

// Ensure TestContainer implements container.Container interface
var _ container.Container = (*TestContainer)(nil)

func (tc *TestContainer) TypeService() services.TypeServiceInterface { return nil }

func (tc *TestContainer) AttachmentService() services.AttachmentServiceInterface {
	return tc.AttachmentServiceMock
}

func (tc *TestContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return tc.EmbeddingServiceMock
}
