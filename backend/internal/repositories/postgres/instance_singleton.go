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

// insertSingletonIfAbsent runs an `INSERT ... ON CONFLICT (id) DO NOTHING
// RETURNING ...` against a singleton table and scans the returned columns into
// dest. It reports whether the row was inserted.
//
// ON CONFLICT DO NOTHING returns no row when one already exists, so
// sql.ErrNoRows is the "already stored" answer rather than a fault. The
// database decides, not a prior read, which is what makes two replicas booting
// at once safe: exactly one of them inserts.
func insertSingletonIfAbsent(
	ctx context.Context, db *database.DB, query string, args []any, dest ...any,
) (bool, error) {
	err := db.QueryRowContext(ctx, query, args...).Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// runAuditedSingletonTx runs change inside a transaction that first takes
// lockSQL (a table-level write lock on the singleton), then hands change the
// current row as read by read (nil when none is stored). It commits only when
// change succeeds and reports it wrote something; any error rolls the whole
// change back.
//
// The table lock, not a row lock, is what serializes audited writers: SELECT
// ... FOR UPDATE on an empty singleton matches nothing and locks nothing, so two
// concurrent first saves would both read before = nil and the audit log would
// lose a transition. Callers pass a SHARE ROW EXCLUSIVE lock, which conflicts
// with itself and every other write but not with plain reads, so requests
// resolving the settings are never blocked.
//
// subject names the settings in errors and logs (e.g. "instance search
// settings"); op names the change ("upsert", "delete").
func runAuditedSingletonTx[T any](
	ctx context.Context, db *database.DB, subject, op, lockSQL string,
	read func(tx *sql.Tx) (*T, error),
	change func(tx *sql.Tx, before *T) (bool, error),
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin %s %s: %w", subject, op, err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			slog.Error("Failed to rollback singleton settings transaction",
				"subject", subject, "op", op, "error", rollbackErr)
		}
	}()

	if _, err = tx.ExecContext(ctx, lockSQL); err != nil {
		return fmt.Errorf("failed to lock %s for %s: %w", subject, op, err)
	}

	before, err := read(tx)
	if errors.Is(err, sql.ErrNoRows) {
		before, err = nil, nil
	}
	if err != nil {
		return fmt.Errorf("failed to read %s for %s: %w", subject, op, err)
	}

	wrote, err := change(tx, before)
	if err != nil || !wrote {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit %s %s: %w", subject, op, err)
	}
	return nil
}

// appendBuiltSingletonAudit builds the audit entry for one singleton change
// through audit and appends it inside tx, so the change and its audit record
// land together or not at all.
func appendBuiltSingletonAudit[T any](
	ctx context.Context, tx *sql.Tx, subject string,
	audit func(before, after *T) (*models.InstanceSettingsAuditEntry, error),
	before, after *T,
) error {
	entry, err := audit(before, after)
	if err != nil {
		return fmt.Errorf("failed to build %s audit entry: %w", subject, err)
	}
	return appendInstanceSettingsAudit(ctx, tx, entry)
}

// checkSingletonVersion applies the optional compare-and-set of an audited
// singleton upsert: expected is the caller's version and stored the row's
// version as read under the lock (nil: no row). The rule itself, including the
// "nothing stored yet" expected version, is
// repositories.InstanceSettingsVersionConflicts.
func checkSingletonVersion(expected, stored *int64) error {
	if repositories.InstanceSettingsVersionConflicts(expected, stored) {
		return repositories.ErrInstanceSettingsVersionConflict
	}
	return nil
}
