package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

func TestFreshnessMetricsRangeDays(t *testing.T) {
	tests := []struct {
		in       string
		wantDays int
		wantOK   bool
	}{
		{in: "7d", wantDays: 7, wantOK: true},
		{in: "14d", wantDays: 14, wantOK: true},
		{in: "30d", wantDays: 30, wantOK: true},
		{in: "60d", wantDays: 60, wantOK: true},
		{in: "90d", wantDays: 90, wantOK: true},
		{in: "180d", wantDays: 180, wantOK: true},
		{in: "", wantDays: 30, wantOK: true},
		{in: "1y"},
		{in: "30"},
		{in: "30D"},
	}

	for _, tt := range tests {
		t.Run("range "+tt.in, func(t *testing.T) {
			days, ok := services.FreshnessMetricsRangeDays(tt.in)

			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantDays, days)
			}
		})
	}
}

// The over-time series is the only metric with real arithmetic in it: flows
// come from the audit log, and the daily LEVEL is reconstructed by walking
// today's live count backwards through those flows.
func TestFreshnessService_GetOverTimeMetrics_ReconstructsTheLevel(t *testing.T) {
	svc, deps := newFreshnessService(t)

	// Two days back, so the window is three inclusive days. The middle day
	// marked 3 and cleared 1; today marked 1. With 6 stale right now that
	// makes the levels 3 → 5 → 6 reading forwards.
	today := time.Now().UTC()
	day := func(offset int) string {
		return today.AddDate(0, 0, offset).Format("2006-01-02")
	}
	deps.audit.EXPECT().
		CountTransitionsByDay(mock.Anything, freshnessTestTeamID, mock.Anything).
		Return([]models.FreshnessTransitionCount{
			{Date: day(-1), Action: models.FreshnessActionMarked, Count: 3},
			{Date: day(-1), Action: models.FreshnessActionCleared, Count: 1},
			{Date: day(0), Action: models.FreshnessActionMarked, Count: 1},
		}, nil).Once()
	deps.freshness.EXPECT().CountStale(mock.Anything, freshnessTestTeamID).Return(6, nil).Once()

	metrics, err := svc.GetOverTimeMetrics(context.Background(), freshnessTestTeamID, 2)

	require.NoError(t, err)
	require.Len(t, metrics.Days, 3, "an N-day range is inclusive of both ends, so N+1 points")
	assert.Equal(t, 4, metrics.TotalMarked)
	assert.Equal(t, 1, metrics.TotalCleared)

	assert.Equal(t, []models.FreshnessDailyStale{
		{Date: day(-2), Marked: 0, Cleared: 0, StaleTotal: 3},
		{Date: day(-1), Marked: 3, Cleared: 1, StaleTotal: 5},
		{Date: day(0), Marked: 1, Cleared: 0, StaleTotal: 6},
	}, metrics.Days)
}

// Every day in the window must be present even when nothing happened, or the
// chart draws gaps where quiet days should be.
func TestFreshnessService_GetOverTimeMetrics_ZeroFillsQuietDays(t *testing.T) {
	svc, deps := newFreshnessService(t)

	deps.audit.EXPECT().
		CountTransitionsByDay(mock.Anything, freshnessTestTeamID, mock.Anything).
		Return([]models.FreshnessTransitionCount{}, nil).Once()
	deps.freshness.EXPECT().CountStale(mock.Anything, freshnessTestTeamID).Return(0, nil).Once()

	metrics, err := svc.GetOverTimeMetrics(context.Background(), freshnessTestTeamID, 7)

	require.NoError(t, err)
	require.Len(t, metrics.Days, 8)
	for _, day := range metrics.Days {
		assert.Zero(t, day.Marked)
		assert.Zero(t, day.Cleared)
		assert.Zero(t, day.StaleTotal)
	}
}

// The backwards walk must never produce a negative level: rows can leave
// resource_freshness through a team or project cascade without an audit entry,
// so the recorded flows can imply more clears than ever happened.
func TestFreshnessService_GetOverTimeMetrics_ClampsAtZero(t *testing.T) {
	svc, deps := newFreshnessService(t)

	today := time.Now().UTC().Format("2006-01-02")
	deps.audit.EXPECT().
		CountTransitionsByDay(mock.Anything, freshnessTestTeamID, mock.Anything).
		Return([]models.FreshnessTransitionCount{
			{Date: today, Action: models.FreshnessActionMarked, Count: 50},
		}, nil).Once()
	deps.freshness.EXPECT().CountStale(mock.Anything, freshnessTestTeamID).Return(1, nil).Once()

	metrics, err := svc.GetOverTimeMetrics(context.Background(), freshnessTestTeamID, 1)

	require.NoError(t, err)
	for _, day := range metrics.Days {
		assert.GreaterOrEqual(t, day.StaleTotal, 0)
	}
}

