package services

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// freshnessMetricsRanges are the reporting windows the over-time metric
// accepts, mirroring the other analytics endpoints so one range selector drives
// every chart. Kept in step with the FreshnessMetricsRange enum in the spec.
var freshnessMetricsRanges = map[string]int{
	"7d":   7,
	"14d":  14,
	"30d":  30,
	"60d":  60,
	"90d":  90,
	"180d": 180,
}

// DefaultFreshnessMetricsRange is used when the range parameter is omitted.
const DefaultFreshnessMetricsRange = "30d"

// freshnessDateLayout renders a series bucket. It must match the layout the
// repository's TO_CHAR produces, or the zero-fill keys will not align.
const freshnessDateLayout = "2006-01-02"

// FreshnessMetricsRangeDays resolves a range option to a number of days,
// reporting whether the option is known. Handlers use the false return to
// produce a 400 rather than silently defaulting, so a typo in a saved
// dashboard URL is visible instead of quietly showing the wrong window.
func FreshnessMetricsRangeDays(rangeStr string) (int, bool) {
	if rangeStr == "" {
		rangeStr = DefaultFreshnessMetricsRange
	}
	days, ok := freshnessMetricsRanges[rangeStr]
	return days, ok
}

// GetOverTimeMetrics builds the daily freshness activity series.
//
// The flows (marked/cleared) come from the audit log rather than from sampling
// the current state, so the history is what actually happened. The level
// (stale_total) is then reconstructed by walking TODAY's live count backwards
// through those flows: the count at the end of day D is the count at the end of
// D+1 minus what was marked on D+1 plus what was cleared on D+1.
//
// That reconstruction is exact only for the period the audit log covers. Before
// the team's first evaluation run there are no transitions, so the walk simply
// carries the earliest known level backwards — the series reads flat rather
// than falsely dropping to zero. There is deliberately no backfill (decision
// #7), so early windows will look quiet; that is accurate, not a bug.
func (s *FreshnessService) GetOverTimeMetrics(
	ctx context.Context, teamID string, rangeDays int,
) (*models.FreshnessOverTimeMetrics, error) {
	if rangeDays < 0 {
		rangeDays = 0
	}
	since := freshnessWindowStart(rangeDays)

	transitions, err := s.audit.CountTransitionsByDay(ctx, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetOverTimeMetrics: %w", err)
	}
	current, err := s.freshness.CountStale(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetOverTimeMetrics: %w", err)
	}

	return buildFreshnessOverTime(transitions, current, since, rangeDays), nil
}

// freshnessWindowStart truncates the window start to UTC midnight so the SQL
// predicate and the zero-filled series agree on which day the range opens with.
func freshnessWindowStart(rangeDays int) time.Time {
	start := time.Now().UTC().AddDate(0, 0, -rangeDays)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
}

// buildFreshnessOverTime zero-fills the series and reconstructs the daily
// level. It is separated from the repository calls so the arithmetic is
// testable without a database.
func buildFreshnessOverTime(
	transitions []models.FreshnessTransitionCount, current int, since time.Time, rangeDays int,
) *models.FreshnessOverTimeMetrics {
	marked := make(map[string]int, len(transitions))
	cleared := make(map[string]int, len(transitions))
	for _, t := range transitions {
		switch t.Action {
		case models.FreshnessActionMarked:
			marked[t.Date] += t.Count
		case models.FreshnessActionCleared:
			cleared[t.Date] += t.Count
		}
	}

	// The window is inclusive of both ends, matching the other analytics
	// series: a 30d range returns 31 points.
	days := make([]models.FreshnessDailyStale, 0, rangeDays+1)
	for offset := 0; offset <= rangeDays; offset++ {
		date := since.AddDate(0, 0, offset).Format(freshnessDateLayout)
		days = append(days, models.FreshnessDailyStale{
			Date:    date,
			Marked:  marked[date],
			Cleared: cleared[date],
		})
	}

	result := &models.FreshnessOverTimeMetrics{RangeDays: rangeDays, Days: days}

	// Walk backwards from today's live count. `level` is the count at the end
	// of the day being written; undoing that day's net change yields the level
	// at the end of the previous one.
	level := current
	for i := len(days) - 1; i >= 0; i-- {
		result.Days[i].StaleTotal = level
		result.TotalMarked += days[i].Marked
		result.TotalCleared += days[i].Cleared

		level -= days[i].Marked
		level += days[i].Cleared
		// A negative level means the audit log is missing transitions the
		// current count implies (rows removed by a team or project cascade,
		// which the log does not record). Clamp rather than render nonsense.
		if level < 0 {
			level = 0
		}
	}
	return result
}

