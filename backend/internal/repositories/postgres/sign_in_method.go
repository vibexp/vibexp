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

	if err = lockUserSignInMethods(ctx, tx, userID); err != nil {
		return err
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
	ctx context.Context, userID string, usable repositories.UsableSignInMethods, opts repositories.RemovePasswordOptions,
) error {
	return r.locked(ctx, userID, func(tx *sql.Tx) error {
		var has, disabled bool
		var identities int
		err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM user_credentials WHERE user_id = $1),
			       EXISTS (SELECT 1 FROM user_credentials WHERE user_id = $1 AND disabled_at IS NOT NULL),
			       (SELECT count(*) FROM user_identities
			         WHERE user_id = $1 AND provider = ANY($2) AND provider <> ALL($3))`,
			userID, pq.Array(usable.Providers), pq.Array(internalIdentityProviders)).Scan(&has, &disabled, &identities)
		if err != nil {
			return fmt.Errorf("failed to read the account's sign-in methods: %w", err)
		}
		switch {
		case !has:
			return repositories.ErrUserCredentialNotFound
		case disabled && !opts.AllowDisabled:
			return repositories.ErrUserCredentialDisabled
		case identities == 0 && !opts.AllowLast:
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
			userID, identityID, pq.Array(internalIdentityProviders)).Scan(&provider)
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
			         WHERE user_id = $1 AND id <> $2 AND provider = ANY($3) AND provider <> ALL($5)),
			       $4 AND EXISTS (SELECT 1 FROM user_credentials WHERE user_id = $1 AND disabled_at IS NULL)`,
			userID, identityID, pq.Array(usable.Providers), usable.PasswordLogin,
			pq.Array(internalIdentityProviders)).
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

// internalIdentityProviders are the user_identities.provider values that are
// bookkeeping, not sign-in methods their owner manages: the legacy Google alias
// of another identity and the localhost-only dev login. They are never a
// target and never count as what remains, even if a real provider is later
// given the slug "dev".
var internalIdentityProviders = []string{models.LegacyGoogleIdentityProvider, models.DevIdentityProvider}
