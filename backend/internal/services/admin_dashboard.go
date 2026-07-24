package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// Dashboard metrics service logic (#451): parameter defaulting/validation and
// gap-filling. The repository stays a thin cross-tenant reader returning sparse
// rows — the same division of labour as resource_access.go, whose documented
// contract is "repository returns sparse rows, caller zero-fills".

const (
	// adminTimeseriesDefaultDays is the range used when neither from nor to is given.
	adminTimeseriesDefaultDays = 30
	// adminTimeseriesMaxDays caps the span so an unbounded range cannot turn into
	// a full-table scan across six tables. ~10 years.
	adminTimeseriesMaxDays = 3660

	adminGranularityDay   = "day"
	adminGranularityWeek  = "week"
	adminGranularityMonth = "month"
)

// ErrAdminTimeseriesRange is returned for any invalid from/to/granularity
// combination. The handler maps it to 400 (never 500).
type ErrAdminTimeseriesRange struct {
	Detail string
}

func (e *ErrAdminTimeseriesRange) Error() string { return e.Detail }

// AdminTimeseriesQuery is the caller's requested range. Nil pointers mean
// "use the default"; Granularity is empty when unset.
type AdminTimeseriesQuery struct {
	From        *time.Time
	To          *time.Time
	Granularity string
}

// adminResolvedRange is a validated, bucket-aligned range.
type adminResolvedRange struct {
	from        time.Time
	to          time.Time
	granularity string
}

// resolveAdminRange applies the defaults, validates, and snaps `from` DOWN to a
// bucket boundary so the gap-filled buckets line up exactly with what
// date_trunc emits for the same range (otherwise the first bucket would be a
// partial one the SQL reports at the boundary and the fill would miss it).
func resolveAdminRange(q AdminTimeseriesQuery, now time.Time) (adminResolvedRange, error) {
	granularity := q.Granularity
	if granularity == "" {
		granularity = adminGranularityDay
	}
	switch granularity {
	case adminGranularityDay, adminGranularityWeek, adminGranularityMonth:
	default:
		return adminResolvedRange{}, &ErrAdminTimeseriesRange{
			Detail: fmt.Sprintf("invalid granularity %q: must be one of day, week, month", q.Granularity),
		}
	}

	to := now.UTC()
	if q.To != nil {
		to = q.To.UTC()
	}
	from := to.AddDate(0, 0, -adminTimeseriesDefaultDays)
	if q.From != nil {
		from = q.From.UTC()
	}

	if !to.After(from) {
		return adminResolvedRange{}, &ErrAdminTimeseriesRange{
			Detail: fmt.Sprintf("invalid range: to (%s) must be after from (%s)",
				to.Format(time.RFC3339), from.Format(time.RFC3339)),
		}
	}
	if to.Sub(from) > adminTimeseriesMaxDays*24*time.Hour {
		return adminResolvedRange{}, &ErrAdminTimeseriesRange{
			Detail: fmt.Sprintf("range too wide: at most %d days", adminTimeseriesMaxDays),
		}
	}

	return adminResolvedRange{
		from:        adminBucketStart(from, granularity),
		to:          to,
		granularity: granularity,
	}, nil
}

