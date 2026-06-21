package server

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/vibexp/vibexp/internal/repositories"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testCreationTeamID = "550e8400-e29b-41d4-a716-446655440000"
	testCreationUserID = "user-xyz"
	testCreationSlug   = "my-project"
)

// createTestProjectCreationMetricsServer builds a minimal chi router with only
// the resource-creation-metrics route wired up (reuses MockProjectStatsContainer
// from project_handlers_test.go, which exposes ProjectService()).
func createTestProjectCreationMetricsServer(c *MockProjectStatsContainer) *Server {
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

	r.Route("/api/v1/{team_id}/projects", func(r chi.Router) {
		r.Get("/{slug}/resource-creation-metrics", srv.handleGetProjectResourceCreationMetrics)
	})

	return srv
}

func creationMetricsRequest(slug, query string) *http.Request {
	url := "/api/v1/" + testCreationTeamID + "/projects/" + slug + "/resource-creation-metrics"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testCreationUserID))
}

// TestHandleGetProjectResourceCreationMetrics_Success exercises the happy path:
// the service returns sparse counts, the handler zero-fills them into a
// continuous daily series with per-day and grand totals, and the 200 response
// conforms to the OpenAPI spec.
func TestHandleGetProjectResourceCreationMetrics_Success(t *testing.T) {
	mockSvc := svcmocks.NewMockProjectServiceInterface(t)
	c := &MockProjectStatsContainer{projectService: mockSvc}

	// Counts for "today" so the date lands inside the zero-filled window
	// regardless of when the test runs. `since` is computed from time.Now()
	// inside the handler, so it is matched with mock.Anything.
	today := time.Now().UTC().Format("2006-01-02")
	rows := []models.ProjectResourceCreationCount{
		{Date: today, ResourceType: "prompts", Count: 3},
		{Date: today, ResourceType: "artifacts", Count: 1},
		{Date: today, ResourceType: "memories", Count: 2},
	}
	mockSvc.EXPECT().
		GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, testCreationSlug, mock.Anything).
		Return(rows, nil).Once()

	srv := createTestProjectCreationMetricsServer(c)
	req := creationMetricsRequest(testCreationSlug, "range=7d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var resp struct {
		Status  string                             `json:"status"`
		Message string                             `json:"message"`
		Data    projectResourceCreationMetricsData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "7d", resp.Data.Range)
	assert.Equal(t, 6, resp.Data.TotalCreated)
	require.Len(t, resp.Data.Counts, 8) // 7-day window is inclusive of both ends.

	last := resp.Data.Counts[len(resp.Data.Counts)-1]
	assert.Equal(t, today, last.Date)
	assert.Equal(t, 3, last.Prompts)
	assert.Equal(t, 1, last.Artifacts)
	assert.Equal(t, 0, last.Blueprints)
	assert.Equal(t, 2, last.Memories)
	assert.Equal(t, 6, last.Total)

	// The oldest day has no creations and is present with zeros (gap-free series).
	first := resp.Data.Counts[0]
	assert.Equal(t, 0, first.Total)

	mockSvc.AssertExpectations(t)
}

// TestHandleGetProjectResourceCreationMetrics_InvalidRange verifies an
// out-of-enum range is rejected with 400 before the service is called.
func TestHandleGetProjectResourceCreationMetrics_InvalidRange(t *testing.T) {
	mockSvc := svcmocks.NewMockProjectServiceInterface(t)
	c := &MockProjectStatsContainer{projectService: mockSvc}

	srv := createTestProjectCreationMetricsServer(c)
	req := creationMetricsRequest(testCreationSlug, "range=999d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	mockSvc.AssertNotCalled(t, "GetProjectResourceCreationMetrics")
}

// TestHandleGetProjectResourceCreationMetrics_NotFound verifies that an
// unknown/inaccessible project maps to 404.
func TestHandleGetProjectResourceCreationMetrics_NotFound(t *testing.T) {
	mockSvc := svcmocks.NewMockProjectServiceInterface(t)
	c := &MockProjectStatsContainer{projectService: mockSvc}

	notFound := fmt.Errorf(
		"%w: slug=missing-project team=%s",
		repositories.ErrProjectNotFoundForRepo, testCreationTeamID,
	)
	mockSvc.EXPECT().
		GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, "missing-project", mock.Anything).
		Return(nil, notFound).
		Once()

	srv := createTestProjectCreationMetricsServer(c)
	req := creationMetricsRequest("missing-project", "range=30d")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestHandleGetProjectResourceCreationMetrics_DefaultRange verifies the range
// defaults to 30d (a 31-day inclusive window) and empty counts zero-fill cleanly.
func TestHandleGetProjectResourceCreationMetrics_DefaultRange(t *testing.T) {
	mockSvc := svcmocks.NewMockProjectServiceInterface(t)
	c := &MockProjectStatsContainer{projectService: mockSvc}

	mockSvc.EXPECT().
		GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, testCreationSlug, mock.Anything).
		Return([]models.ProjectResourceCreationCount{}, nil).Once()

	srv := createTestProjectCreationMetricsServer(c)
	req := creationMetricsRequest(testCreationSlug, "")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data projectResourceCreationMetricsData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "30d", resp.Data.Range)
	require.Len(t, resp.Data.Counts, 31)
	assert.Equal(t, 0, resp.Data.TotalCreated)

	mockSvc.AssertExpectations(t)
}
