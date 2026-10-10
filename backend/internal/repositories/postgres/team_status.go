package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamStatusRepository is the Postgres implementation of
// repositories.TeamStatusRepository. Every statement carries team_id: the
// repository is tenancy-only and decides nothing about roles (epic #220 D3).
type TeamStatusRepository struct {
	db *database.DB
}

// NewTeamStatusRepository creates a new TeamStatusRepository.
func NewTeamStatusRepository(db *database.DB) repositories.TeamStatusRepository {
	return &TeamStatusRepository{db: db}
}

// The statements are whole literals rather than a shared column list
// concatenated in, so each is a compile-time constant a reader (and a static
// analyser) can take in at a glance. Their column order matches scanTeamStatus.
const (
	teamStatusInsertSQL = `
		INSERT INTO team_statuses (team_id, resource_type, slug, name, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	teamStatusGetByIDSQL = `
		SELECT id, team_id, resource_type, slug, name, created_by, created_at, updated_at
		  FROM team_statuses
		 WHERE team_id = $1 AND id = $2`

	teamStatusListSQL = `
		SELECT id, team_id, resource_type, slug, name, created_by, created_at, updated_at
		  FROM team_statuses
		 WHERE team_id = $1 AND resource_type = $2
		 ORDER BY name ASC, slug ASC`

	teamStatusCountSQL = `
		SELECT count(*) FROM team_statuses WHERE team_id = $1 AND resource_type = $2`

	teamStatusUpdateNameSQL = `
		UPDATE team_statuses
		   SET name = $3, updated_at = now()
		 WHERE team_id = $1 AND id = $2
		RETURNING id, team_id, resource_type, slug, name, created_by, created_at, updated_at`

	teamStatusDeleteSQL = `
		DELETE FROM team_statuses WHERE team_id = $1 AND id = $2`

	teamStatusDeleteReturningSQL = `
		DELETE FROM team_statuses
		 WHERE team_id = $1 AND id = $2
		RETURNING id, team_id, resource_type, slug, name, created_by, created_at, updated_at`

	// teamStatusPreserveUpdatedAtSQL switches off, for the current transaction
	// only, the trigger that stamps memories.updated_at on every update
	// (migration 026). The other three resource tables have no such trigger.
	teamStatusPreserveUpdatedAtSQL = `
		SELECT set_config('vibexp.preserve_updated_at', 'on', true)`
)

// teamStatusResourceSQL holds the two statements that read and rewrite the
// status column of one resource table.
type teamStatusResourceSQL struct {
	// usage counts the team's rows per status.
	usage string
	// reassign moves the team's rows from status $3 to status $2. It leaves
	// updated_at alone: a status being deleted is not an edit of the resource,
	// and moving the timestamp would reset its freshness and reorder every
	// list it appears in.
	reassign string
}

// teamStatusResourceTables maps a status resource type to the statements for
// its table. The statements are whole literals selected by a fixed key, so no
// table name is ever built from input. One UPDATE covers every row carrying the
// status; it is unbounded, which is acceptable for a rare, owner-only action on
// tables that filter by team_id first.
var teamStatusResourceTables = map[string]teamStatusResourceSQL{
	models.StatusResourceTypePrompt: {
		usage:    `SELECT status, count(*) FROM prompts WHERE team_id = $1 GROUP BY status`,
		reassign: `UPDATE prompts SET status = $2 WHERE team_id = $1 AND status = $3`,
	},
	models.StatusResourceTypeArtifact: {
		usage:    `SELECT status, count(*) FROM artifacts WHERE team_id = $1 GROUP BY status`,
		reassign: `UPDATE artifacts SET status = $2 WHERE team_id = $1 AND status = $3`,
	},
	models.StatusResourceTypeBlueprint: {
		usage:    `SELECT status, count(*) FROM blueprints WHERE team_id = $1 GROUP BY status`,
		reassign: `UPDATE blueprints SET status = $2 WHERE team_id = $1 AND status = $3`,
	},
	models.StatusResourceTypeMemory: {
		usage:    `SELECT status, count(*) FROM memories WHERE team_id = $1 GROUP BY status`,
		reassign: `UPDATE memories SET status = $2 WHERE team_id = $1 AND status = $3`,
	},
}

// errTeamStatusResourceTypeUnknown is returned for a resource type that has no
// entry in teamStatusResourceTables. The service validates the type first, so
// reaching it means a row or a caller carries a type this build does not know.
var errTeamStatusResourceTypeUnknown = errors.New("resource type has no status table")

func (r *TeamStatusRepository) Create(ctx context.Context, status *models.TeamStatus) error {
	err := r.db.QueryRowContext(ctx, teamStatusInsertSQL,
		status.TeamID, status.ResourceType, status.Slug, status.Name, status.CreatedBy,
	).Scan(&status.ID, &status.CreatedAt, &status.UpdatedAt)
	if err != nil {
		if uniqueViolation(err) != nil {
			return repositories.ErrTeamStatusExists
		}
		if isFKViolation(err) {
			return fmt.Errorf("team or creator not found for team status: %w", err)
		}
		return fmt.Errorf("failed to create team status: %w", err)
	}
	return nil
}

func (r *TeamStatusRepository) GetByID(
	ctx context.Context, teamID, id string,
) (*models.TeamStatus, error) {
	status, err := scanTeamStatus(r.db.QueryRowContext(ctx, teamStatusGetByIDSQL, teamID, id))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get team status: %w", err),
			repositories.ErrTeamStatusNotFound,
		)
	}
	return &status, nil
}

func (r *TeamStatusRepository) List(
	ctx context.Context, teamID, resourceType string,
) ([]models.TeamStatus, error) {
	return queryAdminRows(ctx, r.db, "team statuses",
		func(rows *sql.Rows) (models.TeamStatus, error) { return scanTeamStatus(rows) },
		teamStatusListSQL, teamID, resourceType)
}

func (r *TeamStatusRepository) Count(
	ctx context.Context, teamID, resourceType string,
) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, teamStatusCountSQL, teamID, resourceType).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count team statuses: %w", err)
	}
	return count, nil
}

func (r *TeamStatusRepository) UpdateName(
	ctx context.Context, teamID, id, name string,
) (*models.TeamStatus, error) {
	status, err := scanTeamStatus(r.db.QueryRowContext(ctx, teamStatusUpdateNameSQL, teamID, id, name))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to rename team status: %w", err),
			repositories.ErrTeamStatusNotFound,
		)
	}
	return &status, nil
}

func (r *TeamStatusRepository) Delete(ctx context.Context, teamID, id string) error {
	result, err := r.db.ExecContext(ctx, teamStatusDeleteSQL, teamID, id)
	if err != nil {
		return fmt.Errorf("failed to delete team status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read team status delete result: %w", err)
	}
	if affected == 0 {
		return repositories.ErrTeamStatusNotFound
	}
	return nil
}

func (r *TeamStatusRepository) UsageCounts(
	ctx context.Context, teamID, resourceType string,
) (map[string]int, error) {
	table, ok := teamStatusResourceTables[resourceType]
	if !ok {
		return nil, fmt.Errorf("%w: %q", errTeamStatusResourceTypeUnknown, resourceType)
	}

	type statusCount struct {
		status string
		count  int
	}
	rows, err := queryAdminRows(ctx, r.db, "status usage counts",
		func(rows *sql.Rows) (statusCount, error) {
			var row statusCount
			err := rows.Scan(&row.status, &row.count)
			return row, err
		},
		table.usage, teamID)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.status] = row.count
	}
	return counts, nil
}

func (r *TeamStatusRepository) DeleteAndReassign(
	ctx context.Context, teamID, id, defaultStatus string,
) (*models.TeamStatus, int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to begin team status delete: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back team status delete", "error", rbErr)
		}
	}()

	// Deleting first takes the row lock, so two concurrent deletes of one
	// status cannot both reassign: the second finds no row.
	deleted, err := scanTeamStatus(tx.QueryRowContext(ctx, teamStatusDeleteReturningSQL, teamID, id))
	if err != nil {
		return nil, 0, mapNoRows(
			fmt.Errorf("failed to delete team status: %w", err),
			repositories.ErrTeamStatusNotFound,
		)
	}

	table, ok := teamStatusResourceTables[deleted.ResourceType]
	if !ok {
		return nil, 0, fmt.Errorf("%w: %q", errTeamStatusResourceTypeUnknown, deleted.ResourceType)
	}
	if _, err = tx.ExecContext(ctx, teamStatusPreserveUpdatedAtSQL); err != nil {
		return nil, 0, fmt.Errorf("failed to preserve updated_at for status reassignment: %w", err)
	}
	result, err := tx.ExecContext(ctx, table.reassign, teamID, defaultStatus, deleted.Slug)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to reassign %s resources after status delete: %w",
			deleted.ResourceType, err)
	}
	reassigned, err := result.RowsAffected()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read status reassignment result: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("failed to commit team status delete: %w", err)
	}
	committed = true
	return &deleted, int(reassigned), nil
}

// scanTeamStatus scans one team_statuses row, from a single-row lookup or a
// result set. created_by scans straight into the model's *string: NULL is nil.
func scanTeamStatus(s rowScanner) (models.TeamStatus, error) {
	var status models.TeamStatus
	err := s.Scan(
		&status.ID, &status.TeamID, &status.ResourceType, &status.Slug, &status.Name,
		&status.CreatedBy, &status.CreatedAt, &status.UpdatedAt,
	)
	return status, err
}
