package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	freshnessOverTimePath = "/api/v1/" + testFreshnessTeamID + "/freshness/metrics/over-time"
	freshnessByTypePath   = "/api/v1/" + testFreshnessTeamID + "/freshness/metrics/by-type"
	freshnessByProject    = "/api/v1/" + testFreshnessTeamID + "/freshness/metrics/by-project"
	freshnessByRulePath   = "/api/v1/" + testFreshnessTeamID + "/freshness/metrics/by-rule"
	freshnessAuditPath    = "/api/v1/" + testFreshnessTeamID + "/freshness/audit"
)

// doFreshnessGet runs one GET through the real router and asserts the response
// conforms to the spec before anything else is examined — the shape gate every
// documented operation has to pass.
func doFreshnessGet(t *testing.T, svc *servicesmocks.MockFreshnessServiceInterface, path string) map[string]any {
	t.Helper()

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodGet, path, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// envelopeData unwraps the {status, message, data} envelope the metrics
// endpoints share with the other analytics handlers.
func envelopeData(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	assert.Equal(t, "success", resp["status"])
	assert.NotEmpty(t, resp["message"])
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "response has no data object: %v", resp)
	return data
}

func sampleOverTimeMetrics() *models.FreshnessOverTimeMetrics {
	return &models.FreshnessOverTimeMetrics{
		RangeDays:    2,
		TotalMarked:  5,
		TotalCleared: 2,
		Days: []models.FreshnessDailyStale{
			{Date: "2026-05-01", Marked: 3, Cleared: 0, StaleTotal: 3},
			{Date: "2026-05-02", Marked: 2, Cleared: 1, StaleTotal: 4},
			{Date: "2026-05-03", Marked: 0, Cleared: 1, StaleTotal: 3},
		},
	}
}

func TestGetFreshnessOverTimeMetrics_ReturnsSeries(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	// 30 is the documented default window, applied when range is omitted.
	svc.EXPECT().GetOverTimeMetrics(mock.Anything, testFreshnessTeamID, 30).
		Return(sampleOverTimeMetrics(), nil)

	data := envelopeData(t, doFreshnessGet(t, svc, freshnessOverTimePath))

	assert.Equal(t, "30d", data["range"], "the response echoes the window actually used")
	assert.InDelta(t, 5, data["total_marked"], 0)
	assert.InDelta(t, 2, data["total_cleared"], 0)

	counts, ok := data["counts"].([]any)
	require.True(t, ok)
	require.Len(t, counts, 3)
	first, ok := counts[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2026-05-01", first["date"])
	assert.InDelta(t, 3, first["marked"], 0)
	assert.InDelta(t, 3, first["stale_total"], 0)
	// `total` is the day's ACTIVITY (marked + cleared) — the field the shared
	// chart reads for its per-day total. Without it the tooltip renders
	// "Total: undefined", which is what every other daily payload avoids by
	// carrying the same field.
	assert.InDelta(t, 3, first["total"], 0)

	middle, ok := counts[1].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 3, middle["total"], 0, "2 marked + 1 cleared")
}

// An explicit range must reach the service as the matching number of days —
// otherwise every chart would silently show 30 days whatever the selector said.
func TestGetFreshnessOverTimeMetrics_HonoursTheRange(t *testing.T) {
	for rangeStr, wantDays := range map[string]int{"7d": 7, "14d": 14, "60d": 60, "90d": 90, "180d": 180} {
		t.Run(rangeStr, func(t *testing.T) {
			svc := servicesmocks.NewMockFreshnessServiceInterface(t)
			svc.EXPECT().GetOverTimeMetrics(mock.Anything, testFreshnessTeamID, wantDays).
				Return(&models.FreshnessOverTimeMetrics{RangeDays: wantDays}, nil)

			data := envelopeData(t, doFreshnessGet(t, svc, freshnessOverTimePath+"?range="+rangeStr))

			assert.Equal(t, rangeStr, data["range"])
			assert.Equal(t, []any{}, data["counts"], "an empty series is [] and never null")
		})
	}
}

