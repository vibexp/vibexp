package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const searchTestTeamID = "550e8400-e29b-41d4-a716-446655440000"

// MockSearchContainer implements the Container interface for search handler tests.
type MockSearchContainer struct {
	BaseMockContainer
	searchService *svcmocks.MockSearcher
	authService   *svcmocks.MockAuthServiceInterface
	teamService   *svcmocks.MockTeamServiceInterface
	availability  *svcmocks.MockAISummaryAvailabilityResolver
}

func (m *MockSearchContainer) AISummaryAvailability() services.AISummaryAvailabilityResolver {
	return m.availability
}

func (m *MockSearchContainer) SearchService() services.Searcher {
	return m.searchService
}

func (m *MockSearchContainer) AuthService() services.AuthServiceInterface {
	return m.authService
}

func (m *MockSearchContainer) TeamService() services.TeamServiceInterface {
	return m.teamService
}

func newMockSearchContainer(t *testing.T) *MockSearchContainer {
	return &MockSearchContainer{
		searchService: svcmocks.NewMockSearcher(t),
		authService:   svcmocks.NewMockAuthServiceInterface(t),
		teamService:   svcmocks.NewMockTeamServiceInterface(t),
		availability:  svcmocks.NewMockAISummaryAvailabilityResolver(t),
	}
}

// stubAISummaryAvailability answers the ai_summary lookup every successful
// REST search makes (#1074).
func stubAISummaryAvailability(c *MockSearchContainer, available, enabled bool) {
	c.availability.EXPECT().Availability(mock.Anything, searchTestTeamID).
		Return(models.AISummaryAvailability{Available: available, Enabled: enabled})
}

func createSearchTestServer(c *MockSearchContainer) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: c,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	r.Route("/api/v1/{team_id}/search", func(r chi.Router) {
		r.Use(srv.teamValidationMiddleware())
		r.Post("/", srv.handleSearch)
	})

	return srv
}

func grantSearchTeamAccess(c *MockSearchContainer) {
	c.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", searchTestTeamID).
		Return(true, nil).Maybe()
}

func searchRequest(t *testing.T, body interface{}) *http.Request {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+searchTestTeamID+"/search", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
}

func TestHandleSearch_Success(t *testing.T) {
	c := newMockSearchContainer(t)
	grantSearchTeamAccess(c)

	c.searchService.On("Search", mock.Anything, searchTestTeamID,
		mock.MatchedBy(func(req *models.SearchRequest) bool {
			return req.Query == "retries" && req.Page == 1 && req.PerPage == 10
		})).
		Return(&models.SearchResultsResponse{
			Results: []models.SearchResultItem{
				{
					Type: "artifact", ID: "a-1", Title: "T", Slug: "my-artifact",
					ProjectID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", ProjectName: "My Project",
					Excerpt: "e", Score: 0.9, ChunkID: "c-1", UpdatedAt: time.Now(),
				},
			},
			TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
		}, nil)
	stubAISummaryAvailability(c, true, true)

	srv := createSearchTestServer(c)
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, searchRequest(t, map[string]interface{}{"query": "retries"}))

	require.Equal(t, http.StatusOK, rr.Code)
	var resp models.SearchResultsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.TotalCount)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "artifact", resp.Results[0].Type)
	// The deep-link fields must round-trip through the JSON response under their
	// snake_case keys: slug + project_id build the route, project_name is displayed.
	assert.Equal(t, "my-artifact", resp.Results[0].Slug)
	assert.Equal(t, "7c9e6679-7425-40de-944b-e07fc1f90ae7", resp.Results[0].ProjectID)
	assert.Equal(t, "My Project", resp.Results[0].ProjectName)
	assert.Contains(t, rr.Body.String(), `"slug":"my-artifact"`)
	assert.Contains(t, rr.Body.String(), `"project_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7"`)
	assert.Contains(t, rr.Body.String(), `"project_name":"My Project"`)
}

// Omitted page/per_page (JSON zero values) take the defaults.
func TestHandleSearch_PaginationDefaults(t *testing.T) {
	c := newMockSearchContainer(t)
	grantSearchTeamAccess(c)

	c.searchService.On("Search", mock.Anything, searchTestTeamID,
		mock.MatchedBy(func(req *models.SearchRequest) bool {
			return req.Page == 1 && req.PerPage == 10
		})).
		Return(&models.SearchResultsResponse{Results: []models.SearchResultItem{}, Page: 1, PerPage: 10}, nil)
	stubAISummaryAvailability(c, false, false)

	srv := createSearchTestServer(c)
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, searchRequest(t, map[string]interface{}{"query": "q"}))

	assert.Equal(t, http.StatusOK, rr.Code)
}

// A provided out-of-range page/per_page is a 400 naming the range, never
// silently replaced by the default (#1107).
func TestHandleSearch_OutOfRangePaginationIsRejected(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    map[string]interface{}
		message string
	}{
		{name: "per_page above max", body: map[string]interface{}{"query": "q", "per_page": 9999},
			message: "per_page must be between 1 and 100"},
		{name: "negative per_page", body: map[string]interface{}{"query": "q", "per_page": -1},
			message: "per_page must be between 1 and 100"},
		{name: "page above max", body: map[string]interface{}{"query": "q", "page": 10001},
			message: "page must be between 1 and 10000"},
		{name: "negative page", body: map[string]interface{}{"query": "q", "page": -1},
			message: "page must be between 1 and 10000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newMockSearchContainer(t)
			grantSearchTeamAccess(c)

			srv := createSearchTestServer(c)
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, searchRequest(t, tc.body))

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			var body map[string]any
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
			assert.Equal(t, "VALIDATION_FAILED", body["code"], "same code as the body's other validation failures")
			assert.Equal(t, tc.message, body["detail"])
			c.searchService.AssertNotCalled(t, "Search")
		})
	}
}

func TestHandleSearch_BadRequests(t *testing.T) {
	tests := []struct {
		name string
		body interface{}
		raw  string
	}{
		{name: "empty query", body: map[string]interface{}{"query": ""}},
		{name: "missing query", body: map[string]interface{}{"types": []string{"prompts"}}},
		{name: "unknown type", body: map[string]interface{}{"query": "q", "types": []string{"widgets"}}},
		{name: "non-uuid project_id", body: map[string]interface{}{"query": "q", "project_id": "not-a-uuid"}},
		{name: "malformed json", raw: `{"query": }`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMockSearchContainer(t)
			grantSearchTeamAccess(c)
			srv := createSearchTestServer(c)

			var req *http.Request
			if tt.raw != "" {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/"+searchTestTeamID+"/search",
					bytes.NewReader([]byte(tt.raw)))
				req.Header.Set("Content-Type", "application/json")
				req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
			} else {
				req = searchRequest(t, tt.body)
			}

			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			// Service must never be invoked for invalid requests.
			c.searchService.AssertNotCalled(t, "Search", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestHandleSearch_ServiceError(t *testing.T) {
	c := newMockSearchContainer(t)
	grantSearchTeamAccess(c)
	c.searchService.On("Search", mock.Anything, searchTestTeamID, mock.Anything).
		Return(nil, errors.New("boom"))

	srv := createSearchTestServer(c)
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, searchRequest(t, map[string]interface{}{"query": "q"}))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	c.availability.AssertNotCalled(t, "Availability", mock.Anything, mock.Anything)
}

// TestHandleSearch_AISummaryAvailability pins the ai_summary field on the REST
// search response (#1074): it reflects the resolver verbatim, including the
// fail-open {available:false} it reports when a lookup failed — which must
// still be a 200 search — and every variant conforms to the spec.
func TestHandleSearch_AISummaryAvailability(t *testing.T) {
	for name, tc := range map[string]struct {
		available, enabled bool
		want               string
	}{
		"provider and enabled":            {true, true, `{"available":true,"enabled":true}`},
		"provider but disabled":           {true, false, `{"available":true,"enabled":false}`},
		"no provider":                     {false, true, `{"available":false,"enabled":true}`},
		"resolver failure fails to false": {false, false, `{"available":false,"enabled":false}`},
	} {
		t.Run(name, func(t *testing.T) {
			c := newMockSearchContainer(t)
			grantSearchTeamAccess(c)
			c.searchService.On("Search", mock.Anything, searchTestTeamID, mock.Anything).
				Return(&models.SearchResultsResponse{
					Results: []models.SearchResultItem{{
						Type: "memory", ID: "3f2b8c1e-9a4d-4e6f-8b7a-1c2d3e4f5a6b", Title: "Retry notes",
						ProjectID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", ProjectName: "My Project",
						Excerpt: "e", Score: 0.9, ChunkID: "4a5b6c7d-8e9f-4a0b-9c1d-2e3f4a5b6c7d",
						UpdatedAt: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC),
					}},
					TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
				}, nil)
			stubAISummaryAvailability(c, tc.available, tc.enabled)

			req := searchRequest(t, map[string]interface{}{"query": "retries"})
			rr := httptest.NewRecorder()
			createSearchTestServer(c).router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)

			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
			assert.JSONEq(t, tc.want, string(raw["ai_summary"]))
			// The shared search fields are still flattened at the top level.
			assert.JSONEq(t, `1`, string(raw["total_count"]))
			assert.Contains(t, string(raw["results"]), `"title":"Retry notes"`)
		})
	}
}

// An empty result set still serializes results as [] and carries ai_summary.
func TestHandleSearch_EmptyResultsConformToSpec(t *testing.T) {
	c := newMockSearchContainer(t)
	grantSearchTeamAccess(c)
	c.searchService.On("Search", mock.Anything, searchTestTeamID, mock.Anything).
		Return(&models.SearchResultsResponse{Page: 1, PerPage: 10}, nil)
	stubAISummaryAvailability(c, true, true)

	req := searchRequest(t, map[string]interface{}{"query": "nothing"})
	rr := httptest.NewRecorder()
	createSearchTestServer(c).router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
	assert.Contains(t, rr.Body.String(), `"results":[]`)
}

func TestSearchHandler_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := bytes.NewReader([]byte(`{"query":"q"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+searchTestTeamID+"/search", body)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
