package services_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
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

// The payload-facing reads (#735). They are what every resource list and detail
// response goes through, and the handler tests mock this service — so without
// these the real filtering never executes.

func staleRow(teamID string) *models.ResourceFreshness {
	return &models.ResourceFreshness{
		TeamID:         teamID,
		ProjectID:      freshnessTestProjectID,
		ResourceType:   "artifact",
		ResourceID:     "art-1",
		Status:         models.FreshnessStatusStale,
		MatchedRuleIDs: []string{freshnessTestRuleID},
		Since:          time.Now().UTC().Add(-48 * time.Hour),
		Reason:         models.FreshnessReasonRuleRun,
	}
}

func TestFreshnessService_GetResourceFreshness(t *testing.T) {
	tests := []struct {
		name    string
		row     *models.ResourceFreshness
		wantNil bool
		wantWhy string
	}{
		{name: "stale resource is projected", row: staleRow(freshnessTestTeamID)},
		{name: "fresh resource yields nil", row: nil, wantNil: true,
			wantWhy: "absence in the table IS freshness"},
		{name: "another team's row is not disclosed", row: staleRow("someone-else"), wantNil: true,
			wantWhy: "the table has no team predicate, so this Go check is the only tenancy gate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, deps := newFreshnessService(t)
			deps.freshness.EXPECT().GetByResource(mock.Anything, "artifact", "art-1").
				Return(tt.row, nil).Once()

			state, err := svc.GetResourceFreshness(
				context.Background(), freshnessTestTeamID, "artifact", "art-1")

			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, state, tt.wantWhy)
				return
			}
			require.NotNil(t, state)
			assert.Equal(t, models.FreshnessStatusStale, state.Status)
			assert.Equal(t, models.FreshnessReasonRuleRun, state.Reason)
			assert.Equal(t, tt.row.Since, state.Since)
			assert.Equal(t, []string{freshnessTestRuleID}, []string(state.MatchedRuleIDs))
		})
	}
}

func TestFreshnessService_ListResourceFreshness(t *testing.T) {
	svc, deps := newFreshnessService(t)

	mine := staleRow(freshnessTestTeamID)
	theirs := staleRow("someone-else")
	theirs.ResourceID = "art-2"
	deps.freshness.EXPECT().
		ListByResources(mock.Anything, "artifact", []string{"art-1", "art-2", "art-3"}).
		Return(map[string]*models.ResourceFreshness{"art-1": mine, "art-2": theirs}, nil).Once()

	states, err := svc.ListResourceFreshness(
		context.Background(), freshnessTestTeamID, "artifact", []string{"art-1", "art-2", "art-3"})

	require.NoError(t, err)
	require.Len(t, states, 1, "another team's row must be dropped, not returned")
	require.Contains(t, states, "art-1")
	assert.Equal(t, models.FreshnessStatusStale, states["art-1"].Status)
	assert.NotContains(t, states, "art-2")
	assert.NotContains(t, states, "art-3", "a fresh resource is simply absent")
}

func TestFreshnessService_PayloadReads_PropagateErrors(t *testing.T) {
	failure := errors.New("boom")

	t.Run("get", func(t *testing.T) {
		svc, deps := newFreshnessService(t)
		deps.freshness.EXPECT().GetByResource(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, failure).Once()

		_, err := svc.GetResourceFreshness(context.Background(), freshnessTestTeamID, "artifact", "art-1")

		require.Error(t, err)
		assert.ErrorIs(t, err, failure)
	})

	t.Run("list", func(t *testing.T) {
		svc, deps := newFreshnessService(t)
		deps.freshness.EXPECT().ListByResources(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, failure).Once()

		_, err := svc.ListResourceFreshness(
			context.Background(), freshnessTestTeamID, "artifact", []string{"art-1"})

		require.Error(t, err)
		assert.ErrorIs(t, err, failure)
	})
}

// The repository filter is a pointer so "unset" is distinguishable from a
// value; only the one supported value produces a predicate. Anything else maps
// to nil (no filtering) rather than to a predicate matching nothing — handlers
// reject unknown values with a 400 first, and an empty list would look like an
// answer if one ever slipped through.
func TestFreshnessFilter(t *testing.T) {
	require.Nil(t, services.ExportedFreshnessFilter(""))
	require.Nil(t, services.ExportedFreshnessFilter("fresh"))
	require.Nil(t, services.ExportedFreshnessFilter("STALE"))

	got := services.ExportedFreshnessFilter(services.FreshnessFilterStale)
	require.NotNil(t, got)
	assert.Equal(t, services.FreshnessFilterStale, *got)
}

