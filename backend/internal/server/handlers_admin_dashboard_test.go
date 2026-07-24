package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// adminDashboardConfig is a config with retention TTLs set, so the handler can
// derive a data window.
func adminDashboardConfig(version string) *config.Config {
	cfg := &config.Config{}
	cfg.Server.ServiceVersion = version
	cfg.Retention.ActivityDays = 90
	cfg.Retention.AccessEventDays = 30
	return cfg
}

func adminOverviewFixture() models.AdminDashboardOverview {
	return models.AdminDashboardOverview{
		Counts: models.AdminExtendedCounts{
			Users: 42, Teams: 12, Projects: 30, Prompts: 340, Artifacts: 128,
			Memories: 512, Blueprints: 64, Agents: 9, Feeds: 4, APIKeys: 17,
		},
		Breakdowns: []models.AdminEntityBreakdown{{
			Entity: "prompts", Field: "status",
			Buckets: []models.AdminBreakdownBucket{
				{Value: "active", Count: 300},
				{Value: "draft", Count: 40},
			},
		}},
		SystemHealth: models.AdminSystemHealth{
			DatabaseSizeBytes: 184549376,
			Tables:            []models.AdminTableStat{{Table: "prompts", EstimatedRows: 340}},
		},
		Version: "1.2.3",
	}
}

