package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// knowledgePulseRepository implements KnowledgePulseRepository over the tables
// that already record activity (it has no table of its own).
type knowledgePulseRepository struct {
	db *database.DB
}

// NewKnowledgePulseRepository creates a new PostgreSQL knowledge pulse repository.
func NewKnowledgePulseRepository(db *database.DB) repositories.KnowledgePulseRepository {
	return &knowledgePulseRepository{db: db}
}

// pulseResourceTypes are the resource types the pulse shows. Access events
// also record project and agent reads, which have no place on the ring.
const pulseResourceTypes = `('prompt', 'artifact', 'blueprint', 'memory')`

// pulseResourceJoin builds the LEFT JOINs that resolve a polymorphic
// (type, id) pair to its title, slug and project, plus the three select
// expressions. Each join is also pinned to the team, so a stray id can never
// surface another team's title. alias prefixes the four join aliases, so one
// query can resolve both ends of a relation.
func pulseResourceJoin(alias, typeCol, idCol, teamCol string) (joins, title, slug, project string) {
	p, a, b, m := alias+"p", alias+"a", alias+"b", alias+"m"
	joins = fmt.Sprintf(`
		LEFT JOIN prompts %[1]s    ON %[5]s = 'prompt'    AND %[1]s.id = %[6]s AND %[1]s.team_id = %[7]s
		LEFT JOIN artifacts %[2]s  ON %[5]s = 'artifact'  AND %[2]s.id = %[6]s AND %[2]s.team_id = %[7]s
		LEFT JOIN blueprints %[3]s ON %[5]s = 'blueprint' AND %[3]s.id = %[6]s AND %[3]s.team_id = %[7]s
		LEFT JOIN memories %[4]s   ON %[5]s = 'memory'    AND %[4]s.id = %[6]s AND %[4]s.team_id = %[7]s`,
		p, a, b, m, typeCol, idCol, teamCol)
	title = fmt.Sprintf(`COALESCE(%s.name, %s.title, %s.title, %s, '')`, p, a, b, memoryDisplayNameExpr(m))
	slug = fmt.Sprintf(`COALESCE(%s.slug, %s.slug, %s.slug, '')`, p, a, b)
	project = fmt.Sprintf(`COALESCE(%s.project_id, %s.project_id, %s.project_id, %s.project_id)::text`, p, a, b, m)
	return joins, title, slug, project
}

// pulseQuery runs query and scans each row with scan, closing rows on return.
func pulseQuery(
	ctx context.Context, db *database.DB, what, query string, scan func(*sql.Rows) error, args ...any,
) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to query knowledge pulse "+what, "error", err)
		return fmt.Errorf("failed to query knowledge pulse %s: %w", what, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("failed to scan knowledge pulse %s: %w", what, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate knowledge pulse %s: %w", what, err)
	}
	return nil
}

// ListReadsSince returns the team's resource reads after since.
func (r *knowledgePulseRepository) ListReadsSince(
	ctx context.Context, teamID string, since time.Time, limit int,
) ([]models.PulseReadEvent, error) {
	joins, title, slug, project := pulseResourceJoin("r", "e.resource_type", "e.resource_id", "e.team_id")
	query := fmt.Sprintf(`
		SELECT e.id::text, e.created_at, e.resource_type, e.resource_id::text,
			%s, %s, COALESCE(%s, ''),
			e.source, COALESCE(e.user_id::text, ''), COALESCE(u.name, ''),
			COALESCE(e.api_key_id::text, ''), COALESCE(k.name, ''), COALESCE(e.user_agent, '')
		FROM resource_access_events e
		LEFT JOIN users u    ON u.id = e.user_id
		LEFT JOIN api_keys k ON k.id = e.api_key_id
		%s
		WHERE e.team_id = $1 AND e.created_at > $2 AND e.resource_type IN %s
		ORDER BY e.created_at ASC, e.id
		LIMIT $3`, title, slug, project, joins, pulseResourceTypes)

	events := []models.PulseReadEvent{}
	err := pulseQuery(ctx, r.db, "reads", query, func(rows *sql.Rows) error {
		var ev models.PulseReadEvent
		if err := rows.Scan(&ev.ID, &ev.At, &ev.Resource.Type, &ev.Resource.ID,
			&ev.Resource.Title, &ev.Resource.Slug, &ev.Resource.ProjectID,
			&ev.Actor.Source, &ev.Actor.UserID, &ev.Actor.UserName,
			&ev.Actor.APIKeyID, &ev.Actor.APIKeyName, &ev.Actor.UserAgent); err != nil {
			return err
		}
		events = append(events, ev)
		return nil
	}, teamID, since, limit)
	return events, err
}