// A range outside the enum must be a 400. oapi-codegen does NOT validate a
// query param's enum, so the handler's own table is the only thing enforcing
// it — the mock has no expectation, so a call through to the service fails
// this test.
func TestGetFreshnessOverTimeMetrics_RejectsAnUnknownRange(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	srv := createTestFreshnessServer(svc)

	req := makeFreshnessRequest(http.MethodGet, freshnessOverTimePath+"?range=1y", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetFreshnessByTypeMetrics_ReturnsEveryType(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().GetByTypeMetrics(mock.Anything, testFreshnessTeamID).
		Return(&models.FreshnessTypeMetrics{
			TotalStale: 7,
			Counts: []models.FreshnessBucketCount{
				{Key: "artifact", Count: 4},
				{Key: "prompt", Count: 3},
				{Key: "blueprint", Count: 0},
				{Key: "memory", Count: 0},
			},
		}, nil)

	data := envelopeData(t, doFreshnessGet(t, svc, freshnessByTypePath))

	assert.InDelta(t, 7, data["total_stale"], 0)
	counts, ok := data["counts"].([]any)
	require.True(t, ok)
	require.Len(t, counts, 4, "a type with nothing stale must still report 0")
}

func TestGetFreshnessByProjectMetrics_CarriesNameAndSlug(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().GetByProjectMetrics(mock.Anything, testFreshnessTeamID).
		Return(&models.FreshnessProjectMetrics{
			TotalStale: 4,
			Counts: []models.FreshnessProjectStale{
				{ProjectID: testFreshnessProjectID, Name: "Platform", Slug: "platform", Count: 4},
			},
		}, nil)

	data := envelopeData(t, doFreshnessGet(t, svc, freshnessByProject))

	counts, ok := data["counts"].([]any)
	require.True(t, ok)
	require.Len(t, counts, 1)
	project, ok := counts[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, testFreshnessProjectID, project["project_id"])
	assert.Equal(t, "Platform", project["name"])
	assert.Equal(t, "platform", project["slug"], "the slug is what the client deep-links on")
	assert.InDelta(t, 4, project["count"], 0)
}

func TestGetFreshnessByRuleMetrics_ReturnsRuleShapeAndCount(t *testing.T) {
	projectID := testFreshnessProjectID
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().GetByRuleMetrics(mock.Anything, testFreshnessTeamID).
		Return(&models.FreshnessRuleMetrics{
			TotalStale: 6,
			Counts: []models.FreshnessRuleImpact{
				{
					RuleID: testFreshnessRuleID, ProjectID: &projectID,
					ResourceTypes: []string{"artifact"}, ThresholdDays: 90, Enabled: true, Count: 6,
				},
				{
					RuleID: "990e8400-e29b-41d4-a716-446655440004", ProjectID: nil,
					ResourceTypes: []string{"prompt", "memory"}, ThresholdDays: 30, Enabled: false, Count: 0,
				},
			},
		}, nil)

	data := envelopeData(t, doFreshnessGet(t, svc, freshnessByRulePath))

	assert.InDelta(t, 6, data["total_stale"], 0)
	counts, ok := data["counts"].([]any)
	require.True(t, ok)
	require.Len(t, counts, 2, "a rule matching nothing is still listed")

	disabled, ok := counts[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, disabled["enabled"])
	assert.InDelta(t, 0, disabled["count"], 0)
	assert.Nil(t, disabled["project_id"], "an any-project rule serializes project_id as null")
	assert.Equal(t, []any{"prompt", "memory"}, disabled["resource_types"])
}

const testFreshnessAuditProjectID = "cc0e8400-e29b-41d4-a716-446655440007"

func sampleAuditEntry(ruleID *string, action, reason string) *models.ResourceFreshnessAudit {
	slug := "my-artifact"
	projectID := testFreshnessAuditProjectID
	return &models.ResourceFreshnessAudit{
		ID:           "aa0e8400-e29b-41d4-a716-446655440005",
		TeamID:       testFreshnessTeamID,
		ResourceType: "artifact",
		ResourceID:   "bb0e8400-e29b-41d4-a716-446655440006",
		RuleID:       ruleID,
		Action:       action,
		Reason:       reason,
		Slug:         &slug,
		ProjectID:    &projectID,
		CreatedAt:    time.Now().UTC(),
	}
}

func TestListFreshnessAudit_ReturnsAPage(t *testing.T) {
	ruleID := testFreshnessRuleID
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	// Defaults applied when the paging params are omitted.
	svc.EXPECT().ListAudit(mock.Anything, testFreshnessTeamID, 1, 20).
		Return(&models.FreshnessAuditPage{
			Entries: []*models.ResourceFreshnessAudit{
				sampleAuditEntry(&ruleID, models.FreshnessActionMarked, models.FreshnessReasonRuleRun),
				sampleAuditEntry(nil, models.FreshnessActionCleared, models.FreshnessReasonAccessed),
			},
			TotalCount: 42,
			Page:       1,
			PerPage:    20,
		}, nil)

	resp := doFreshnessGet(t, svc, freshnessAuditPath)

	assert.InDelta(t, 42, resp["total_count"], 0)
	assert.InDelta(t, 1, resp["page"], 0)
	assert.InDelta(t, 20, resp["per_page"], 0)
	assert.InDelta(t, 3, resp["total_pages"], 0, "42 entries at 20 per page is 3 pages")

	entries, ok := resp["entries"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 2)

	marked, ok := entries[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "marked", marked["action"])
	assert.Equal(t, "rule_run", marked["reason"])
	assert.Equal(t, testFreshnessRuleID, marked["rule_id"])

	cleared, ok := entries[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "cleared", cleared["action"])
	assert.Equal(t, "accessed", cleared["reason"])
	assert.Nil(t, cleared["rule_id"], "a reversal is not attributable to a rule")

	// AssertConformsToSpec cannot catch an omitted optional field, so the
	// deep-link identifiers need explicit value assertions (#789).
	assert.Equal(t, "my-artifact", marked["slug"])
	assert.Equal(t, testFreshnessAuditProjectID, marked["project_id"])
}

// A resource deleted after its event was logged resolves to no slug and no
// project, and both keys must be absent rather than an empty string or a zero
// UUID — either would make the client render a link that 404s.
func TestListFreshnessAudit_DeletedResourceOmitsDeepLinkFields(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	entry := sampleAuditEntry(nil, models.FreshnessActionMarked, models.FreshnessReasonRuleRun)
	entry.Slug = nil
	entry.ProjectID = nil
	svc.EXPECT().ListAudit(mock.Anything, testFreshnessTeamID, 1, 20).
		Return(&models.FreshnessAuditPage{
			Entries:    []*models.ResourceFreshnessAudit{entry},
			TotalCount: 1,
			Page:       1,
			PerPage:    20,
		}, nil)

	resp := doFreshnessGet(t, svc, freshnessAuditPath)

	entries, ok := resp["entries"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 1)
	row, ok := entries[0].(map[string]any)
	require.True(t, ok)

	_, hasSlug := row["slug"]
	assert.False(t, hasSlug, "a deleted resource omits slug rather than sending an empty one")
	_, hasProject := row["project_id"]
	assert.False(t, hasProject, "a deleted resource omits project_id rather than sending a zero UUID")
}

// A project_id that is not a UUID is a converter bug, not a client error, and
// must surface as a failure rather than being silently dropped from the payload.
func TestListFreshnessAudit_InvalidProjectIDIsAnError(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	entry := sampleAuditEntry(nil, models.FreshnessActionMarked, models.FreshnessReasonRuleRun)
	bad := "not-a-uuid"
	entry.ProjectID = &bad
	svc.EXPECT().ListAudit(mock.Anything, testFreshnessTeamID, 1, 20).
		Return(&models.FreshnessAuditPage{
			Entries:    []*models.ResourceFreshnessAudit{entry},
			TotalCount: 1,
			Page:       1,
			PerPage:    20,
		}, nil)

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodGet, freshnessAuditPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "body: %s", w.Body.String())
}

// Explicit paging must reach the service unchanged, and a page past the end
// must still be a well-formed empty page rather than an error.
func TestListFreshnessAudit_PagingAndEmptyLastPage(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().ListAudit(mock.Anything, testFreshnessTeamID, 9, 5).
		Return(&models.FreshnessAuditPage{
			Entries:    []*models.ResourceFreshnessAudit{},
			TotalCount: 12,
			Page:       9,
			PerPage:    5,
		}, nil)

	resp := doFreshnessGet(t, svc, freshnessAuditPath+"?page=9&limit=5")

	assert.Equal(t, []any{}, resp["entries"], "an empty page is [] and never null")
	assert.InDelta(t, 3, resp["total_pages"], 0)
}

// An empty log still reports one page, so a client never renders "page 1 of 0".
func TestListFreshnessAudit_EmptyLogIsOnePage(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().ListAudit(mock.Anything, testFreshnessTeamID, 1, 20).
		Return(&models.FreshnessAuditPage{
			Entries: []*models.ResourceFreshnessAudit{}, TotalCount: 0, Page: 1, PerPage: 20,
		}, nil)

	resp := doFreshnessGet(t, svc, freshnessAuditPath)

	assert.InDelta(t, 0, resp["total_count"], 0)
	assert.InDelta(t, 1, resp["total_pages"], 0)
}

// Paging outside the spec's bounds must be a 400. oapi-codegen enforces
// neither `minimum` nor `maximum` on a query param, so — apart from a
// non-integer, which fails to bind — every one of these is caught by the
// handler's own check.
func TestListFreshnessAudit_RejectsInvalidPaging(t *testing.T) {
	for _, query := range []string{
		"?page=0", "?limit=0", "?limit=101", "?page=abc",
		// A page whose offset would overflow int: the repository clamps a
		// negative offset to zero, so without this check the response would
		// be page 1 wearing the requested page number.
		"?page=100000000000000000&limit=100",
	} {
		t.Run(query, func(t *testing.T) {
			svc := servicesmocks.NewMockFreshnessServiceInterface(t)
			srv := createTestFreshnessServer(svc)

			req := makeFreshnessRequest(http.MethodGet, freshnessAuditPath+query, "")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
		})
	}
}

// A stored id that is not a UUID cannot be rendered into a spec-typed response.
// It means the database holds something the schema says is impossible, so the
// honest answer is a 500 — not a half-built payload that fails conformance at
// the client instead.
func TestFreshnessMetrics_MalformedStoredIDsAre500(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(svc *servicesmocks.MockFreshnessServiceInterface)
	}{
		{
			name: "project id",
			path: freshnessByProject,
			call: func(svc *servicesmocks.MockFreshnessServiceInterface) {
				svc.EXPECT().GetByProjectMetrics(mock.Anything, testFreshnessTeamID).
					Return(&models.FreshnessProjectMetrics{
						Counts: []models.FreshnessProjectStale{{ProjectID: "not-a-uuid", Name: "X", Slug: "x"}},
					}, nil)
			},
		},
		{
			name: "rule id",
			path: freshnessByRulePath,
			call: func(svc *servicesmocks.MockFreshnessServiceInterface) {
				svc.EXPECT().GetByRuleMetrics(mock.Anything, testFreshnessTeamID).
					Return(&models.FreshnessRuleMetrics{
						Counts: []models.FreshnessRuleImpact{{RuleID: "not-a-uuid"}},
					}, nil)
			},
		},
		{
			name: "audit entry id",
			path: freshnessAuditPath,
			call: func(svc *servicesmocks.MockFreshnessServiceInterface) {
				svc.EXPECT().ListAudit(mock.Anything, testFreshnessTeamID, 1, 20).
					Return(&models.FreshnessAuditPage{
						Entries: []*models.ResourceFreshnessAudit{{ID: "not-a-uuid"}},
						Page:    1, PerPage: 20,
					}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicesmocks.NewMockFreshnessServiceInterface(t)
			tt.call(svc)

			srv := createTestFreshnessServer(svc)
			req := makeFreshnessRequest(http.MethodGet, tt.path, "")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusInternalServerError, w.Code, "body: %s", w.Body.String())
		})
	}
}

// A service error must surface as a 500 rather than an empty 200 chart.
func TestFreshnessMetrics_ServiceErrorsAre500(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().GetOverTimeMetrics(mock.Anything, testFreshnessTeamID, 30).
		Return(nil, assert.AnError)

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodGet, freshnessOverTimePath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
