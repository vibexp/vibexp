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

	// teamStatusLockNameSQL reads the name a rename is about to replace and
	// locks the row until the rename commits, so a concurrent rename waits and
	// then reads the name this one stored. FOR NO KEY UPDATE is the lock the
	// rename's own UPDATE takes (name is not a key column): unlike FOR UPDATE
	// it does not conflict with ConfirmSlug's FOR KEY SHARE, so a resource
	// write confirming its status never waits for a rename.
	teamStatusLockNameSQL = `
		SELECT name
		  FROM team_statuses
		 WHERE team_id = $1 AND id = $2
		   FOR NO KEY UPDATE`

	teamStatusUpdateNameSQL = `
		UPDATE team_statuses
		   SET name = $3, updated_at = now()
		 WHERE team_id = $1 AND id = $2
		RETURNING id, team_id, resource_type, slug, name, created_by, created_at, updated_at`

	teamStatusDeleteReturningSQL = `
		DELETE FROM team_statuses
		 WHERE team_id = $1 AND id = $2
		RETURNING id, team_id, resource_type, slug, name, created_by, created_at, updated_at`

	// teamStatusConfirmSlugSQL takes FOR KEY SHARE on the status row, which
	// conflicts with the lock DeleteAndReassign's DELETE holds until it commits:
	// the statement waits a concurrent delete out and then finds no row.
	teamStatusConfirmSlugSQL = `
		SELECT 1
		  FROM team_statuses
		 WHERE team_id = $1 AND resource_type = $2 AND slug = $3
		   FOR KEY SHARE`

	// teamStatusPreserveUpdatedAtSQL switches off, for the current transaction
	// only, the trigger that stamps memories.updated_at on every update
	// (migration 019_consolidated). The other three resource tables have no such trigger.
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
	// reassignOne is reassign for the single row $4: the repair of a resource
	// whose write landed after reassign had already run. On a lockable table
	// it skips a locked row, whose status is frozen.
	reassignOne string
	// locked lists the team's locked rows carrying status $2, and lockedOne
	// does so for the single row $3. Both are empty for memories, which
	// cannot be locked. status is a frozen field of a locked resource, so
	// neither reassignment may rewrite one (issue #1290).
	locked    string
	lockedOne string
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
		reassignOne: `UPDATE prompts SET status = $2
			WHERE team_id = $1 AND status = $3 AND id = $4 AND locked_by IS NULL`,
		locked: `SELECT 'prompt', id::text, slug, name FROM prompts
			WHERE team_id = $1 AND status = $2 AND locked_by IS NOT NULL
			ORDER BY name, id LIMIT $3`,
		lockedOne: `SELECT 'prompt', id::text, slug, name FROM prompts
			WHERE team_id = $1 AND status = $2 AND id = $3 AND locked_by IS NOT NULL
			LIMIT $4`,
	},
	models.StatusResourceTypeArtifact: {
		usage:    `SELECT status, count(*) FROM artifacts WHERE team_id = $1 GROUP BY status`,
		reassign: `UPDATE artifacts SET status = $2 WHERE team_id = $1 AND status = $3`,
		reassignOne: `UPDATE artifacts SET status = $2
			WHERE team_id = $1 AND status = $3 AND id = $4 AND locked_by IS NULL`,
		locked: `SELECT 'artifact', id::text, slug, title FROM artifacts
			WHERE team_id = $1 AND status = $2 AND locked_by IS NOT NULL
			ORDER BY title, id LIMIT $3`,
		lockedOne: `SELECT 'artifact', id::text, slug, title FROM artifacts
			WHERE team_id = $1 AND status = $2 AND id = $3 AND locked_by IS NOT NULL
			LIMIT $4`,
	},
	models.StatusResourceTypeBlueprint: {
		usage:    `SELECT status, count(*) FROM blueprints WHERE team_id = $1 GROUP BY status`,
		reassign: `UPDATE blueprints SET status = $2 WHERE team_id = $1 AND status = $3`,
		reassignOne: `UPDATE blueprints SET status = $2
			WHERE team_id = $1 AND status = $3 AND id = $4 AND locked_by IS NULL`,
		locked: `SELECT 'blueprint', id::text, slug, title FROM blueprints
			WHERE team_id = $1 AND status = $2 AND locked_by IS NOT NULL
			ORDER BY title, id LIMIT $3`,
		lockedOne: `SELECT 'blueprint', id::text, slug, title FROM blueprints
			WHERE team_id = $1 AND status = $2 AND id = $3 AND locked_by IS NOT NULL
			LIMIT $4`,
	},
	models.StatusResourceTypeMemory: {
		usage:       `SELECT status, count(*) FROM memories WHERE team_id = $1 GROUP BY status`,
		reassign:    `UPDATE memories SET status = $2 WHERE team_id = $1 AND status = $3`,
		reassignOne: `UPDATE memories SET status = $2 WHERE team_id = $1 AND status = $3 AND id = $4`,
	},
}

