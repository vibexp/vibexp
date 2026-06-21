package resourceaccess

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// recordTimeout bounds the inner repository write for an asynchronously recorded
// access event. The event is decoupled from the request, so a fresh background
// context with a short deadline keeps a slow database from leaking goroutines.
const recordTimeout = 5 * time.Second

// taskSubmitter is the async-execution seam. It is satisfied by *events.WorkerPool
// in production and by a synchronous fake in tests, so RecordAccess can be asserted
// deterministically without sleeps.
type taskSubmitter interface {
	Submit(task func())
}

// Service implements ResourceAccessService.
type Service struct {
	repo          repositories.ResourceAccessRepository
	submitter     taskSubmitter
	logger        *logrus.Logger
	retentionDays int
}

// NewService creates a new resource access service.
func NewService(
	repo repositories.ResourceAccessRepository,
	pool *events.WorkerPool,
	logger *logrus.Logger,
	retentionDays int,
) *Service {
	return &Service{
		repo:          repo,
		submitter:     pool,
		logger:        logger,
		retentionDays: retentionDays,
	}
}

// RecordAccess records a resource detail-access event asynchronously.
// It is fire-and-forget: it returns immediately, never propagates an error to the
// caller, and recovers from any panic so a recording failure can never affect the
// read request that triggered it.
//
// The event pointer is captured and persisted on a worker goroutine. Callers must
// pass a freshly-allocated event and must not mutate or reuse it after the call
// returns, to avoid a data race with the background write.
func (s *Service) RecordAccess(event *models.ResourceAccessEvent) {
	if event == nil {
		return
	}

	s.submitter.Submit(func() {
		s.persistAccess(event)
	})
}

// persistAccess writes a single access event using a fresh, time-bounded context.
// It is panic-safe and logs (Warn) on failure rather than propagating.
func (s *Service) persistAccess(event *models.ResourceAccessEvent) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.WithField("panic", r).Warn("recovered from panic while recording resource access event")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()

	if err := s.repo.Create(ctx, event); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"team_id":       event.TeamID,
			"resource_type": event.ResourceType,
			"resource_id":   event.ResourceID,
			"source":        event.Source,
		}).Warn("failed to record resource access event")
	}
}

// GetMetrics returns per-source daily access counts for a resource over the last
// rangeDays days, zero-filled so every day in the range has a point per source.
func (s *Service) GetMetrics(
	ctx context.Context,
	teamID, resourceType, resourceID string,
	rangeDays int,
) (*MetricsResult, error) {
	// Clamp negative input: a 0-day range is a single bucket for "today". This guards
	// against a panic in the make([]DailyMetrics, 0, rangeDays+1) capacity and against
	// a negative SQL window, since GetMetrics is exported and callers don't validate.
	if rangeDays < 0 {
		rangeDays = 0
	}

	// Align the SQL window start with the zero-filled series start by truncating to
	// UTC midnight. AddDate carries the current time-of-day, but the series emits the
	// oldest day as a full bucket, so without this the oldest day would silently
	// undercount events that occurred before now's time-of-day.
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -rangeDays)
	since := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	counts, err := s.repo.GetMetricsByResource(ctx, teamID, resourceType, resourceID, since)
	if err != nil {
		return nil, fmt.Errorf("get resource access metrics: %w", err)
	}

	return &MetricsResult{
		TeamID:       teamID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		RangeDays:    rangeDays,
		Days:         zeroFillDailySeries(counts, since, rangeDays),
	}, nil
}

// GetTeamMetrics returns per-source daily access counts across the whole team
// (every resource) over the last rangeDays days, zero-filled so every day in the
// range has a point per source. Mirrors GetMetrics but is team-wide rather than
// scoped to a single resource. Team membership is authorized by the caller.
func (s *Service) GetTeamMetrics(
	ctx context.Context,
	teamID string,
	rangeDays int,
) (*MetricsResult, error) {
	// Clamp negative input: a 0-day range is a single bucket for "today". Guards the
	// make([]DailyMetrics, 0, rangeDays+1) capacity and the SQL window, since
	// GetTeamMetrics is exported and callers don't validate.
	if rangeDays < 0 {
		rangeDays = 0
	}

	// Align the SQL window start with the zero-filled series start by truncating to
	// UTC midnight (see GetMetrics for why).
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -rangeDays)
	since := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	counts, err := s.repo.GetTeamMetrics(ctx, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("get team resource access metrics: %w", err)
	}

	return &MetricsResult{
		TeamID:    teamID,
		RangeDays: rangeDays,
		Days:      zeroFillDailySeries(counts, since, rangeDays),
	}, nil
}

