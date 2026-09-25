package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func TestGetProjectCreationMetrics_GapFillsEveryTypeAndBucket(t *testing.T) {
	svc, repo := newInsightsService(t)
	from := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	d1, d2, d3 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)

	repo.On("ProjectTeamID", mock.Anything, "p").Return("t", true, nil)
	repo.On("GetProjectCreationSeries", mock.Anything, "p", d1, to, adminGranularityDay).
		Return([]models.AdminGrowthCount{
			{Entity: models.AdminResourceTypePrompt, Bucket: d1, Count: 2},
			{Entity: models.AdminResourceTypeMemory, Bucket: d1, Count: 1},
			{Entity: models.AdminResourceTypeArtifact, Bucket: d3, Count: 4},
			{Entity: models.AdminResourceTypeBlueprint, Bucket: d3, Count: 5},
			{Entity: models.AdminResourceTypeFeedItem, Bucket: d3, Count: 6},
			// A type that is not project-scoped is ignored rather than miscounted.
			{Entity: models.AdminResourceTypeAgent, Bucket: d3, Count: 9},
			// A bucket the Go side did not generate is dropped.
			{Entity: models.AdminResourceTypePrompt, Bucket: d3.Add(time.Hour), Count: 7},
		}, nil)

	got, err := svc.GetProjectCreationMetrics(context.Background(), "p", AdminTimeseriesQuery{From: &from, To: &to})
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, d1, got.From, "from snaps down to the bucket start")
	assert.Equal(t, to, got.To)
	assert.Equal(t, adminGranularityDay, got.Granularity)
	assert.Equal(t, []models.AdminProjectCreationPoint{
		{Bucket: d1, Prompts: 2, Memories: 1},
		{Bucket: d2},
		{Bucket: d3, Artifacts: 4, Blueprints: 5, FeedItems: 6},
	}, got.Series)
}

func TestGetProjectAccessMetrics_GapFillsAndScopesToTheProjectTeam(t *testing.T) {
	svc, repo := newInsightsService(t)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)

	repo.On("ProjectTeamID", mock.Anything, "p").Return("team-1", true, nil)
	repo.On("GetProjectAccessBySourceSeries", mock.Anything, "p", "team-1", from, to, adminGranularityDay).
		Return([]models.AdminSourcePoint{{Bucket: d2, Source: "cli", Count: 3}}, nil)

	got, err := svc.GetProjectAccessMetrics(context.Background(), "p", AdminTimeseriesQuery{From: &from, To: &to})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []models.AdminSourcePoint{
		{Bucket: from, Source: "cli", Count: 0}, {Bucket: d2, Source: "cli", Count: 3},
	}, got.AccessBySource)
}

func TestGetProjectTopAccessedResources(t *testing.T) {
	to := time.Date(2026, 9, 10, 15, 30, 0, 0, time.UTC)

	t.Run("defaults: 30-day unsnapped window and limit 10, scoped to the project team", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		wantFrom := to.AddDate(0, 0, -30)
		rows := []models.AdminTopAccessedResource{{ResourceType: "prompt", AccessCount: 3}}
		repo.On("ProjectTeamID", mock.Anything, "p").Return("team-1", true, nil)
		repo.On("GetProjectTopAccessedResources", mock.Anything, "p", "team-1", wantFrom, to,
			AdminTopResourcesDefaultLimit).Return(rows, nil)

		got, err := svc.GetProjectTopAccessedResources(context.Background(), "p", AdminTopResourcesQuery{To: &to})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, wantFrom, got.From)
		assert.Equal(t, to, got.To)
		assert.Equal(t, rows, got.Items)
	})

	for _, limit := range []int{-1, AdminTopResourcesMaxLimit + 1} {
		t.Run("limit out of range is ErrAdminTimeseriesRange before any query", func(t *testing.T) {
			svc, _ := newInsightsService(t)
			_, err := svc.GetProjectTopAccessedResources(context.Background(), "p", AdminTopResourcesQuery{Limit: limit})
			var rangeErr *ErrAdminTimeseriesRange
			require.ErrorAs(t, err, &rangeErr)
		})
	}

	t.Run("an inverted window is ErrAdminTimeseriesRange before any query", func(t *testing.T) {
		svc, _ := newInsightsService(t)
		from := to.Add(time.Hour)
		_, err := svc.GetProjectTopAccessedResources(context.Background(), "p",
			AdminTopResourcesQuery{From: &from, To: &to})
		var rangeErr *ErrAdminTimeseriesRange
		require.ErrorAs(t, err, &rangeErr)
	})

	t.Run("query error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("ProjectTeamID", mock.Anything, "p").Return("team-1", true, nil)
		repo.On("GetProjectTopAccessedResources", mock.Anything, "p", "team-1",
			mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("boom"))
		_, err := svc.GetProjectTopAccessedResources(context.Background(), "p", AdminTopResourcesQuery{})
		require.Error(t, err)
	})
}

