package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// userSignInLockNamespace keeps this lock's keys apart from every other
// advisory lock in the database.
const userSignInLockNamespace = "sign-in-methods:"

// lockUserSignInMethods takes the per-user lock that serializes every change
// to WHICH sign-in methods an account has, when that change depends on the
// methods it already has (removing one but never the last, #1437). It is the
// one lock for that purpose: a new check-then-write over an account's password
// or identities takes it through this function, so the paths exclude each
// other.
//
// The lock is transaction-scoped: it is released when tx commits or rolls
// back. Take it FIRST, then read and write through the SAME tx. Running the
// work on other pooled connections while tx only holds the lock starves the
// pool: every waiter keeps a connection and the holder needs more.
func lockUserSignInMethods(ctx context.Context, tx *sql.Tx, userID string) error {
	_, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userSignInLockNamespace+userID)
	if err != nil {
		return fmt.Errorf("failed to take the user's sign-in method lock: %w", err)
	}
	return nil
}