// GetTopAccessedResources returns the team's most-accessed resources over the last
// rangeDays days, ranked by access count descending and capped at `limit`, with each
// resource's display name resolved. An empty or "all" source aggregates across access
// channels; a concrete source (web/cli/mcp/api) restricts the ranking to that channel.
// Team membership is authorized by the caller.
func (s *Service) GetTopAccessedResources(
	ctx context.Context,
	teamID string,
	rangeDays int,
	source string,
	limit int,
) ([]models.TopAccessedResource, error) {
	// Clamp negative input: a 0-day range is a single bucket for "today". Guards the
	// SQL window, since GetTopAccessedResources is exported and callers don't validate.
	if rangeDays < 0 {
		rangeDays = 0
	}

	// Align the window start with the daily-series convention by truncating to UTC
	// midnight (see GetTeamMetrics for why).
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -rangeDays)
	since := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	items, err := s.repo.GetTopAccessedResources(ctx, teamID, since, source, limit)
	if err != nil {
		return nil, fmt.Errorf("get top accessed resources: %w", err)
	}

	return items, nil
}

// RunRetentionJob deletes access events older than the configured retention window.
func (s *Service) RunRetentionJob(ctx context.Context) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -s.retentionDays)

	count, err := s.repo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("run resource access retention job: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"deleted_count":  count,
		"older_than":     cutoff.Format(time.RFC3339),
		"retention_days": s.retentionDays,
	}).Info("Resource access retention job completed")

	return nil
}

// dateLayout is the day-granularity key used to match raw counts to series days.
const dateLayout = "2006-01-02"

// zeroFillDailySeries pivots the raw per-day/per-source counts into a contiguous,
// gap-free daily series. Every day from the start of the range through today has a
// row, and every source observed in the range appears on every day (zero when absent).
func zeroFillDailySeries(counts []models.DailyAccessCount, since time.Time, rangeDays int) []DailyMetrics {
	byDay := indexCountsByDay(counts)
	sources := observedSources(counts)

	startDay := since.UTC().Truncate(24 * time.Hour)
	days := make([]DailyMetrics, 0, rangeDays+1)
	for offset := 0; offset <= rangeDays; offset++ {
		date := startDay.AddDate(0, 0, offset).Format(dateLayout)
		days = append(days, DailyMetrics{
			Date:    date,
			Sources: sourcePointsForDay(byDay[date], sources),
		})
	}

	return days
}

// indexCountsByDay groups raw counts as date -> source -> count.
func indexCountsByDay(counts []models.DailyAccessCount) map[string]map[string]int {
	byDay := make(map[string]map[string]int)
	for _, c := range counts {
		if byDay[c.Date] == nil {
			byDay[c.Date] = make(map[string]int)
		}
		byDay[c.Date][c.Source] += c.Count
	}
	return byDay
}

// observedSources returns the sorted set of sources seen across the range, falling
// back to the canonical source set when the range is empty so the series shape is
// stable regardless of traffic.
func observedSources(counts []models.DailyAccessCount) []string {
	seen := make(map[string]struct{})
	for _, c := range counts {
		seen[c.Source] = struct{}{}
	}
	if len(seen) == 0 {
		return []string{SourceWeb, SourceCLI, SourceMCP, SourceAPI}
	}

	for _, s := range []string{SourceWeb, SourceCLI, SourceMCP, SourceAPI} {
		seen[s] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for _, s := range []string{SourceWeb, SourceCLI, SourceMCP, SourceAPI} {
		if _, ok := seen[s]; ok {
			out = append(out, s)
			delete(seen, s)
		}
	}
	extras := make([]string, 0, len(seen))
	for s := range seen {
		extras = append(extras, s)
	}
	sort.Strings(extras)
	return append(out, extras...)
}

// sourcePointsForDay builds the zero-filled per-source points for a single day.
func sourcePointsForDay(dayCounts map[string]int, sources []string) []SourceMetricPoint {
	points := make([]SourceMetricPoint, 0, len(sources))
	for _, source := range sources {
		points = append(points, SourceMetricPoint{
			Source: source,
			Count:  dayCounts[source],
		})
	}
	return points
}