// All four types are always reported: the repository can only return the ones
// with something stale, and a missing bar reads as "no data" rather than "none".
func TestFreshnessService_GetByTypeMetrics_ZeroFillsEveryType(t *testing.T) {
	svc, deps := newFreshnessService(t)

	deps.freshness.EXPECT().CountStaleByType(mock.Anything, freshnessTestTeamID).
		Return([]models.FreshnessBucketCount{{Key: "artifact", Count: 4}}, nil).Once()

	metrics, err := svc.GetByTypeMetrics(context.Background(), freshnessTestTeamID)

	require.NoError(t, err)
	assert.Equal(t, 4, metrics.TotalStale)
	assert.Equal(t, []models.FreshnessBucketCount{
		{Key: "artifact", Count: 4},
		{Key: "prompt", Count: 0},
		{Key: "blueprint", Count: 0},
		{Key: "memory", Count: 0},
	}, metrics.Counts)
}

// Projects with nothing stale are listed with 0 — "this project is clean" has
// to be distinguishable from "this project was not returned".
func TestFreshnessService_GetByProjectMetrics_IncludesCleanProjects(t *testing.T) {
	svc, deps := newFreshnessService(t)

	deps.freshness.EXPECT().CountStaleByProject(mock.Anything, freshnessTestTeamID).
		Return([]models.FreshnessBucketCount{{Key: "project-b", Count: 5}}, nil).Once()
	deps.projects.EXPECT().ListByTeamID(mock.Anything, freshnessTestTeamID).
		Return([]models.Project{
			{ID: "project-a", Name: "Alpha", Slug: "alpha"},
			{ID: "project-b", Name: "Beta", Slug: "beta"},
			{ID: "project-c", Name: "Gamma", Slug: "gamma"},
		}, nil).Once()

	metrics, err := svc.GetByProjectMetrics(context.Background(), freshnessTestTeamID)

	require.NoError(t, err)
	assert.Equal(t, 5, metrics.TotalStale)
	// Busiest first, then by name so the order is stable between requests.
	assert.Equal(t, []models.FreshnessProjectStale{
		{ProjectID: "project-b", Name: "Beta", Slug: "beta", Count: 5},
		{ProjectID: "project-a", Name: "Alpha", Slug: "alpha", Count: 0},
		{ProjectID: "project-c", Name: "Gamma", Slug: "gamma", Count: 0},
	}, metrics.Counts)
}

// A disabled rule reports 0 rather than vanishing, and the total counts
// DISTINCT resources — not the sum of the per-rule counts, which double-counts
// anything two rules both match.
func TestFreshnessService_GetByRuleMetrics_ListsEveryRuleAndCountsDistinct(t *testing.T) {
	svc, deps := newFreshnessService(t)

	projectID := freshnessTestProjectID
	deps.freshness.EXPECT().CountStaleByRule(mock.Anything, freshnessTestTeamID).
		Return([]models.FreshnessBucketCount{
			{Key: "rule-1", Count: 6},
			{Key: "rule-2", Count: 4},
		}, nil).Once()
	deps.rules.EXPECT().ListByTeam(mock.Anything, freshnessTestTeamID, false).
		Return([]*models.FreshnessRule{
			{ID: "rule-1", ProjectID: &projectID, ResourceTypes: []string{"artifact"}, ThresholdDays: 90, Enabled: true},
			{ID: "rule-2", ResourceTypes: []string{"prompt"}, ThresholdDays: 30, Enabled: true},
			{ID: "rule-3", ResourceTypes: []string{"memory"}, ThresholdDays: 10, Enabled: false},
		}, nil).Once()
	// Seven distinct resources although the per-rule counts sum to ten: three
	// resources match both rules.
	deps.freshness.EXPECT().CountStale(mock.Anything, freshnessTestTeamID).Return(7, nil).Once()

	metrics, err := svc.GetByRuleMetrics(context.Background(), freshnessTestTeamID)

	require.NoError(t, err)
	assert.Equal(t, 7, metrics.TotalStale, "distinct resources, not the sum of the rule counts")
	require.Len(t, metrics.Counts, 3)
	assert.Equal(t, "rule-1", metrics.Counts[0].RuleID)
	assert.Equal(t, 6, metrics.Counts[0].Count)
	assert.Equal(t, "rule-3", metrics.Counts[2].RuleID)
	assert.Equal(t, 0, metrics.Counts[2].Count)
	assert.False(t, metrics.Counts[2].Enabled)
}

