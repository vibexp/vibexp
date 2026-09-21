package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// Handler tests for POST /api/v1/{team_id}/search/summary (#1073).

const (
	summaryTestProviderID = "22222222-2222-4222-8222-222222222222"
	summaryTestDocID      = "33333333-3333-4333-8333-333333333333"
	summaryTestProjectID  = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
)

// MockSearchSummaryContainer implements the Container interface for search
// summary handler tests.
type MockSearchSummaryContainer struct {
	BaseMockContainer
	summaryService *svcmocks.MockSearchSummaryServiceInterface
	searchService  *svcmocks.MockSearcher
	teamService    *svcmocks.MockTeamServiceInterface
	availability   *svcmocks.MockAISummaryAvailabilityResolver
}

func (m *MockSearchSummaryContainer) AISummaryAvailability() services.AISummaryAvailabilityResolver {
	return m.availability
}

func (m *MockSearchSummaryContainer) SearchSummaryService() services.SearchSummaryServiceInterface {
	return m.summaryService
}

func (m *MockSearchSummaryContainer) SearchService() services.Searcher {
	return m.searchService
}

func (m *MockSearchSummaryContainer) TeamService() services.TeamServiceInterface {
	return m.teamService
}

func newMockSearchSummaryContainer(t *testing.T) *MockSearchSummaryContainer {
	return &MockSearchSummaryContainer{
		summaryService: svcmocks.NewMockSearchSummaryServiceInterface(t),
		searchService:  svcmocks.NewMockSearcher(t),
		availability:   svcmocks.NewMockAISummaryAvailabilityResolver(t),
		teamService:    svcmocks.NewMockTeamServiceInterface(t),
	}
}

// createSearchSummaryTestServer mounts the production route. The tenancy
// middleware is omitted on purpose, as for the other strict-server domains:
// these tests drive the handler, and team access has its own tests.
func createSearchSummaryTestServer(c *MockSearchSummaryContainer) *Server {
	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: c,
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	srv.mountSearchSummaryHandlers(r)
	return srv
}

func searchSummaryPath(teamID string) string {
	return "/api/v1/" + teamID + "/search/summary"
}

func searchSummaryRequest(t *testing.T, body any) *http.Request {
	t.Helper()
	var raw []byte
	switch b := body.(type) {
	case string:
		raw = []byte(b)
	default:
		var err error
		raw, err = json.Marshal(b)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, searchSummaryPath(searchTestTeamID), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
}

func sampleSearchSummary() *models.SearchSummary {
	return &models.SearchSummary{
		Summary: "Retries back off exponentially [1].",
		Sources: []models.SearchSummarySource{{
			Index:       1,
			Type:        "memory",
			ID:          summaryTestDocID,
			Title:       "Retry notes",
			Slug:        "",
			ProjectID:   summaryTestProjectID,
			ProjectName: "My Project",
			UpdatedAt:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
			Truncated:   true,
		}},
		Model:       "gpt-4o-mini",
		ProviderID:  summaryTestProviderID,
		GeneratedAt: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC),
		Usage:       &models.TokenUsage{PromptTokens: 90, CompletionTokens: 12},
	}
}

func TestSummarizeSearchResults_SpecConformance(t *testing.T) {
	withUsage := sampleSearchSummary()
	withoutUsage := sampleSearchSummary()
	withoutUsage.Usage = nil

	for name, summary := range map[string]*models.SearchSummary{
		"with usage":    withUsage,
		"without usage": withoutUsage,
	} {
		t.Run(name, func(t *testing.T) {
			c := newMockSearchSummaryContainer(t)
			c.summaryService.EXPECT().
				Summarize(mock.Anything, searchTestTeamID, &models.SearchSummaryRequest{
					Query:     "retries",
					Types:     []string{"memories", "blueprints"},
					ProjectID: summaryTestProjectID,
				}).
				Return(summary, nil)

			req := searchSummaryRequest(t, map[string]any{
				"query":      "  retries  ",
				"types":      []string{"memories", "blueprints"},
				"project_id": summaryTestProjectID,
			})
			w := httptest.NewRecorder()
			createSearchSummaryTestServer(c).router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)

			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
			assert.JSONEq(t, `"Retries back off exponentially [1]."`, string(raw["summary"]))
			assert.JSONEq(t, `"gpt-4o-mini"`, string(raw["model"]))
			assert.JSONEq(t, `"`+summaryTestProviderID+`"`, string(raw["provider_id"]))
			assert.JSONEq(t, `"2026-09-22T10:00:00Z"`, string(raw["generated_at"]))
			assert.JSONEq(t, `[{
				"index": 1, "type": "memory", "id": "`+summaryTestDocID+`",
				"title": "Retry notes", "slug": "", "project_id": "`+summaryTestProjectID+`",
				"project_name": "My Project", "updated_at": "2026-09-01T12:00:00Z", "truncated": true
			}]`, string(raw["sources"]))
			if summary.Usage == nil {
				_, present := raw["usage"]
				assert.False(t, present, "unreported usage is omitted, not zeroed")
			} else {
				assert.JSONEq(t, `{"prompt_tokens":90,"completion_tokens":12}`, string(raw["usage"]))
			}
		})
	}
}

func TestSummarizeSearchResults_EmptySourcesSerializeAsArray(t *testing.T) {
	summary := sampleSearchSummary()
	summary.Sources = nil
	c := newMockSearchSummaryContainer(t)
	c.summaryService.EXPECT().Summarize(mock.Anything, searchTestTeamID, mock.Anything).Return(summary, nil)

	w := httptest.NewRecorder()
	createSearchSummaryTestServer(c).router.ServeHTTP(w, searchSummaryRequest(t, map[string]any{"query": "q"}))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// AssertConformsToSpec accepts null for a required array, so only a literal
	// check proves it marshals as [] (#125).
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["sources"]))
}

func TestSummarizeSearchResults_ClassifiedErrors(t *testing.T) {
	// Each error carries internal detail that must never reach the client.
	const leak = "http://10.0.0.7:11434 said: invalid api key sk-live-abc"
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{services.ErrAISummaryDisabled, http.StatusConflict, "AI_SUMMARY_DISABLED"},
		{fmt.Errorf("%w: %s", services.ErrNoModelProvider, leak), http.StatusConflict, "AI_SUMMARY_NO_PROVIDER"},
		{services.ErrAISummaryNoResults, http.StatusUnprocessableEntity, "AI_SUMMARY_NO_RESULTS"},
		{fmt.Errorf("%w: %s", services.ErrProviderUnreachable, leak), http.StatusBadGateway, "AI_SUMMARY_PROVIDER_UNREACHABLE"},
		{fmt.Errorf("%w: %s", services.ErrProviderUnauthorized, leak), http.StatusBadGateway, "AI_SUMMARY_UNAUTHORIZED"},
		{fmt.Errorf("%w: %s", services.ErrModelRejected, leak), http.StatusBadGateway, "AI_SUMMARY_MODEL_ERROR"},
		{fmt.Errorf("%w: %s", services.ErrContextTooLarge, leak), http.StatusBadGateway, "AI_SUMMARY_MODEL_ERROR"},
		{fmt.Errorf("%w: %s", services.ErrCompletionTimeout, leak), http.StatusGatewayTimeout, "AI_SUMMARY_TIMEOUT"},
		{errors.New(leak), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.code+"/"+tt.err.Error(), func(t *testing.T) {
			c := newMockSearchSummaryContainer(t)
			c.summaryService.EXPECT().Summarize(mock.Anything, searchTestTeamID, mock.Anything).Return(nil, tt.err)

			req := searchSummaryRequest(t, map[string]any{"query": "q"})
			w := httptest.NewRecorder()
			createSearchSummaryTestServer(c).router.ServeHTTP(w, req)

			require.Equal(t, tt.status, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tt.code, body["code"])
			assert.NotContains(t, w.Body.String(), "10.0.0.7")
			assert.NotContains(t, w.Body.String(), "sk-live")
		})
	}
}

func TestSummarizeSearchResults_ValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		body   any
		detail string
	}{
		{"missing query", map[string]any{}, msgSearchSummaryQueryRequired},
		{"blank query", map[string]any{"query": "   "}, msgSearchSummaryQueryRequired},
		{"query too long", map[string]any{"query": strings.Repeat("é", 1001)}, msgSearchSummaryQueryTooLong},
		{"unknown type", map[string]any{"query": "q", "types": []string{"memories", "notes"}}, msgSearchSummaryInvalidType},
		{"malformed json", `{"query":`, msgInvalidBodyWellFormedJSON},
		{"malformed project_id", map[string]any{"query": "q", "project_id": "not-a-uuid"}, msgInvalidBodyWellFormedJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The mock has no expectation: reaching the service fails the test.
			c := newMockSearchSummaryContainer(t)

			req := searchSummaryRequest(t, tt.body)
			w := httptest.NewRecorder()
			createSearchSummaryTestServer(c).router.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tt.detail, body["detail"])
		})
	}
}

func TestSummarizeSearchResults_QueryAtTheLimitIsAccepted(t *testing.T) {
	query := strings.Repeat("é", 1000) // 1000 runes, 2000 bytes
	c := newMockSearchSummaryContainer(t)
	c.summaryService.EXPECT().
		Summarize(mock.Anything, searchTestTeamID, &models.SearchSummaryRequest{Query: query}).
		Return(sampleSearchSummary(), nil)

	w := httptest.NewRecorder()
	createSearchSummaryTestServer(c).router.ServeHTTP(w, searchSummaryRequest(t, map[string]any{"query": query}))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestSummarizeSearchResults_InvalidTeamID(t *testing.T) {
	c := newMockSearchSummaryContainer(t)
	req := httptest.NewRequest(http.MethodPost, searchSummaryPath("not-a-uuid"), strings.NewReader(`{"query":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	createSearchSummaryTestServer(c).router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "team_id must be a valid UUID")
}

func TestSummarizeSearchResults_MalformedIdentifierIsInternal(t *testing.T) {
	summary := sampleSearchSummary()
	summary.ProviderID = "not-a-uuid"
	c := newMockSearchSummaryContainer(t)
	c.summaryService.EXPECT().Summarize(mock.Anything, searchTestTeamID, mock.Anything).Return(summary, nil)

	w := httptest.NewRecorder()
	createSearchSummaryTestServer(c).router.ServeHTTP(w, searchSummaryRequest(t, map[string]any{"query": "q"}))

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestToGenSearchSummaryResponse_RejectsMalformedSourceIDs(t *testing.T) {
	badID := sampleSearchSummary()
	badID.Sources[0].ID = "nope"
	_, err := toGenSearchSummaryResponse(badID)
	require.Error(t, err)

	badProject := sampleSearchSummary()
	badProject.Sources[0].ProjectID = "nope"
	_, err = toGenSearchSummaryResponse(badProject)
	require.Error(t, err)
}

func TestSearchSummaryResponseErrorHandler_UnknownErrorIsInternal(t *testing.T) {
	srv := createSearchSummaryTestServer(newMockSearchSummaryContainer(t))
	w := httptest.NewRecorder()

	srv.searchSummaryResponseErrorHandler(w, httptest.NewRequest(http.MethodPost, "/", nil), errors.New("boom"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "boom")
}

// TestSetupSearchRoutes_SummaryAndSearchCoexist drives the production
// setupSearchRoutes, tenancy middleware included: the summary's absolute route
// must win over the /search subrouter's catch-all, and /search itself must keep
// reaching the chi handler.
func TestSetupSearchRoutes_SummaryAndSearchCoexist(t *testing.T) {
	c := newMockSearchSummaryContainer(t)
	c.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", searchTestTeamID).Return(true, nil)
	c.summaryService.EXPECT().Summarize(mock.Anything, searchTestTeamID, mock.Anything).
		Return(sampleSearchSummary(), nil).Once()
	c.searchService.EXPECT().Search(mock.Anything, searchTestTeamID, mock.Anything).
		Return(&models.SearchResultsResponse{}, nil).Once()
	c.availability.EXPECT().Availability(mock.Anything, searchTestTeamID).
		Return(models.AISummaryAvailability{}).Once()

	r := chi.NewRouter()
	srv := &Server{
		port: "8080", container: c, logger: slog.New(slog.DiscardHandler),
		config: &config.Config{}, router: r,
	}
	srv.setupSearchRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, searchSummaryRequest(t, map[string]any{"query": "q"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"summary"`)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, searchRequest(t, map[string]any{"query": "q"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"results"`)
}

// TestSetupSearchRoutes_SummaryRequiresTeamMembership pins that the summary is
// behind the same tenancy middleware as /search.
func TestSetupSearchRoutes_SummaryRequiresTeamMembership(t *testing.T) {
	c := newMockSearchSummaryContainer(t)
	c.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", searchTestTeamID).Return(false, nil)

	r := chi.NewRouter()
	srv := &Server{
		port: "8080", container: c, logger: slog.New(slog.DiscardHandler),
		config: &config.Config{}, router: r,
	}
	srv.setupSearchRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, searchSummaryRequest(t, map[string]any{"query": "q"}))

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}