// GetByTypeMetrics returns the current stale counts per resource type.
//
// All four types are always present: the repository can only report types that
// have something stale, and a bar disappearing from a chart reads as "no data"
// rather than "none stale".
func (s *FreshnessService) GetByTypeMetrics(
	ctx context.Context, teamID string,
) (*models.FreshnessTypeMetrics, error) {
	buckets, err := s.freshness.CountStaleByType(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetByTypeMetrics: %w", err)
	}

	byType := indexFreshnessBuckets(buckets)
	counts := make([]models.FreshnessBucketCount, 0, len(freshnessRuleResourceTypes))
	total := 0
	for _, resourceType := range freshnessRuleResourceTypes {
		counts = append(counts, models.FreshnessBucketCount{Key: resourceType, Count: byType[resourceType]})
		total += byType[resourceType]
	}

	return &models.FreshnessTypeMetrics{TotalStale: total, Counts: counts}, nil
}

// GetByProjectMetrics returns the current stale counts per project, including
// the projects with nothing stale — which is what makes "this project is
// clean" distinguishable from "this project was not returned".
func (s *FreshnessService) GetByProjectMetrics(
	ctx context.Context, teamID string,
) (*models.FreshnessProjectMetrics, error) {
	buckets, err := s.freshness.CountStaleByProject(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetByProjectMetrics: %w", err)
	}
	projects, err := s.projects.ListByTeamID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetByProjectMetrics: %w", err)
	}

	byProject := indexFreshnessBuckets(buckets)
	counts := make([]models.FreshnessProjectStale, 0, len(projects))
	total := 0
	for _, project := range projects {
		count := byProject[project.ID]
		counts = append(counts, models.FreshnessProjectStale{
			ProjectID: project.ID,
			Name:      project.Name,
			Slug:      project.Slug,
			Count:     count,
		})
		total += count
	}

	// Busiest first is what the chart reads top-down; name breaks ties so the
	// order is stable between requests rather than following the database.
	sort.SliceStable(counts, func(i, j int) bool {
		if counts[i].Count != counts[j].Count {
			return counts[i].Count > counts[j].Count
		}
		return counts[i].Name < counts[j].Name
	})

	return &models.FreshnessProjectMetrics{TotalStale: total, Counts: counts}, nil
}

// GetByRuleMetrics returns how many resources each of the team's rules
// currently marks — the view that makes an over-broad rule obvious.
//
// Disabled rules are included, reporting 0. Omitting them would make a
// disabled rule indistinguishable from a deleted one at exactly the moment
// someone is deciding whether the rule was the problem.
func (s *FreshnessService) GetByRuleMetrics(
	ctx context.Context, teamID string,
) (*models.FreshnessRuleMetrics, error) {
	buckets, err := s.freshness.CountStaleByRule(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetByRuleMetrics: %w", err)
	}
	rules, err := s.rules.ListByTeam(ctx, teamID, false)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetByRuleMetrics: %w", err)
	}
	// Distinct resources, not the sum of the per-rule counts: a resource
	// matched by two rules is one stale resource but two rule impacts.
	total, err := s.freshness.CountStale(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetByRuleMetrics: %w", err)
	}

	byRule := indexFreshnessBuckets(buckets)
	counts := make([]models.FreshnessRuleImpact, 0, len(rules))
	for _, rule := range rules {
		counts = append(counts, models.FreshnessRuleImpact{
			RuleID:        rule.ID,
			ProjectID:     rule.ProjectID,
			ResourceTypes: slices.Clone(rule.ResourceTypes),
			ThresholdDays: rule.ThresholdDays,
			Enabled:       rule.Enabled,
			Count:         byRule[rule.ID],
		})
	}

	sort.SliceStable(counts, func(i, j int) bool {
		if counts[i].Count != counts[j].Count {
			return counts[i].Count > counts[j].Count
		}
		return counts[i].RuleID < counts[j].RuleID
	})

	return &models.FreshnessRuleMetrics{TotalStale: total, Counts: counts}, nil
}

// ListAudit returns one page of the team's freshness audit log, newest first.
func (s *FreshnessService) ListAudit(
	ctx context.Context, teamID string, page, limit int,
) (*models.FreshnessAuditPage, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultFreshnessAuditPageSize
	}

	entries, total, err := s.audit.ListByTeam(ctx, teamID, limit, (page-1)*limit)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.ListAudit: %w", err)
	}

	return &models.FreshnessAuditPage{
		Entries:    entries,
		TotalCount: total,
		Page:       page,
		PerPage:    limit,
	}, nil
}

// defaultFreshnessAuditPageSize matches the spec's default for the `limit`
// query parameter.
const defaultFreshnessAuditPageSize = 20

// indexFreshnessBuckets turns the repository's sparse rows into a lookup the
// zero-fill loops can index without a linear scan per bucket.
func indexFreshnessBuckets(buckets []models.FreshnessBucketCount) map[string]int {
	byKey := make(map[string]int, len(buckets))
	for _, b := range buckets {
		byKey[strings.TrimSpace(b.Key)] = b.Count
	}
	return byKey
}