// Page and limit become the repository's limit/offset. Getting this arithmetic
// wrong silently returns the wrong slice of the log rather than failing.
func TestFreshnessService_ListAudit_TranslatesPagingToOffset(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		limit      int
		wantLimit  int
		wantOffset int
	}{
		{name: "first page", page: 1, limit: 20, wantLimit: 20, wantOffset: 0},
		{name: "third page", page: 3, limit: 20, wantLimit: 20, wantOffset: 40},
		{name: "page below one is clamped", page: 0, limit: 20, wantLimit: 20, wantOffset: 0},
		{name: "limit below one falls back to the default", page: 1, limit: 0, wantLimit: 20, wantOffset: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, deps := newFreshnessService(t)
			deps.audit.EXPECT().
				ListByTeam(mock.Anything, freshnessTestTeamID, tt.wantLimit, tt.wantOffset).
				Return([]*models.ResourceFreshnessAudit{}, 0, nil).Once()

			page, err := svc.ListAudit(context.Background(), freshnessTestTeamID, tt.page, tt.limit)

			require.NoError(t, err)
			assert.Equal(t, tt.wantLimit, page.PerPage)
			assert.NotNil(t, page.Entries)
		})
	}
}

func TestFreshnessService_Metrics_PropagateRepositoryErrors(t *testing.T) {
	failure := errors.New("boom")

	tests := []struct {
		name  string
		setup func(deps freshnessDeps)
		call  func(svc *services.FreshnessService) error
	}{
		{
			name: "over-time transitions",
			setup: func(deps freshnessDeps) {
				deps.audit.EXPECT().CountTransitionsByDay(mock.Anything, mock.Anything, mock.Anything).
					Return(nil, failure).Once()
			},
			call: func(svc *services.FreshnessService) error {
				_, err := svc.GetOverTimeMetrics(context.Background(), freshnessTestTeamID, 7)
				return err
			},
		},
		{
			name: "by type",
			setup: func(deps freshnessDeps) {
				deps.freshness.EXPECT().CountStaleByType(mock.Anything, mock.Anything).
					Return(nil, failure).Once()
			},
			call: func(svc *services.FreshnessService) error {
				_, err := svc.GetByTypeMetrics(context.Background(), freshnessTestTeamID)
				return err
			},
		},
		{
			name: "by project",
			setup: func(deps freshnessDeps) {
				deps.freshness.EXPECT().CountStaleByProject(mock.Anything, mock.Anything).
					Return(nil, failure).Once()
			},
			call: func(svc *services.FreshnessService) error {
				_, err := svc.GetByProjectMetrics(context.Background(), freshnessTestTeamID)
				return err
			},
		},
		{
			name: "by project listing",
			setup: func(deps freshnessDeps) {
				deps.freshness.EXPECT().CountStaleByProject(mock.Anything, mock.Anything).
					Return([]models.FreshnessBucketCount{}, nil).Once()
				deps.projects.EXPECT().ListByTeamID(mock.Anything, mock.Anything).
					Return(nil, failure).Once()
			},
			call: func(svc *services.FreshnessService) error {
				_, err := svc.GetByProjectMetrics(context.Background(), freshnessTestTeamID)
				return err
			},
		},
		{
			name: "by rule",
			setup: func(deps freshnessDeps) {
				deps.freshness.EXPECT().CountStaleByRule(mock.Anything, mock.Anything).
					Return(nil, failure).Once()
			},
			call: func(svc *services.FreshnessService) error {
				_, err := svc.GetByRuleMetrics(context.Background(), freshnessTestTeamID)
				return err
			},
		},
		{
			name: "audit list",
			setup: func(deps freshnessDeps) {
				deps.audit.EXPECT().ListByTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, 0, failure).Once()
			},
			call: func(svc *services.FreshnessService) error {
				_, err := svc.ListAudit(context.Background(), freshnessTestTeamID, 1, 20)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, deps := newFreshnessService(t)
			tt.setup(deps)

			err := tt.call(svc)

			require.Error(t, err)
			assert.ErrorIs(t, err, failure)
		})
	}
}

// Reads must not consult authz — every member may see what the engine did. The
// authz mock has no expectation, so any call fails these tests.
func TestFreshnessService_MetricsReadsDoNotCheckPermissions(t *testing.T) {
	svc, deps := newFreshnessService(t)

	deps.freshness.EXPECT().CountStaleByType(mock.Anything, freshnessTestTeamID).
		Return([]models.FreshnessBucketCount{}, nil).Once()

	_, err := svc.GetByTypeMetrics(context.Background(), freshnessTestTeamID)
	require.NoError(t, err)
}