// ListWritesSince returns the team's resource creates and updates after since.
func (r *knowledgePulseRepository) ListWritesSince(
	ctx context.Context, teamID string, since time.Time, limit int,
) ([]models.PulseWriteEvent, error) {
	joins, title, slug, project := pulseResourceJoin("r", "e.resource_type", "e.resource_id", "e.team_id")
	query := fmt.Sprintf(`
		SELECT e.id::text, e.created_at, e.action, e.resource_type, e.resource_id::text,
			%s, %s, COALESCE(e.project_id::text, %s, ''),
			e.source, COALESCE(e.user_id::text, ''), COALESCE(u.name, ''),
			COALESCE(e.api_key_id::text, ''), COALESCE(k.name, ''), COALESCE(e.assistant_name, '')
		FROM resource_write_events e
		LEFT JOIN users u    ON u.id = e.user_id
		LEFT JOIN api_keys k ON k.id = e.api_key_id
		%s
		WHERE e.team_id = $1 AND e.created_at > $2
		ORDER BY e.created_at ASC, e.id
		LIMIT $3`, title, slug, project, joins)

	events := []models.PulseWriteEvent{}
	err := pulseQuery(ctx, r.db, "writes", query, func(rows *sql.Rows) error {
		var ev models.PulseWriteEvent
		if err := rows.Scan(&ev.ID, &ev.At, &ev.Action, &ev.Resource.Type, &ev.Resource.ID,
			&ev.Resource.Title, &ev.Resource.Slug, &ev.Resource.ProjectID,
			&ev.Actor.Source, &ev.Actor.UserID, &ev.Actor.UserName,
			&ev.Actor.APIKeyID, &ev.Actor.APIKeyName, &ev.Actor.AssistantName); err != nil {
			return err
		}
		events = append(events, ev)
		return nil
	}, teamID, since, limit)
	return events, err
}

// ListLinksSince returns the team's new relations after since, with both
// endpoints resolved.
func (r *knowledgePulseRepository) ListLinksSince(
	ctx context.Context, teamID string, since time.Time, limit int,
) ([]models.PulseLinkEvent, error) {
	fJoins, fTitle, fSlug, _ := pulseResourceJoin("f", "rel.from_type", "rel.from_id", "rel.team_id")
	tJoins, tTitle, tSlug, _ := pulseResourceJoin("t", "rel.to_type", "rel.to_id", "rel.team_id")
	query := fmt.Sprintf(`
		SELECT rel.id::text, rel.created_at, rel.relation_type, rel.origin,
			COALESCE(rel.created_by::text, ''), COALESCE(u.name, ''),
			rel.from_type, rel.from_id::text, %s, %s, rel.project_id::text,
			rel.to_type, rel.to_id::text, %s, %s
		FROM resource_relations rel
		LEFT JOIN users u ON u.id = rel.created_by
		%s
		%s
		WHERE rel.team_id = $1 AND rel.created_at > $2
		ORDER BY rel.created_at ASC, rel.id
		LIMIT $3`, fTitle, fSlug, tTitle, tSlug, fJoins, tJoins)

	events := []models.PulseLinkEvent{}
	err := pulseQuery(ctx, r.db, "links", query, func(rows *sql.Rows) error {
		var ev models.PulseLinkEvent
		if err := rows.Scan(&ev.ID, &ev.At, &ev.RelationType, &ev.Origin,
			&ev.CreatedByID, &ev.CreatedByName,
			&ev.From.Type, &ev.From.ID, &ev.From.Title, &ev.From.Slug, &ev.From.ProjectID,
			&ev.To.Type, &ev.To.ID, &ev.To.Title, &ev.To.Slug); err != nil {
			return err
		}
		ev.To.ProjectID = ev.From.ProjectID
		events = append(events, ev)
		return nil
	}, teamID, since, limit)
	return events, err
}

// ListFeedPostsSince returns the team's unarchived feed posts after since.
func (r *knowledgePulseRepository) ListFeedPostsSince(
	ctx context.Context, teamID string, since time.Time, limit int,
) ([]models.PulseFeedEvent, error) {
	const query = `
		SELECT f.id::text, f.posted_at, f.title, COALESCE(f.project_id::text, ''),
			f.ai_assistant_name, f.posted_by_user_id::text, COALESCE(u.name, '')
		FROM feed_items f
		LEFT JOIN users u ON u.id = f.posted_by_user_id
		WHERE f.team_id = $1 AND f.posted_at > $2 AND f.archived_at IS NULL
		ORDER BY f.posted_at ASC, f.id
		LIMIT $3`

	events := []models.PulseFeedEvent{}
	err := pulseQuery(ctx, r.db, "feed posts", query, func(rows *sql.Rows) error {
		var ev models.PulseFeedEvent
		if err := rows.Scan(&ev.ID, &ev.At, &ev.Title, &ev.ProjectID,
			&ev.AssistantName, &ev.UserID, &ev.UserName); err != nil {
			return err
		}
		events = append(events, ev)
		return nil
	}, teamID, since, limit)
	return events, err
}

// GetSnapshot returns the ranked nodes, the edges between them and the
// recently active actors.
func (r *knowledgePulseRepository) GetSnapshot(
	ctx context.Context, teamID string, activitySince time.Time, nodeLimit int,
) (*models.PulseSnapshotRaw, error) {
	snap := &models.PulseSnapshotRaw{
		Nodes: []models.PulseNode{}, Edges: []models.PulseEdge{}, Actors: []models.PulseActorRaw{},
	}
	if err := r.snapshotNodes(ctx, teamID, activitySince, nodeLimit, snap); err != nil {
		return nil, err
	}
	if err := r.snapshotEdges(ctx, teamID, snap); err != nil {
		return nil, err
	}
	if err := r.snapshotActors(ctx, teamID, activitySince, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

func (r *knowledgePulseRepository) snapshotNodes(
	ctx context.Context, teamID string, activitySince time.Time, nodeLimit int, snap *models.PulseSnapshotRaw,
) error {
	joins, title, slug, project := pulseResourceJoin("n", "c.t", "c.id", "$1::uuid")
	// A node is any resource with a relation or recent activity; resources
	// that no longer exist (no join matched) are dropped before ranking.
	query := fmt.Sprintf(`
		WITH deg AS (
			SELECT t, id, COUNT(*) AS degree FROM (
				SELECT from_type AS t, from_id AS id FROM resource_relations WHERE team_id = $1
				UNION ALL
				SELECT to_type, to_id FROM resource_relations WHERE team_id = $1
			) x GROUP BY t, id
		), act AS (
			SELECT t, id, COUNT(*) AS activity FROM (
				SELECT resource_type AS t, resource_id AS id FROM resource_access_events
				 WHERE team_id = $1 AND created_at > $2 AND resource_type IN %s
				UNION ALL
				SELECT resource_type, resource_id FROM resource_write_events
				 WHERE team_id = $1 AND created_at > $2
			) y GROUP BY t, id
		), c AS (
			SELECT COALESCE(deg.t, act.t) AS t, COALESCE(deg.id, act.id) AS id,
				COALESCE(deg.degree, 0) AS degree, COALESCE(act.activity, 0) AS activity
			FROM deg FULL OUTER JOIN act ON act.t = deg.t AND act.id = deg.id
		)
		SELECT c.t, c.id::text, %s, %s, COALESCE(%s, ''), c.degree
		FROM c
		%s
		WHERE COALESCE(np.id, na.id, nb.id, nm.id) IS NOT NULL
		ORDER BY c.degree DESC, c.activity DESC, c.id
		LIMIT $3`, pulseResourceTypes, title, slug, project, joins)

	return pulseQuery(ctx, r.db, "snapshot nodes", query, func(rows *sql.Rows) error {
		var n models.PulseNode
		if err := rows.Scan(&n.Resource.Type, &n.Resource.ID, &n.Resource.Title,
			&n.Resource.Slug, &n.Resource.ProjectID, &n.Degree); err != nil {
			return err
		}
		snap.Nodes = append(snap.Nodes, n)
		return nil
	}, teamID, activitySince, nodeLimit)
}

func (r *knowledgePulseRepository) snapshotEdges(
	ctx context.Context, teamID string, snap *models.PulseSnapshotRaw,
) error {
	if len(snap.Nodes) == 0 {
		return nil
	}
	ids := make([]string, 0, len(snap.Nodes))
	for _, n := range snap.Nodes {
		ids = append(ids, n.Resource.ID)
	}
	const query = `
		SELECT from_id::text, to_id::text, relation_type
		FROM resource_relations
		WHERE team_id = $1 AND from_id = ANY($2::uuid[]) AND to_id = ANY($2::uuid[])
		ORDER BY created_at, id`
	return pulseQuery(ctx, r.db, "snapshot edges", query, func(rows *sql.Rows) error {
		var e models.PulseEdge
		if err := rows.Scan(&e.FromID, &e.ToID, &e.RelationType); err != nil {
			return err
		}
		snap.Edges = append(snap.Edges, e)
		return nil
	}, teamID, pq.Array(ids))
}

func (r *knowledgePulseRepository) snapshotActors(
	ctx context.Context, teamID string, activitySince time.Time, snap *models.PulseSnapshotRaw,
) error {
	// The distinct actor fingerprints of recent reads and writes; the service
	// resolves and de-duplicates them with the same rules as the events.
	query := fmt.Sprintf(`
		SELECT DISTINCT a.source, COALESCE(a.user_id::text, ''), COALESCE(u.name, ''),
			COALESCE(a.api_key_id::text, ''), COALESCE(k.name, ''), a.user_agent, a.assistant_name
		FROM (
			SELECT source, user_id, api_key_id, COALESCE(user_agent, '') AS user_agent, '' AS assistant_name
			FROM resource_access_events
			WHERE team_id = $1 AND created_at > $2 AND resource_type IN %s
			UNION
			SELECT source, user_id, api_key_id, '', COALESCE(assistant_name, '')
			FROM resource_write_events
			WHERE team_id = $1 AND created_at > $2
		) a
		LEFT JOIN users u    ON u.id = a.user_id
		LEFT JOIN api_keys k ON k.id = a.api_key_id
		LIMIT 500`, pulseResourceTypes)
	return pulseQuery(ctx, r.db, "snapshot actors", query, func(rows *sql.Rows) error {
		var a models.PulseActorRaw
		if err := rows.Scan(&a.Source, &a.UserID, &a.UserName, &a.APIKeyID, &a.APIKeyName,
			&a.UserAgent, &a.AssistantName); err != nil {
			return err
		}
		snap.Actors = append(snap.Actors, a)
		return nil
	}, teamID, activitySince)
}

// GetDayStats counts the day's activity in one round trip.
func (r *knowledgePulseRepository) GetDayStats(
	ctx context.Context, teamID string, dayStart time.Time,
) (models.PulseDayStats, error) {
	var s models.PulseDayStats
	if r.db == nil {
		return s, fmt.Errorf("database connection is nil")
	}
	query := strings.TrimSpace(fmt.Sprintf(`
		SELECT
			(SELECT COUNT(*) FROM resource_access_events
			  WHERE team_id = $1 AND created_at >= $2 AND resource_type IN %s),
			(SELECT COUNT(*) FROM resource_write_events
			  WHERE team_id = $1 AND created_at >= $2),
			(SELECT COUNT(*) FROM resource_relations
			  WHERE team_id = $1 AND created_at >= $2),
			(SELECT COUNT(*) FROM resource_relations
			  WHERE team_id = $1 AND created_at >= $2 AND relation_type = 'supersedes'),
			(SELECT COUNT(*) FROM resource_write_events w
			  WHERE w.team_id = $1 AND w.created_at >= $2 AND w.action = 'update'
			    AND EXISTS (SELECT 1 FROM resource_relations rel
			                 WHERE rel.team_id = w.team_id
			                   AND ((rel.from_type = w.resource_type AND rel.from_id = w.resource_id)
			                     OR (rel.to_type = w.resource_type AND rel.to_id = w.resource_id))))`,
		pulseResourceTypes))
	if err := r.db.QueryRowContext(ctx, query, teamID, dayStart).
		Scan(&s.Reads, &s.Writes, &s.Links, &s.Supersedes, &s.LinkedUpdates); err != nil {
		return s, fmt.Errorf("failed to count knowledge pulse day stats: %w", err)
	}
	return s, nil
}
