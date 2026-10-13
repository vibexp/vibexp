package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// signInMethodLockNamespace keeps this lock's keys apart from every other
// advisory lock in the database.
const signInMethodLockNamespace = "sign-in-methods:"

// SignInMethodLock implements repositories.SignInMethodLock with a PostgreSQL
// transaction-scoped advisory lock.
type SignInMethodLock struct {
	db *database.DB
}

// NewSignInMethodLock creates a new SignInMethodLock.
func NewSignInMethodLock(db *database.DB) repositories.SignInMethodLock {
	return &SignInMethodLock{db: db}
}

// WithLock implements repositories.SignInMethodLock.
//
// The lock lives in a transaction opened only to hold it: fn runs on the
// repositories' own connections and commits its work itself, and the lock is
// released when this transaction ends, whichever way fn returned. An advisory
// lock is used rather than a lock on the users row, which fn's own statements
// (on other connections) could then wait on forever.
func (l *SignInMethodLock) WithLock(
	ctx context.Context, userID string, fn func(ctx context.Context) error,
) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin the sign-in method lock: %w", err)
	}
	// Rollback releases the lock; nothing was written in this transaction.
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			slog.Error("Failed to release the sign-in method lock", "error", rollbackErr)
		}
	}()

	_, err = tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, signInMethodLockNamespace+userID)
	if err != nil {
		return fmt.Errorf("take the sign-in method lock: %w", err)
	}
	return fn(ctx)
}
