package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ResourceLockEventRepository implements
// repositories.ResourceLockEventRepository for PostgreSQL. The table is
// append-only: this repository deliberately exposes no update or delete.
type ResourceLockEventRepository struct {
	db *database.DB
}

// NewResourceLockEventRepository creates a new ResourceLockEventRepository.
func NewResourceLockEventRepository(db *database.DB) repositories.ResourceLockEventRepository {
	return &ResourceLockEventRepository{db: db}
}

// Whole literals, like team_status.go: each statement is a compile-time
// constant.
const (
	resourceLockEventInsertSQL = `
		INSERT INTO resource_lock_events
			(team_id, resource_type, resource_id, action, actor_user_id, comment)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	// The actor's name is a read-time join, not a write-time snapshot: a name
	// is mutable, and the join yields NULL in exactly one case, the account
	// being gone. `id DESC` breaks created_at ties, which are real: now() is
	// transaction-start time, so two events written in one transaction carry
	// the same timestamp. A NULL limit means no limit.
	resourceLockEventListSQL = `
		SELECT e.id, e.team_id, e.resource_type, e.resource_id, e.action,
		       e.actor_user_id, e.comment, e.created_at, u.name
		  FROM resource_lock_events e
		  LEFT JOIN users u ON u.id = e.actor_user_id
		 WHERE e.team_id = $1 AND e.resource_type = $2 AND e.resource_id = $3
		 ORDER BY e.created_at DESC, e.id DESC
		 LIMIT $4`
)

// Append records one lock or unlock event and populates ID and CreatedAt from
// the persisted row.
//
// resource_type and action are validated here because nothing else will: the
// table has no CHECK on either (a constraint would make a new lockable type a
// migration), so an unvalidated typo would be stored and then never matched by
// any read.
func (r *ResourceLockEventRepository) Append(ctx context.Context, event *models.ResourceLockEvent) error {
	if !models.IsLockableResourceType(event.ResourceType) {
		return fmt.Errorf("%w: resource type %q is not lockable",
			repositories.ErrInvalidResourceLockEvent, event.ResourceType)
	}
	if !models.IsValidLockAction(event.Action) {
		return fmt.Errorf("%w: unknown action %q", repositories.ErrInvalidResourceLockEvent, event.Action)
	}

	if err := r.db.QueryRowContext(
		ctx, resourceLockEventInsertSQL,
		event.TeamID, event.ResourceType, event.ResourceID, event.Action, event.ActorUserID, event.Comment,
	).Scan(&event.ID, &event.CreatedAt); err != nil {
		return fmt.Errorf("failed to append resource lock event: %w", err)
	}
	return nil
}

// ListByResource returns one resource's lock history newest first.
//
// Tenancy-only predicate, no role predicates (authz decision D3).
func (r *ResourceLockEventRepository) ListByResource(
	ctx context.Context, teamID, resourceType, resourceID string, limit int,
) ([]*models.ResourceLockEvent, error) {
	// A zero or negative limit means "no limit", which NULL expresses without a
	// second query shape (mirrors TeamSettingsAuditRepository.ListByTeam).
	var limitArg *int
	if limit > 0 {
		limitArg = &limit
	}

	rows, err := r.db.QueryContext(ctx, resourceLockEventListSQL, teamID, resourceType, resourceID, limitArg)
	if err != nil {
		return nil, fmt.Errorf("failed to list resource lock events: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close resource lock event rows", "error", closeErr)
		}
	}()

	events := make([]*models.ResourceLockEvent, 0)
	for rows.Next() {
		var event models.ResourceLockEvent
		var actorName sql.NullString
		if err := rows.Scan(
			&event.ID, &event.TeamID, &event.ResourceType, &event.ResourceID, &event.Action,
			&event.ActorUserID, &event.Comment, &event.CreatedAt, &actorName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan resource lock event: %w", err)
		}
		event.ActorName = actorName.String
		events = append(events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate resource lock events: %w", err)
	}
	return events, nil
}
