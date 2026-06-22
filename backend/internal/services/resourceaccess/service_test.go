package resourceaccess

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
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// syncSubmitter runs submitted tasks inline so RecordAccess is deterministic in tests.
type syncSubmitter struct {
	submitted int
}

func (s *syncSubmitter) Submit(task func()) {
	s.submitted++
	task()
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newServiceWithFake(
	repo *mocks.MockResourceAccessRepository,
	submitter taskSubmitter,
	retentionDays int,
) *Service {
	return &Service{
		repo:          repo,
		submitter:     submitter,
		logger:        newTestLogger(),
		retentionDays: retentionDays,
	}
}

func strPtr(s string) *string { return &s }

func sampleEvent() *models.ResourceAccessEvent {
	return &models.ResourceAccessEvent{
		ID:           "evt-1",
		TeamID:       "team-1",
		UserID:       strPtr("user-1"),
		ResourceType: "prompt",
		ResourceID:   "res-1",
		Source:       SourceWeb,
	}
}

func TestService_RecordAccess_SubmitsAndPersists(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	submitter := &syncSubmitter{}
	svc := newServiceWithFake(repo, submitter, 90)

	event := sampleEvent()
	repo.EXPECT().
		Create(mock.Anything, event).
		Return(nil).
		Once()

	svc.RecordAccess(event)

	assert.Equal(t, 1, submitter.submitted, "task should be submitted exactly once")
	repo.AssertExpectations(t)
}

func TestService_RecordAccess_NilEventIsNoOp(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	submitter := &syncSubmitter{}
	svc := newServiceWithFake(repo, submitter, 90)

	svc.RecordAccess(nil)

	assert.Equal(t, 0, submitter.submitted, "nil event must not be submitted")
}

func TestService_RecordAccess_SwallowsRepoError(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	submitter := &syncSubmitter{}
	svc := newServiceWithFake(repo, submitter, 90)

	repo.EXPECT().
		Create(mock.Anything, mock.Anything).
		Return(errors.New("db down")).
		Once()

	assert.NotPanics(t, func() {
		svc.RecordAccess(sampleEvent())
	}, "RecordAccess must never panic, even when persistence fails")

	repo.AssertExpectations(t)
}

func TestService_RecordAccess_RecoversFromPanic(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	submitter := &syncSubmitter{}
	svc := newServiceWithFake(repo, submitter, 90)

	repo.EXPECT().
		Create(mock.Anything, mock.Anything).
		Panic("boom").
		Once()

	assert.NotPanics(t, func() {
		svc.RecordAccess(sampleEvent())
	}, "a panic inside the persist path must not escape")
}

func TestService_RunRetentionJob_ComputesCutoff(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	submitter := &syncSubmitter{}
	const retentionDays = 30
	svc := newServiceWithFake(repo, submitter, retentionDays)

	expectedCutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	repo.EXPECT().
		DeleteOlderThan(mock.Anything, mock.MatchedBy(func(before time.Time) bool {
			return before.Sub(expectedCutoff).Abs() < time.Minute
		})).
		Return(int64(7), nil).
		Once()

	err := svc.RunRetentionJob(context.Background())

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestService_RunRetentionJob_WrapsError(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	svc := newServiceWithFake(repo, &syncSubmitter{}, 90)

	sentinel := errors.New("delete failed")
	repo.EXPECT().
		DeleteOlderThan(mock.Anything, mock.Anything).
		Return(int64(0), sentinel).
		Once()

	err := svc.RunRetentionJob(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestService_GetMetrics_ZeroFillsGaps(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	svc := newServiceWithFake(repo, &syncSubmitter{}, 90)

	const rangeDays = 3
	since := time.Now().UTC().AddDate(0, 0, -rangeDays)
	startDay := since.Truncate(24 * time.Hour)

	// Only one day has data, for a single source. Every other day/source must be zero-filled.
	hitDay := startDay.AddDate(0, 0, 1).Format(dateLayout)
	repo.EXPECT().
		GetMetricsByResource(mock.Anything, "team-1", "prompt", "res-1", mock.Anything).
		Return([]models.DailyAccessCount{
			{Date: hitDay, Source: SourceWeb, Count: 5},
			{Date: hitDay, Source: SourceCLI, Count: 2},
		}, nil).
		Once()

	result, err := svc.GetMetrics(context.Background(), "team-1", "prompt", "res-1", rangeDays)

	require.NoError(t, err)
	require.Len(t, result.Days, rangeDays+1, "series must contain one row per day inclusive of both ends")

	// Every day carries the same set of sources (the canonical four here).
	for _, day := range result.Days {
		assert.Len(t, day.Sources, 4, "each day must carry every observed source")
	}

	counts := sourceCounts(t, result.Days, hitDay)
	assert.Equal(t, 5, counts[SourceWeb])
	assert.Equal(t, 2, counts[SourceCLI])
	assert.Equal(t, 0, counts[SourceMCP])
	assert.Equal(t, 0, counts[SourceAPI])

	// A day with no data must still be present and fully zero.
	emptyDay := startDay.Format(dateLayout)
	emptyCounts := sourceCounts(t, result.Days, emptyDay)
	for _, source := range []string{SourceWeb, SourceCLI, SourceMCP, SourceAPI} {
		assert.Equal(t, 0, emptyCounts[source], "gap day must be zero for %s", source)
	}
}

func TestService_GetMetrics_EmptyRangeStillFilled(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	svc := newServiceWithFake(repo, &syncSubmitter{}, 90)

	repo.EXPECT().
		GetMetricsByResource(mock.Anything, "team-1", "prompt", "res-1", mock.Anything).
		Return([]models.DailyAccessCount{}, nil).
		Once()

	result, err := svc.GetMetrics(context.Background(), "team-1", "prompt", "res-1", 2)

	require.NoError(t, err)
	require.Len(t, result.Days, 3)
	for _, day := range result.Days {
		assert.Len(t, day.Sources, 4)
		for _, point := range day.Sources {
			assert.Equal(t, 0, point.Count)
		}
	}
}

func TestService_GetMetrics_NegativeRangeDoesNotPanic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rangeDays int
	}{
		{name: "minus one", rangeDays: -1},
		{name: "minus two (would overflow cap)", rangeDays: -2},
		{name: "large negative", rangeDays: -365},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := mocks.NewMockResourceAccessRepository(t)
			svc := newServiceWithFake(repo, &syncSubmitter{}, 90)

			repo.EXPECT().
				GetMetricsByResource(mock.Anything, "team-1", "prompt", "res-1", mock.Anything).
				Return([]models.DailyAccessCount{}, nil).
				Once()

			var result *MetricsResult
			var err error
			require.NotPanics(t, func() {
				result, err = svc.GetMetrics(context.Background(), "team-1", "prompt", "res-1", tc.rangeDays)
			}, "negative rangeDays must be clamped, not panic")

			require.NoError(t, err)
			// Clamped to a single-bucket "today" range.
			assert.Equal(t, 0, result.RangeDays)
			require.Len(t, result.Days, 1)
		})
	}
}

