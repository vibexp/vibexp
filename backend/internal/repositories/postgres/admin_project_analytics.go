package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// Project detail analytics for the instance-admin surface (#1145). Every query
// here is keyed on a project id with no membership check. No query selects a
// title, slug or body column.
//
// Only the five tables with a direct project_id count per project (prompts,
// memories, artifacts, blueprints, feed_items); the timestamp-family rules of
// admin_dashboard.go's header apply, so `memories` is the naive branch and
// feed_items buckets on posted_at.
//
// resource_access_events has no project_id, so access is attributed by joining
// (resource_type, resource_id) to the resource's CURRENT project through
// adminProjectResourcesCTE. Events for deleted resources drop out and a
// migrated resource brings its history along; project migration is same-team
// only, so filtering events on the project's team_id stays correct and lets
// idx_rae_resource_created (team_id, resource_type, resource_id, created_at)
// serve each probe.

// ProjectTeamID returns the team of a project; found is false for an unknown id.
func (r *AdminRepository) ProjectTeamID(ctx context.Context, id string) (string, bool, error) {
	var teamID string
	err := r.db.QueryRowContext(ctx, "SELECT team_id FROM projects WHERE id = $1", id).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to look up admin project team: %w", err)
	}
	return teamID, true, nil
}

// adminProjectCreationQueryFmt counts a project's newly created resources per
// type per bucket. It follows adminUserCreationQueryFmt: `$2`/`$3` bound the
// aware branches and the naive `memories` branch gets its own `$4`/`$5`, bound
// to the same instants in UTC (lib/pq allows one type per placeholder).
//
// The single %[1]s is the date_trunc unit from adminTruncUnit's allowlist.
const adminProjectCreationQueryFmt = `
SELECT resource_type, bucket, COUNT(*) AS count FROM (
	SELECT 'prompt' AS resource_type, date_trunc('%[1]s', created_at AT TIME ZONE 'UTC') AS bucket
		FROM prompts WHERE project_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'memory', date_trunc('%[1]s', created_at)
		FROM memories WHERE project_id = $1 AND created_at >= $4::timestamp AND created_at < $5::timestamp
	UNION ALL
	SELECT 'artifact', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM artifacts WHERE project_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'blueprint', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM blueprints WHERE project_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'feed_item', date_trunc('%[1]s', posted_at AT TIME ZONE 'UTC')
		FROM feed_items WHERE project_id = $1 AND posted_at >= $2 AND posted_at < $3
) g
GROUP BY resource_type, bucket
ORDER BY bucket, resource_type
`

// GetProjectCreationSeries returns sparse (type, bucket, count) rows of the
// resources created in a project in [from, to); the caller pivots and
// gap-fills. The Entity field of each row carries the singular resource type.
func (r *AdminRepository) GetProjectCreationSeries(
	ctx context.Context, projectID string, from, to time.Time, granularity string,
) ([]models.AdminGrowthCount, error) {
	query := fmt.Sprintf(adminProjectCreationQueryFmt, adminTruncUnit(granularity))
	rows, err := r.db.QueryContext(ctx, query, projectID, from, to, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("failed to query project creation series: %w", err)
	}
	defer closeAdminRows(rows, "project creation series")

	counts := make([]models.AdminGrowthCount, 0)
	for rows.Next() {
		var c models.AdminGrowthCount
		if scanErr := rows.Scan(&c.Entity, &c.Bucket, &c.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan project creation row: %w", scanErr)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate project creation series: %w", err)
	}
	return counts, nil
}

// adminProjectResourcesCTE lists the project's current resources of every
// access-recorded, project-scoped type, plus the project itself. `$1` is only
// ever compared with a uuid column, so it keeps one inferred type.
const adminProjectResourcesCTE = `
WITH project_resources AS (
	SELECT 'prompt'::text AS resource_type, id AS resource_id FROM prompts WHERE project_id = $1
	UNION ALL SELECT 'artifact', id FROM artifacts WHERE project_id = $1
	UNION ALL SELECT 'blueprint', id FROM blueprints WHERE project_id = $1
	UNION ALL SELECT 'memory', id FROM memories WHERE project_id = $1
	UNION ALL SELECT 'project', id FROM projects WHERE id = $1
)`

