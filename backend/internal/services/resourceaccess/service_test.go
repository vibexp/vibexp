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
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/freshness"
)

// syncSubmitter runs submitted tasks inline so RecordAccess is deterministic in tests.
type syncSubmitter struct {
	submitted int
}

func (s *syncSubmitter) Submit(task func()) {
	s.submitted++
	task()
}

// fakeClearer records the freshness reversal calls the access path makes.
//
// Hand-written rather than generated: `freshnessClearer` is unexported, and the
// generated mocks package imports this one (for ResourceAccessService), so an
// internal test importing it back would be an import cycle. Same reason
// syncSubmitter is hand-written.
type fakeClearer struct {
	calls  []clearCall
	err    error
	panics bool
}

type clearCall struct {
	teamID       string
	resourceType string
	resourceID   string
	reason       string
	medium       string
}

func (f *fakeClearer) ClearIfStale(
	_ context.Context, teamID, resourceType, resourceID, reason, medium string,
) error {
	f.calls = append(f.calls, clearCall{teamID, resourceType, resourceID, reason, medium})
	if f.panics {
		panic("boom")
	}
	return f.err
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// newServiceWithFake builds a Service directly, bypassing NewService, so tests
// can inject a synchronous submitter.
//
// lastAccessed is explicit rather than defaulted: the metrics and retention
// tests never reach the denormalization path and pass nil, while every test
// that exercises RecordAccess passes a real mock. Defaulting it to nil for
// everyone would let the nil-guard in denormalizeLastAccessed silently absorb a
// regression in the access path.
func newServiceWithFake(
	repo *mocks.MockResourceAccessRepository,
	lastAccessed repositories.ResourceLastAccessedRepository,
	submitter taskSubmitter,
	retentionDays int,
) *Service {
	return &Service{
		repo:          repo,
		lastAccessed:  lastAccessed,
		submitter:     submitter,
		logger:        newTestLogger(),
		retentionDays: retentionDays,
	}
}

// newServiceWithClearer is newServiceWithFake plus the freshness reversal seam
// (#733). It is separate so the existing tests keep proving that the clear is
// optional — a nil clearer must leave the access path working unchanged.
func newServiceWithClearer(
	repo *mocks.MockResourceAccessRepository,
	lastAccessed repositories.ResourceLastAccessedRepository,
	clearer freshnessClearer,
	submitter taskSubmitter,
) *Service {
	svc := newServiceWithFake(repo, lastAccessed, submitter, 90)
	svc.clearer = clearer
	return svc
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
	lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
	submitter := &syncSubmitter{}
	svc := newServiceWithFake(repo, lastAccessed, submitter, 90)

	event := sampleEvent()
	// Create stamps CreatedAt from the database's RETURNING clause; the
	// denormalized column must carry that same instant, not a second app clock.
	created := time.Date(2026, 8, 9, 10, 30, 0, 0, time.UTC)
	repo.EXPECT().
		Create(mock.Anything, event).
		Run(func(_ context.Context, e *models.ResourceAccessEvent) { e.CreatedAt = created }).
		Return(nil).
		Once()
	lastAccessed.EXPECT().
		UpdateLastAccessed(mock.Anything, "prompt", "res-1", SourceWeb, created).
		Return(nil).
		Once()

	svc.RecordAccess(event)

	assert.Equal(t, 1, submitter.submitted, "task should be submitted exactly once")
	repo.AssertExpectations(t)
	lastAccessed.AssertExpectations(t)
}

// The denormalization is a consequence of the event write, not a parallel one:
// if the event failed to persist there is nothing to denormalize, and issuing
// the column update anyway would record an access the log has no row for.
func TestService_RecordAccess_SkipsDenormalizationWhenEventWriteFails(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
	svc := newServiceWithFake(repo, lastAccessed, &syncSubmitter{}, 90)

	repo.EXPECT().
		Create(mock.Anything, mock.Anything).
		Return(errors.New("db down")).
		Once()

	svc.RecordAccess(sampleEvent())

	lastAccessed.AssertNotCalled(t, "UpdateLastAccessed",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// Every source the access path can classify must reach its column; a medium
// missing from the repository's map would otherwise be a silent no-op.
func TestService_RecordAccess_DenormalizesEverySource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{SourceWeb, SourceCLI, SourceMCP, SourceAPI} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			repo := mocks.NewMockResourceAccessRepository(t)
			lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
			svc := newServiceWithFake(repo, lastAccessed, &syncSubmitter{}, 90)

			event := sampleEvent()
			event.Source = source
			repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
			lastAccessed.EXPECT().
				UpdateLastAccessed(mock.Anything, "prompt", "res-1", source, mock.Anything).
				Return(nil).
				Once()

			svc.RecordAccess(event)

			lastAccessed.AssertExpectations(t)
		})
	}
}