// errTeamStatusResourceTypeUnknown is returned for a resource type that has no
// entry in teamStatusResourceTables. The service validates the type first, so
// reaching it means a row or a caller carries a type this build does not know.
var errTeamStatusResourceTypeUnknown = errors.New("resource type has no status table")

// errTeamStatusAuditRequired is returned by a write called without an audit
// builder, or whose builder returned no entry. Every custom-status change is
// audited in its own transaction, so a write with nothing to record is a
// programming error: a missing builder is refused before the database is
// touched, and a missing entry rolls the change back.
var errTeamStatusAuditRequired = errors.New("team status write requires an audit builder")

// runAuditedTx is the one transaction shape behind the three custom-status
// writes (#1333): run change, build the audit entry from what it did, insert
// that entry, commit. Any failure -- including the builder's -- rolls
// everything back, so a change is never stored without its audit entry and an
// audit entry never describes a change that did not happen. op names the write
// in error messages.
func (r *TeamStatusRepository) runAuditedTx(
	ctx context.Context, op string, audit repositories.TeamStatusAuditBuilder,
	change func(tx *sql.Tx) (repositories.TeamStatusChange, error),
) (repositories.TeamStatusChange, error) {
	var none repositories.TeamStatusChange
	if audit == nil {
		return none, fmt.Errorf("team status %s: %w", op, errTeamStatusAuditRequired)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return none, fmt.Errorf("failed to begin team status %s: %w", op, err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back team status write", "op", op, "error", rbErr)
		}
	}()

	done, err := change(tx)
	if err != nil {
		return none, err
	}

	entry, err := audit(done)
	if err != nil {
		return none, fmt.Errorf("failed to build team status %s audit entry: %w", op, err)
	}
	if entry == nil {
		return none, fmt.Errorf("team status %s: builder returned no entry: %w", op, errTeamStatusAuditRequired)
	}
	if err = appendTeamSettingsAudit(ctx, tx, entry); err != nil {
		return none, fmt.Errorf("failed to audit team status %s: %w", op, err)
	}

	if err = tx.Commit(); err != nil {
		return none, fmt.Errorf("failed to commit team status %s: %w", op, err)
	}
	return done, nil
}

func (r *TeamStatusRepository) Create(
	ctx context.Context, status *models.TeamStatus, audit repositories.TeamStatusAuditBuilder,
) error {
	done, err := r.runAuditedTx(ctx, "create", audit,
		func(tx *sql.Tx) (repositories.TeamStatusChange, error) {
			// The insert fills a copy: the caller's model is only populated
			// once the row and its audit entry are both committed.
			created := *status
			err := tx.QueryRowContext(ctx, teamStatusInsertSQL,
				created.TeamID, created.ResourceType, created.Slug, created.Name, created.CreatedBy,
			).Scan(&created.ID, &created.CreatedAt, &created.UpdatedAt)
			if err != nil {
				return repositories.TeamStatusChange{}, mapTeamStatusInsertError(err)
			}
			return repositories.TeamStatusChange{Status: &created}, nil
		})
	if err != nil {
		return err
	}
	*status = *done.Status
	return nil
}

