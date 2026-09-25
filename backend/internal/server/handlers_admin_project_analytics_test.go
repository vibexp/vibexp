package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// adminProjectAnalyticsPaths lists every #1145 op for one project id.
func adminProjectAnalyticsPaths(projectID string) map[string]string {
	base := "/api/v1/admin/projects/" + projectID + "/"
	return map[string]string{
		"creation": base + "resource-creation-metrics",
		"access":   base + "resource-access-metrics",
		"top":      base + "top-accessed-resources",
		"config":   base + "config",
	}
}

func TestGetAdminProjectResourceCreationMetrics(t *testing.T) {
	projectID := uuid.NewString()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	path := adminProjectAnalyticsPaths(projectID)["creation"]

	t.Run("200 passes the range through and conforms", func(t *testing.T) {
		metrics := &models.AdminProjectCreationMetrics{
			From: from, To: to, Granularity: "week",
			Series: []models.AdminProjectCreationPoint{
				{Bucket: from, Prompts: 2, Memories: 1, Artifacts: 3, Blueprints: 4, FeedItems: 5},
			},
		}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectCreationMetrics", mock.Anything, projectID,
			mock.MatchedBy(func(q services.AdminTimeseriesQuery) bool {
				return q.Granularity == "week" && q.From != nil && q.From.Equal(from) && q.To != nil && q.To.Equal(to)
			}),
		).Return(metrics, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			path+"?from=2026-09-01T00:00:00Z&to=2026-09-03T00:00:00Z&granularity=week")

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assertNoResourceContent(t, rr.Body.String())
		assertJSONKeyPaths(t, rr.Body.Bytes(), "from", "to", "granularity", "series",
			"series[].bucket", "series[].prompts", "series[].memories", "series[].artifacts",
			"series[].blueprints", "series[].feed_items")
		assert.Contains(t, rr.Body.String(), `"feed_items":5`)
	})

	t.Run("an empty series serializes as []", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectCreationMetrics", mock.Anything, projectID, mock.Anything).
			Return(&models.AdminProjectCreationMetrics{From: from, To: to, Granularity: "day"}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), `"series":[]`)
	})

	t.Run("400 for an out-of-enum granularity", func(t *testing.T) {
		req, rr := serveAdminInsights(t, &adminMockContainer{}, path+"?granularity=hour")
		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	assertAdminProjectServiceErrors(t, "GetProjectCreationMetrics", projectID, path, true)
}

func TestGetAdminProjectResourceAccessMetrics(t *testing.T) {
	projectID := uuid.NewString()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	path := adminProjectAnalyticsPaths(projectID)["access"]

	t.Run("200 passes the range through and conforms", func(t *testing.T) {
		metrics := &models.AdminProjectAccessMetrics{
			From: from, To: to, Granularity: "month",
			AccessBySource: []models.AdminSourcePoint{{Bucket: from, Source: "web", Count: 9}},
		}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectAccessMetrics", mock.Anything, projectID,
			mock.MatchedBy(func(q services.AdminTimeseriesQuery) bool {
				return q.Granularity == "month" && q.From != nil && q.From.Equal(from) && q.To != nil && q.To.Equal(to)
			}),
		).Return(metrics, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			path+"?from=2026-09-01T00:00:00Z&to=2026-09-03T00:00:00Z&granularity=month")

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assertNoResourceContent(t, rr.Body.String())
		assert.Contains(t, rr.Body.String(), `"source":"web"`)
		assert.Contains(t, rr.Body.String(), `"earliest_retained_at"`)
	})

	t.Run("an empty series serializes as []", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectAccessMetrics", mock.Anything, projectID, mock.Anything).
			Return(&models.AdminProjectAccessMetrics{From: from, To: to, Granularity: "day"}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), `"access_by_source":[]`)
	})

	t.Run("400 for an out-of-enum granularity", func(t *testing.T) {
		req, rr := serveAdminInsights(t, &adminMockContainer{}, path+"?granularity=year")
		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	assertAdminProjectServiceErrors(t, "GetProjectAccessMetrics", projectID, path, true)
}

func TestGetAdminProjectTopAccessedResources(t *testing.T) {
	projectID, teamID := uuid.NewString(), uuid.NewString()
	resourceID := uuid.NewString()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	path := adminProjectAnalyticsPaths(projectID)["top"]
	projectName := "Platform"

	t.Run("200 carries opaque rows with exactly the documented keys", func(t *testing.T) {
		top := &models.AdminTopAccessedResources{
			From: from, To: to,
			Items: []models.AdminTopAccessedResource{{
				ResourceType: "blueprint", ResourceID: resourceID, TeamID: teamID, TeamName: "Acme",
				ProjectID: &projectID, ProjectName: &projectName, AccessCount: 11,
			}},
		}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectTopAccessedResources", mock.Anything, projectID,
			mock.MatchedBy(func(q services.AdminTopResourcesQuery) bool {
				return q.Limit == 50 && q.From != nil && q.From.Equal(from) && q.To != nil && q.To.Equal(to)
			}),
		).Return(top, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			path+"?from=2026-09-01T00:00:00Z&to=2026-09-03T00:00:00Z&limit=50")

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assertNoResourceContent(t, rr.Body.String())
		assert.NotContains(t, rr.Body.String(), resourceID, "the full resource id never leaves the server")

		var resp struct {
			Items []map[string]json.RawMessage `json:"items"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Len(t, resp.Items, 1)
		keys := make([]string, 0, len(resp.Items[0]))
		for k := range resp.Items[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		assert.Equal(t, []string{
			"access_count", "project_id", "project_name", "resource_deleted",
			"resource_short_id", "resource_type", "team_id", "team_name",
		}, keys)
		assert.JSONEq(t, `"`+resourceID[:8]+`"`, string(resp.Items[0]["resource_short_id"]))
		assert.JSONEq(t, `false`, string(resp.Items[0]["resource_deleted"]))
	})

	t.Run("no accesses serializes items as []", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectTopAccessedResources", mock.Anything, projectID,
			mock.MatchedBy(func(q services.AdminTopResourcesQuery) bool { return q.Limit == 0 }),
		).Return(&models.AdminTopAccessedResources{From: from, To: to}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), `"items":[]`)
	})

	for _, limit := range []string{"0", "51", "-1"} {
		t.Run("400 for limit "+limit, func(t *testing.T) {
			req, rr := serveAdminInsights(t, &adminMockContainer{}, path+"?limit="+limit)
			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}

	assertAdminProjectServiceErrors(t, "GetProjectTopAccessedResources", projectID, path, true)

	t.Run("500 when the database hands back a non-UUID team id", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectTopAccessedResources", mock.Anything, projectID, mock.Anything).
			Return(&models.AdminTopAccessedResources{Items: []models.AdminTopAccessedResource{
				{ResourceType: "prompt", ResourceID: resourceID, TeamID: "nope"},
			}}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})
}

func TestGetAdminProjectConfig(t *testing.T) {
	projectID, teamID := uuid.NewString(), uuid.NewString()
	path := adminProjectAnalyticsPaths(projectID)["config"]
	now := time.Now().UTC()

	t.Run("200 lists project and team-wide rules separately", func(t *testing.T) {
		projectRule := &models.FreshnessRule{ID: uuid.NewString(), TeamID: teamID, ProjectID: &projectID,
			ResourceTypes: []string{"prompt"}, Mediums: []string{"mcp"}, ThresholdDays: 60,
			CreatedAt: now, UpdatedAt: now}
		teamRule := &models.FreshnessRule{ID: uuid.NewString(), TeamID: teamID,
			ResourceTypes: []string{"memory"}, ThresholdDays: 30, Enabled: true, CreatedAt: now, UpdatedAt: now}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectConfig", mock.Anything, projectID).Return(&models.AdminProjectConfig{
			ProjectRules:  []*models.FreshnessRule{projectRule},
			TeamWideRules: []*models.FreshnessRule{teamRule},
		}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		var resp struct {
			ProjectRules  []map[string]json.RawMessage `json:"project_rules"`
			TeamWideRules []map[string]json.RawMessage `json:"team_wide_rules"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Len(t, resp.ProjectRules, 1)
		require.Len(t, resp.TeamWideRules, 1)
		assert.JSONEq(t, `"`+projectRule.ID+`"`, string(resp.ProjectRules[0]["id"]))
		assert.JSONEq(t, `"`+projectID+`"`, string(resp.ProjectRules[0]["project_id"]))
		assert.JSONEq(t, `"`+teamRule.ID+`"`, string(resp.TeamWideRules[0]["id"]))
		assert.JSONEq(t, `null`, string(resp.TeamWideRules[0]["project_id"]))
		assert.JSONEq(t, `[]`, string(resp.TeamWideRules[0]["mediums"]))

		rulePaths := func(prefix string) []string {
			return []string{prefix + "[].id", prefix + "[].project_id", prefix + "[].resource_types",
				prefix + "[].mediums", prefix + "[].threshold_days", prefix + "[].enabled",
				prefix + "[].created_at", prefix + "[].updated_at"}
		}
		want := append([]string{"project_rules", "team_wide_rules"}, rulePaths("project_rules")...)
		assertJSONKeyPaths(t, rr.Body.Bytes(), append(want, rulePaths("team_wide_rules")...)...)
	})

	t.Run("no rules serializes both lists as []", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetProjectConfig", mock.Anything, projectID).Return(&models.AdminProjectConfig{}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.JSONEq(t, `{"project_rules":[],"team_wide_rules":[]}`, rr.Body.String())
	})

	for name, rules := range map[string]*models.AdminProjectConfig{
		"project rule": {ProjectRules: []*models.FreshnessRule{{ID: "nope"}}},
		"team rule":    {TeamWideRules: []*models.FreshnessRule{{ID: "nope"}}},
	} {
		t.Run("500 when a "+name+" has a non-UUID id", func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On("GetProjectConfig", mock.Anything, projectID).Return(rules, nil)

			req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
			require.Equal(t, http.StatusInternalServerError, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}

	assertAdminProjectServiceErrors(t, "GetProjectConfig", projectID, path, false)
}

// assertAdminProjectServiceErrors pins the shared error mapping of a #1145 op:
// (nil, nil) is 404, a range error is 400 (for ops that take a range) and any
// other error is a 500 that leaks nothing. The config op takes no range.
func assertAdminProjectServiceErrors(t *testing.T, method, projectID, path string, ranged bool) {
	t.Helper()
	call := func(m *servicesmocks.MockAdminServiceInterface) *mock.Call {
		if ranged {
			return m.On(method, mock.Anything, projectID, mock.Anything)
		}
		return m.On(method, mock.Anything, projectID)
	}

	t.Run("404 for an unknown project", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		call(mockAdmin).Return(nil, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusNotFound, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	if ranged {
		t.Run("400 for an invalid range", func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			call(mockAdmin).Return(nil, &services.ErrAdminTimeseriesRange{Detail: "invalid range"})

			req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}

	t.Run("500 when the service fails", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		call(mockAdmin).Return(nil, errors.New("db down"))

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.NotContains(t, rr.Body.String(), "db down")
		specconformance.AssertConformsToSpec(t, req, rr)
	})
}

func TestAdminProjectAnalytics_MalformedIDIs400(t *testing.T) {
	for name, path := range adminProjectAnalyticsPaths("not-a-uuid") {
		t.Run(name, func(t *testing.T) {
			req, rr := serveAdminInsights(t, &adminMockContainer{}, path)
			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestAdminProjectAnalytics_UnauthenticatedGets404(t *testing.T) {
	for name, path := range adminProjectAnalyticsPaths(uuid.NewString()) {
		t.Run(name, func(t *testing.T) {
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService: servicesmocks.NewMockAdminServiceInterface(t),
				authService:  servicesmocks.NewMockAuthServiceInterface(t),
			})

			req := httptest.NewRequest("GET", path, nil)
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusNotFound, rr.Code)
		})
	}
}
