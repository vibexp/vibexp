package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	ramocks "github.com/vibexp/vibexp/internal/services/resourceaccess/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testCreationTeamID = "550e8400-e29b-41d4-a716-446655440000"
	testCreationUserID = "user-xyz"
	testCreationSlug   = "my-project"
)

// creationMetricsContainer wires the three services the two creation-metrics
// reads touch: TeamService (membership + the team series), ProjectService (the
// project series) and ResourceWriteService (the per-channel breakdown).
type creationMetricsContainer struct {
	BaseMockContainer
	teams    services.TeamServiceInterface
	projects services.ProjectServiceInterface
	writes   resourceaccess.ResourceWriteService
}

func (c *creationMetricsContainer) TeamService() services.TeamServiceInterface { return c.teams }
func (c *creationMetricsContainer) ProjectService() services.ProjectServiceInterface {
	return c.projects
}
func (c *creationMetricsContainer) ResourceWriteService() resourceaccess.ResourceWriteService {
	return c.writes
}

type creationMetricsFixture struct {
	srv      *Server
	teams    *svcmocks.MockTeamServiceInterface
	projects *svcmocks.MockProjectServiceInterface
	writes   *ramocks.MockResourceWriteService
}

// newCreationMetricsFixture mounts the generated handler on a router that also
// carries the two legacy subtrees it sits beneath in production
// (r.Route("/api/v1/teams") and r.Route("/api/v1/{team_id}/projects")), so every
// request below proves the absolute routes are reachable past those mounts.
func newCreationMetricsFixture(t *testing.T) *creationMetricsFixture {
	t.Helper()

	f := &creationMetricsFixture{
		teams:    svcmocks.NewMockTeamServiceInterface(t),
		projects: svcmocks.NewMockProjectServiceInterface(t),
		writes:   ramocks.NewMockResourceWriteService(t),
	}

	r := chi.NewRouter()
	f.srv = &Server{
		port:      "8080",
		container: &creationMetricsContainer{teams: f.teams, projects: f.projects, writes: f.writes},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}

	legacy := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }
	r.Route("/api/v1/teams", func(r chi.Router) {
		r.Get("/{id}", legacy)
		r.Get("/{id}/stats", legacy)
	})
	r.Route("/api/v1/{team_id}/projects", func(r chi.Router) {
		r.Get("/{slug}", legacy)
		r.Get("/{slug}/stats", legacy)
	})
	f.srv.setupCreationMetricsRoutes(r)

	return f
}

func (f *creationMetricsFixture) allowMember() {
	f.teams.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testCreationUserID, testCreationTeamID).
		Return(true, nil).Once()
}

func (f *creationMetricsFixture) do(req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, req)
	return w
}

func teamCreationMetricsRequest(teamID, query string) *http.Request {
	url := "/api/v1/teams/" + teamID + "/resource-creation-metrics"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testCreationUserID))
}

func projectCreationMetricsRequest(slug, query string) *http.Request {
	url := "/api/v1/" + testCreationTeamID + "/projects/" + slug + "/resource-creation-metrics"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testCreationUserID))
}

// The wire shape, decoded independently of the generated types so the tests pin
// the JSON keys rather than the Go field names.
type creationSourceCountsBody struct {
	Prompts    int `json:"prompts"`
	Artifacts  int `json:"artifacts"`
	Blueprints int `json:"blueprints"`
	Memories   int `json:"memories"`
	Total      int `json:"total"`
}

type creationDayBody struct {
	Date       string `json:"date"`
	Prompts    int    `json:"prompts"`
	Artifacts  int    `json:"artifacts"`
	Blueprints int    `json:"blueprints"`
	Memories   int    `json:"memories"`
	Projects   int    `json:"projects"`
	Total      int    `json:"total"`
}

type creationMetricsBody struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		TotalCreated   int                                 `json:"total_created"`
		Range          string                              `json:"range"`
		Counts         []creationDayBody                   `json:"counts"`
		BySource       map[string]creationSourceCountsBody `json:"by_source"`
		WritesBySource map[string]int                      `json:"writes_by_source"`
	} `json:"data"`
}