// adminProjectAccessBySourceQueryFmt is adminAccessBySourceQueryFmt scoped to
// one project's resources. The single %[1]s is the date_trunc unit from
// adminTruncUnit's allowlist.
const adminProjectAccessBySourceQueryFmt = adminProjectResourcesCTE + `
SELECT date_trunc('%[1]s', e.created_at AT TIME ZONE 'UTC') AS bucket, e.source, COUNT(*) AS count
FROM resource_access_events e
JOIN project_resources r ON r.resource_type = e.resource_type AND r.resource_id = e.resource_id
WHERE e.team_id = $2 AND e.created_at >= $3 AND e.created_at < $4
GROUP BY bucket, e.source
ORDER BY bucket, e.source
`

// GetProjectAccessBySourceSeries returns sparse (bucket, source, count) rows of
// the accesses to a project's resources and to the project itself in
// [from, to). teamID must be the project's own team.
func (r *AdminRepository) GetProjectAccessBySourceSeries(
	ctx context.Context, projectID, teamID string, from, to time.Time, granularity string,
) ([]models.AdminSourcePoint, error) {
	query := fmt.Sprintf(adminProjectAccessBySourceQueryFmt, adminTruncUnit(granularity))
	rows, err := r.db.QueryContext(ctx, query, projectID, teamID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query project access-by-source series: %w", err)
	}
	defer closeAdminRows(rows, "project access-by-source series")

	points := make([]models.AdminSourcePoint, 0)
	for rows.Next() {
		var p models.AdminSourcePoint
		if scanErr := rows.Scan(&p.Bucket, &p.Source, &p.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan project access-by-source row: %w", scanErr)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate project access-by-source series: %w", err)
	}
	return points, nil
}

// adminProjectTopAccessedQuery ranks a project's accessed resources, excluding
// the project's own page, then resolves only the team and project NAMES — never
// a resource title. Every ranked resource exists (it joined the CTE), so
// resource_deleted is constant false.
const adminProjectTopAccessedQuery = adminProjectResourcesCTE + `,
ranked AS (
	SELECT e.resource_type, e.resource_id, COUNT(*) AS access_count
	FROM resource_access_events e
	JOIN project_resources r ON r.resource_type = e.resource_type AND r.resource_id = e.resource_id
	WHERE e.team_id = $2 AND e.created_at >= $3 AND e.created_at < $4 AND r.resource_type <> 'project'
	GROUP BY e.resource_type, e.resource_id
	ORDER BY access_count DESC, e.resource_id
	LIMIT $5
)
SELECT k.resource_type, k.resource_id, p.team_id, t.name, p.id, p.name, FALSE AS resource_deleted, k.access_count
FROM ranked k
JOIN projects p ON p.id = $1
JOIN teams t ON t.id = p.team_id
ORDER BY k.access_count DESC, k.resource_id
`

// GetProjectTopAccessedResources returns up to limit of a project's
// most-accessed resources in [from, to), ranked by access count with resource
// id as the stable tie-break. teamID must be the project's own team.
func (r *AdminRepository) GetProjectTopAccessedResources(
	ctx context.Context, projectID, teamID string, from, to time.Time, limit int,
) ([]models.AdminTopAccessedResource, error) {
	rows, err := r.db.QueryContext(ctx, adminProjectTopAccessedQuery, projectID, teamID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query project top accessed resources: %w", err)
	}
	defer closeAdminRows(rows, "project top accessed resources")

	items := make([]models.AdminTopAccessedResource, 0)
	for rows.Next() {
		var it models.AdminTopAccessedResource
		if scanErr := rows.Scan(&it.ResourceType, &it.ResourceID, &it.TeamID, &it.TeamName,
			&it.ProjectID, &it.ProjectName, &it.ResourceDeleted, &it.AccessCount); scanErr != nil {
			return nil, fmt.Errorf("failed to scan project top accessed resource: %w", scanErr)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate project top accessed resources: %w", err)
	}
	return items, nil
}
