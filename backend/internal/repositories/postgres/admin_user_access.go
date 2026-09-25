package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// Per-user resource access analytics for the instance-admin surface (#1136).
// Both queries are keyed on resource_access_events.user_id with no membership
// check, and both lead with the (user_id, created_at) predicate that
// idx_rae_user_created (migration 019) serves. No query selects a title, slug
// or body column. resource_access_events.created_at is the AWARE family.

// adminUserAccessBySourceQueryFmt is adminAccessBySourceQueryFmt scoped to one
// user. The single %[1]s is the date_trunc unit from adminTruncUnit's allowlist.
const adminUserAccessBySourceQueryFmt = `
SELECT date_trunc('%[1]s', created_at AT TIME ZONE 'UTC') AS bucket, source, COUNT(*) AS count
FROM resource_access_events
WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
GROUP BY bucket, source
ORDER BY bucket, source
`

// GetUserAccessBySourceSeries returns sparse (bucket, source, count) rows of
// one user's resource accesses in [from, to).
func (r *AdminRepository) GetUserAccessBySourceSeries(
	ctx context.Context, userID string, from, to time.Time, granularity string,
) ([]models.AdminSourcePoint, error) {
	query := fmt.Sprintf(adminUserAccessBySourceQueryFmt, adminTruncUnit(granularity))
	rows, err := r.db.QueryContext(ctx, query, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query user access-by-source series: %w", err)
	}
	defer closeAdminRows(rows, "user access-by-source series")

	points := make([]models.AdminSourcePoint, 0)
	for rows.Next() {
		var p models.AdminSourcePoint
		if scanErr := rows.Scan(&p.Bucket, &p.Source, &p.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan user access-by-source row: %w", scanErr)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate user access-by-source series: %w", err)
	}
	return points, nil
}

// adminUserTopAccessedQuery ranks one user's accessed resources, then resolves
// each to its team and project WITHOUT any title: the resource tables are
// joined for project_id (and existence) only. A `project` row resolves to the
// project itself; an agent has no project. resource_id has no FK, so a row
// whose resource was deleted joins nothing and reports resource_deleted.
const adminUserTopAccessedQuery = `
WITH ranked AS (
	SELECT team_id, resource_type, resource_id, COUNT(*) AS access_count
	FROM resource_access_events
	WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	GROUP BY team_id, resource_type, resource_id
	ORDER BY access_count DESC, resource_id
	LIMIT $4
)
SELECT r.resource_type, r.resource_id, r.team_id, t.name,
	COALESCE(p.project_id, a.project_id, b.project_id, m.project_id, pr.id) AS project_id,
	proj.name,
	CASE r.resource_type
		WHEN 'prompt' THEN p.id IS NULL
		WHEN 'artifact' THEN a.id IS NULL
		WHEN 'blueprint' THEN b.id IS NULL
		WHEN 'memory' THEN m.id IS NULL
		WHEN 'project' THEN pr.id IS NULL
		WHEN 'agent' THEN ag.id IS NULL
		ELSE TRUE
	END AS resource_deleted,
	r.access_count
FROM ranked r
JOIN teams t ON t.id = r.team_id
LEFT JOIN prompts p ON r.resource_type = 'prompt' AND p.id = r.resource_id
LEFT JOIN artifacts a ON r.resource_type = 'artifact' AND a.id = r.resource_id
LEFT JOIN blueprints b ON r.resource_type = 'blueprint' AND b.id = r.resource_id
LEFT JOIN memories m ON r.resource_type = 'memory' AND m.id = r.resource_id
LEFT JOIN projects pr ON r.resource_type = 'project' AND pr.id = r.resource_id
LEFT JOIN agents ag ON r.resource_type = 'agent' AND ag.id = r.resource_id
LEFT JOIN projects proj ON proj.id = COALESCE(p.project_id, a.project_id, b.project_id, m.project_id, pr.id)
ORDER BY r.access_count DESC, r.resource_id
`

// GetUserTopAccessedResources returns up to limit of one user's most-accessed
// resources in [from, to), ranked by access count with resource id as the
// stable tie-break.
func (r *AdminRepository) GetUserTopAccessedResources(
	ctx context.Context, userID string, from, to time.Time, limit int,
) ([]models.AdminTopAccessedResource, error) {
	rows, err := r.db.QueryContext(ctx, adminUserTopAccessedQuery, userID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query user top accessed resources: %w", err)
	}
	defer closeAdminRows(rows, "user top accessed resources")

	items := make([]models.AdminTopAccessedResource, 0)
	for rows.Next() {
		var it models.AdminTopAccessedResource
		if scanErr := rows.Scan(&it.ResourceType, &it.ResourceID, &it.TeamID, &it.TeamName,
			&it.ProjectID, &it.ProjectName, &it.ResourceDeleted, &it.AccessCount); scanErr != nil {
			return nil, fmt.Errorf("failed to scan user top accessed resource: %w", scanErr)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate user top accessed resources: %w", err)
	}
	return items, nil
}
