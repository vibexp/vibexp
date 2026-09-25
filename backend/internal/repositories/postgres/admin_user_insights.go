package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// User detail insights for the instance-admin surface (#1135). Every query here
// is scoped by AUTHORSHIP (the author column of each table: user_id, or
// feeds.created_by_user_id, or feed_items.posted_by_user_id) and never by
// membership, so a former member's resources still count. No query selects a
// title, slug or body column: rows carry ids, types, names of teams/projects
// and timestamps only.
//
// The timestamp-family rules of admin_dashboard.go's header apply: `memories`
// is the one naive (`timestamp without time zone`) table among the nine, and
// feed_items has no created_at at all — its creation time is posted_at.

// UserExists reports whether a user with that id exists.
func (r *AdminRepository) UserExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)", id,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check admin user existence: %w", err)
	}
	return exists, nil
}

// adminUserResourceCountsQuery counts a user's authored resources per
// (type, team, project). Only the four types with a NOT NULL project_id are
// attributed to a project; the others use NULL::uuid and count per team only.
// attachments.user_id is nullable (ON DELETE SET NULL), and a NULL author never
// equals $1, so those rows count for nobody — as in the admin user list.
const adminUserResourceCountsQuery = `
SELECT c.resource_type, c.team_id, t.name, (tm.user_id IS NOT NULL) AS is_member,
	c.project_id, p.name, c.n
FROM (
	SELECT 'prompt' AS resource_type, team_id, project_id, COUNT(*) AS n
		FROM prompts WHERE user_id = $1 GROUP BY team_id, project_id
	UNION ALL
	SELECT 'memory', team_id, project_id, COUNT(*)
		FROM memories WHERE user_id = $1 GROUP BY team_id, project_id
	UNION ALL
	SELECT 'artifact', team_id, project_id, COUNT(*)
		FROM artifacts WHERE user_id = $1 GROUP BY team_id, project_id
	UNION ALL
	SELECT 'blueprint', team_id, project_id, COUNT(*)
		FROM blueprints WHERE user_id = $1 GROUP BY team_id, project_id
	UNION ALL
	SELECT 'agent', team_id, NULL::uuid, COUNT(*)
		FROM agents WHERE user_id = $1 GROUP BY team_id
	UNION ALL
	SELECT 'feed', team_id, NULL::uuid, COUNT(*)
		FROM feeds WHERE created_by_user_id = $1 GROUP BY team_id
	UNION ALL
	SELECT 'feed_item', team_id, NULL::uuid, COUNT(*)
		FROM feed_items WHERE posted_by_user_id = $1 GROUP BY team_id
	UNION ALL
	SELECT 'comment', team_id, NULL::uuid, COUNT(*)
		FROM comments WHERE user_id = $1 GROUP BY team_id
	UNION ALL
	SELECT 'attachment', team_id, NULL::uuid, COUNT(*)
		FROM attachments WHERE user_id = $1 GROUP BY team_id
) c
JOIN teams t ON t.id = c.team_id
LEFT JOIN projects p ON p.id = c.project_id
LEFT JOIN team_members tm ON tm.team_id = c.team_id AND tm.user_id = $1
ORDER BY t.name, c.team_id, p.name NULLS FIRST, c.project_id, c.resource_type
`

// GetUserResourceCounts returns sparse (type, team, project) counts of the
// resources a user authored, ordered by team then project name. The caller
// pivots them into totals, teams and projects.
func (r *AdminRepository) GetUserResourceCounts(
	ctx context.Context, userID string,
) ([]models.AdminUserResourceCountRow, error) {
	rows, err := r.db.QueryContext(ctx, adminUserResourceCountsQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user resource counts: %w", err)
	}
	defer closeAdminRows(rows, "user resource counts")

	counts := make([]models.AdminUserResourceCountRow, 0)
	for rows.Next() {
		var c models.AdminUserResourceCountRow
		if scanErr := rows.Scan(
			&c.ResourceType, &c.TeamID, &c.TeamName, &c.IsMember, &c.ProjectID, &c.ProjectName, &c.Count,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan user resource count: %w", scanErr)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate user resource counts: %w", err)
	}
	return counts, nil
}

