package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	ramocks "github.com/vibexp/vibexp/internal/services/resourceaccess/mocks"
)

const (
	testRAMetricsUserID = "user-123"
	testRAMetricsTeamID = "550e8400-e29b-41d4-a716-446655440000"
	testRAResourceID    = "660e8400-e29b-41d4-a716-446655440111"
)

// MockResourceAccessContainer implements the Container interface for resource
// access metrics handler tests.
type MockResourceAccessContainer struct {
	BaseMockContainer
	resourceAccessService *ramocks.MockResourceAccessService
	teamService           *svcmocks.MockTeamServiceInterface
}

func (m *MockResourceAccessContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return m.resourceAccessService
}

func (m *MockResourceAccessContainer) TeamService() services.TeamServiceInterface {
	return m.teamService
}

func newMockResourceAccessContainer(t *testing.T) *MockResourceAccessContainer {
	return &MockResourceAccessContainer{
		resourceAccessService: ramocks.NewMockResourceAccessService(t),
		teamService:           svcmocks.NewMockTeamServiceInterface(t),
	}
}

func createTestResourceAccessServer(container *MockResourceAccessContainer) *Server {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    &config.Config{},
		router:    r,
	}

	srv.setupResourceAccessMetricsRoutes(r)

	return srv
}

func resourceAccessMetricsURL(query string) string {
	return "/api/v1/" + testRAMetricsTeamID + "/resource-access-metrics?" + query
}

func makeResourceAccessRequest(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testRAMetricsUserID))
	return req
}

// dailyMetrics is a small helper to build a zero-filled service day.
func dailyMetrics(date string, web, cli, mcp, api int) resourceaccess.DailyMetrics {
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

func TestHandleGetResourceAccessMetrics_Success(t *testing.T) {
	container := newMockResourceAccessContainer(t)
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, testRAMetricsUserID, testRAMetricsTeamID).
		Return(true, nil)

	result := &resourceaccess.MetricsResult{
		TeamID:       testRAMetricsTeamID,
		ResourceType: "prompt",
		ResourceID:   testRAResourceID,
		RangeDays:    30,
		Days: []resourceaccess.DailyMetrics{
			dailyMetrics("2026-05-01", 3, 1, 0, 0),
			dailyMetrics("2026-05-02", 0, 0, 0, 0),
			dailyMetrics("2026-05-03", 2, 0, 1, 4),
		},
	}
	container.resourceAccessService.EXPECT().
		GetMetrics(mock.Anything, testRAMetricsTeamID, "prompt", testRAResourceID, 30).
		Return(result, nil)

	srv := createTestResourceAccessServer(container)
	url := resourceAccessMetricsURL("resource_type=prompt&resource_id=" + testRAResourceID + "&range=30d")
	req := makeResourceAccessRequest(url)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Status  string                    `json:"status"`
		Message string                    `json:"message"`
		Data    resourceAccessMetricsData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "30d", resp.Data.Range)
	// 3+1 + 0 + 2+1+4 = 11
	assert.Equal(t, 11, resp.Data.TotalAccesses)
	require.Len(t, resp.Data.Counts, 3)

	// Zero-filled middle day is present.
	assert.Equal(t, "2026-05-02", resp.Data.Counts[1].Date)
	assert.Equal(t, 0, resp.Data.Counts[1].Total)

	// Source grouping and per-day total on the last day.
	last := resp.Data.Counts[2]
	assert.Equal(t, "2026-05-03", last.Date)
	assert.Equal(t, 2, last.Web)
	assert.Equal(t, 0, last.CLI)
	assert.Equal(t, 1, last.MCP)
	assert.Equal(t, 4, last.API)
	assert.Equal(t, 7, last.Total)
}

func TestHandleGetResourceAccessMetrics_DefaultRange(t *testing.T) {
	container := newMockResourceAccessContainer(t)
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, testRAMetricsUserID, testRAMetricsTeamID).
		Return(true, nil)

	// Range omitted -> defaults to 30 days; echoed back as "30d".
	container.resourceAccessService.EXPECT().
		GetMetrics(mock.Anything, testRAMetricsTeamID, "agent", testRAResourceID, 30).
		Return(&resourceaccess.MetricsResult{Days: []resourceaccess.DailyMetrics{}}, nil)

	srv := createTestResourceAccessServer(container)
	url := resourceAccessMetricsURL("resource_type=agent&resource_id=" + testRAResourceID)
	req := makeResourceAccessRequest(url)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data resourceAccessMetricsData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "30d", resp.Data.Range)
	assert.Equal(t, 0, resp.Data.TotalAccesses)
	assert.Empty(t, resp.Data.Counts)
}

func TestHandleGetResourceAccessMetrics_ValidationErrors(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "invalid resource_type",
			query: "resource_type=invalid&resource_id=" + testRAResourceID + "&range=30d",
		},
		{
			name:  "non-uuid resource_id",
			query: "resource_type=prompt&resource_id=not-a-uuid&range=30d",
		},
		{
			name:  "missing resource_id",
			query: "resource_type=prompt&range=30d",
		},
		{
			name:  "invalid range",
			query: "resource_type=prompt&resource_id=" + testRAResourceID + "&range=999d",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := newMockResourceAccessContainer(t)
			// Membership passes so we reach handler-level validation.
			container.teamService.On("IsUserMemberOfTeam", mock.Anything, testRAMetricsUserID, testRAMetricsTeamID).
				Return(true, nil)

			srv := createTestResourceAccessServer(container)
			url := resourceAccessMetricsURL(tc.query)
			req := makeResourceAccessRequest(url)
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			// GetMetrics must never be called when validation fails.
			container.resourceAccessService.AssertNotCalled(t, "GetMetrics")
		})
	}
}

func TestHandleGetResourceAccessMetrics_NonMemberForbidden(t *testing.T) {
	container := newMockResourceAccessContainer(t)
	// Caller is not a member of the team.
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, testRAMetricsUserID, testRAMetricsTeamID).
		Return(false, nil)

	srv := createTestResourceAccessServer(container)
	url := resourceAccessMetricsURL("resource_type=prompt&resource_id=" + testRAResourceID + "&range=30d")
	req := makeResourceAccessRequest(url)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	container.resourceAccessService.AssertNotCalled(t, "GetMetrics")
}

func TestHandleGetResourceAccessMetrics_ServiceError(t *testing.T) {
	container := newMockResourceAccessContainer(t)
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, testRAMetricsUserID, testRAMetricsTeamID).
		Return(true, nil)

	container.resourceAccessService.EXPECT().
		GetMetrics(mock.Anything, testRAMetricsTeamID, "prompt", testRAResourceID, 7).
		Return(nil, assert.AnError)

	srv := createTestResourceAccessServer(container)
	url := resourceAccessMetricsURL("resource_type=prompt&resource_id=" + testRAResourceID + "&range=7d")
	req := makeResourceAccessRequest(url)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