// mapTeamStatusInsertError names the two constraint failures an insert into
// team_statuses can hit.
func mapTeamStatusInsertError(err error) error {
	if uniqueViolation(err) != nil {
		return repositories.ErrTeamStatusExists
	}
	if isFKViolation(err) {
		return fmt.Errorf("team or creator not found for team status: %w", err)
	}
	return fmt.Errorf("failed to create team status: %w", err)
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
	ctx context.Context, teamID, id, name string, audit repositories.TeamStatusAuditBuilder,
) (*models.TeamStatus, error) {
	done, err := r.runAuditedTx(ctx, "rename", audit,
		func(tx *sql.Tx) (repositories.TeamStatusChange, error) {
			var previous string
			if err := tx.QueryRowContext(ctx, teamStatusLockNameSQL, teamID, id).Scan(&previous); err != nil {
				return repositories.TeamStatusChange{}, mapNoRows(
					fmt.Errorf("failed to read team status for rename: %w", err),
					repositories.ErrTeamStatusNotFound,
				)
			}
			renamed, err := scanTeamStatus(tx.QueryRowContext(ctx, teamStatusUpdateNameSQL, teamID, id, name))
			if err != nil {
				return repositories.TeamStatusChange{}, mapNoRows(
					fmt.Errorf("failed to rename team status: %w", err),
					repositories.ErrTeamStatusNotFound,
				)
			}
			return repositories.TeamStatusChange{Status: &renamed, PreviousName: previous}, nil
		})
	if err != nil {
		return nil, err
	}
	return done.Status, nil
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
	ctx context.Context, teamID, id, defaultStatus string, audit repositories.TeamStatusAuditBuilder,
) (*models.TeamStatus, int, error) {
	done, err := r.runAuditedTx(ctx, "delete", audit,
		func(tx *sql.Tx) (repositories.TeamStatusChange, error) {
			return deleteAndReassignTeamStatus(ctx, tx, teamID, id, defaultStatus)
		})
	if err != nil {
		return nil, 0, err
	}
	return done.Status, done.ReassignedCount, nil
}

// deleteAndReassignTeamStatus removes the status row and moves the resources
// carrying its slug to defaultStatus, on tx.
func deleteAndReassignTeamStatus(
	ctx context.Context, tx *sql.Tx, teamID, id, defaultStatus string,
) (repositories.TeamStatusChange, error) {
	var none repositories.TeamStatusChange

	// Deleting first takes the row lock, so two concurrent deletes of one
	// status cannot both reassign: the second finds no row.
	deleted, err := scanTeamStatus(tx.QueryRowContext(ctx, teamStatusDeleteReturningSQL, teamID, id))
	if err != nil {
		return none, mapNoRows(
			fmt.Errorf("failed to delete team status: %w", err),
			repositories.ErrTeamStatusNotFound,
		)
	}

	table, ok := teamStatusResourceTables[deleted.ResourceType]
	if !ok {
		return none, fmt.Errorf("%w: %q", errTeamStatusResourceTypeUnknown, deleted.ResourceType)
	}
	// Refusing after the DELETE lets the transaction's rollback restore the
	// status row, so nothing changes.
	if table.locked != "" {
		if err = refuseIfLocked(ctx, tx, repositories.ErrTeamStatusUsedByLockedResource,
			table.locked, teamID, deleted.Slug); err != nil {
			return none, err
		}
	}
	if _, err = tx.ExecContext(ctx, teamStatusPreserveUpdatedAtSQL); err != nil {
		return none, fmt.Errorf("failed to preserve updated_at for status reassignment: %w", err)
	}
	result, err := tx.ExecContext(ctx, table.reassign, teamID, defaultStatus, deleted.Slug)
	if err != nil {
		return none, fmt.Errorf("failed to reassign %s resources after status delete: %w",
			deleted.ResourceType, err)
	}
	reassigned, err := result.RowsAffected()
	if err != nil {
		return none, fmt.Errorf("failed to read status reassignment result: %w", err)
	}
	return repositories.TeamStatusChange{Status: &deleted, ReassignedCount: int(reassigned)}, nil
}

func (r *TeamStatusRepository) ConfirmSlug(
	ctx context.Context, teamID, resourceType, slug string,
) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx, teamStatusConfirmSlugSQL, teamID, resourceType, slug).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to confirm team status: %w", err)
	}
	return true, nil
}

func (r *TeamStatusRepository) ReassignResource(
	ctx context.Context, teamID, resourceType, resourceID, fromSlug, toStatus string,
) error {
	table, ok := teamStatusResourceTables[resourceType]
	if !ok {
		return fmt.Errorf("%w: %q", errTeamStatusResourceTypeUnknown, resourceType)
	}
	result, err := r.db.ExecContext(ctx, table.reassignOne, teamID, toStatus, fromSlug, resourceID)
	if err != nil {
		return fmt.Errorf("failed to reassign %s %s after status delete: %w", resourceType, resourceID, err)
	}
	if table.lockedOne == "" {
		return nil
	}
	// Nothing moved: either the row no longer carries fromSlug, which is fine,
	// or it is locked and was skipped, which the caller has to hear about --
	// the resource still carries a status the catalog no longer lists.
	if moved, rowsErr := result.RowsAffected(); rowsErr != nil || moved > 0 {
		return rowsErr
	}
	return refuseIfLocked(ctx, r.db, repositories.ErrTeamStatusUsedByLockedResource,
		table.lockedOne, teamID, fromSlug, resourceID)
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