// TestGetAdminDashboardOverview_Success asserts the full payload round-trips and
// conforms to the spec.
func TestGetAdminDashboardOverview_Success(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetDashboardOverview", mock.Anything, "1.2.3").Return(adminOverviewFixture(), nil)
	srv := newAdminTestServer(adminDashboardConfig("1.2.3"), &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/dashboard/overview", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminDashboardOverview
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	// Every extended count must actually be mapped — a dropped field would
	// serialize as 0 and still satisfy the schema.
	assert.Equal(t, int64(42), resp.Counts.Users)
	assert.Equal(t, int64(30), resp.Counts.Projects)
	assert.Equal(t, int64(64), resp.Counts.Blueprints)
	assert.Equal(t, int64(9), resp.Counts.Agents)
	assert.Equal(t, int64(4), resp.Counts.Feeds)
	assert.Equal(t, int64(17), resp.Counts.ApiKeys)

	require.Len(t, resp.Breakdowns, 1)
	assert.Equal(t, "prompts", resp.Breakdowns[0].Entity)
	assert.Equal(t, "status", resp.Breakdowns[0].Field)
	require.Len(t, resp.Breakdowns[0].Buckets, 2)
	assert.Equal(t, "active", resp.Breakdowns[0].Buckets[0].Value)

	assert.Equal(t, int64(184549376), resp.SystemHealth.DatabaseSizeBytes)
	require.Len(t, resp.SystemHealth.Tables, 1)
	// Value-asserted, not just counted: a dropped mapping would serialize as
	// "" / 0 and still satisfy the schema.
	assert.Equal(t, "prompts", resp.SystemHealth.Tables[0].Table)
	assert.Equal(t, int64(340), resp.SystemHealth.Tables[0].EstimatedRows)
	assert.Equal(t, "1.2.3", resp.Version)

	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestGetAdminDashboardOverview_EmptyArraysSerializeAsBrackets covers #125 for
// the two new required arrays: a service returning nil slices must still emit
// [] rather than null.
func TestGetAdminDashboardOverview_EmptyArraysSerializeAsBrackets(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetDashboardOverview", mock.Anything, "dev").
		Return(models.AdminDashboardOverview{Version: "dev"}, nil)
	srv := newAdminTestServer(adminDashboardConfig(""), &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/dashboard/overview", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.NotContains(t, body, `"breakdowns":null`)
	assert.NotContains(t, body, `"tables":null`)
	assert.Contains(t, body, `"breakdowns":[]`)
	assert.Contains(t, body, `"tables":[]`)
	// Version falls back to "dev" when server.service_version is unset, matching
	// GetAdminStats.
	assert.Contains(t, body, `"version":"dev"`)

	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminDashboardOverview_ServiceError(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetDashboardOverview", mock.Anything, mock.Anything).
		Return(models.AdminDashboardOverview{}, errors.New("db down"))
	srv := newAdminTestServer(adminDashboardConfig(""), &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/dashboard/overview", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

func adminTimeseriesFixture() models.AdminTimeseries {
	b0 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	b1 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	return models.AdminTimeseries{
		From:        b0,
		To:          time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		Granularity: "day",
		Growth: []models.AdminGrowthPoint{
			{Bucket: b0, Users: 2, Teams: 1, Projects: 3, Prompts: 4, Artifacts: 5, Memories: 6},
			{Bucket: b1},
		},
		SignIns: []models.AdminCountPoint{
			{Bucket: b0, Count: 7},
			{Bucket: b1, Count: 0},
		},
		AccessBySource: []models.AdminSourcePoint{
			{Bucket: b0, Source: "mcp", Count: 11},
			{Bucket: b1, Source: "mcp", Count: 0},
		},
		DataWindow: models.AdminDataWindow{
			SignInsEarliestRetainedAt:        b0.AddDate(0, 0, -90),
			AccessBySourceEarliestRetainedAt: b0.AddDate(0, 0, -30),
		},
	}
}

// TestGetAdminDashboardTimeseries_Success asserts the full series payload maps
// and conforms, including the per-entity growth fields.
func TestGetAdminDashboardTimeseries_Success(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetDashboardTimeseries", mock.Anything, mock.Anything, mock.Anything).
		Return(adminTimeseriesFixture(), nil)
	srv := newAdminTestServer(adminDashboardConfig("1.0.0"), &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/dashboard/timeseries?granularity=day", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminTimeseriesResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	require.Len(t, resp.Growth, 2)
	assert.Equal(t, int64(2), resp.Growth[0].Users)
	assert.Equal(t, int64(1), resp.Growth[0].Teams)
	assert.Equal(t, int64(3), resp.Growth[0].Projects)
	assert.Equal(t, int64(4), resp.Growth[0].Prompts)
	assert.Equal(t, int64(5), resp.Growth[0].Artifacts)
	assert.Equal(t, int64(6), resp.Growth[0].Memories)
	// The gap-filled bucket is present with explicit zeros, not omitted.
	assert.Zero(t, resp.Growth[1].Users)

	require.Len(t, resp.SignIns, 2)
	assert.Equal(t, int64(7), resp.SignIns[0].Count)
	require.Len(t, resp.AccessBySource, 2)
	assert.Equal(t, "mcp", resp.AccessBySource[0].Source)

	assert.Equal(t, admingen.AdminTimeseriesResponseGranularity("day"), resp.Granularity)
	// The two window instants must be the RIGHT way round: this is what tells an
	// operator whether a flat chart means "no activity" or "pruned", so a swapped
	// mapping is a real misreport. Both are non-zero and distinct, so !IsZero()
	// alone would not catch a transposition.
	fixture := adminTimeseriesFixture()
	assert.Equal(t, fixture.DataWindow.SignInsEarliestRetainedAt.UTC(),
		resp.DataWindow.SignInsEarliestRetainedAt.UTC())
	assert.Equal(t, fixture.DataWindow.AccessBySourceEarliestRetainedAt.UTC(),
		resp.DataWindow.AccessBySourceEarliestRetainedAt.UTC())
	assert.True(t, resp.DataWindow.AccessBySourceEarliestRetainedAt.
		After(resp.DataWindow.SignInsEarliestRetainedAt),
		"the 30-day access window must be more recent than the 90-day sign-in window")

	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestGetAdminDashboardTimeseries_PassesParamsThrough checks the handler hands
// the parsed from/to/granularity to the service unchanged, and derives the data
// window from the configured retention TTLs.
func TestGetAdminDashboardTimeseries_PassesParamsThrough(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetDashboardTimeseries", mock.Anything,
		mock.MatchedBy(func(q services.AdminTimeseriesQuery) bool {
			return q.From != nil && q.From.Equal(from) &&
				q.To != nil && q.To.Equal(to) &&
				q.Granularity == "month"
		}),
		mock.MatchedBy(func(w models.AdminDataWindow) bool {
			// 90 / 30 days from adminDashboardConfig.
			return !w.SignInsEarliestRetainedAt.IsZero() &&
				w.AccessBySourceEarliestRetainedAt.After(w.SignInsEarliestRetainedAt)
		}),
	).Return(adminTimeseriesFixture(), nil)
	srv := newAdminTestServer(adminDashboardConfig("1.0.0"), &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET",
		"/api/v1/admin/dashboard/timeseries?from="+from.Format(time.RFC3339)+
			"&to="+to.Format(time.RFC3339)+"&granularity=month", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

// TestGetAdminDashboardTimeseries_InvalidGranularityReturns400 pins the
// acceptance criterion. The generated binder does NOT enforce the enum, so
// without the explicit check this would 200 with silently-defaulted buckets.
// No service call is expected — the mock has no expectations.
func TestGetAdminDashboardTimeseries_InvalidGranularityReturns400(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"unknown value", "/api/v1/admin/dashboard/timeseries?granularity=fortnight"},
		{"injection-shaped", "/api/v1/admin/dashboard/timeseries?granularity=day%27%29+OR+1%3D1--"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(adminDashboardConfig(""), &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", tc.path, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestGetAdminDashboardTimeseries_RangeErrorReturns400 proves a range rejection
// raised by the SERVICE (not the enum check) is also a 400, never a 500.
func TestGetAdminDashboardTimeseries_RangeErrorReturns400(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetDashboardTimeseries", mock.Anything, mock.Anything, mock.Anything).
		Return(models.AdminTimeseries{}, &services.ErrAdminTimeseriesRange{Detail: "to must be after from"})
	srv := newAdminTestServer(adminDashboardConfig(""), &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/dashboard/timeseries", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "to must be after from")
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestGetAdminDashboardTimeseries_ServiceError maps a genuine failure to 500.
func TestGetAdminDashboardTimeseries_ServiceError(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetDashboardTimeseries", mock.Anything, mock.Anything, mock.Anything).
		Return(models.AdminTimeseries{}, errors.New("db down"))
	srv := newAdminTestServer(adminDashboardConfig(""), &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/dashboard/timeseries", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestGetAdminDashboardTimeseries_EmptySeriesSerializeAsBrackets covers #125 for
// the three new required series arrays.
func TestGetAdminDashboardTimeseries_EmptySeriesSerializeAsBrackets(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetDashboardTimeseries", mock.Anything, mock.Anything, mock.Anything).
		Return(models.AdminTimeseries{Granularity: "day"}, nil)
	srv := newAdminTestServer(adminDashboardConfig(""), &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/dashboard/timeseries", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	for _, field := range []string{"growth", "sign_ins", "access_by_source"} {
		assert.NotContains(t, body, `"`+field+`":null`)
		assert.Contains(t, body, `"`+field+`":[]`)
	}
}

// TestAdminDashboardRoutes_NonAdminGets404 locks the gating contract for the two
// new operations: they must be invisible to non-admins like the rest of the
// surface, not 401/403.
func TestAdminDashboardRoutes_NonAdminGets404(t *testing.T) {
	for _, path := range []string{
		"/api/v1/admin/dashboard/overview",
		"/api/v1/admin/dashboard/timeseries",
	} {
		t.Run(path, func(t *testing.T) {
			cfg := adminDashboardConfig("")
			srv := newAdminTestServer(cfg, &adminMockContainer{
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
