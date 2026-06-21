package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	ramocks "github.com/vibexp/vibexp/internal/services/resourceaccess/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testTeamAnalyticsTeamID = "550e8400-e29b-41d4-a716-446655440000"
	testTeamAnalyticsUserID = "user-team-analytics"
)

// mockTeamAnalyticsContainer wires only the services the team analytics handlers
// touch: TeamService (membership auth + stats + creation metrics) and
// ResourceAccessService (team-wide access metrics).
type mockTeamAnalyticsContainer struct {
	BaseMockContainer
	teamService           services.TeamServiceInterface
	resourceAccessService resourceaccess.ResourceAccessService
}

func (m *mockTeamAnalyticsContainer) TeamService() services.TeamServiceInterface {
	return m.teamService
}

func (m *mockTeamAnalyticsContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return m.resourceAccessService
}

func createTestTeamAnalyticsServer(c *mockTeamAnalyticsContainer) *Server {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: c,
		logger:    logger,
		config:    &config.Config{},
		router:    r,
	}

	r.Route("/api/v1/teams", func(r chi.Router) {
		r.Get("/{id}/stats", srv.handleGetTeamStats)
		r.Get("/{id}/resource-creation-metrics", srv.handleGetTeamResourceCreationMetrics)
		r.Get("/{id}/resource-access-metrics", srv.handleGetTeamResourceAccessMetrics)
		r.Get("/{id}/feed-creation-metrics", srv.handleGetTeamFeedCreationMetrics)
		r.Get("/{id}/top-accessed-resources", srv.handleGetTeamTopAccessedResources)
	})

	return srv
}

func teamAnalyticsRequest(teamID, suffix, query string) *http.Request {
	url := "/api/v1/teams/" + teamID + suffix
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testTeamAnalyticsUserID))
}

// teamDailyAccess builds one zero-filled service day with the four canonical
// sources, mirroring the resource access handler test helper.
func teamDailyAccess(date string, web, cli, mcp, api int) resourceaccess.DailyMetrics {
	return resourceaccess.DailyMetrics{
		Date: date,
		Sources: []resourceaccess.SourceMetricPoint{
			{Source: resourceaccess.SourceWeb, Count: web},
			{Source: resourceaccess.SourceCLI, Count: cli},
			{Source: resourceaccess.SourceMCP, Count: mcp},
			{Source: resourceaccess.SourceAPI, Count: api},
		},
	}
}

func TestHandleGetTeamStats_Success(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()
	teamSvc.EXPECT().
		GetTeamStats(mock.Anything, testTeamAnalyticsTeamID).
		Return(&models.TeamStatsResponse{
			TotalProjects:   4,
			TotalPrompts:    25,
			TotalArtifacts:  13,
			TotalBlueprints: 6,
			TotalMemories:   40,
			TotalFeedItems:  52,
		}, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{teamService: teamSvc})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/stats", "")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var resp models.TeamStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 4, resp.TotalProjects)
	assert.Equal(t, 52, resp.TotalFeedItems)
	teamSvc.AssertExpectations(t)
}

func TestHandleGetTeamStats_Forbidden(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(false, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{teamService: teamSvc})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/stats", "")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	teamSvc.AssertNotCalled(t, "GetTeamStats")
}

func TestHandleGetTeamStats_InvalidTeamID(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{teamService: teamSvc})
	req := teamAnalyticsRequest("not-a-uuid", "/stats", "")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	teamSvc.AssertNotCalled(t, "IsUserMemberOfTeam")
	teamSvc.AssertNotCalled(t, "GetTeamStats")
}

func TestHandleGetTeamResourceCreationMetrics_Success(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	today := time.Now().UTC().Format("2006-01-02")
	rows := []models.TeamResourceCreationCount{
		{Date: today, ResourceType: "prompts", Count: 3},
		{Date: today, ResourceType: "projects", Count: 1},
		{Date: today, ResourceType: "memories", Count: 2},
	}
	teamSvc.EXPECT().
		GetTeamResourceCreationMetrics(mock.Anything, testTeamAnalyticsTeamID, mock.Anything).
		Return(rows, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{teamService: teamSvc})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/resource-creation-metrics", "range=7d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var resp struct {
		Status string                          `json:"status"`
		Data   teamResourceCreationMetricsData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "7d", resp.Data.Range)
	assert.Equal(t, 6, resp.Data.TotalCreated)
	require.Len(t, resp.Data.Counts, 8) // 7-day window inclusive of both ends.

	last := resp.Data.Counts[len(resp.Data.Counts)-1]
	assert.Equal(t, today, last.Date)
	assert.Equal(t, 3, last.Prompts)
	assert.Equal(t, 1, last.Projects)
	assert.Equal(t, 2, last.Memories)
	assert.Equal(t, 6, last.Total)
	assert.Equal(t, 0, resp.Data.Counts[0].Total) // gap-free zero-fill.
	teamSvc.AssertExpectations(t)
}