// TestProjectAnalytics_SharedErrors covers what every ranged project op shares:
// a bad range fails before any query, an unknown project is (nil, nil), and a
// lookup or series error propagates.
func TestProjectAnalytics_SharedErrors(t *testing.T) {
	ops := map[string]struct {
		series string
		nargs  int // the series method's argument count, ctx included
		call   func(*AdminService, AdminTimeseriesQuery) (any, error)
	}{
		"creation": {"GetProjectCreationSeries", 5, func(s *AdminService, q AdminTimeseriesQuery) (any, error) {
			return s.GetProjectCreationMetrics(context.Background(), "p", q)
		}},
		"access": {"GetProjectAccessBySourceSeries", 6, func(s *AdminService, q AdminTimeseriesQuery) (any, error) {
			return s.GetProjectAccessMetrics(context.Background(), "p", q)
		}},
	}
	for name, op := range ops {
		t.Run(name+": invalid granularity", func(t *testing.T) {
			svc, _ := newInsightsService(t)
			_, err := op.call(svc, AdminTimeseriesQuery{Granularity: "hour"})
			var rangeErr *ErrAdminTimeseriesRange
			require.ErrorAs(t, err, &rangeErr)
		})
		t.Run(name+": unknown project", func(t *testing.T) {
			svc, repo := newInsightsService(t)
			repo.On("ProjectTeamID", mock.Anything, "p").Return("", false, nil)
			got, err := op.call(svc, AdminTimeseriesQuery{})
			require.NoError(t, err)
			assert.Nil(t, got)
		})
		t.Run(name+": lookup error", func(t *testing.T) {
			svc, repo := newInsightsService(t)
			repo.On("ProjectTeamID", mock.Anything, "p").Return("", false, errors.New("boom"))
			_, err := op.call(svc, AdminTimeseriesQuery{})
			require.Error(t, err)
		})
		t.Run(name+": series error", func(t *testing.T) {
			svc, repo := newInsightsService(t)
			repo.On("ProjectTeamID", mock.Anything, "p").Return("t", true, nil)
			args := make([]any, op.nargs)
			for i := range args {
				args[i] = mock.Anything
			}
			repo.On(op.series, args...).Return(nil, errors.New("boom"))
			_, err := op.call(svc, AdminTimeseriesQuery{})
			require.Error(t, err)
		})
	}

	t.Run("top: unknown project", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("ProjectTeamID", mock.Anything, "p").Return("", false, nil)
		got, err := svc.GetProjectTopAccessedResources(context.Background(), "p", AdminTopResourcesQuery{})
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

// fakeRuleLister is a FreshnessServiceInterface that only answers ListRules
// (the services mocks package imports services, so it cannot be used here).
// Any other method panics through the nil embedded interface.
type fakeRuleLister struct {
	FreshnessServiceInterface
	teamID string
	rules  []*models.FreshnessRule
	err    error
	calls  int
}

func (f *fakeRuleLister) ListRules(_ context.Context, teamID string) ([]*models.FreshnessRule, error) {
	f.calls++
	if teamID != f.teamID {
		return nil, errors.New("unexpected team " + teamID)
	}
	return f.rules, f.err
}

func TestGetProjectConfig_PartitionsRulesAcrossProjects(t *testing.T) {
	svc, repo := newInsightsService(t)
	thisProject, otherProject := "p", "other"
	teamWide := &models.FreshnessRule{ID: "r1"}
	own := &models.FreshnessRule{ID: "r2", ProjectID: &thisProject}
	foreign := &models.FreshnessRule{ID: "r3", ProjectID: &otherProject}
	ownDisabled := &models.FreshnessRule{ID: "r4", ProjectID: &thisProject, Enabled: false}
	teamWide2 := &models.FreshnessRule{ID: "r5"}

	repo.On("ProjectTeamID", mock.Anything, "p").Return("team-1", true, nil)
	svc.freshness = &fakeRuleLister{teamID: "team-1",
		rules: []*models.FreshnessRule{teamWide, own, foreign, ownDisabled, teamWide2}}

	got, err := svc.GetProjectConfig(context.Background(), "p")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []*models.FreshnessRule{own, ownDisabled}, got.ProjectRules,
		"own rules, disabled included, in ListRules order")
	assert.Equal(t, []*models.FreshnessRule{teamWide, teamWide2}, got.TeamWideRules)
}

func TestGetProjectConfig_EdgeCases(t *testing.T) {
	t.Run("a team with no rules yields two empty, non-nil lists", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		svc.freshness = &fakeRuleLister{teamID: "team-1", rules: []*models.FreshnessRule{}}
		repo.On("ProjectTeamID", mock.Anything, "p").Return("team-1", true, nil)

		got, err := svc.GetProjectConfig(context.Background(), "p")
		require.NoError(t, err)
		require.NotNil(t, got.ProjectRules)
		require.NotNil(t, got.TeamWideRules)
		assert.Empty(t, got.ProjectRules)
		assert.Empty(t, got.TeamWideRules)
	})
	t.Run("unknown project is (nil, nil) without listing rules", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		fresh := &fakeRuleLister{}
		svc.freshness = fresh
		repo.On("ProjectTeamID", mock.Anything, "p").Return("", false, nil)
		got, err := svc.GetProjectConfig(context.Background(), "p")
		require.NoError(t, err)
		assert.Nil(t, got)
		assert.Zero(t, fresh.calls)
	})
	t.Run("lookup error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("ProjectTeamID", mock.Anything, "p").Return("", false, errors.New("boom"))
		_, err := svc.GetProjectConfig(context.Background(), "p")
		require.Error(t, err)
	})
	t.Run("rules error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		svc.freshness = &fakeRuleLister{teamID: "team-1", err: errors.New("boom")}
		repo.On("ProjectTeamID", mock.Anything, "p").Return("team-1", true, nil)
		_, err := svc.GetProjectConfig(context.Background(), "p")
		require.Error(t, err)
	})
	t.Run("an unwired freshness service is an error, not a panic", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("ProjectTeamID", mock.Anything, "p").Return("team-1", true, nil)
		_, err := svc.GetProjectConfig(context.Background(), "p")
		require.ErrorIs(t, err, errAdminFreshnessUnwired)
	})
}