func decodeCreationMetrics(t *testing.T, w *httptest.ResponseRecorder) creationMetricsBody {
	t.Helper()
	var body creationMetricsBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

type apiErrorBody struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func decodeAPIError(t *testing.T, w *httptest.ResponseRecorder) apiErrorBody {
	t.Helper()
	var body apiErrorBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// sampleCreationSources is a breakdown whose per-type sums are known: prompts
// 3, artifacts 0, blueprints 0, memories 2.
func sampleCreationSources() *resourceaccess.CreationSources {
	return &resourceaccess.CreationSources{
		Created: map[string]resourceaccess.SourceCreationCounts{
			resourceaccess.SourceWeb:     {Prompts: 2, Total: 2},
			resourceaccess.SourceCLI:     {},
			resourceaccess.SourceMCP:     {Memories: 1, Total: 1},
			resourceaccess.SourceAPI:     {},
			resourceaccess.SourceUnknown: {Prompts: 1, Memories: 1, Total: 2},
		},
		Writes: map[string]int{
			resourceaccess.SourceWeb: 5, resourceaccess.SourceCLI: 0,
			resourceaccess.SourceMCP: 9, resourceaccess.SourceAPI: 0,
		},
	}
}

// sumBySource adds one resource type across the five channels.
func sumBySource(bySource map[string]creationSourceCountsBody, pick func(creationSourceCountsBody) int) int {
	total := 0
	for _, counts := range bySource {
		total += pick(counts)
	}
	return total
}

// ---------------------------------------------------------------------------
// Team
// ---------------------------------------------------------------------------

func TestGetTeamResourceCreationMetrics_Success(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()

	today := time.Now().UTC().Format("2006-01-02")
	f.teams.EXPECT().
		GetTeamResourceCreationMetrics(mock.Anything, testCreationTeamID, mock.Anything).
		Return([]models.TeamResourceCreationCount{
			{Date: today, ResourceType: "prompts", Count: 3},
			{Date: today, ResourceType: "projects", Count: 1},
			{Date: today, ResourceType: "memories", Count: 2},
		}, nil).Once()
	f.writes.EXPECT().
		GetTeamCreationSources(mock.Anything, testCreationTeamID, mock.Anything).
		Return(sampleCreationSources(), nil).Once()

	req := teamCreationMetricsRequest(testCreationTeamID, "range=7d")
	w := f.do(req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	body := decodeCreationMetrics(t, w)
	assert.Equal(t, "success", body.Status)
	assert.Equal(t, "Team resource creation metrics retrieved successfully", body.Message)
	assert.Equal(t, "7d", body.Data.Range)
	assert.Equal(t, 6, body.Data.TotalCreated)
	require.Len(t, body.Data.Counts, 8) // 7-day window inclusive of both ends.

	last := body.Data.Counts[len(body.Data.Counts)-1]
	assert.Equal(t, today, last.Date)
	assert.Equal(t, 3, last.Prompts)
	assert.Equal(t, 1, last.Projects)
	assert.Equal(t, 2, last.Memories)
	assert.Equal(t, 6, last.Total)
	assert.Equal(t, 0, body.Data.Counts[0].Total) // gap-free zero-fill.

	// All five channels and all four write channels are present.
	require.Len(t, body.Data.BySource, 5)
	for _, source := range []string{"web", "cli", "mcp", "api", "unknown"} {
		assert.Contains(t, body.Data.BySource, source)
	}
	assert.Equal(t, creationSourceCountsBody{Prompts: 2, Total: 2}, body.Data.BySource["web"])
	assert.Equal(t, creationSourceCountsBody{Memories: 1, Total: 1}, body.Data.BySource["mcp"])
	assert.Equal(t, creationSourceCountsBody{Prompts: 1, Memories: 1, Total: 2}, body.Data.BySource["unknown"])
	assert.Equal(t, map[string]int{"web": 5, "cli": 0, "mcp": 9, "api": 0}, body.Data.WritesBySource)

	// The breakdown sums to the series, type by type. Projects are in the
	// series and not in the breakdown.
	assert.Equal(t, 3, sumBySource(body.Data.BySource, func(c creationSourceCountsBody) int { return c.Prompts }))
	assert.Equal(t, 2, sumBySource(body.Data.BySource, func(c creationSourceCountsBody) int { return c.Memories }))
	assert.Equal(t, body.Data.TotalCreated-1,
		sumBySource(body.Data.BySource, func(c creationSourceCountsBody) int { return c.Total }))
}

// The window handed to both services is the same instant: UTC midnight of the
// first day of the series. A breakdown computed over a different window could
// not sum to the series.
func TestGetTeamResourceCreationMetrics_SameWindowForSeriesAndSources(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()

	var seriesSince, sourcesSince time.Time
	f.teams.EXPECT().
		GetTeamResourceCreationMetrics(mock.Anything, testCreationTeamID, mock.Anything).
		Run(func(_ context.Context, _ string, since time.Time) { seriesSince = since }).
		Return([]models.TeamResourceCreationCount{}, nil).Once()
	f.writes.EXPECT().
		GetTeamCreationSources(mock.Anything, testCreationTeamID, mock.Anything).
		Run(func(_ context.Context, _ string, since time.Time) { sourcesSince = since }).
		Return(sampleCreationSources(), nil).Once()

	w := f.do(teamCreationMetricsRequest(testCreationTeamID, "range=14d"))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, seriesSince.Equal(sourcesSince))
	assert.Equal(t, time.UTC, seriesSince.Location())
	h, m, s := seriesSince.Clock()
	assert.Zero(t, h+m+s, "since must be UTC midnight")

	body := decodeCreationMetrics(t, w)
	assert.Equal(t, seriesSince.Format("2006-01-02"), body.Data.Counts[0].Date)
}

// The 180d option on the shared range map zero-fills to 181 inclusive days.
func TestGetTeamResourceCreationMetrics_ExtendedRange(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()
	f.teams.EXPECT().
		GetTeamResourceCreationMetrics(mock.Anything, testCreationTeamID, mock.Anything).
		Return([]models.TeamResourceCreationCount{}, nil).Once()
	f.writes.EXPECT().
		GetTeamCreationSources(mock.Anything, testCreationTeamID, mock.Anything).
		Return(sampleCreationSources(), nil).Once()

	req := teamCreationMetricsRequest(testCreationTeamID, "range=180d")
	w := f.do(req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	body := decodeCreationMetrics(t, w)
	assert.Equal(t, "180d", body.Data.Range)
	require.Len(t, body.Data.Counts, 181)
}

func TestGetTeamResourceCreationMetrics_InvalidRange(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()

	req := teamCreationMetricsRequest(testCreationTeamID, "range=999d")
	w := f.do(req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	body := decodeAPIError(t, w)
	assert.Equal(t, "VALIDATION_FAILED", body.Code)
	assert.Equal(t, "range is invalid", body.Detail)
	// No service expectation is set: reaching either one would fail the test.
}

// A caller who is not a member of the team is denied before any data is read,
// with the answer the legacy handler gave.
func TestGetTeamResourceCreationMetrics_NonMemberForbidden(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.teams.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testCreationUserID, testCreationTeamID).
		Return(false, nil).Once()

	req := teamCreationMetricsRequest(testCreationTeamID, "range=7d")
	w := f.do(req)

	require.Equal(t, http.StatusForbidden, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	body := decodeAPIError(t, w)
	assert.Equal(t, "access_denied", body.Code)
	assert.Equal(t, "Access denied", body.Detail)
}

// A membership lookup that errors is a denial, not a 500 and not a pass.
func TestGetTeamResourceCreationMetrics_MembershipErrorForbidden(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.teams.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testCreationUserID, testCreationTeamID).
		Return(false, errors.New("db down")).Once()

	w := f.do(teamCreationMetricsRequest(testCreationTeamID, ""))

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetTeamResourceCreationMetrics_InvalidTeamID(t *testing.T) {
	f := newCreationMetricsFixture(t)

	req := teamCreationMetricsRequest("not-a-uuid", "")
	w := f.do(req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "team id must be a valid UUID", decodeAPIError(t, w).Detail)
}

func TestGetTeamResourceCreationMetrics_ServiceErrors(t *testing.T) {
	t.Run("series", func(t *testing.T) {
		f := newCreationMetricsFixture(t)
		f.allowMember()
		f.teams.EXPECT().
			GetTeamResourceCreationMetrics(mock.Anything, testCreationTeamID, mock.Anything).
			Return(nil, errors.New("db down")).Once()

		req := teamCreationMetricsRequest(testCreationTeamID, "")
		w := f.do(req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodeAPIError(t, w)
		assert.Equal(t, "Failed to get team resource creation metrics", body.Detail)
		assert.NotContains(t, w.Body.String(), "db down")
	})

	t.Run("sources", func(t *testing.T) {
		f := newCreationMetricsFixture(t)
		f.allowMember()
		f.teams.EXPECT().
			GetTeamResourceCreationMetrics(mock.Anything, testCreationTeamID, mock.Anything).
			Return([]models.TeamResourceCreationCount{}, nil).Once()
		f.writes.EXPECT().
			GetTeamCreationSources(mock.Anything, testCreationTeamID, mock.Anything).
			Return(nil, errors.New("db down")).Once()

		w := f.do(teamCreationMetricsRequest(testCreationTeamID, ""))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotContains(t, w.Body.String(), "db down")
	})
}

// ---------------------------------------------------------------------------
// Project
// ---------------------------------------------------------------------------

func TestGetProjectResourceCreationMetrics_Success(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()

	today := time.Now().UTC().Format("2006-01-02")
	f.projects.EXPECT().
		GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, testCreationSlug, mock.Anything).
		Return([]models.ProjectResourceCreationCount{
			{Date: today, ResourceType: "prompts", Count: 3},
			{Date: today, ResourceType: "memories", Count: 2},
		}, nil).Once()
	f.writes.EXPECT().
		GetProjectCreationSources(mock.Anything, testCreationTeamID, testCreationSlug, mock.Anything).
		Return(sampleCreationSources(), nil).Once()

	req := projectCreationMetricsRequest(testCreationSlug, "range=7d")
	w := f.do(req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	body := decodeCreationMetrics(t, w)
	assert.Equal(t, "success", body.Status)
	assert.Equal(t, "Resource creation metrics retrieved successfully", body.Message)
	assert.Equal(t, "7d", body.Data.Range)
	assert.Equal(t, 5, body.Data.TotalCreated)
	require.Len(t, body.Data.Counts, 8)

	last := body.Data.Counts[len(body.Data.Counts)-1]
	assert.Equal(t, today, last.Date)
	assert.Equal(t, 3, last.Prompts)
	assert.Equal(t, 0, last.Artifacts)
	assert.Equal(t, 0, last.Blueprints)
	assert.Equal(t, 2, last.Memories)
	assert.Equal(t, 5, last.Total)
	assert.Equal(t, 0, body.Data.Counts[0].Total)

	// A project series has no projects key at all.
	assert.NotContains(t, w.Body.String(), `"projects"`)

	// For a project the breakdown sums to total_created exactly.
	require.Len(t, body.Data.BySource, 5)
	assert.Equal(t, body.Data.TotalCreated,
		sumBySource(body.Data.BySource, func(c creationSourceCountsBody) int { return c.Total }))
	assert.Equal(t, map[string]int{"web": 5, "cli": 0, "mcp": 9, "api": 0}, body.Data.WritesBySource)
}

// The range defaults to 30d (a 31-day inclusive window) and an empty project
// zero-fills cleanly, including the breakdown.
func TestGetProjectResourceCreationMetrics_DefaultRange(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()
	f.projects.EXPECT().
		GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, testCreationSlug, mock.Anything).
		Return([]models.ProjectResourceCreationCount{}, nil).Once()
	f.writes.EXPECT().
		GetProjectCreationSources(mock.Anything, testCreationTeamID, testCreationSlug, mock.Anything).
		Return(&resourceaccess.CreationSources{}, nil).Once()

	req := projectCreationMetricsRequest(testCreationSlug, "")
	w := f.do(req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	body := decodeCreationMetrics(t, w)
	assert.Equal(t, "30d", body.Data.Range)
	require.Len(t, body.Data.Counts, 31)
	assert.Equal(t, 0, body.Data.TotalCreated)
	// Even when the service reports no channel at all, every key is on the wire.
	require.Len(t, body.Data.BySource, 5)
	assert.Equal(t, map[string]int{"web": 0, "cli": 0, "mcp": 0, "api": 0}, body.Data.WritesBySource)
}

// chi hands the handler a percent-encoded slug; the generated binder decodes it
// once, as a path segment, so a literal "+" survives (#251/#257).
func TestGetProjectResourceCreationMetrics_DecodesSlug(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()
	f.projects.EXPECT().
		GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, "a+b c", mock.Anything).
		Return([]models.ProjectResourceCreationCount{}, nil).Once()
	f.writes.EXPECT().
		GetProjectCreationSources(mock.Anything, testCreationTeamID, "a+b c", mock.Anything).
		Return(sampleCreationSources(), nil).Once()

	w := f.do(projectCreationMetricsRequest("a+b%20c", ""))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestGetProjectResourceCreationMetrics_InvalidRange(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()

	req := projectCreationMetricsRequest(testCreationSlug, "range=999d")
	w := f.do(req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Equal(t, "VALIDATION_FAILED", decodeAPIError(t, w).Code)
}

// An unknown or inaccessible project is a 404, and the per-channel queries,
// which trust the slug, are never reached.
func TestGetProjectResourceCreationMetrics_NotFound(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.allowMember()

	notFound := fmt.Errorf(
		"%w: slug=missing-project team=%s", repositories.ErrProjectNotFoundForRepo, testCreationTeamID,
	)
	f.projects.EXPECT().
		GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, "missing-project", mock.Anything).
		Return(nil, notFound).Once()

	req := projectCreationMetricsRequest("missing-project", "range=30d")
	w := f.do(req)

	require.Equal(t, http.StatusNotFound, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Equal(t, "Project not found", decodeAPIError(t, w).Detail)
}

// The project route used to sit behind teamValidationMiddleware; the generated
// handler makes the same membership check itself.
func TestGetProjectResourceCreationMetrics_NonMemberForbidden(t *testing.T) {
	f := newCreationMetricsFixture(t)
	f.teams.EXPECT().
		IsUserMemberOfTeam(mock.Anything, testCreationUserID, testCreationTeamID).
		Return(false, nil).Once()

	req := projectCreationMetricsRequest(testCreationSlug, "")
	w := f.do(req)

	require.Equal(t, http.StatusForbidden, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Equal(t, "access_denied", decodeAPIError(t, w).Code)
}

func TestGetProjectResourceCreationMetrics_InvalidTeamID(t *testing.T) {
	f := newCreationMetricsFixture(t)

	url := "/api/v1/not-a-uuid/projects/" + testCreationSlug + "/resource-creation-metrics"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testCreationUserID))
	w := f.do(req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "team_id must be a valid UUID", decodeAPIError(t, w).Detail)
}

func TestGetProjectResourceCreationMetrics_ServiceErrors(t *testing.T) {
	t.Run("series", func(t *testing.T) {
		f := newCreationMetricsFixture(t)
		f.allowMember()
		f.projects.EXPECT().
			GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, testCreationSlug, mock.Anything).
			Return(nil, errors.New("db down")).Once()

		w := f.do(projectCreationMetricsRequest(testCreationSlug, ""))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "Failed to get project resource creation metrics", decodeAPIError(t, w).Detail)
		assert.NotContains(t, w.Body.String(), "db down")
	})

	t.Run("sources", func(t *testing.T) {
		f := newCreationMetricsFixture(t)
		f.allowMember()
		f.projects.EXPECT().
			GetProjectResourceCreationMetrics(testCreationTeamID, testCreationUserID, testCreationSlug, mock.Anything).
			Return([]models.ProjectResourceCreationCount{}, nil).Once()
		f.writes.EXPECT().
			GetProjectCreationSources(mock.Anything, testCreationTeamID, testCreationSlug, mock.Anything).
			Return(nil, errors.New("db down")).Once()

		w := f.do(projectCreationMetricsRequest(testCreationSlug, ""))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotContains(t, w.Body.String(), "db down")
	})
}

// ---------------------------------------------------------------------------
// Routing and helpers
// ---------------------------------------------------------------------------

// The absolute generated routes must not swallow the legacy routes of the two
// subtrees they sit beneath.
func TestCreationMetricsRoutes_CoexistWithLegacySubtrees(t *testing.T) {
	f := newCreationMetricsFixture(t)

	for _, path := range []string{
		"/api/v1/teams/" + testCreationTeamID,
		"/api/v1/teams/" + testCreationTeamID + "/stats",
		"/api/v1/" + testCreationTeamID + "/projects/" + testCreationSlug,
		"/api/v1/" + testCreationTeamID + "/projects/" + testCreationSlug + "/stats",
	} {
		w := f.do(httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusTeapot, w.Code, path)
	}
}

func TestResolveCreationMetricsWindow(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		window, err := resolveCreationMetricsWindow("")
		require.NoError(t, err)
		assert.Equal(t, "30d", window.rangeStr)
		assert.Equal(t, 30, window.rangeDays)

		today := time.Now().UTC()
		want := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -30)
		assert.True(t, window.since.Equal(want))
	})

	t.Run("unknown range", func(t *testing.T) {
		_, err := resolveCreationMetricsWindow("1y")
		require.Error(t, err)
	})
}

func TestZeroFillCreationDays(t *testing.T) {
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	window := creationMetricsWindow{rangeStr: "7d", rangeDays: 2, since: since}

	days := zeroFillCreationDays([]creationRow{
		{date: "2026-10-02", resourceType: "prompts", count: 2},
		{date: "2026-10-02", resourceType: "prompts", count: 1}, // same key twice accumulates
		{date: "2026-10-03", resourceType: "projects", count: 4},
		{date: "2026-09-30", resourceType: "memories", count: 9}, // outside the window: dropped
	}, window)

	require.Len(t, days, 3)
	assert.Equal(t, creationDay{date: since}, days[0])
	assert.Equal(t, creationDay{date: since.AddDate(0, 0, 1), prompts: 3}, days[1])
	assert.Equal(t, creationDay{date: since.AddDate(0, 0, 2), projects: 4}, days[2])
}