// A type with no columns (project/agent are recorded but not freshness-eligible)
// and a genuine repository failure must both be swallowed — the read that
// triggered this was served long ago.
func TestService_RecordAccess_SwallowsDenormalizationOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "unsupported resource type is an expected no-op", err: repositories.ErrUnsupportedLastAccessedResource},
		{name: "repository failure is logged and swallowed", err: errors.New("db down")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := mocks.NewMockResourceAccessRepository(t)
			lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
			svc := newServiceWithFake(repo, lastAccessed, &syncSubmitter{}, 90)

			repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
			lastAccessed.EXPECT().
				UpdateLastAccessed(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(tt.err).
				Once()

			assert.NotPanics(t, func() { svc.RecordAccess(sampleEvent()) })
			lastAccessed.AssertExpectations(t)
		})
	}
}

// A panic in the denormalization must be contained by the same recover that
// guards the event write — it runs on the same worker goroutine.
func TestService_RecordAccess_RecoversFromDenormalizationPanic(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
	svc := newServiceWithFake(repo, lastAccessed, &syncSubmitter{}, 90)

	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	lastAccessed.EXPECT().
		UpdateLastAccessed(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Panic("boom").
		Once()

	assert.NotPanics(t, func() { svc.RecordAccess(sampleEvent()) })
}

// GREATEST(col, '0001-01-01') returns col, so a zero timestamp would make the
// update a silent no-op — success, no log, and stale data. The guard must
// substitute a real instant rather than pass the zero value through.
func TestService_RecordAccess_ZeroCreatedAtFallsBackToNow(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
	svc := newServiceWithFake(repo, lastAccessed, &syncSubmitter{}, 90)

	// Create deliberately leaves CreatedAt zero, unlike the real repository.
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

	var got time.Time
	lastAccessed.EXPECT().
		UpdateLastAccessed(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _, _, _ string, at time.Time) { got = at }).
		Return(nil).
		Once()

	before := time.Now().UTC()
	svc.RecordAccess(sampleEvent())

	require.False(t, got.IsZero(), "a zero CreatedAt must not reach the repository")
	assert.False(t, got.Before(before), "the fallback should be the current time")
}

// The service must stay usable when no last-accessed repository is wired.
func TestService_RecordAccess_NilLastAccessedRepoIsNoOp(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

	assert.NotPanics(t, func() { svc.RecordAccess(sampleEvent()) })
	repo.AssertExpectations(t)
}

func TestService_RecordAccess_NilEventIsNoOp(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	submitter := &syncSubmitter{}
	svc := newServiceWithFake(repo, nil, submitter, 90)

	svc.RecordAccess(nil)

	assert.Equal(t, 0, submitter.submitted, "nil event must not be submitted")
}

func TestService_RecordAccess_SwallowsRepoError(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	submitter := &syncSubmitter{}
	svc := newServiceWithFake(repo, nil, submitter, 90)

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
	svc := newServiceWithFake(repo, nil, submitter, 90)

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
	svc := newServiceWithFake(repo, nil, submitter, retentionDays)

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
	svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

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
	svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

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
	svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

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
			svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

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
	svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

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
	svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

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
	svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

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
	svc := newServiceWithFake(repo, nil, &syncSubmitter{}, 90)

	repo.EXPECT().
		GetTopAccessedResources(mock.Anything, "team-1", mock.Anything, "", 5).
		Return([]models.TopAccessedResource{}, nil).
		Once()

	require.NotPanics(t, func() {
		_, err := svc.GetTopAccessedResources(context.Background(), "team-1", -10, "", 5)
		require.NoError(t, err)
	})
}

