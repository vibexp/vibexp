package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// signInMethodLockNamespace keeps this lock's keys apart from every other
// advisory lock in the database.
const signInMethodLockNamespace = "sign-in-methods:"

// SignInMethodRepository implements repositories.SignInMethodRepository for
// PostgreSQL. Every statement is keyed by the user id: a sign-in method belongs
// to one user and to no team.
type SignInMethodRepository struct {
	db *database.DB
}

// NewSignInMethodRepository creates a new SignInMethodRepository.
func NewSignInMethodRepository(db *database.DB) repositories.SignInMethodRepository {
	return &SignInMethodRepository{db: db}
}

// locked runs fn in a transaction that first takes the user's sign-in method
// lock, and commits when fn returns nil. The lock is transaction-scoped, so it
// is released by the commit or the rollback, on the one connection that took
// it. Everything fn does goes through tx: a waiting caller holds exactly one
// pooled connection and the holder needs no second one, so any number of
// callers finish one after another instead of starving each other of
// connections.
func (r *SignInMethodRepository) locked(ctx context.Context, userID string, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin the sign-in method change: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back a sign-in method change", "error", rollbackErr)
		}
	}()

	_, err = tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, signInMethodLockNamespace+userID)
	if err != nil {
		return fmt.Errorf("failed to take the sign-in method lock: %w", err)
	}
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit the sign-in method change: %w", err)
	}
	return nil
}

// RemovePassword implements repositories.SignInMethodRepository.
func (r *SignInMethodRepository) RemovePassword(
	ctx context.Context, userID string, usable repositories.UsableSignInMethods,
) error {
	return r.locked(ctx, userID, func(tx *sql.Tx) error {
		var has bool
		var identities int
		err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM user_credentials WHERE user_id = $1),
			       (SELECT count(*) FROM user_identities WHERE user_id = $1 AND provider = ANY($2))`,
			userID, pq.Array(usable.Providers)).Scan(&has, &identities)
		if err != nil {
			return fmt.Errorf("failed to read the account's sign-in methods: %w", err)
		}
		if !has {
			return repositories.ErrUserCredentialNotFound
		}
		if identities == 0 {
			return repositories.ErrLastSignInMethod
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM user_credentials WHERE user_id = $1`, userID); err != nil {
			return fmt.Errorf("failed to delete user credential: %w", err)
		}
		return nil
	})
}

// RemoveIdentity implements repositories.SignInMethodRepository.
func (r *SignInMethodRepository) RemoveIdentity(
	ctx context.Context, userID, identityID string, usable repositories.UsableSignInMethods,
) (string, error) {
	var provider string
	err := r.locked(ctx, userID, func(tx *sql.Tx) error {
		// The internal rows are not methods their owner manages.
		err := tx.QueryRowContext(ctx, `
			SELECT provider FROM user_identities
			WHERE id = $2 AND user_id = $1 AND provider <> ALL($3)`,
			userID, identityID,
			pq.Array([]string{models.LegacyGoogleIdentityProvider, internalDevIdentityProvider})).Scan(&provider)
		if errors.Is(err, sql.ErrNoRows) {
			return repositories.ErrUserIdentityNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to read the sign-in identity: %w", err)
		}

		var otherIdentities int
		var usablePassword bool
		err = tx.QueryRowContext(ctx, `
			SELECT (SELECT count(*) FROM user_identities
			         WHERE user_id = $1 AND id <> $2 AND provider = ANY($3)),
			       $4 AND EXISTS (SELECT 1 FROM user_credentials WHERE user_id = $1 AND disabled_at IS NULL)`,
			userID, identityID, pq.Array(usable.Providers), usable.PasswordLogin).
			Scan(&otherIdentities, &usablePassword)
		if err != nil {
			return fmt.Errorf("failed to read the account's sign-in methods: %w", err)
		}
		if otherIdentities == 0 && !usablePassword {
			return repositories.ErrLastSignInMethod
		}

		// The legacy Google alias with the same subject goes with it (a no-op
		// for a non-Google identity, whose subject no legacy row carries).
		_, err = tx.ExecContext(ctx, `
			WITH removed AS (
				DELETE FROM user_identities WHERE id = $2 AND user_id = $1 RETURNING subject
			)
			DELETE FROM user_identities
			WHERE user_id = $1 AND provider = $3 AND subject IN (SELECT subject FROM removed)`,
			userID, identityID, models.LegacyGoogleIdentityProvider)
		if err != nil {
			return fmt.Errorf("failed to delete user identity: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return provider, nil
}

// internalDevIdentityProvider is the provider of the localhost-only dev login.
const internalDevIdentityProvider = "dev"