func TestHandleGetTeamResourceCreationMetrics_InvalidRange(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{teamService: teamSvc})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/resource-creation-metrics", "range=999d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	teamSvc.AssertNotCalled(t, "GetTeamResourceCreationMetrics")
}

func TestHandleGetTeamResourceAccessMetrics_Success(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	raSvc := ramocks.NewMockResourceAccessService(t)
	raSvc.EXPECT().
		GetTeamMetrics(mock.Anything, testTeamAnalyticsTeamID, 30).
		Return(&resourceaccess.MetricsResult{
			TeamID:    testTeamAnalyticsTeamID,
			RangeDays: 30,
			Days: []resourceaccess.DailyMetrics{
				teamDailyAccess("2026-05-01", 3, 1, 0, 0),
				teamDailyAccess("2026-05-02", 0, 0, 0, 0),
				teamDailyAccess("2026-05-03", 2, 0, 1, 4),
			},
		}, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/resource-access-metrics", "range=30d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var resp struct {
		Status string                    `json:"status"`
		Data   resourceAccessMetricsData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "30d", resp.Data.Range)
	assert.Equal(t, 11, resp.Data.TotalAccesses)
	require.Len(t, resp.Data.Counts, 3)
	assert.Equal(t, 0, resp.Data.Counts[1].Total) // zero-filled middle day present.

	last := resp.Data.Counts[2]
	assert.Equal(t, 2, last.Web)
	assert.Equal(t, 1, last.MCP)
	assert.Equal(t, 4, last.API)
	assert.Equal(t, 7, last.Total)
	teamSvc.AssertExpectations(t)
	raSvc.AssertExpectations(t)
}

func TestHandleGetTeamResourceAccessMetrics_InvalidRange(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()
	raSvc := ramocks.NewMockResourceAccessService(t)

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/resource-access-metrics", "range=bogus")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	raSvc.AssertNotCalled(t, "GetTeamMetrics")
}

// TestHandleGetTeamResourceCreationMetrics_ExtendedRange covers the new 180d
// (6-month) range option on the shared range map: the window must zero-fill to
// 181 inclusive days.
func TestHandleGetTeamResourceCreationMetrics_ExtendedRange(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()
	teamSvc.EXPECT().
		GetTeamResourceCreationMetrics(mock.Anything, testTeamAnalyticsTeamID, mock.Anything).
		Return([]models.TeamResourceCreationCount{}, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{teamService: teamSvc})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/resource-creation-metrics", "range=180d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var resp struct {
		Data teamResourceCreationMetricsData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "180d", resp.Data.Range)
	require.Len(t, resp.Data.Counts, 181) // 180-day window inclusive of both ends.
	teamSvc.AssertExpectations(t)
}

// TestHandleGetTeamResourceAccessMetrics_ExtendedRange covers the new 60d
// (2-month) range option: the handler must pass 60 days to the access service.
func TestHandleGetTeamResourceAccessMetrics_ExtendedRange(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	raSvc := ramocks.NewMockResourceAccessService(t)
	raSvc.EXPECT().
		GetTeamMetrics(mock.Anything, testTeamAnalyticsTeamID, 60).
		Return(&resourceaccess.MetricsResult{TeamID: testTeamAnalyticsTeamID, RangeDays: 60}, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/resource-access-metrics", "range=60d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	raSvc.AssertExpectations(t)
}

func TestHandleGetTeamFeedCreationMetrics_Success(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	today := time.Now().UTC().Format("2006-01-02")
	rows := []models.TeamFeedCreationCount{
		{Date: today, EntityType: "feeds", Count: 1},
		{Date: today, EntityType: "feed_items", Count: 4},
	}
	teamSvc.EXPECT().
		GetTeamFeedCreationMetrics(mock.Anything, testTeamAnalyticsTeamID, mock.Anything).
		Return(rows, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{teamService: teamSvc})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/feed-creation-metrics", "range=7d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var resp struct {
		Status string                      `json:"status"`
		Data   teamFeedCreationMetricsData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "7d", resp.Data.Range)
	assert.Equal(t, 5, resp.Data.TotalCreated)
	require.Len(t, resp.Data.Counts, 8) // 7-day window inclusive of both ends.

	last := resp.Data.Counts[len(resp.Data.Counts)-1]
	assert.Equal(t, today, last.Date)
	assert.Equal(t, 1, last.Feeds)
	assert.Equal(t, 4, last.FeedItems)
	assert.Equal(t, 5, last.Total)
	assert.Equal(t, 0, resp.Data.Counts[0].Total) // gap-free zero-fill.
	teamSvc.AssertExpectations(t)
}

func TestHandleGetTeamFeedCreationMetrics_InvalidRange(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{teamService: teamSvc})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/feed-creation-metrics", "range=999d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	teamSvc.AssertNotCalled(t, "GetTeamFeedCreationMetrics")
}

func TestHandleGetTeamTopAccessedResources_Success(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	raSvc := ramocks.NewMockResourceAccessService(t)
	raSvc.EXPECT().
		GetTopAccessedResources(mock.Anything, testTeamAnalyticsTeamID, 30, "", 2).
		Return([]models.TopAccessedResource{
			{
				ResourceType: "prompt",
				ResourceID:   "b3f1c2d4-5678-49ab-9cde-0123456789ab",
				Name:         "Onboarding checklist",
				AccessCount:  128,
			},
			{ResourceType: "artifact", ResourceID: "c4f2d3e5-6789-4abc-8def-1234567890bc", Name: "Q2 report", AccessCount: 64},
		}, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/top-accessed-resources", "range=30d&limit=2")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var resp struct {
		Status string                       `json:"status"`
		Data   teamTopAccessedResourcesData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "30d", resp.Data.Range)
	require.Len(t, resp.Data.Items, 2)
	assert.Equal(t, "prompt", resp.Data.Items[0].ResourceType)
	assert.Equal(t, 128, resp.Data.Items[0].AccessCount)
	assert.Equal(t, "Onboarding checklist", resp.Data.Items[0].Name)
	teamSvc.AssertExpectations(t)
	raSvc.AssertExpectations(t)
}

// TestHandleGetTeamTopAccessedResources_DefaultLimit confirms the limit param
// defaults to 5 when omitted.
func TestHandleGetTeamTopAccessedResources_DefaultLimit(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	raSvc := ramocks.NewMockResourceAccessService(t)
	raSvc.EXPECT().
		GetTopAccessedResources(mock.Anything, testTeamAnalyticsTeamID, 30, "", 5).
		Return([]models.TopAccessedResource{}, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/top-accessed-resources", "")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	raSvc.AssertExpectations(t)
}

// TestHandleGetTeamTopAccessedResources_SourceFilter confirms a valid `source`
// query param is forwarded to the service so the ranking is restricted to that
// access channel.
func TestHandleGetTeamTopAccessedResources_SourceFilter(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()

	raSvc := ramocks.NewMockResourceAccessService(t)
	raSvc.EXPECT().
		GetTopAccessedResources(mock.Anything, testTeamAnalyticsTeamID, 30, "cli", 5).
		Return([]models.TopAccessedResource{
			{ResourceType: "prompt", ResourceID: "b3f1c2d4-5678-49ab-9cde-0123456789ab", Name: "CLI snippet", AccessCount: 9},
		}, nil).Once()

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/top-accessed-resources", "range=30d&limit=5&source=cli")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	raSvc.AssertExpectations(t)
}

// TestHandleGetTeamTopAccessedResources_InvalidSource confirms an unknown source
// is rejected with 400 before the service is consulted.
func TestHandleGetTeamTopAccessedResources_InvalidSource(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()
	raSvc := ramocks.NewMockResourceAccessService(t)

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/top-accessed-resources", "source=bogus")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	raSvc.AssertNotCalled(t, "GetTopAccessedResources")
}

func TestHandleGetTeamTopAccessedResources_InvalidLimit(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	// Range defaults to a valid 30d, so membership runs before the limit is rejected —
	// once per bad-limit iteration below.
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Times(4)
	raSvc := ramocks.NewMockResourceAccessService(t)

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})

	for _, bad := range []string{"limit=0", "limit=51", "limit=-1", "limit=abc"} {
		req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/top-accessed-resources", bad)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equalf(t, http.StatusBadRequest, w.Code, "expected 400 for %q", bad)
	}
	raSvc.AssertNotCalled(t, "GetTopAccessedResources")
}

func TestHandleGetTeamTopAccessedResources_InvalidRange(t *testing.T) {
	teamSvc := svcmocks.NewMockTeamServiceInterface(t)
	teamSvc.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testTeamAnalyticsUserID, testTeamAnalyticsTeamID).
		Return(true, nil).Once()
	raSvc := ramocks.NewMockResourceAccessService(t)

	srv := createTestTeamAnalyticsServer(&mockTeamAnalyticsContainer{
		teamService:           teamSvc,
		resourceAccessService: raSvc,
	})
	req := teamAnalyticsRequest(testTeamAnalyticsTeamID, "/top-accessed-resources", "range=bogus")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	raSvc.AssertNotCalled(t, "GetTopAccessedResources")
}