// The service→repository mapping of the stale filter (#735), for EVERY service.
//
// Without these, deleting `Freshness: freshnessFilter(filters.Freshness)` from
// a service's repo-filter literal passes every other test: the handler tests
// stop at the service boundary and the repository tests start below it, so the
// line that joins them is exactly the one nothing observes. That is one line
// per service — five call sites, since artifacts map twice.
func TestServices_ForwardTheStaleFilterToTheRepository(t *testing.T) {
	stale := services.FreshnessFilterStale

	// Each case asserts the repository saw the pointer the service should have
	// derived, for both the set and the unset filter.
	tests := []struct {
		name string
		call func(t *testing.T, filter string, wantSet bool)
	}{
		{
			name: "memory",
			call: func(t *testing.T, filter string, wantSet bool) {
				repo := repomocks.NewMockMemoryRepository(t)
				repo.EXPECT().List(mock.Anything, mock.Anything,
					mock.MatchedBy(func(f repositories.MemoryFilters) bool {
						return freshnessPointerMatches(f.Freshness, wantSet)
					})).Return([]models.Memory{}, 0, nil).Once()

				svc := services.NewMemoryService(repo, nil, permissiveFreshnessAuthz(t), nil,
					discardTestLogger(), nil, nil, nil, nil, nil)
				_, err := svc.ListMemories("user-1", services.MemoryFilters{
					TeamID: freshnessTestTeamID, Freshness: filter, Page: 1, Limit: 20,
				})
				require.NoError(t, err)
			},
		},
		{
			name: "prompt",
			call: func(t *testing.T, filter string, wantSet bool) {
				repo := repomocks.NewMockPromptRepository(t)
				repo.EXPECT().List(mock.Anything, mock.Anything,
					mock.MatchedBy(func(f repositories.PromptFilters) bool {
						return freshnessPointerMatches(f.Freshness, wantSet)
					})).Return([]models.Prompt{}, 0, nil).Once()

				svc := services.NewPromptService(services.PromptServiceDeps{
					Repo: repo, Authz: permissiveFreshnessAuthz(t), Logger: discardTestLogger(),
				})
				_, err := svc.ListPrompts("user-1", services.PromptFilters{
					TeamID: freshnessTestTeamID, Freshness: filter, Page: 1, Limit: 20,
				})
				require.NoError(t, err)
			},
		},
		{
			name: "blueprint",
			call: func(t *testing.T, filter string, wantSet bool) {
				repo := repomocks.NewMockBlueprintRepository(t)
				repo.EXPECT().List(mock.Anything, mock.Anything,
					mock.MatchedBy(func(f repositories.BlueprintFilters) bool {
						return freshnessPointerMatches(f.Freshness, wantSet)
					})).Return([]models.Blueprint{}, 0, nil).Once()

				svc := services.NewBlueprintService(services.BlueprintServiceDeps{
					Repo: repo, Authz: permissiveFreshnessAuthz(t), Logger: discardTestLogger(),
				})
				_, err := svc.ListBlueprints("user-1", services.BlueprintFilters{
					TeamID: freshnessTestTeamID, Freshness: filter, Page: 1, Limit: 20,
				})
				require.NoError(t, err)
			},
		},
		{
			name: "artifact",
			call: func(t *testing.T, filter string, wantSet bool) {
				repo := repomocks.NewMockArtifactRepository(t)
				repo.EXPECT().List(mock.Anything, mock.Anything,
					mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
						return freshnessPointerMatches(f.Freshness, wantSet)
					})).Return([]models.Artifact{}, 0, nil).Once()

				svc := services.NewArtifactService(services.ArtifactServiceDeps{
					Repo: repo, Authz: permissiveFreshnessAuthz(t), Logger: discardTestLogger(),
				})
				_, err := svc.ListArtifacts("user-1", services.ArtifactFilters{
					TeamID: freshnessTestTeamID, Freshness: filter, Page: 1, Limit: 20,
				})
				require.NoError(t, err)
			},
		},
		{
			name: "artifact cross-team",
			call: func(t *testing.T, filter string, wantSet bool) {
				repo := repomocks.NewMockArtifactRepository(t)
				repo.EXPECT().ListCrossTeam(mock.Anything, mock.Anything,
					mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
						return freshnessPointerMatches(f.Freshness, wantSet)
					})).Return([]models.Artifact{}, 0, nil).Once()

				svc := services.NewArtifactService(services.ArtifactServiceDeps{
					Repo: repo, Authz: permissiveFreshnessAuthz(t), Logger: discardTestLogger(),
				})
				_, err := svc.ListArtifactsByProjectCrossTeam("user-1", "project-1",
					services.ArtifactFilters{Freshness: filter, Page: 1, Limit: 20})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/stale forwards a predicate", func(t *testing.T) {
			tt.call(t, stale, true)
		})
		t.Run(tt.name+"/unset forwards nothing", func(t *testing.T) {
			tt.call(t, "", false)
		})
	}
}

// freshnessPointerMatches reports whether the repository filter carries the
// stale predicate exactly when it should.
func freshnessPointerMatches(got *string, wantSet bool) bool {
	if !wantSet {
		return got == nil
	}
	return got != nil && *got == services.FreshnessFilterStale
}

func permissiveFreshnessAuthz(t *testing.T) services.AuthorizationServiceInterface {
	t.Helper()
	authzMock := servicemocks.NewMockAuthorizationServiceInterface(t)
	authzMock.EXPECT().Can(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()
	return authzMock
}

func discardTestLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }
