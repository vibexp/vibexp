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
)

const searchTestTeamID = "550e8400-e29b-41d4-a716-446655440000"

// MockSearchContainer implements the Container interface for search handler tests.
type MockSearchContainer struct {
	BaseMockContainer
	searchService *svcmocks.MockSearcher
	authService   *svcmocks.MockAuthServiceInterface
	teamService   *svcmocks.MockTeamServiceInterface
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
	}
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

func TestHandleSearch_PaginationClamping(t *testing.T) {
	c := newMockSearchContainer(t)
	grantSearchTeamAccess(c)

	// per_page above max should clamp to default 10 (validatePaginationParams ignores out-of-range).
	c.searchService.On("Search", mock.Anything, searchTestTeamID,
		mock.MatchedBy(func(req *models.SearchRequest) bool {
			return req.Page == 1 && req.PerPage == 10
		})).
		Return(&models.SearchResultsResponse{Results: []models.SearchResultItem{}, Page: 1, PerPage: 10}, nil)

	srv := createSearchTestServer(c)
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, searchRequest(t, map[string]interface{}{"query": "q", "per_page": 9999}))

	assert.Equal(t, http.StatusOK, rr.Code)
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