// adminBucketStart snaps t down to the start of its bucket, matching Postgres
// date_trunc semantics exactly: 'week' truncates to MONDAY (not Sunday), 'month'
// to the 1st, 'day' to midnight. All in UTC.
func adminBucketStart(t time.Time, granularity string) time.Time {
	t = t.UTC()
	switch granularity {
	case adminGranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	case adminGranularityWeek:
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		// Go's Sunday==0; Postgres weeks start Monday, so Sunday is 6 days in.
		offset := (int(day.Weekday()) + 6) % 7
		return day.AddDate(0, 0, -offset)
	default:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// adminNextBucket steps one bucket forward from a bucket start.
func adminNextBucket(t time.Time, granularity string) time.Time {
	switch granularity {
	case adminGranularityMonth:
		return t.AddDate(0, 1, 0)
	case adminGranularityWeek:
		return t.AddDate(0, 0, 7)
	default:
		return t.AddDate(0, 0, 1)
	}
}

// adminBuckets enumerates every bucket start in [from, to). from must already be
// bucket-aligned. Always returns at least one bucket for a non-empty range.
func adminBuckets(r adminResolvedRange) []time.Time {
	buckets := make([]time.Time, 0)
	for b := r.from; b.Before(r.to); b = adminNextBucket(b, r.granularity) {
		buckets = append(buckets, b)
	}
	return buckets
}

// GetDashboardOverview returns totals, breakdowns and system health.
func (s *AdminService) GetDashboardOverview(
	ctx context.Context, version string,
) (models.AdminDashboardOverview, error) {
	counts, err := s.adminRepo.GetExtendedCounts(ctx)
	if err != nil {
		return models.AdminDashboardOverview{}, err
	}
	breakdowns, err := s.adminRepo.GetEntityBreakdowns(ctx)
	if err != nil {
		return models.AdminDashboardOverview{}, err
	}
	health, err := s.adminRepo.GetSystemHealth(ctx)
	if err != nil {
		return models.AdminDashboardOverview{}, err
	}

	return models.AdminDashboardOverview{
		Counts:       counts,
		Breakdowns:   breakdowns,
		SystemHealth: health,
		Version:      version,
	}, nil
}

// GetDashboardTimeseries resolves the range, pulls the three sparse series, and
// gap-fills each so every bucket is present with an explicit 0.
func (s *AdminService) GetDashboardTimeseries(
	ctx context.Context, q AdminTimeseriesQuery, window models.AdminDataWindow,
) (models.AdminTimeseries, error) {
	resolved, err := resolveAdminRange(q, time.Now())
	if err != nil {
		return models.AdminTimeseries{}, err
	}

	growthRows, err := s.adminRepo.GetGrowthSeries(ctx, resolved.from, resolved.to, resolved.granularity)
	if err != nil {
		return models.AdminTimeseries{}, err
	}
	signInRows, err := s.adminRepo.GetSignInSeries(ctx, resolved.from, resolved.to, resolved.granularity)
	if err != nil {
		return models.AdminTimeseries{}, err
	}
	accessRows, err := s.adminRepo.GetAccessBySourceSeries(ctx, resolved.from, resolved.to, resolved.granularity)
	if err != nil {
		return models.AdminTimeseries{}, err
	}

	buckets := adminBuckets(resolved)
	return models.AdminTimeseries{
		From:           resolved.from,
		To:             resolved.to,
		Granularity:    resolved.granularity,
		Growth:         fillGrowth(buckets, growthRows),
		SignIns:        fillCounts(buckets, signInRows),
		AccessBySource: fillSources(buckets, accessRows),
		DataWindow:     window,
	}, nil
}

// fillGrowth pivots the sparse (entity, bucket, count) rows into one point per
// bucket, with every entity present as an explicit 0 when it had no rows.
func fillGrowth(buckets []time.Time, rows []models.AdminGrowthCount) []models.AdminGrowthPoint {
	byBucket := make(map[time.Time]*models.AdminGrowthPoint, len(buckets))
	points := make([]models.AdminGrowthPoint, len(buckets))
	for i, b := range buckets {
		points[i] = models.AdminGrowthPoint{Bucket: b}
		byBucket[b] = &points[i]
	}

	for _, row := range rows {
		point, ok := byBucket[row.Bucket.UTC()]
		if !ok {
			// A bucket outside the enumerated range can only appear if the SQL and
			// the Go bucketing disagreed; dropping it keeps the series contiguous.
			continue
		}
		switch row.Entity {
		case "users":
			point.Users = row.Count
		case "teams":
			point.Teams = row.Count
		case "projects":
			point.Projects = row.Count
		case "prompts":
			point.Prompts = row.Count
		case "artifacts":
			point.Artifacts = row.Count
		case "memories":
			point.Memories = row.Count
		}
	}
	return points
}

// fillCounts zero-fills a single-value series across every bucket.
func fillCounts(buckets []time.Time, rows []models.AdminCountPoint) []models.AdminCountPoint {
	counts := make(map[time.Time]int64, len(rows))
	for _, row := range rows {
		counts[row.Bucket.UTC()] = row.Count
	}

	points := make([]models.AdminCountPoint, len(buckets))
	for i, b := range buckets {
		points[i] = models.AdminCountPoint{Bucket: b, Count: counts[b]}
	}
	return points
}

// fillSources zero-fills the per-source series. Only sources actually observed
// in the range are emitted (there is no fixed source enum to enumerate), but
// each observed source gets a point in EVERY bucket so a client can plot one
// contiguous line per source.
func fillSources(buckets []time.Time, rows []models.AdminSourcePoint) []models.AdminSourcePoint {
	type key struct {
		bucket time.Time
		source string
	}
	counts := make(map[key]int64, len(rows))
	sources := make([]string, 0)
	seen := make(map[string]struct{})
	for _, row := range rows {
		counts[key{row.Bucket.UTC(), row.Source}] = row.Count
		if _, ok := seen[row.Source]; !ok {
			seen[row.Source] = struct{}{}
			sources = append(sources, row.Source)
		}
	}
	// Rows arrive ordered by (bucket, source), so `sources` is in first-appearance
	// order; sort it so the emitted order is deterministic regardless of which
	// bucket a given source first showed up in.
	sort.Strings(sources)

	points := make([]models.AdminSourcePoint, 0, len(buckets)*len(sources))
	for _, b := range buckets {
		for _, src := range sources {
			points = append(points, models.AdminSourcePoint{
				Bucket: b, Source: src, Count: counts[key{b, src}],
			})
		}
	}
	return points
}

// AdminDataWindowFor derives the retention window from the configured TTLs.
func AdminDataWindowFor(now time.Time, activityDays, accessEventDays int) models.AdminDataWindow {
	utc := now.UTC()
	return models.AdminDataWindow{
		SignInsEarliestRetainedAt:        utc.AddDate(0, 0, -activityDays),
		AccessBySourceEarliestRetainedAt: utc.AddDate(0, 0, -accessEventDays),
	}
}
