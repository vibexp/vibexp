package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Dashboard metrics for the instance-admin surface (#451). Everything here is
// computed on demand — there are no rollup or snapshot tables.
//
// # UTC bucketing across two timestamp families
//
// The tables aggregated below do NOT agree on their created_at type
// (migrations/001_baseline.up.sql): `memories` and `activities` are
// `timestamp without time zone`, while `users`, `teams`, `projects`, `prompts`,
// `artifacts` and `resource_access_events` are `timestamp with time zone`.
//
// `AT TIME ZONE 'UTC'` is OVERLOADED and does opposite things to the two:
// applied to an aware value it CONVERTS to a UTC-naive timestamp; applied to a
// naive value it ASSUMES UTC and produces an aware value. Applying it blindly to
// both families would silently offset one of them by the server's timezone. So:
// aware columns are converted (`created_at AT TIME ZONE 'UTC'`), naive columns
// are taken as written (they are produced by CURRENT_TIMESTAMP on a
// UTC-configured server). Both then yield a UTC wall-clock the buckets share.
//
// Range predicates stay on the RAW column of each branch so the created_at
// indexes remain usable; only the bucket expression is normalized. A naive
// column is therefore bounded with `$n::timestamp` (the cast drops the offset
// lib/pq sends, making the comparison independent of the session timezone).

// adminTruncUnit maps the granularity enum to the date_trunc unit literal. This
// is an SQL-injection control: the request's granularity is never interpolated,
// only one of these three constants, chosen by the switch. Anything unrecognized
// falls back to "day" (the service rejects out-of-enum values before this).
func adminTruncUnit(granularity string) string {
	switch granularity {
	case "week":
		return "week"
	case "month":
		return "month"
	default:
		return "day"
	}
}

// adminExtendedCountsQuery gathers every instance-wide total in one round-trip
// via correlated scalar subqueries. Table names are hardcoded (not user input),
// so there is no injection surface.
const adminExtendedCountsQuery = `
SELECT
	(SELECT COUNT(*) FROM users)      AS users,
	(SELECT COUNT(*) FROM teams)      AS teams,
	(SELECT COUNT(*) FROM projects)   AS projects,
	(SELECT COUNT(*) FROM prompts)    AS prompts,
	(SELECT COUNT(*) FROM artifacts)  AS artifacts,
	(SELECT COUNT(*) FROM memories)   AS memories,
	(SELECT COUNT(*) FROM blueprints) AS blueprints,
	(SELECT COUNT(*) FROM agents)     AS agents,
	(SELECT COUNT(*) FROM feeds)      AS feeds,
	(SELECT COUNT(*) FROM api_keys)   AS api_keys
`

// GetExtendedCounts returns unscoped totals for every top-level entity.
func (r *AdminRepository) GetExtendedCounts(ctx context.Context) (models.AdminExtendedCounts, error) {
	var c models.AdminExtendedCounts
	err := r.db.QueryRowContext(ctx, adminExtendedCountsQuery).Scan(
		&c.Users, &c.Teams, &c.Projects, &c.Prompts, &c.Artifacts,
		&c.Memories, &c.Blueprints, &c.Agents, &c.Feeds, &c.APIKeys,
	)
	if err != nil {
		return models.AdminExtendedCounts{}, fmt.Errorf("failed to query extended counts: %w", err)
	}
	return c, nil
}

// adminBreakdownsQuery groups every entity table that actually has a status or
// type column (verified against migrations/001_baseline.up.sql: prompts.status,
// artifacts.status/type, blueprints.status/type, agents.status). Entity and
// field labels are SQL literals, not user input. A NULL column value collapses
// to an empty string so the response field can stay a non-nullable string.
// (Do not write a bare two-quote literal in this doc comment: gofmt applies
// godoc's legacy quote conversion and rewrites it to a typographic quote.)
const adminBreakdownsQuery = `
SELECT entity, field, value, count FROM (
	SELECT 'prompts' AS entity, 'status' AS field, COALESCE(status, '') AS value, COUNT(*) AS count
		FROM prompts GROUP BY status
	UNION ALL
	SELECT 'artifacts', 'status', COALESCE(status, ''), COUNT(*) FROM artifacts GROUP BY status
	UNION ALL
	SELECT 'artifacts', 'type', COALESCE(type, ''), COUNT(*) FROM artifacts GROUP BY type
	UNION ALL
	SELECT 'blueprints', 'status', COALESCE(status, ''), COUNT(*) FROM blueprints GROUP BY status
	UNION ALL
	SELECT 'blueprints', 'type', COALESCE(type, ''), COUNT(*) FROM blueprints GROUP BY type
	UNION ALL
	SELECT 'agents', 'status', COALESCE(status, ''), COUNT(*) FROM agents GROUP BY status
) b
ORDER BY entity, field, count DESC, value
`

// GetEntityBreakdowns returns one AdminEntityBreakdown per entity/column pair,
// buckets ordered most frequent first. Pairs with no rows at all are omitted.
func (r *AdminRepository) GetEntityBreakdowns(ctx context.Context) ([]models.AdminEntityBreakdown, error) {
	rows, err := r.db.QueryContext(ctx, adminBreakdownsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity breakdowns: %w", err)
	}
	defer closeAdminRows(rows, "entity breakdown")

	breakdowns := make([]models.AdminEntityBreakdown, 0)
	for rows.Next() {
		var entity, field string
		var bucket models.AdminBreakdownBucket
		if scanErr := rows.Scan(&entity, &field, &bucket.Value, &bucket.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan entity breakdown: %w", scanErr)
		}
		// Rows arrive grouped by (entity, field), so appending to the last
		// breakdown is enough to assemble them.
		last := len(breakdowns) - 1
		if last < 0 || breakdowns[last].Entity != entity || breakdowns[last].Field != field {
			breakdowns = append(breakdowns, models.AdminEntityBreakdown{
				Entity:  entity,
				Field:   field,
				Buckets: make([]models.AdminBreakdownBucket, 0, 1),
			})
			last = len(breakdowns) - 1
		}
		breakdowns[last].Buckets = append(breakdowns[last].Buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate entity breakdowns: %w", err)
	}
	return breakdowns, nil
}

// adminTableStatsQuery reads ESTIMATED row counts from the stats collector.
// An exact COUNT(*) per table does not scale, and this figure is only used for
// relative sizing in the dashboard.
const adminTableStatsQuery = `
SELECT relname, GREATEST(n_live_tup, 0)
FROM pg_stat_user_tables
ORDER BY n_live_tup DESC, relname
`

// GetSystemHealth returns the database size plus per-table estimated row counts.
func (r *AdminRepository) GetSystemHealth(ctx context.Context) (models.AdminSystemHealth, error) {
	var health models.AdminSystemHealth
	if err := r.db.QueryRowContext(ctx,
		"SELECT pg_database_size(current_database())",
	).Scan(&health.DatabaseSizeBytes); err != nil {
		return models.AdminSystemHealth{}, fmt.Errorf("failed to query database size: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, adminTableStatsQuery)
	if err != nil {
		return models.AdminSystemHealth{}, fmt.Errorf("failed to query table stats: %w", err)
	}
	defer closeAdminRows(rows, "table stat")

	health.Tables = make([]models.AdminTableStat, 0)
	for rows.Next() {
		var stat models.AdminTableStat
		if scanErr := rows.Scan(&stat.Table, &stat.EstimatedRows); scanErr != nil {
			return models.AdminSystemHealth{}, fmt.Errorf("failed to scan table stat: %w", scanErr)
		}
		health.Tables = append(health.Tables, stat)
	}
	if err := rows.Err(); err != nil {
		return models.AdminSystemHealth{}, fmt.Errorf("failed to iterate table stats: %w", err)
	}
	return health, nil
}

// adminGrowthQueryFmt counts newly created rows per entity per bucket. Each
// UNION branch keeps its range predicate on the RAW created_at column so the
// per-table indexes stay usable, and normalizes only the bucket expression —
// see the file header on the two timestamp families. `memories` is the naive
// one here; every other branch is aware.
//
// The single %[1]s is the date_trunc unit, which comes from adminTruncUnit's
// allowlist and never from the request.
const adminGrowthQueryFmt = `
SELECT entity, bucket, COUNT(*) AS count FROM (
	SELECT 'users' AS entity, date_trunc('%[1]s', created_at AT TIME ZONE 'UTC') AS bucket
		FROM users WHERE created_at >= $1 AND created_at < $2
	UNION ALL
	SELECT 'teams', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM teams WHERE created_at >= $1 AND created_at < $2
	UNION ALL
	SELECT 'projects', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM projects WHERE created_at >= $1 AND created_at < $2
	UNION ALL
	SELECT 'prompts', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM prompts WHERE created_at >= $1 AND created_at < $2
	UNION ALL
	SELECT 'artifacts', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM artifacts WHERE created_at >= $1 AND created_at < $2
	UNION ALL
	SELECT 'memories', date_trunc('%[1]s', created_at)
		FROM memories WHERE created_at >= $1::timestamp AND created_at < $2::timestamp
) g
GROUP BY entity, bucket
ORDER BY bucket, entity
`

// GetGrowthSeries returns sparse (entity, bucket, count) rows for the range;
// the caller pivots and gap-fills them.
func (r *AdminRepository) GetGrowthSeries(
	ctx context.Context, from, to time.Time, granularity string,
) ([]models.AdminGrowthCount, error) {
	query := fmt.Sprintf(adminGrowthQueryFmt, adminTruncUnit(granularity))
	rows, err := r.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query growth series: %w", err)
	}
	defer closeAdminRows(rows, "growth series")

	counts := make([]models.AdminGrowthCount, 0)
	for rows.Next() {
		var c models.AdminGrowthCount
		if scanErr := rows.Scan(&c.Entity, &c.Bucket, &c.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan growth row: %w", scanErr)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate growth series: %w", err)
	}
	return counts, nil
}

// adminSignInQueryFmt counts auth_login activities per bucket. activities.created_at
// is the NAIVE family, so it is bucketed as-written and bounded with ::timestamp.
const adminSignInQueryFmt = `
SELECT date_trunc('%[1]s', created_at) AS bucket, COUNT(*) AS count
FROM activities
WHERE activity_type = $1 AND created_at >= $2::timestamp AND created_at < $3::timestamp
GROUP BY bucket
ORDER BY bucket
`

// GetSignInSeries returns sparse (bucket, count) rows of successful sign-ins.
func (r *AdminRepository) GetSignInSeries(
	ctx context.Context, from, to time.Time, granularity string,
) ([]models.AdminCountPoint, error) {
	query := fmt.Sprintf(adminSignInQueryFmt, adminTruncUnit(granularity))
	rows, err := r.db.QueryContext(ctx, query, activities.ActivityTypeAuthLogin, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query sign-in series: %w", err)
	}
	defer closeAdminRows(rows, "sign-in series")

	points := make([]models.AdminCountPoint, 0)
	for rows.Next() {
		var p models.AdminCountPoint
		if scanErr := rows.Scan(&p.Bucket, &p.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan sign-in row: %w", scanErr)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate sign-in series: %w", err)
	}
	return points, nil
}

// adminAccessBySourceQueryFmt counts resource accesses per bucket per source.
// resource_access_events.created_at is the AWARE family, so it is converted for
// bucketing while the predicate stays on the raw column.
const adminAccessBySourceQueryFmt = `
SELECT date_trunc('%[1]s', created_at AT TIME ZONE 'UTC') AS bucket, source, COUNT(*) AS count
FROM resource_access_events
WHERE created_at >= $1 AND created_at < $2
GROUP BY bucket, source
ORDER BY bucket, source
`

// GetAccessBySourceSeries returns sparse (bucket, source, count) rows.
func (r *AdminRepository) GetAccessBySourceSeries(
	ctx context.Context, from, to time.Time, granularity string,
) ([]models.AdminSourcePoint, error) {
	query := fmt.Sprintf(adminAccessBySourceQueryFmt, adminTruncUnit(granularity))
	rows, err := r.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query access-by-source series: %w", err)
	}
	defer closeAdminRows(rows, "access-by-source series")

	points := make([]models.AdminSourcePoint, 0)
	for rows.Next() {
		var p models.AdminSourcePoint
		if scanErr := rows.Scan(&p.Bucket, &p.Source, &p.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan access-by-source row: %w", scanErr)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate access-by-source series: %w", err)
	}
	return points, nil
}

// closeAdminRows closes a result set, logging (never returning) a close failure
// — the same contract the rest of this repository uses.
func closeAdminRows(rows interface{ Close() error }, what string) {
	if err := rows.Close(); err != nil {
		slog.Error("Failed to close admin rows", "rows", what, "error", err)
	}
}
