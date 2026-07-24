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
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// adminNow is the fixed "now" the range tests resolve against — a Wednesday, so
// the week-alignment cases have a non-zero offset to correct.
var adminNow = time.Date(2026, 7, 15, 13, 45, 30, 0, time.UTC)

func timePtr(t time.Time) *time.Time { return &t }

// TestResolveAdminRange_Defaults pins the documented defaulting: no params ->
// last 30 days at day granularity, with `from` snapped to a bucket boundary.
func TestResolveAdminRange_Defaults(t *testing.T) {
	got, err := resolveAdminRange(AdminTimeseriesQuery{}, adminNow)
	require.NoError(t, err)

	assert.Equal(t, adminGranularityDay, got.granularity)
	assert.Equal(t, adminNow, got.to)
	// 2026-07-15 minus 30 days = 2026-06-15, snapped to midnight.
	assert.Equal(t, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), got.from)
}

// TestResolveAdminRange_Invalid covers every rejection path. Each must surface
// *ErrAdminTimeseriesRange so the handler can map it to 400 rather than 500.
func TestResolveAdminRange_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		query AdminTimeseriesQuery
	}{
		{
			name:  "granularity outside the enum",
			query: AdminTimeseriesQuery{Granularity: "fortnight"},
		},
		{
			name: "to equals from",
			query: AdminTimeseriesQuery{
				From: timePtr(adminNow), To: timePtr(adminNow),
			},
		},
		{
			name: "to before from",
			query: AdminTimeseriesQuery{
				From: timePtr(adminNow), To: timePtr(adminNow.Add(-time.Hour)),
			},
		},
		{
			name: "range wider than the cap",
			query: AdminTimeseriesQuery{
				From: timePtr(adminNow.AddDate(-11, 0, 0)), To: timePtr(adminNow),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveAdminRange(tc.query, adminNow)
			require.Error(t, err)

			var rangeErr *ErrAdminTimeseriesRange
			require.True(t, errors.As(err, &rangeErr), "must be *ErrAdminTimeseriesRange so the handler 400s")
			assert.NotEmpty(t, rangeErr.Detail)
		})
	}
}

// TestResolveAdminRange_AtTheCap proves the cap boundary is inclusive, so a
// range of exactly the maximum span is accepted rather than rejected.
func TestResolveAdminRange_AtTheCap(t *testing.T) {
	from := adminNow.Add(-adminTimeseriesMaxDays * 24 * time.Hour)
	_, err := resolveAdminRange(AdminTimeseriesQuery{From: &from, To: timePtr(adminNow)}, adminNow)
	require.NoError(t, err)
}

// TestAdminBucketStart pins Go's bucket alignment against Postgres date_trunc
// semantics — notably that a 'week' starts on MONDAY, not Sunday.
func TestAdminBucketStart(t *testing.T) {
	tests := []struct {
		name        string
		in          time.Time
		granularity string
		want        time.Time
	}{
		{
			name: "day truncates the clock",
			in:   time.Date(2026, 7, 15, 23, 59, 59, 0, time.UTC),
			// granularity day
			granularity: adminGranularityDay,
			want:        time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "week from a Wednesday snaps back to Monday",
			in:          time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC), // Wednesday
			granularity: adminGranularityWeek,
			want:        time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), // Monday
		},
		{
			name:        "week from a Sunday snaps back six days, not zero",
			in:          time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC), // Sunday
			granularity: adminGranularityWeek,
			want:        time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), // the Monday before
		},
		{
			name:        "week from a Monday is already aligned",
			in:          time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC),
			granularity: adminGranularityWeek,
			want:        time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "month snaps to the first",
			in:          time.Date(2026, 7, 31, 23, 0, 0, 0, time.UTC),
			granularity: adminGranularityMonth,
			want:        time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, adminBucketStart(tc.in, tc.granularity))
		})
	}
}

// TestAdminBuckets_Contiguous checks bucket enumeration per granularity,
// including that month stepping handles unequal month lengths.
func TestAdminBuckets_Contiguous(t *testing.T) {
	tests := []struct {
		name        string
		from        time.Time
		to          time.Time
		granularity string
		wantCount   int
		wantLast    time.Time
	}{
		{
			name:        "seven daily buckets",
			from:        time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			to:          time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
			granularity: adminGranularityDay,
			wantCount:   7,
			wantLast:    time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "three weekly buckets",
			from:        time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC), // Monday
			to:          time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
			granularity: adminGranularityWeek,
			wantCount:   3,
			wantLast:    time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "monthly buckets step across unequal months",
			from:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			to:          time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			granularity: adminGranularityMonth,
			wantCount:   3,
			wantLast:    time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := adminBuckets(adminResolvedRange{
				from: tc.from, to: tc.to, granularity: tc.granularity,
			})
			require.Len(t, got, tc.wantCount)
			assert.Equal(t, tc.from, got[0])
			assert.Equal(t, tc.wantLast, got[len(got)-1])

			// Contiguity: every step is exactly one bucket.
			for i := 1; i < len(got); i++ {
				assert.Equal(t, adminNextBucket(got[i-1], tc.granularity), got[i])
			}
		})
	}
}