func TestService_GetMetrics_AlignsWindowToUTCMidnight(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	svc := newServiceWithFake(repo, &syncSubmitter{}, 90)

	const rangeDays = 3
	var capturedSince time.Time
	repo.EXPECT().
		GetMetricsByResource(mock.Anything, "team-1", "prompt", "res-1", mock.MatchedBy(func(since time.Time) bool {
			capturedSince = since
			return true
		})).
		Return([]models.DailyAccessCount{}, nil).
		Once()

	result, err := svc.GetMetrics(context.Background(), "team-1", "prompt", "res-1", rangeDays)
	require.NoError(t, err)

	// The SQL window start must be truncated to UTC midnight so it aligns with the
	// zero-filled series start (no partial-day undercount on the oldest day).
	assert.Equal(t, time.UTC, capturedSince.Location())
	assert.Equal(t, 0, capturedSince.Hour())
	assert.Equal(t, 0, capturedSince.Minute())
	assert.Equal(t, 0, capturedSince.Second())
	assert.Equal(t, 0, capturedSince.Nanosecond())

	// And it must match the oldest day emitted in the series.
	expectedStart := time.Now().UTC().AddDate(0, 0, -rangeDays)
	expectedStartDay := time.Date(expectedStart.Year(), expectedStart.Month(), expectedStart.Day(), 0, 0, 0, 0, time.UTC)
	assert.True(t, capturedSince.Equal(expectedStartDay), "window start should be the oldest day at UTC midnight")
	require.Len(t, result.Days, rangeDays+1)
	assert.Equal(t, expectedStartDay.Format(dateLayout), result.Days[0].Date)
}

