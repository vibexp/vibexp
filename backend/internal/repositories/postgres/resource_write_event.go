package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// resourceWriteEventRepository implements the ResourceWriteEventRepository
// interface using PostgreSQL.
type resourceWriteEventRepository struct {
	db *database.DB
}

// NewResourceWriteEventRepository creates a new PostgreSQL resource write event
// repository.
func NewResourceWriteEventRepository(db *database.DB) repositories.ResourceWriteEventRepository {
	return &resourceWriteEventRepository{db: db}
}

// Create persists a new resource write event and back-fills the generated id
// and created_at.
func (r *resourceWriteEventRepository) Create(ctx context.Context, event *models.ResourceWriteEvent) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	const query = `
		INSERT INTO resource_write_events
		(team_id, project_id, user_id, resource_type, resource_id, action, source, assistant_name, api_key_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`

	err := r.db.QueryRowContext(ctx, query,
		event.TeamID,
		event.ProjectID,
		event.UserID,
		event.ResourceType,
		event.ResourceID,
		event.Action,
		event.Source,
		event.AssistantName,
		event.APIKeyID,
	).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		slog.Error("Failed to create resource write event", "error", err)
		return fmt.Errorf("failed to create resource write event: %w", err)
	}

	return nil
}

// The two creation-by-source statements count the SAME resource rows, under the
// SAME predicates, as the per-day creation metrics (TeamRepository.
// GetTeamResourceCreationMetrics and projectResourceCreationQuery), and then
// LEFT JOIN each row to its create event. So the five sources of a resource
// type always sum to that type's existing total: a resource with no create
// event is not dropped, it is counted under 'unknown'. The partial unique index
// idx_resource_write_events_create guarantees at most one create event per
// resource, so the join cannot fan a resource out.
//
// memories.created_at is a naive TIMESTAMP where the other three are
// TIMESTAMPTZ, so that branch keeps its own $3::timestamp placeholder, bound to
// the same instant in UTC. Sharing $2 would compare the naive column through
// the session time zone and drop memories near the window edge. See
// GetTeamResourceCreationMetrics, which documents the defect in full.
const teamCreationBySourceQuery = `
	SELECT 'prompts' AS resource_type, COALESCE(e.source, 'unknown') AS source, COUNT(*) AS count
	FROM prompts r
	LEFT JOIN resource_write_events e
		ON e.resource_type = 'prompt' AND e.resource_id = r.id AND e.action = 'create'
	WHERE r.team_id = $1 AND r.created_at >= $2
	GROUP BY COALESCE(e.source, 'unknown')
	UNION ALL
	SELECT 'artifacts', COALESCE(e.source, 'unknown'), COUNT(*)
	FROM artifacts r
	LEFT JOIN resource_write_events e
		ON e.resource_type = 'artifact' AND e.resource_id = r.id AND e.action = 'create'
	WHERE r.team_id = $1 AND r.created_at >= $2
	GROUP BY COALESCE(e.source, 'unknown')
	UNION ALL
	SELECT 'blueprints', COALESCE(e.source, 'unknown'), COUNT(*)
	FROM blueprints r
	LEFT JOIN resource_write_events e
		ON e.resource_type = 'blueprint' AND e.resource_id = r.id AND e.action = 'create'
	WHERE r.team_id = $1 AND r.created_at >= $2
	GROUP BY COALESCE(e.source, 'unknown')
	UNION ALL
	SELECT 'memories', COALESCE(e.source, 'unknown'), COUNT(*)
	FROM memories r
	LEFT JOIN resource_write_events e
		ON e.resource_type = 'memory' AND e.resource_id = r.id AND e.action = 'create'
	WHERE r.team_id = $1 AND r.created_at >= $3::timestamp
	GROUP BY COALESCE(e.source, 'unknown')
	ORDER BY resource_type, source`

