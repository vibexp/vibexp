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
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The WIRING tests (#735).
//
// The helpers — attachPageFreshness, parseFreshnessFilter, freshnessFilter —
// are already covered in isolation. What those tests cannot see is whether a
// handler CALLS the attach, or whether a service FORWARDS the filter: delete
// either line and every isolated test still passes, shipping a silently
// badge-less list or a filter that quietly does nothing.
//
// So these assert the seams: the page a handler actually serializes carries
// freshness, and the filter a handler parses actually reaches the service.

const wiringStaleMemoryID = "memory-1"

func wiringFreshnessState() *models.ResourceFreshnessState {
	return &models.ResourceFreshnessState{
		Status:         models.FreshnessStatusStale,
		MatchedRuleIDs: models.JSONArray[string]{"rule-1"},
		Reason:         models.FreshnessReasonRuleRun,
	}
}

// The memories list must carry freshness on the serialized page. Deleting the
// attachPageFreshness call from handleListMemories turns this red.
func TestHandleListMemories_PageCarriesFreshness(t *testing.T) {
	container := newMockMemoryContainer(t)

	container.memoryService.On("ListMemories", "test-user-123", mock.Anything).
		Return(&models.MemoryListResponse{
			Memories: []models.Memory{
				{ID: wiringStaleMemoryID, TeamID: "team-123", ProjectID: testHandlerProjectID,
					Text: "stale", Status: models.MemoryStatusActive, Metadata: map[string]interface{}{}},
				{ID: "memory-2", TeamID: "team-123", ProjectID: testHandlerProjectID,
					Text: "fresh", Status: models.MemoryStatusActive, Metadata: map[string]interface{}{}},
			},
			TotalCount: 2, Page: 1, PerPage: 10, TotalPages: 1,
		}, nil)

	freshSvc := svcmocks.NewMockFreshnessServiceInterface(t)
	freshSvc.EXPECT().
		ListResourceFreshness(mock.Anything, "team-123", models.RelationResourceTypeMemory,
			[]string{wiringStaleMemoryID, "memory-2"}).
		Return(map[string]*models.ResourceFreshnessState{wiringStaleMemoryID: wiringFreshnessState()}, nil).
		Once()
	container.freshnessService = freshSvc

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest("GET", "/api/v1/team-123/memories", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleListMemories(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response models.MemoryListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	require.Len(t, response.Memories, 2)
	require.NotNil(t, response.Memories[0].Freshness, "the stale memory must carry its badge")
	assert.Equal(t, models.FreshnessStatusStale, response.Memories[0].Freshness.Status)
	assert.Nil(t, response.Memories[1].Freshness, "the fresh one must not")
}

// `?freshness=stale` must reach the SERVICE filter. Deleting the
// parseFreshnessFilter call — or the Freshness field from the filters literal —
// turns this red.
func TestHandleListMemories_StaleFilterReachesTheService(t *testing.T) {
	container := newMockMemoryContainer(t)

	container.memoryService.On("ListMemories", "test-user-123",
		mock.MatchedBy(func(filters services.MemoryFilters) bool {
			return filters.Freshness == services.FreshnessFilterStale
		}),
	).Return(&models.MemoryListResponse{
		Memories: []models.Memory{}, TotalCount: 0, Page: 1, PerPage: 10, TotalPages: 0,
	}, nil)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest(
		"GET", "/api/v1/team-123/memories?freshness=stale", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleListMemories(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	container.memoryService.AssertExpectations(t)
}

// An unknown value is rejected before the service is reached — the mock has no
// ListMemories expectation, so a call through would fail this test.
func TestHandleListMemories_RejectsUnknownFreshnessValue(t *testing.T) {
	container := newMockMemoryContainer(t)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest(
		"GET", "/api/v1/team-123/memories?freshness=stail", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleListMemories(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// The detail GET must attach freshness too, and the memories detail is one of
// the operations whose ledger entry this change removes.
func TestHandleGetMemory_CarriesFreshnessAndConformsToSpec(t *testing.T) {
	container := newMockMemoryContainer(t)

	container.memoryService.On("GetMemory", "test-user-123", "team-123", wiringStaleMemoryID).
		Return(&models.Memory{
			ID: wiringStaleMemoryID, TeamID: "team-123", ProjectID: testHandlerProjectID,
			Text: "stale", Status: models.MemoryStatusActive, Metadata: map[string]interface{}{},
		}, nil)

	freshSvc := svcmocks.NewMockFreshnessServiceInterface(t)
	freshSvc.EXPECT().
		GetResourceFreshness(mock.Anything, "team-123", models.RelationResourceTypeMemory, wiringStaleMemoryID).
		Return(wiringFreshnessState(), nil).Once()
	container.freshnessService = freshSvc

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest(
		"GET", "/api/v1/team-123/memories/"+wiringStaleMemoryID, nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123", "id": wiringStaleMemoryID})
	w := httptest.NewRecorder()

	srv.handleGetMemory(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var memory models.Memory
	require.NoError(t, json.NewDecoder(w.Body).Decode(&memory))
	require.NotNil(t, memory.Freshness)
	assert.Equal(t, models.FreshnessStatusStale, memory.Freshness.Status)
}

// The artifacts list is the second wiring seam, and — like the memories detail
// — one whose payload-coverage ledger entry this change removes.
func TestHandleListArtifacts_PageCarriesFreshnessAndConformsToSpec(t *testing.T) {
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	artSvc := svcmocks.NewMockArtifactServiceInterface(t)
	artSvc.On("ListArtifacts", "user-123", mock.MatchedBy(func(filters services.ArtifactFilters) bool {
		return filters.Freshness == services.FreshnessFilterStale
	})).Return(&models.ArtifactListResponse{
		Artifacts: []models.Artifact{
			{ID: "art-1", ProjectID: "test-project", Slug: "s1", Title: "One", Content: "c",
				UserID: "user-123", Type: "general", Status: "active",
				Metadata: map[string]interface{}{}},
		},
		TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
	}, nil)

	freshSvc := svcmocks.NewMockFreshnessServiceInterface(t)
	freshSvc.EXPECT().
		ListResourceFreshness(mock.Anything, teamID, models.RelationResourceTypeArtifact, []string{"art-1"}).
		Return(map[string]*models.ResourceFreshnessState{"art-1": wiringFreshnessState()}, nil).Once()

	srv := newArtifactRelatedServer(artSvc, svcmocks.NewMockRelationServiceInterface(t))
	srv.container.(*MockArtifactContainer).FreshnessServiceMock = freshSvc

	req := createAuthenticatedRequest("GET", "/api/v1/"+teamID+"/artifacts?freshness=stale", "", "user-123")
	req = addURLParams(req, map[string]string{"team_id": teamID})
	rr := httptest.NewRecorder()

	srv.handleListArtifacts(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var response models.ArtifactListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	require.Len(t, response.Artifacts, 1)
	require.NotNil(t, response.Artifacts[0].Freshness,
		"the list attach must run — deleting it from handleListArtifacts turns this red")
	assert.Equal(t, models.FreshnessStatusStale, response.Artifacts[0].Freshness.Status)
}