func TestService_GetMetrics_WrapsRepoError(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	svc := newServiceWithFake(repo, &syncSubmitter{}, 90)

	sentinel := errors.New("query failed")
	repo.EXPECT().
		GetMetricsByResource(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, sentinel).
		Once()

	_, err := svc.GetMetrics(context.Background(), "team-1", "prompt", "res-1", 7)

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// sourceCounts returns a source->count map for the named day in the series.
func sourceCounts(t *testing.T, days []DailyMetrics, date string) map[string]int {
	t.Helper()
	for _, day := range days {
		if day.Date == date {
			out := make(map[string]int, len(day.Sources))
			for _, point := range day.Sources {
				out[point.Source] = point.Count
			}
			return out
		}
	}
	t.Fatalf("day %s not found in series", date)
	return nil
}

func TestDeriveSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		authMethod string
		path       string
		userAgent  string
		want       string
	}{
		{
			name:       "cookie auth is always web",
			authMethod: "cookie",
			path:       "/api/v1/teams/x/prompts/y",
			userAgent:  "VibeXP-CLI/1.2.3",
			want:       SourceWeb,
		},
		{
			name:       "api key on mcp path is mcp",
			authMethod: "api_key",
			path:       "/mcp/messages",
			userAgent:  "node",
			want:       SourceMCP,
		},
		{
			name:       "api key with cli user agent is cli",
			authMethod: "api_key",
			path:       "/api/v1/teams/x/prompts/y",
			userAgent:  "VibeXP-CLI/0.9.0",
			want:       SourceCLI,
		},
		{
			name:       "api key mcp takes priority over cli user agent",
			authMethod: "api_key",
			path:       "/mcp/tools/call",
			userAgent:  "VibeXP-CLI/1.0.0",
			want:       SourceMCP,
		},
		{
			name:       "api key without cli ua or mcp path is api",
			authMethod: "api_key",
			path:       "/api/v1/teams/x/prompts/y",
			userAgent:  "curl/8.0",
			want:       SourceAPI,
		},
		{
			name:       "unknown auth method falls back to api",
			authMethod: "",
			path:       "/api/v1/teams/x/prompts/y",
			userAgent:  "",
			want:       SourceAPI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DeriveSource(tc.authMethod, tc.path, tc.userAgent)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestDeriveSource_OAuth pins that AuthKit-JWT clients (auth_type "oauth",
// e.g. mobile) classify as SourceAPI.
func TestDeriveSource_OAuth(t *testing.T) {
	t.Parallel()
	assert.Equal(t, SourceAPI,
		DeriveSource("oauth", "/api/v1/teams/x/prompts/y", "VibeXP-Mobile/1.0.0"))
}

// TestService_GetTopAccessedResources passes the limit through and returns the
// repository's resolved ranking verbatim.
func TestService_GetTopAccessedResources(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	svc := newServiceWithFake(repo, &syncSubmitter{}, 90)

	want := []models.TopAccessedResource{
		{ResourceType: "prompt", ResourceID: "res-1", Name: "Checklist", AccessCount: 12},
		{ResourceType: "artifact", ResourceID: "res-2", Name: "Report", AccessCount: 5},
	}
	repo.EXPECT().
		GetTopAccessedResources(mock.Anything, "team-1", mock.Anything, "cli", 5).
		Return(want, nil).
		Once()

	got, err := svc.GetTopAccessedResources(context.Background(), "team-1", 30, "cli", 5)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestService_GetTopAccessedResources_NegativeRangeDoesNotPanic confirms a negative
// range is clamped rather than producing a negative SQL window.
func TestService_GetTopAccessedResources_NegativeRangeDoesNotPanic(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	svc := newServiceWithFake(repo, &syncSubmitter{}, 90)

	repo.EXPECT().
		GetTopAccessedResources(mock.Anything, "team-1", mock.Anything, "", 5).
		Return([]models.TopAccessedResource{}, nil).
		Once()

	require.NotPanics(t, func() {
		_, err := svc.GetTopAccessedResources(context.Background(), "team-1", -10, "", 5)
		require.NoError(t, err)
	})
}