// A recorded access must attempt to reverse the resource's stale state, with
// the reason that distinguishes a read from an edit in the audit log. The
// clearer itself owns the policy (is it stale, is reversibility on), so all
// this path has to prove is that it is called with the right arguments.
func TestService_RecordAccess_ClearsFreshness(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
	clearer := &fakeClearer{}
	submitter := &syncSubmitter{}
	event := sampleEvent()

	repo.EXPECT().Create(mock.Anything, event).Return(nil).Once()
	lastAccessed.EXPECT().
		UpdateLastAccessed(mock.Anything, "prompt", "res-1", SourceWeb, mock.Anything).
		Return(nil).Once()

	newServiceWithClearer(repo, lastAccessed, clearer, submitter).RecordAccess(event)

	assert.Equal(t, 1, submitter.submitted)
	// The medium is the event's own source, not a constant: it is what scopes
	// the clear to the rules that actually watch that medium (#770), so passing
	// the wrong one — or none — would resurrect the per-interval flap.
	assert.Equal(t, []clearCall{{
		teamID:       "team-1",
		resourceType: "prompt",
		resourceID:   "res-1",
		reason:       models.FreshnessReasonAccessed,
		medium:       event.Source,
	}}, clearer.calls)
	require.Equal(t, SourceWeb, event.Source, "this assertion only means anything while the event carries a source")
}

// The clear is fire-and-forget like everything else on this goroutine: the read
// that triggered it was served long ago, so a freshness failure must not panic,
// must not propagate, and must not stop the event or the denormalization that
// already succeeded.
func TestService_RecordAccess_ClearFailureIsContained(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		clearer *fakeClearer
	}{
		{name: "error", clearer: &fakeClearer{err: errors.New("boom")}},
		{name: "panic", clearer: &fakeClearer{panics: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := mocks.NewMockResourceAccessRepository(t)
			lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
			repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
			lastAccessed.EXPECT().
				UpdateLastAccessed(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil).Once()

			svc := newServiceWithClearer(repo, lastAccessed, tt.clearer, &syncSubmitter{})

			assert.NotPanics(t, func() { svc.RecordAccess(sampleEvent()) })
			assert.Len(t, tt.clearer.calls, 1, "the clear was attempted before it failed")
		})
	}
}

// A Service built without a clearer must behave exactly as before #733 — the
// guard is what lets every other test in this file keep passing nil.
func TestService_RecordAccess_NilClearerIsSkipped(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceAccessRepository(t)
	lastAccessed := mocks.NewMockResourceLastAccessedRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	lastAccessed.EXPECT().
		UpdateLastAccessed(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	svc := newServiceWithFake(repo, lastAccessed, &syncSubmitter{}, 90)

	assert.NotPanics(t, func() { svc.RecordAccess(sampleEvent()) })
}

// NewService stores a nil *freshness.Clearer as a nil INTERFACE, not as a
// non-nil interface holding a nil pointer — otherwise the guard in
// reverseFreshness would pass and the call would panic on a deployment that
// wired no clearer.
//
// The comparison is `== nil` rather than assert.Nil: assert.Nil reports a
// non-nil interface holding a nil pointer as nil (it falls through to
// reflect.Value.IsNil), which is exactly the state this test exists to rule
// out — so the obvious spelling would pass with the guard deleted.
func TestNewService_NilClearerStaysNil(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, nil, newTestLogger(), 90)

	require.True(t, svc.clearer == nil, "a nil *freshness.Clearer must not become a non-nil interface")
	assert.NotPanics(t, func() { svc.reverseFreshness(context.Background(), sampleEvent()) })
}

// The other half of the same constructor branch: a real clearer must actually
// be stored, or the feature would be silently wired off.
func TestNewService_RealClearerIsStored(t *testing.T) {
	t.Parallel()

	clearer := freshness.NewClearer(nil, nil, nil, nil, newTestLogger())

	svc := NewService(nil, nil, clearer, nil, newTestLogger(), 90)

	require.False(t, svc.clearer == nil)
}
