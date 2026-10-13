package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// AccountTokenRepository implements repositories.AccountTokenRepository for
// PostgreSQL. A link belongs to one user and to no team, so there is no
// tenancy or role predicate; lookups are by token hash.
type AccountTokenRepository struct {
	db *database.DB
}

// NewAccountTokenRepository creates a new AccountTokenRepository.
func NewAccountTokenRepository(db *database.DB) repositories.AccountTokenRepository {
	return &AccountTokenRepository{db: db}
}

const accountLinkColumns = `id, user_id, purpose, delivery, token_hash, expires_at, consumed_at,
		created_by, created_at`

func scanAccountToken(row rowScanner) (*models.AccountToken, error) {
	var t models.AccountToken
	if err := row.Scan(
		&t.ID, &t.UserID, &t.Purpose, &t.Delivery, &t.TokenHash, &t.ExpiresAt, &t.ConsumedAt,
		&t.CreatedBy, &t.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &t, nil
}

// consumeAccountLink marks a link consumed if it is still live at $2. Its
// RETURNING list is accountLinkColumns written out, because the statement
// runs on a transaction and must be one literal; scanAccountToken reads both,
// and TestAccountLinkQueriesReturnTheSameColumns keeps them equal.
const consumeAccountLink = `
	UPDATE account_tokens
	SET consumed_at = $2
	WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
	RETURNING id, user_id, purpose, delivery, token_hash, expires_at, consumed_at,
		created_by, created_at`

// inTx runs fn in a transaction: it commits when fn returns nil and rolls back
// otherwise. op names the operation in error messages.
func (r *AccountTokenRepository) inTx(ctx context.Context, op string, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin account token %s: %w", op, err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back account token transaction", "op", op, "error", rollbackErr)
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit account token %s: %w", op, err)
	}
	return nil
}

// Replace stores token as the user's one live link of its purpose and
// delivery; see the interface. The delete and the insert share a transaction,
// so there is never a moment with two live links of one kind, and never one
// with none because the insert failed.
func (r *AccountTokenRepository) Replace(ctx context.Context, token *models.AccountToken) error {
	return r.inTx(ctx, "replace", func(tx *sql.Tx) error {
		return replaceAccountLink(ctx, tx, token)
	})
}

func replaceAccountLink(ctx context.Context, tx *sql.Tx, token *models.AccountToken) error {
	// Lock the user's row first. Two requests issuing the same kind of link for
	// one user would otherwise both delete, both insert, and the second would
	// fail on the one-live-link index; queued on this lock, the second simply
	// replaces the first. NO KEY UPDATE is enough and does not block inserts
	// that reference the user.
	var locked bool
	err := tx.QueryRowContext(ctx,
		`SELECT true FROM users WHERE id = $1 FOR NO KEY UPDATE`, token.UserID).Scan(&locked)
	if err != nil {
		return mapNoRows(fmt.Errorf("failed to lock the user of an account token: %w", err),
			repositories.ErrUserNotFound)
	}

	if _, err = tx.ExecContext(ctx, `
		DELETE FROM account_tokens
		WHERE user_id = $1 AND purpose = $2 AND delivery = $3 AND consumed_at IS NULL`,
		token.UserID, token.Purpose, token.Delivery,
	); err != nil {
		return fmt.Errorf("failed to remove the previous account token: %w", err)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO account_tokens (user_id, purpose, delivery, token_hash, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		token.UserID, token.Purpose, token.Delivery, token.TokenHash, token.ExpiresAt, token.CreatedBy,
	).Scan(&token.ID, &token.CreatedAt)
	if err != nil {
		if isFKViolation(err) {
			return fmt.Errorf("%w: %w", repositories.ErrUserNotFound, err)
		}
		// The driver's error names the constraint, never the hash.
		return fmt.Errorf("failed to store account token: %w", err)
	}
	return nil
}

// GetLive returns the link when it can still be completed at now; see the
// interface.
func (r *AccountTokenRepository) GetLive(
	ctx context.Context, tokenHash []byte, now time.Time,
) (*models.AccountToken, error) {
	query := `SELECT ` + accountLinkColumns + `
		FROM account_tokens
		WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2`

	t, err := scanAccountToken(r.db.QueryRowContext(ctx, query, tokenHash, now))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get account token: %w", err),
			repositories.ErrAccountTokenNotFound)
	}
	return t, nil
}

// Consume marks the link consumed, removes every other live link of the user
// and returns the consumed link; see the interface.
//
// A user may hold several live links at once (emailed and copied, setup and
// reset), and completing any one must end them all: a link left live after
// the password was set through another would be a way back in for whoever
// holds it. So the transaction first locks the user's
// row, the same lock Replace takes. Requests completing links of one user,
// the same link or different ones, therefore run one at a time: the first
// consumes its link and deletes the rest, and each later one finds its link
// consumed or gone. A link issued while a completion is in flight waits for it
// and survives it.
func (r *AccountTokenRepository) Consume(
	ctx context.Context, tokenHash []byte, now time.Time,
) (*models.AccountToken, error) {
	var consumed *models.AccountToken
	err := r.inTx(ctx, "consume", func(tx *sql.Tx) error {
		var err error
		consumed, err = consumeAccountLinkTx(ctx, tx, tokenHash, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return consumed, nil
}

func consumeAccountLinkTx(
	ctx context.Context, tx *sql.Tx, tokenHash []byte, now time.Time,
) (*models.AccountToken, error) {
	var userID string
	err := tx.QueryRowContext(ctx,
		`SELECT user_id FROM account_tokens WHERE token_hash = $1`, tokenHash,
	).Scan(&userID)
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to find account token: %w", err),
			repositories.ErrAccountTokenNotFound)
	}

	var locked bool
	err = tx.QueryRowContext(ctx,
		`SELECT true FROM users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&locked)
	if err != nil {
		// The user was deleted in between, and the link with them.
		return nil, mapNoRows(fmt.Errorf("failed to lock the user of an account token: %w", err),
			repositories.ErrAccountTokenNotFound)
	}

	consumed, err := scanAccountToken(tx.QueryRowContext(ctx, consumeAccountLink, tokenHash, now))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to consume account token: %w", err),
			repositories.ErrAccountTokenNotFound)
	}

	if _, err = tx.ExecContext(ctx, `
		DELETE FROM account_tokens
		WHERE user_id = $1 AND consumed_at IS NULL`, userID,
	); err != nil {
		return nil, fmt.Errorf("failed to remove the user's other account tokens: %w", err)
	}
	return consumed, nil
}

// Delete removes the link with this token hash; see the interface.
func (r *AccountTokenRepository) Delete(ctx context.Context, tokenHash []byte) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM account_tokens WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return false, fmt.Errorf("failed to delete account token: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to count deleted account tokens: %w", err)
	}
	return removed > 0, nil
}

// DeleteForUser deletes every link of the user; see the interface.
func (r *AccountTokenRepository) DeleteForUser(ctx context.Context, userID string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM account_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete the account tokens of a user: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to count deleted account tokens: %w", err)
	}
	return removed, nil
}

// DeleteDead deletes every consumed or expired link; see the interface.
func (r *AccountTokenRepository) DeleteDead(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM account_tokens WHERE consumed_at IS NOT NULL OR expires_at <= $1`, now)
	if err != nil {
		return 0, fmt.Errorf("failed to delete dead account tokens: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to count deleted account tokens: %w", err)
	}
	return removed, nil
}