// adminUserCreationQueryFmt counts a user's newly created resources per type per
// bucket. It follows adminGrowthQueryFmt: range predicates stay on the raw
// column of each branch and only the bucket expression is normalized. `$2`/`$3`
// bound the eight aware branches; the naive `memories` branch gets its own
// `$4`/`$5`, bound to the same instants in UTC, because a placeholder shared
// with a timestamptz column would be inferred as timestamptz and the
// `::timestamp` cast would then convert in the session timezone.
//
// The single %[1]s is the date_trunc unit from adminTruncUnit's allowlist.
const adminUserCreationQueryFmt = `
SELECT resource_type, bucket, COUNT(*) AS count FROM (
	SELECT 'prompt' AS resource_type, date_trunc('%[1]s', created_at AT TIME ZONE 'UTC') AS bucket
		FROM prompts WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'memory', date_trunc('%[1]s', created_at)
		FROM memories WHERE user_id = $1 AND created_at >= $4::timestamp AND created_at < $5::timestamp
	UNION ALL
	SELECT 'artifact', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM artifacts WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'blueprint', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM blueprints WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'agent', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM agents WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'feed', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM feeds WHERE created_by_user_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'feed_item', date_trunc('%[1]s', posted_at AT TIME ZONE 'UTC')
		FROM feed_items WHERE posted_by_user_id = $1 AND posted_at >= $2 AND posted_at < $3
	UNION ALL
	SELECT 'comment', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM comments WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	UNION ALL
	SELECT 'attachment', date_trunc('%[1]s', created_at AT TIME ZONE 'UTC')
		FROM attachments WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
) g
GROUP BY resource_type, bucket
ORDER BY bucket, resource_type
`

// GetUserCreationSeries returns sparse (type, bucket, count) rows of the
// resources a user created in [from, to); the caller pivots and gap-fills.
// The Entity field of each row carries the singular resource type.
func (r *AdminRepository) GetUserCreationSeries(
	ctx context.Context, userID string, from, to time.Time, granularity string,
) ([]models.AdminGrowthCount, error) {
	query := fmt.Sprintf(adminUserCreationQueryFmt, adminTruncUnit(granularity))
	return queryAdminRows(ctx, r.db, "user creation series", scanAdminGrowthCount,
		query, userID, from, to, from.UTC(), to.UTC())
}

// adminUserTimelineEventsSQL is the inner event set of a user's timeline: one
// `created` branch per table, and an `updated` branch for the tables whose
// updated_at means an edit. Every occurred_at is normalized to timestamptz
// (the naive `memories` columns via AT TIME ZONE 'UTC', which assumes UTC), so
// the cursor placeholder only ever meets an aware expression.
//
// `updated` is emitted only when updated_at is more than one second after
// created_at: prompt and blueprint creation stamp CreatedAt and UpdatedAt from
// two separate time.Now() calls, so a strict `>` would report a phantom update
// for every never-edited row. agents (updated_at tracks execution stats),
// feed_items (no updated_at) and attachments (no updated_at) never produce one.
const adminUserTimelineEventsSQL = `
	SELECT 'prompt' AS resource_type, 'created' AS action, id, team_id, project_id,
		created_at AS occurred_at
		FROM prompts WHERE user_id = $1
	UNION ALL
	SELECT 'prompt', 'updated', id, team_id, project_id, updated_at
		FROM prompts WHERE user_id = $1 AND updated_at > created_at + interval '1 second'
	UNION ALL
	SELECT 'memory', 'created', id, team_id, project_id, created_at AT TIME ZONE 'UTC'
		FROM memories WHERE user_id = $1
	UNION ALL
	SELECT 'memory', 'updated', id, team_id, project_id, updated_at AT TIME ZONE 'UTC'
		FROM memories WHERE user_id = $1 AND updated_at > created_at + interval '1 second'
	UNION ALL
	SELECT 'artifact', 'created', id, team_id, project_id, created_at
		FROM artifacts WHERE user_id = $1
	UNION ALL
	SELECT 'artifact', 'updated', id, team_id, project_id, updated_at
		FROM artifacts WHERE user_id = $1 AND updated_at > created_at + interval '1 second'
	UNION ALL
	SELECT 'blueprint', 'created', id, team_id, project_id, created_at
		FROM blueprints WHERE user_id = $1
	UNION ALL
	SELECT 'blueprint', 'updated', id, team_id, project_id, updated_at
		FROM blueprints WHERE user_id = $1 AND updated_at > created_at + interval '1 second'
	UNION ALL
	SELECT 'agent', 'created', id, team_id, NULL::uuid, created_at
		FROM agents WHERE user_id = $1
	UNION ALL
	SELECT 'feed', 'created', id, team_id, NULL::uuid, created_at
		FROM feeds WHERE created_by_user_id = $1
	UNION ALL
	SELECT 'feed', 'updated', id, team_id, NULL::uuid, updated_at
		FROM feeds WHERE created_by_user_id = $1 AND updated_at > created_at + interval '1 second'
	UNION ALL
	SELECT 'feed_item', 'created', id, team_id, project_id, posted_at
		FROM feed_items WHERE posted_by_user_id = $1
	UNION ALL
	SELECT 'comment', 'created', id, team_id, NULL::uuid, created_at
		FROM comments WHERE user_id = $1
	UNION ALL
	SELECT 'comment', 'updated', id, team_id, NULL::uuid, updated_at
		FROM comments WHERE user_id = $1 AND updated_at > created_at + interval '1 second'
	UNION ALL
	SELECT 'attachment', 'created', id, team_id, NULL::uuid, created_at
		FROM attachments WHERE user_id = $1
`

