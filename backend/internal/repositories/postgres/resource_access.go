package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// resourceAccessRepository implements the ResourceAccessRepository interface using PostgreSQL.
type resourceAccessRepository struct {
	db *database.DB
}

// NewResourceAccessRepository creates a new PostgreSQL resource access repository.
func NewResourceAccessRepository(db *database.DB) repositories.ResourceAccessRepository {
	return &resourceAccessRepository{db: db}
}

// Create persists a new resource access event and back-fills the generated id and created_at.
func (r *resourceAccessRepository) Create(ctx context.Context, event *models.ResourceAccessEvent) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `
		INSERT INTO resource_access_events
		(team_id, user_id, resource_type, resource_id, source, api_key_id, user_agent, source_ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`

	err := r.db.QueryRowContext(ctx, query,
		event.TeamID,
		event.UserID,
		event.ResourceType,
		event.ResourceID,
		event.Source,
		event.APIKeyID,
		event.UserAgent,
		event.SourceIP,
	).Scan(&event.ID, &event.CreatedAt)

	if err != nil {
		logrus.WithError(err).Error("Failed to create resource access event")
		return fmt.Errorf("failed to create resource access event: %w", err)
	}

	return nil
}

// GetMetricsByResource returns daily access counts grouped by source for a specific resource.
func (r *resourceAccessRepository) GetMetricsByResource(
	ctx context.Context,
	teamID, resourceType, resourceID string,
	since time.Time,
) ([]models.DailyAccessCount, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// DATE(created_at) buckets each TIMESTAMPTZ in the connection's session timezone
	// (UTC in production), so day boundaries are UTC-based. Downstream pivot/zero-fill
	// (issue #1452) must build its date series on the same UTC basis.
	//
	// The bucket is rendered as text with TO_CHAR(..., 'YYYY-MM-DD') rather than returned
	// as a raw SQL date. A bare `date` is decoded by lib/pq into a time.Time and then
	// formatted into DailyAccessCount.Date (a string) as RFC3339 — "2026-05-31T00:00:00Z" —
	// which never matches the "2006-01-02" keys the zero-fill series builds, so every day's
	// count is silently dropped and every resource reports zero accesses. Emitting text in
	// the exact series layout keeps the pivot keys aligned.
	query := `
		SELECT TO_CHAR(DATE(created_at), 'YYYY-MM-DD') AS date, source, COUNT(*) AS count
		FROM resource_access_events
		WHERE team_id = $1 AND resource_type = $2 AND resource_id = $3 AND created_at >= $4
		GROUP BY DATE(created_at), source
		ORDER BY DATE(created_at), source`

	rows, err := r.db.QueryContext(ctx, query, teamID, resourceType, resourceID, since)
	if err != nil {
		logrus.WithError(err).Error("Failed to query resource access metrics")
		return nil, fmt.Errorf("failed to query resource access metrics: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	var counts []models.DailyAccessCount
	for rows.Next() {
		var item models.DailyAccessCount
		if scanErr := rows.Scan(&item.Date, &item.Source, &item.Count); scanErr != nil {
			logrus.WithError(scanErr).Error("Failed to scan resource access metric row")
			return nil, fmt.Errorf("failed to scan resource access metric: %w", scanErr)
		}
		counts = append(counts, item)
	}

	if err := rows.Err(); err != nil {
		logrus.WithError(err).Error("Failed to iterate resource access metric rows")
		return nil, fmt.Errorf("failed to iterate resource access metrics: %w", err)
	}

	if counts == nil {
		counts = []models.DailyAccessCount{}
	}

	return counts, nil
}

// GetTeamMetrics returns daily access counts grouped by source across the whole
// team (every resource), for the team analytics page. Unlike GetMetricsByResource
// it is not scoped to a single resource_type/resource_id — it aggregates all of a
// team's resource_access_events. Sparse rows are returned; the caller zero-fills.
func (r *resourceAccessRepository) GetTeamMetrics(
	ctx context.Context,
	teamID string,
	since time.Time,
) ([]models.DailyAccessCount, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// See GetMetricsByResource: DATE(created_at) buckets in UTC and the bucket is
	// rendered as text in the exact "YYYY-MM-DD" series layout so the downstream
	// zero-fill keys align.
	query := `
		SELECT TO_CHAR(DATE(created_at), 'YYYY-MM-DD') AS date, source, COUNT(*) AS count
		FROM resource_access_events
		WHERE team_id = $1 AND created_at >= $2
		GROUP BY DATE(created_at), source
		ORDER BY DATE(created_at), source`

	rows, err := r.db.QueryContext(ctx, query, teamID, since)
	if err != nil {
		logrus.WithError(err).Error("Failed to query team resource access metrics")
		return nil, fmt.Errorf("failed to query team resource access metrics: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	var counts []models.DailyAccessCount
	for rows.Next() {
		var item models.DailyAccessCount
		if scanErr := rows.Scan(&item.Date, &item.Source, &item.Count); scanErr != nil {
			logrus.WithError(scanErr).Error("Failed to scan team resource access metric row")
			return nil, fmt.Errorf("failed to scan team resource access metric: %w", scanErr)
		}
		counts = append(counts, item)
	}

	if err := rows.Err(); err != nil {
		logrus.WithError(err).Error("Failed to iterate team resource access metric rows")
		return nil, fmt.Errorf("failed to iterate team resource access metrics: %w", err)
	}

	if counts == nil {
		counts = []models.DailyAccessCount{}
	}

	return counts, nil
}

// GetTopAccessedResources returns the team's most-accessed resources since the
// given time, ranked by access count descending and capped at `limit`. Each row's
// display name is resolved by left-joining the owning resource table per type:
// prompts/projects expose `name`, artifacts/blueprints expose `title`, and memory
// uses a truncated `text` (no name column). COALESCE to an empty string guarantees
// a non-null name even for an unknown type ('agent') or a since-deleted resource, so
// the scan target is a plain string. Ties break on resource_id for a stable ordering.
func (r *resourceAccessRepository) GetTopAccessedResources(
	ctx context.Context,
	teamID string,
	since time.Time,
	source string,
	limit int,
) ([]models.TopAccessedResource, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query, args := buildTopAccessedQuery(teamID, since, source, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		logrus.WithError(err).Error("Failed to query top accessed resources")
		return nil, fmt.Errorf("failed to query top accessed resources: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	items := []models.TopAccessedResource{}
	for rows.Next() {
		var item models.TopAccessedResource
		if scanErr := rows.Scan(&item.ResourceType, &item.ResourceID, &item.AccessCount, &item.Name); scanErr != nil {
			logrus.WithError(scanErr).Error("Failed to scan top accessed resource row")
			return nil, fmt.Errorf("failed to scan top accessed resource: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logrus.WithError(err).Error("Failed to iterate top accessed resource rows")
		return nil, fmt.Errorf("failed to iterate top accessed resources: %w", err)
	}

	return items, nil
}

// buildTopAccessedQuery assembles the ranked-resources SQL and its positional args
// with an optional access-channel filter. An empty or "all" source preserves the
// original aggregate ranking; a concrete source adds an `AND source = $N` predicate
// before grouping, shifting the LIMIT placeholder by one. Args are returned in
// placeholder order.
func buildTopAccessedQuery(teamID string, since time.Time, source string, limit int) (string, []interface{}) {
	args := []interface{}{teamID, since}
	sourceFilter := ""
	if source != "" && source != "all" {
		args = append(args, source)
		sourceFilter = fmt.Sprintf(" AND source = $%d", len(args))
	}
	args = append(args, limit)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))

	query := fmt.Sprintf(`
		WITH ranked AS (
			SELECT resource_type, resource_id, COUNT(*) AS access_count
			FROM resource_access_events
			WHERE team_id = $1 AND created_at >= $2%s
			GROUP BY resource_type, resource_id
			ORDER BY access_count DESC, resource_id
			LIMIT %s
		)
		SELECT
			ranked.resource_type,
			ranked.resource_id::text,
			ranked.access_count,
			COALESCE(p.name, a.title, b.title, LEFT(m.text, 120), pr.name, '') AS name
		FROM ranked
		LEFT JOIN prompts p     ON ranked.resource_type = 'prompt'    AND p.id = ranked.resource_id
		LEFT JOIN artifacts a   ON ranked.resource_type = 'artifact'  AND a.id = ranked.resource_id
		LEFT JOIN blueprints b  ON ranked.resource_type = 'blueprint' AND b.id = ranked.resource_id
		LEFT JOIN memories m    ON ranked.resource_type = 'memory'    AND m.id = ranked.resource_id
		LEFT JOIN projects pr   ON ranked.resource_type = 'project'   AND pr.id = ranked.resource_id
		ORDER BY ranked.access_count DESC, ranked.resource_id`, sourceFilter, limitPlaceholder)

	return query, args
}

// resourceAccessRetentionBatchSize is the number of rows deleted per batch in DeleteOlderThan.
// Keeps each DELETE transaction short to reduce WAL pressure and lock contention.
const resourceAccessRetentionBatchSize = 10000

// resourceAccessRetentionAdvisoryLockID is the Postgres advisory lock key for the retention job.
// Prevents concurrent runs (e.g. Cloud Scheduler at-least-once delivery on multi-instance Cloud Run).
const resourceAccessRetentionAdvisoryLockID = 7391029 // arbitrary stable constant for resource access retention

// DeleteOlderThan deletes resource access event rows with created_at before the given cutoff time in batches.
// Uses a Postgres advisory lock to prevent concurrent execution across Cloud Run instances.
// Returns the total number of rows deleted.
func (r *resourceAccessRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	if r.db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	// Acquire advisory lock; bail early if another instance already holds it.
	var locked bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock($1)`, resourceAccessRetentionAdvisoryLockID,
	).Scan(&locked); err != nil {
		return 0, fmt.Errorf("delete old resource access events: acquire advisory lock: %w", err)
	}
	if !locked {
		return 0, nil // Another instance is already running the job — skip silently.
	}
	defer func() {
		// Best-effort unlock; the lock auto-releases when the connection closes anyway.
		if _, err := r.db.ExecContext(ctx,
			`SELECT pg_advisory_unlock($1)`, resourceAccessRetentionAdvisoryLockID); err != nil {
			return
		}
	}()

	var total int64
	for {
		res, err := r.db.ExecContext(ctx,
			`DELETE FROM resource_access_events `+
				`WHERE id IN (SELECT id FROM resource_access_events WHERE created_at < $1 LIMIT $2)`,
			before, resourceAccessRetentionBatchSize,
		)
		if err != nil {
			return total, fmt.Errorf("delete old resource access events: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("delete old resource access events: get rows affected: %w", err)
		}
		total += n
		if n < resourceAccessRetentionBatchSize {
			break // Last batch — no more rows to delete.
		}
	}

	return total, nil
}