// TestGetDashboardTimeseries_GapFillsEmptyRange is the acceptance criterion
// "every bucket present with an explicit 0" — asserted over a range with NO
// data at all, where a sparse pass-through would return empty series.
func TestGetDashboardTimeseries_GapFillsEmptyRange(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetGrowthSeries", mock.Anything, mock.Anything, mock.Anything, adminGranularityDay).
		Return([]models.AdminGrowthCount{}, nil)
	repo.On("GetSignInSeries", mock.Anything, mock.Anything, mock.Anything, adminGranularityDay).
		Return([]models.AdminCountPoint{}, nil)
	repo.On("GetAccessBySourceSeries", mock.Anything, mock.Anything, mock.Anything, adminGranularityDay).
		Return([]models.AdminSourcePoint{}, nil)

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -6)
	got, err := NewAdminService(repo).GetDashboardTimeseries(context.Background(), AdminTimeseriesQuery{
		From: &from, To: &to,
	}, models.AdminDataWindow{})
	require.NoError(t, err)

	// 6 days back, snapped down to midnight, up to now => 7 buckets.
	require.Len(t, got.Growth, 7)
	require.Len(t, got.SignIns, 7)
	for _, p := range got.Growth {
		assert.Zero(t, p.Users)
		assert.Zero(t, p.Memories)
	}
	for _, p := range got.SignIns {
		assert.Zero(t, p.Count)
	}
	// No sources were observed, so there is nothing to plot — not one row per
	// bucket with an invented source.
	assert.Empty(t, got.AccessBySource)
}

// TestFillGrowth_PivotsAndZeroFills checks the sparse->dense pivot: a bucket
// with rows for only some entities still reports 0 for the rest.
func TestFillGrowth_PivotsAndZeroFills(t *testing.T) {
	b0 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	b1 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	buckets := []time.Time{b0, b1}

	got := fillGrowth(buckets, []models.AdminGrowthCount{
		{Entity: "users", Bucket: b0, Count: 3},
		{Entity: "memories", Bucket: b0, Count: 9},
		{Entity: "prompts", Bucket: b1, Count: 4},
	})

	require.Len(t, got, 2)
	assert.Equal(t, int64(3), got[0].Users)
	assert.Equal(t, int64(9), got[0].Memories)
	assert.Zero(t, got[0].Prompts, "an entity with no rows in the bucket must be 0, not absent")
	assert.Equal(t, int64(4), got[1].Prompts)
	assert.Zero(t, got[1].Users)
}

// TestFillGrowth_IgnoresOutOfRangeBuckets guards the defensive branch: a row
// whose bucket is not in the enumerated range must not corrupt or extend the
// series.
func TestFillGrowth_IgnoresOutOfRangeBuckets(t *testing.T) {
	b0 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	got := fillGrowth([]time.Time{b0}, []models.AdminGrowthCount{
		{Entity: "users", Bucket: b0, Count: 2},
		{Entity: "users", Bucket: b0.AddDate(0, 0, 40), Count: 99},
	})

	require.Len(t, got, 1)
	assert.Equal(t, int64(2), got[0].Users)
}

// TestFillSources_OneLinePerObservedSource asserts each observed source gets a
// point in EVERY bucket (so a client can draw contiguous lines) and that the
// output order is deterministic.
func TestFillSources_OneLinePerObservedSource(t *testing.T) {
	b0 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	b1 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)

	got := fillSources([]time.Time{b0, b1}, []models.AdminSourcePoint{
		{Bucket: b1, Source: "web", Count: 5},
		{Bucket: b0, Source: "mcp", Count: 2},
	})

	// 2 buckets x 2 observed sources, sorted by bucket then source.
	require.Len(t, got, 4)
	assert.Equal(t, models.AdminSourcePoint{Bucket: b0, Source: "mcp", Count: 2}, got[0])
	assert.Equal(t, models.AdminSourcePoint{Bucket: b0, Source: "web", Count: 0}, got[1])
	assert.Equal(t, models.AdminSourcePoint{Bucket: b1, Source: "mcp", Count: 0}, got[2])
	assert.Equal(t, models.AdminSourcePoint{Bucket: b1, Source: "web", Count: 5}, got[3])
}

// TestGetDashboardTimeseries_InvalidRangeSkipsRepository proves validation runs
// BEFORE any query — the mock has no expectations, so any repository call fails.
func TestGetDashboardTimeseries_InvalidRangeSkipsRepository(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)

	_, err := NewAdminService(repo).GetDashboardTimeseries(context.Background(), AdminTimeseriesQuery{
		Granularity: "century",
	}, models.AdminDataWindow{})

	var rangeErr *ErrAdminTimeseriesRange
	require.True(t, errors.As(err, &rangeErr))
}