// adminUserTimelineQueryFmt pages the event set newest first. %[1]s is either
// empty or the constant keyset predicate below; the sort key
// (occurred_at, resource_type, action, id) is unique per event, so the
// row-value comparison against the previous page's last event never skips or
// repeats a row, even across ties on occurred_at. A NULL occurred_at (the
// nullable created_at of prompts/memories/artifacts/blueprints) cannot be
// ordered against a cursor and is excluded.
const adminUserTimelineQueryFmt = `
SELECT e.resource_type, e.action, e.id, e.team_id, t.name, e.project_id, p.name, e.occurred_at
FROM (` + adminUserTimelineEventsSQL + `) e
JOIN teams t ON t.id = e.team_id
LEFT JOIN projects p ON p.id = e.project_id
WHERE e.occurred_at IS NOT NULL%[1]s
ORDER BY e.occurred_at DESC, e.resource_type DESC, e.action DESC, e.id DESC
LIMIT $2
`

// adminUserTimelineKeyset is the cursor predicate, appended only when a cursor
// is present. Every placeholder is cast explicitly, so their types never depend
// on inference.
const adminUserTimelineKeyset = `
	AND (e.occurred_at, e.resource_type, e.action, e.id) < ($3::timestamptz, $4::text, $5::text, $6::uuid)`

// ListUserTimeline returns up to limit events of a user's timeline, newest
// first, strictly after the cursor when one is given.
func (r *AdminRepository) ListUserTimeline(
	ctx context.Context, userID string, cursor *models.AdminTimelineCursor, limit int,
) ([]models.AdminUserTimelineEvent, error) {
	predicate := ""
	args := []interface{}{userID, limit}
	if cursor != nil {
		predicate = adminUserTimelineKeyset
		args = append(args, cursor.OccurredAt, cursor.ResourceType, cursor.Action, cursor.ResourceID)
	}

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(adminUserTimelineQueryFmt, predicate), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query user timeline: %w", err)
	}
	defer closeAdminRows(rows, "user timeline")

	events := make([]models.AdminUserTimelineEvent, 0)
	for rows.Next() {
		var ev models.AdminUserTimelineEvent
		if scanErr := rows.Scan(
			&ev.ResourceType, &ev.Action, &ev.ResourceID, &ev.TeamID, &ev.TeamName,
			&ev.ProjectID, &ev.ProjectName, &ev.OccurredAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan user timeline event: %w", scanErr)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate user timeline: %w", err)
	}
	return events, nil
}
