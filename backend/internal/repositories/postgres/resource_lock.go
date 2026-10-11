package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Lock state for prompts, blueprints and artifacts lives on the resource row
// (migration 019_consolidated, issue #1286). Every read of those three tables projects the
// same four columns -- locked_by, locked_at, lock_comment and the locker's
// display name from a LEFT JOIN on users aliased lock_u -- and scans them
// through resourceLockScan, the one place a ResourceLock is built.
//
// The single-row reads spell the columns and the join out inside their own
// statement literal. The squirrel-built list reads take them from
// resourceLockColumns and resourceLockJoin below.

// resourceLockColumns returns the lock projection for a resource table aliased
// as alias, in the order every reader scans them into a resourceLockScan. locked_by is cast to
// text because the column is a uuid and the scan target is a string.
func resourceLockColumns(alias string) []string {
	return []string{
		alias + ".locked_by::text", alias + ".locked_at", alias + ".lock_comment", "lock_u.name",
	}
}

// resourceLockJoin is the LEFT JOIN operand that resolves the locker's display
// name. users.id is the primary key, so the join matches at most one row and
// cannot duplicate the resource row; LEFT keeps unlocked rows. There is no
// team predicate, unlike projectSummaryJoin: a user is not owned by a team,
// and locked_by is written only by SetLock on behalf of a caller the service
// has already authorized in the resource's own team.
func resourceLockJoin(alias string) string {
	return "users lock_u ON lock_u.id = " + alias + ".locked_by"
}

// resourceLockScan holds the four nullable lock columns while a row is
// scanned. Readers pass its fields to Scan in resourceLockColumns order:
// lockedBy, lockedAt, comment, lockerName.
type resourceLockScan struct {
	lockedBy   sql.NullString
	lockedAt   sql.NullTime
	comment    sql.NullString
	lockerName sql.NullString
}

// value converts the scanned columns to a lock, or nil when the row is
// unlocked.
//
// locked_by alone decides. Deleting the locker's account sets locked_by to
// NULL (ON DELETE SET NULL) and leaves locked_at and lock_comment behind, so a
// reader that tested either of those would report a lock nobody holds.
func (s *resourceLockScan) value() *models.ResourceLock {
	if !s.lockedBy.Valid {
		return nil
	}
	return &models.ResourceLock{
		LockedBy:     s.lockedBy.String,
		LockedByName: s.lockerName.String,
		Comment:      s.comment.String,
		LockedAt:     s.lockedAt.Time,
	}
}

// applyLockedFilter appends the `locked` list filter for a resource table
// aliased as alias. Nil adds nothing, so an unfiltered list is byte-identical
// to what it was before the filter existed. It tests locked_by, the same
// predicate resourceLockScan.value uses, so the filter and the Lock a row
// reports can never disagree.
func applyLockedFilter(where squirrel.And, alias string, locked *bool) squirrel.And {
	if locked == nil {
		return where
	}
	if *locked {
		return append(where, squirrel.Expr(alias+".locked_by IS NOT NULL"))
	}
	return append(where, squirrel.Expr(alias+".locked_by IS NULL"))
}

// The lock write statements, one whole literal per table. Each is a single
// conditional UPDATE: the lock-state predicate in the WHERE is what makes two
// concurrent SetLock calls impossible to both win, with no read-then-write
// window. None touches updated_at or version -- locking is not an edit.
// Tenancy-only: id and team_id, no user or role predicate (decision D3).
const (
	promptSetLockSQL = `
		UPDATE prompts
		   SET locked_by = $3, locked_at = now(), lock_comment = $4
		 WHERE id = $1 AND team_id = $2 AND locked_by IS NULL`

	promptClearLockSQL = `
		UPDATE prompts
		   SET locked_by = NULL, locked_at = NULL, lock_comment = NULL
		 WHERE id = $1 AND team_id = $2 AND locked_by IS NOT NULL`

	blueprintSetLockSQL = `
		UPDATE blueprints
		   SET locked_by = $3, locked_at = now(), lock_comment = $4
		 WHERE id = $1 AND team_id = $2 AND locked_by IS NULL`

	blueprintClearLockSQL = `
		UPDATE blueprints
		   SET locked_by = NULL, locked_at = NULL, lock_comment = NULL
		 WHERE id = $1 AND team_id = $2 AND locked_by IS NOT NULL`

	artifactSetLockSQL = `
		UPDATE artifacts
		   SET locked_by = $3, locked_at = now(), lock_comment = $4
		 WHERE id = $1 AND team_id = $2 AND locked_by IS NULL`

	artifactClearLockSQL = `
		UPDATE artifacts
		   SET locked_by = NULL, locked_at = NULL, lock_comment = NULL
		 WHERE id = $1 AND team_id = $2 AND locked_by IS NOT NULL`
)

// resourceInTeamFunc proves a resource exists within a team, returning the
// resource's own not-found sentinel when it does not. Each repository already
// has one (validatePromptInTeam and friends).
type resourceInTeamFunc func(ctx context.Context, resourceID, teamID string) error

// setResourceLock runs one of the *SetLockSQL statements. When it matches no
// row, either the resource is not in the team or it is already locked;
// inTeam tells the two apart.
func setResourceLock(
	ctx context.Context, db *database.DB, query string, inTeam resourceInTeamFunc,
	teamID, resourceID, lockerID, comment string,
) error {
	result, err := db.ExecContext(ctx, query, resourceID, teamID, lockerID, comment)
	if err != nil {
		return fmt.Errorf("failed to lock resource: %w", err)
	}
	return lockWriteOutcome(ctx, result, inTeam, teamID, resourceID, repositories.ErrResourceAlreadyLocked)
}

// clearResourceLock runs one of the *ClearLockSQL statements. When it matches
// no row, either the resource is not in the team or it is not locked.
func clearResourceLock(
	ctx context.Context, db *database.DB, query string, inTeam resourceInTeamFunc,
	teamID, resourceID string,
) error {
	result, err := db.ExecContext(ctx, query, resourceID, teamID)
	if err != nil {
		return fmt.Errorf("failed to unlock resource: %w", err)
	}
	return lockWriteOutcome(ctx, result, inTeam, teamID, resourceID, repositories.ErrResourceNotLocked)
}

// lockWriteOutcome maps a lock UPDATE's result to an error. A matched row is
// success. Otherwise the existence check runs AFTER the write, so it can only
// ever explain a write that already did nothing: a missing resource reports
// its not-found sentinel, and a present one reports wrongState.
func lockWriteOutcome(
	ctx context.Context, result sql.Result, inTeam resourceInTeamFunc,
	teamID, resourceID string, wrongState error,
) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected > 0 {
		return nil
	}
	if err := inTeam(ctx, resourceID, teamID); err != nil {
		return err
	}
	return wrongState
}