// TestGetDashboardOverview_Composes checks the overview stitches the three
// repository reads together and stamps the supplied version.
func TestGetDashboardOverview_Composes(t *testing.T) {
	counts := models.AdminExtendedCounts{Users: 5, Teams: 2, Projects: 7, APIKeys: 3}
	breakdowns := []models.AdminEntityBreakdown{{
		Entity: "prompts", Field: "status",
		Buckets: []models.AdminBreakdownBucket{{Value: "active", Count: 4}},
	}}
	health := models.AdminSystemHealth{
		DatabaseSizeBytes: 1234,
		Tables:            []models.AdminTableStat{{Table: "prompts", EstimatedRows: 4}},
	}

	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetExtendedCounts", mock.Anything).Return(counts, nil)
	repo.On("GetEntityBreakdowns", mock.Anything).Return(breakdowns, nil)
	repo.On("GetSystemHealth", mock.Anything).Return(health, nil)

	got, err := NewAdminService(repo).GetDashboardOverview(context.Background(), "1.2.3")
	require.NoError(t, err)
	assert.Equal(t, counts, got.Counts)
	assert.Equal(t, breakdowns, got.Breakdowns)
	assert.Equal(t, health, got.SystemHealth)
	assert.Equal(t, "1.2.3", got.Version)
}

// TestGetDashboardOverview_PropagatesErrors ensures a failure in ANY of the
// three reads aborts rather than returning a half-built payload.
func TestGetDashboardOverview_PropagatesErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*repomocks.MockAdminRepository)
	}{
		{
			name: "counts fail",
			setup: func(r *repomocks.MockAdminRepository) {
				r.On("GetExtendedCounts", mock.Anything).
					Return(models.AdminExtendedCounts{}, errors.New("boom"))
			},
		},
		{
			name: "breakdowns fail",
			setup: func(r *repomocks.MockAdminRepository) {
				r.On("GetExtendedCounts", mock.Anything).Return(models.AdminExtendedCounts{}, nil)
				r.On("GetEntityBreakdowns", mock.Anything).Return(nil, errors.New("boom"))
			},
		},
		{
			name: "system health fails",
			setup: func(r *repomocks.MockAdminRepository) {
				r.On("GetExtendedCounts", mock.Anything).Return(models.AdminExtendedCounts{}, nil)
				r.On("GetEntityBreakdowns", mock.Anything).Return([]models.AdminEntityBreakdown{}, nil)
				r.On("GetSystemHealth", mock.Anything).
					Return(models.AdminSystemHealth{}, errors.New("boom"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := repomocks.NewMockAdminRepository(t)
			tc.setup(repo)

			_, err := NewAdminService(repo).GetDashboardOverview(context.Background(), "dev")
			require.Error(t, err)
		})
	}
}

// TestGetDashboardTimeseries_PropagatesSeriesErrors mirrors the above for the
// three series reads.
func TestGetDashboardTimeseries_PropagatesSeriesErrors(t *testing.T) {
	growthOK := func(r *repomocks.MockAdminRepository) {
		r.On("GetGrowthSeries", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return([]models.AdminGrowthCount{}, nil)
	}
	signInOK := func(r *repomocks.MockAdminRepository) {
		r.On("GetSignInSeries", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return([]models.AdminCountPoint{}, nil)
	}

	tests := []struct {
		name  string
		setup func(*repomocks.MockAdminRepository)
	}{
		{
			name: "growth fails",
			setup: func(r *repomocks.MockAdminRepository) {
				r.On("GetGrowthSeries", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("boom"))
			},
		},
		{
			name: "sign-ins fail",
			setup: func(r *repomocks.MockAdminRepository) {
				growthOK(r)
				r.On("GetSignInSeries", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("boom"))
			},
		},
		{
			name: "access-by-source fails",
			setup: func(r *repomocks.MockAdminRepository) {
				growthOK(r)
				signInOK(r)
				r.On("GetAccessBySourceSeries", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("boom"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := repomocks.NewMockAdminRepository(t)
			tc.setup(repo)

			_, err := NewAdminService(repo).GetDashboardTimeseries(
				context.Background(), AdminTimeseriesQuery{}, models.AdminDataWindow{},
			)
			require.Error(t, err)
		})
	}
}

// TestAdminDataWindowFor derives the retention window from the configured TTLs.
func TestAdminDataWindowFor(t *testing.T) {
	got := AdminDataWindowFor(adminNow, 90, 30)
	assert.Equal(t, adminNow.AddDate(0, 0, -90), got.SignInsEarliestRetainedAt)
	assert.Equal(t, adminNow.AddDate(0, 0, -30), got.AccessBySourceEarliestRetainedAt)
}
