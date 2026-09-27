package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vibexp/vibexp/internal/database"
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