// projectCreationBySourceQuery is teamCreationBySourceQuery narrowed to one
// project, resolved from its team and slug ($1, $4). An unknown slug matches no
// rows; the caller has already resolved and authorized the project.
const projectCreationBySourceQuery = `
	WITH target AS (
		SELECT id FROM projects WHERE team_id = $1 AND slug = $4
	)
	SELECT 'prompts' AS resource_type, COALESCE(e.source, 'unknown') AS source, COUNT(*) AS count
	FROM prompts r
	LEFT JOIN resource_write_events e
		ON e.resource_type = 'prompt' AND e.resource_id = r.id AND e.action = 'create'
	WHERE r.project_id = (SELECT id FROM target) AND r.created_at >= $2
	GROUP BY COALESCE(e.source, 'unknown')
	UNION ALL
	SELECT 'artifacts', COALESCE(e.source, 'unknown'), COUNT(*)
	FROM artifacts r
	LEFT JOIN resource_write_events e
		ON e.resource_type = 'artifact' AND e.resource_id = r.id AND e.action = 'create'
	WHERE r.project_id = (SELECT id FROM target) AND r.created_at >= $2
	GROUP BY COALESCE(e.source, 'unknown')
	UNION ALL
	SELECT 'blueprints', COALESCE(e.source, 'unknown'), COUNT(*)
	FROM blueprints r
	LEFT JOIN resource_write_events e
		ON e.resource_type = 'blueprint' AND e.resource_id = r.id AND e.action = 'create'
	WHERE r.project_id = (SELECT id FROM target) AND r.created_at >= $2
	GROUP BY COALESCE(e.source, 'unknown')
	UNION ALL
	SELECT 'memories', COALESCE(e.source, 'unknown'), COUNT(*)
	FROM memories r
	LEFT JOIN resource_write_events e
		ON e.resource_type = 'memory' AND e.resource_id = r.id AND e.action = 'create'
	WHERE r.project_id = (SELECT id FROM target) AND r.created_at >= $3::timestamp
	GROUP BY COALESCE(e.source, 'unknown')
	ORDER BY resource_type, source`

// GetTeamCreationCountsBySource returns, for each resource type, how many of the
// team's resources created at or after since arrived through each channel.
func (r *resourceWriteEventRepository) GetTeamCreationCountsBySource(
	ctx context.Context, teamID string, since time.Time,
) ([]models.ResourceCreationSourceCount, error) {
	return r.queryCreationBySource(ctx, teamCreationBySourceQuery, teamID, since, since.UTC())
}

// GetProjectCreationCountsBySource is GetTeamCreationCountsBySource for one
// project of the team, identified by its slug.
func (r *resourceWriteEventRepository) GetProjectCreationCountsBySource(
	ctx context.Context, teamID, projectSlug string, since time.Time,
) ([]models.ResourceCreationSourceCount, error) {
	return r.queryCreationBySource(ctx, projectCreationBySourceQuery, teamID, since, since.UTC(), projectSlug)
}

func (r *resourceWriteEventRepository) queryCreationBySource(
	ctx context.Context, query string, args ...any,
) ([]models.ResourceCreationSourceCount, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query resource creation counts by source: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	counts := []models.ResourceCreationSourceCount{}
	for rows.Next() {
		var c models.ResourceCreationSourceCount
		if scanErr := rows.Scan(&c.ResourceType, &c.Source, &c.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan resource creation count by source: %w", scanErr)
		}
		counts = append(counts, c)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("failed to iterate resource creation counts by source: %w", rowsErr)
	}

	return counts, nil
}

// GetTeamWriteCountsBySource returns how many write events (creates and
// updates) the team recorded per channel at or after since.
func (r *resourceWriteEventRepository) GetTeamWriteCountsBySource(
	ctx context.Context, teamID string, since time.Time,
) ([]models.ResourceWriteSourceCount, error) {
	const query = `
		SELECT source, COUNT(*) AS count
		FROM resource_write_events
		WHERE team_id = $1 AND created_at >= $2
		GROUP BY source
		ORDER BY source`

	return r.queryWritesBySource(ctx, query, teamID, since)
}

// GetProjectWriteCountsBySource is GetTeamWriteCountsBySource for one project of
// the team, identified by its slug. An event belongs to the project the
// resource was in when it was written.
func (r *resourceWriteEventRepository) GetProjectWriteCountsBySource(
	ctx context.Context, teamID, projectSlug string, since time.Time,
) ([]models.ResourceWriteSourceCount, error) {
	const query = `
		SELECT source, COUNT(*) AS count
		FROM resource_write_events
		WHERE team_id = $1 AND created_at >= $2
			AND project_id = (SELECT id FROM projects WHERE team_id = $1 AND slug = $3)
		GROUP BY source
		ORDER BY source`

	return r.queryWritesBySource(ctx, query, teamID, since, projectSlug)
}

func (r *resourceWriteEventRepository) queryWritesBySource(
	ctx context.Context, query string, args ...any,
) ([]models.ResourceWriteSourceCount, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query resource write counts by source: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	counts := []models.ResourceWriteSourceCount{}
	for rows.Next() {
		var c models.ResourceWriteSourceCount
		if scanErr := rows.Scan(&c.Source, &c.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan resource write count by source: %w", scanErr)
		}
		counts = append(counts, c)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("failed to iterate resource write counts by source: %w", rowsErr)
	}

	return counts, nil
}
