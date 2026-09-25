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

func TestGetUserAccessMetrics_GapFillsEverySourceAndBucket(t *testing.T) {
	svc, repo := newInsightsService(t)
	from := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	d1, d2, d3 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)

	repo.On("UserExists", mock.Anything, "u").Return(true, nil)
	repo.On("GetUserAccessBySourceSeries", mock.Anything, "u", d1, to, adminGranularityDay).
		Return([]models.AdminSourcePoint{
			{Bucket: d1, Source: "web", Count: 2},
			{Bucket: d3, Source: "mcp", Count: 5},
		}, nil)

	got, err := svc.GetUserAccessMetrics(context.Background(), "u", AdminTimeseriesQuery{From: &from, To: &to})
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, d1, got.From, "from snaps down to the bucket start")
	assert.Equal(t, to, got.To)
	assert.Equal(t, adminGranularityDay, got.Granularity)
	assert.Equal(t, []models.AdminSourcePoint{
		{Bucket: d1, Source: "mcp", Count: 0}, {Bucket: d1, Source: "web", Count: 2},
		{Bucket: d2, Source: "mcp", Count: 0}, {Bucket: d2, Source: "web", Count: 0},
		{Bucket: d3, Source: "mcp", Count: 5}, {Bucket: d3, Source: "web", Count: 0},
	}, got.AccessBySource)
}

func TestGetUserAccessMetrics_Errors(t *testing.T) {
	t.Run("invalid granularity is ErrAdminTimeseriesRange before any query", func(t *testing.T) {
		svc, _ := newInsightsService(t)
		_, err := svc.GetUserAccessMetrics(context.Background(), "u", AdminTimeseriesQuery{Granularity: "hour"})
		var rangeErr *ErrAdminTimeseriesRange
		require.ErrorAs(t, err, &rangeErr)
	})
	t.Run("unknown user is (nil, nil)", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(false, nil)
		got, err := svc.GetUserAccessMetrics(context.Background(), "u", AdminTimeseriesQuery{})
		require.NoError(t, err)
		assert.Nil(t, got)
	})
	t.Run("existence error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(false, errors.New("boom"))
		_, err := svc.GetUserAccessMetrics(context.Background(), "u", AdminTimeseriesQuery{})
		require.Error(t, err)
	})
	t.Run("series error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(true, nil)
		repo.On("GetUserAccessBySourceSeries", mock.Anything, "u", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("boom"))
		_, err := svc.GetUserAccessMetrics(context.Background(), "u", AdminTimeseriesQuery{})
		require.Error(t, err)
	})
}

func TestGetUserTopAccessedResources(t *testing.T) {
	to := time.Date(2026, 9, 10, 15, 30, 0, 0, time.UTC)

	t.Run("defaults: 30-day unsnapped window and limit 10", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		wantFrom := to.AddDate(0, 0, -30)
		rows := []models.AdminTopAccessedResource{{ResourceType: "prompt", AccessCount: 3}}
		repo.On("UserExists", mock.Anything, "u").Return(true, nil)
		repo.On("GetUserTopAccessedResources", mock.Anything, "u", wantFrom, to, AdminTopResourcesDefaultLimit).
			Return(rows, nil)

		got, err := svc.GetUserTopAccessedResources(context.Background(), "u", AdminTopResourcesQuery{To: &to})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, wantFrom, got.From)
		assert.Equal(t, to, got.To)
		assert.Equal(t, rows, got.Items)
	})

	t.Run("explicit window and limit pass through", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		from := to.Add(-time.Hour)
		repo.On("UserExists", mock.Anything, "u").Return(true, nil)
		repo.On("GetUserTopAccessedResources", mock.Anything, "u", from, to, 50).
			Return([]models.AdminTopAccessedResource{}, nil)

		got, err := svc.GetUserTopAccessedResources(context.Background(), "u",
			AdminTopResourcesQuery{From: &from, To: &to, Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, from, got.From, "the window is not bucket-snapped")
	})

	for name, q := range map[string]AdminTopResourcesQuery{
		"to not after from": {From: &to, To: &to},
		"range too wide":    {From: timePtr(to.AddDate(-11, 0, 0)), To: &to},
		"limit too large":   {To: &to, Limit: 51},
		"negative limit":    {To: &to, Limit: -1},
	} {
		t.Run(name+" is ErrAdminTimeseriesRange before any query", func(t *testing.T) {
			svc, _ := newInsightsService(t)
			_, err := svc.GetUserTopAccessedResources(context.Background(), "u", q)
			var rangeErr *ErrAdminTimeseriesRange
			require.ErrorAs(t, err, &rangeErr)
		})
	}

	t.Run("unknown user is (nil, nil)", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(false, nil)
		got, err := svc.GetUserTopAccessedResources(context.Background(), "u", AdminTopResourcesQuery{})
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("query error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(true, nil)
		repo.On("GetUserTopAccessedResources", mock.Anything, "u", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("boom"))
		_, err := svc.GetUserTopAccessedResources(context.Background(), "u", AdminTopResourcesQuery{})
		require.Error(t, err)
	})
}
