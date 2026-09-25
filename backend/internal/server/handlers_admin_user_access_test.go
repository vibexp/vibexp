package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

func TestGetAdminUserResourceAccessMetrics(t *testing.T) {
	userID := uuid.NewString()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	path := "/api/v1/admin/users/" + userID + "/resource-access-metrics"

	t.Run("200 passes the range through and conforms", func(t *testing.T) {
		metrics := &models.AdminUserAccessMetrics{
			From: from, To: to, Granularity: "week",
			AccessBySource: []models.AdminSourcePoint{{Bucket: from, Source: "mcp", Count: 4}},
		}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserAccessMetrics", mock.Anything, userID,
			mock.MatchedBy(func(q services.AdminTimeseriesQuery) bool {
				return q.Granularity == "week" && q.From != nil && q.From.Equal(from) && q.To != nil && q.To.Equal(to)
			}),
		).Return(metrics, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			path+"?from=2026-09-01T00:00:00Z&to=2026-09-03T00:00:00Z&granularity=week")

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assertNoResourceContent(t, rr.Body.String())
		assert.Contains(t, rr.Body.String(), `"source":"mcp"`)
		assert.Contains(t, rr.Body.String(), `"earliest_retained_at"`)
	})

	t.Run("an empty series serializes as []", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserAccessMetrics", mock.Anything, userID, mock.Anything).
			Return(&models.AdminUserAccessMetrics{From: from, To: to, Granularity: "day"}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), `"access_by_source":[]`)
	})

	t.Run("400 for an out-of-enum granularity", func(t *testing.T) {
		req, rr := serveAdminInsights(t, &adminMockContainer{}, path+"?granularity=hour")
		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("400 for an invalid range", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserAccessMetrics", mock.Anything, userID, mock.Anything).
			Return(nil, &services.ErrAdminTimeseriesRange{Detail: "invalid range"})

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("404 for an unknown user", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserAccessMetrics", mock.Anything, userID, mock.Anything).Return(nil, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusNotFound, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("500 when the service fails", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserAccessMetrics", mock.Anything, userID, mock.Anything).Return(nil, errors.New("boom"))

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})
}

func TestGetAdminUserTopAccessedResources(t *testing.T) {
	userID := uuid.NewString()
	teamID, projectID := uuid.NewString(), uuid.NewString()
	resourceID, deletedID := uuid.NewString(), uuid.NewString()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	path := "/api/v1/admin/users/" + userID + "/top-accessed-resources"
	projectName := "Platform"

	t.Run("200 carries opaque rows with exactly the documented keys", func(t *testing.T) {
		top := &models.AdminTopAccessedResources{
			From: from, To: to,
			Items: []models.AdminTopAccessedResource{
				{
					ResourceType: "prompt", ResourceID: resourceID, TeamID: teamID, TeamName: "Acme",
					ProjectID: &projectID, ProjectName: &projectName, AccessCount: 7,
				},
				{ResourceType: "memory", ResourceID: deletedID, TeamID: teamID, TeamName: "Acme",
					ResourceDeleted: true, AccessCount: 2},
			},
		}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTopAccessedResources", mock.Anything, userID,
			mock.MatchedBy(func(q services.AdminTopResourcesQuery) bool {
				return q.Limit == 5 && q.From != nil && q.From.Equal(from) && q.To != nil && q.To.Equal(to)
			}),
		).Return(top, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			path+"?from=2026-09-01T00:00:00Z&to=2026-09-03T00:00:00Z&limit=5")

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assertNoResourceContent(t, rr.Body.String())
		assert.NotContains(t, rr.Body.String(), resourceID, "the full resource id never leaves the server")

		var resp struct {
			Items []map[string]json.RawMessage `json:"items"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Len(t, resp.Items, 2)
		for _, item := range resp.Items {
			keys := make([]string, 0, len(item))
			for k := range item {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			assert.Equal(t, []string{
				"access_count", "project_id", "project_name", "resource_deleted",
				"resource_short_id", "resource_type", "team_id", "team_name",
			}, keys)
		}
		assert.JSONEq(t, `"`+resourceID[:8]+`"`, string(resp.Items[0]["resource_short_id"]))
		assert.JSONEq(t, `7`, string(resp.Items[0]["access_count"]))
		assert.JSONEq(t, `null`, string(resp.Items[1]["project_id"]))
		assert.JSONEq(t, `true`, string(resp.Items[1]["resource_deleted"]))
	})

	t.Run("no accesses serializes items as []", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTopAccessedResources", mock.Anything, userID,
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

	t.Run("400 for an invalid range", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTopAccessedResources", mock.Anything, userID, mock.Anything).
			Return(nil, &services.ErrAdminTimeseriesRange{Detail: "invalid range"})

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("404 for an unknown user", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTopAccessedResources", mock.Anything, userID, mock.Anything).Return(nil, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusNotFound, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("500 when the service fails", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTopAccessedResources", mock.Anything, userID, mock.Anything).
			Return(nil, errors.New("boom"))

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("500 when the database hands back a non-UUID team id", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTopAccessedResources", mock.Anything, userID, mock.Anything).
			Return(&models.AdminTopAccessedResources{Items: []models.AdminTopAccessedResource{
				{ResourceType: "agent", ResourceID: resourceID, TeamID: "nope"},
			}}, nil)

		_, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}
