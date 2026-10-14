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

// recentAccountLinkExists reports whether the user has an unconsumed link of
// one purpose and delivery that was issued less than $4 seconds ago.
const recentAccountLinkExists = `
	SELECT EXISTS (
		SELECT 1 FROM account_tokens
		WHERE user_id = $1 AND purpose = $2 AND delivery = $3 AND consumed_at IS NULL
		  AND created_at > CURRENT_TIMESTAMP - make_interval(secs => $4))`

// inUserTx runs fn in a transaction that holds userID's sign-in method lock:
// it takes the lock first, runs fn on the same connection, commits when fn
// returns nil and rolls back otherwise. Every operation here that changes what
// an account can sign in with goes through it, which is what makes them
// exclude each other (see Complete). A waiting caller holds exactly one pooled
// connection and the holder needs no second one. A user that does not exist
// is ErrUserNotFound. op names the operation in error messages.
//
// If the commit itself reports an error the outcome is not known for certain
// (the server may have committed before the connection failed). The caller is
// told it failed; for a completion that means a retry may find the link
// already used, which is answered as an unusable link.
func (r *AccountTokenRepository) inUserTx(
	ctx context.Context, op, userID string, fn func(tx *sql.Tx) error,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin account token %s: %w", op, err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back account token transaction", "op", op, "error", rollbackErr)
		}
	}()

	if err = lockUserSignInMethods(ctx, tx, userID); err != nil {
		return err
	}
	// Then the user's row, before any row that references it. Deleting a user
	// locks the users row first and cascades into that user's links and
	// credential; touching a link first and the user second is the opposite
	// order, and the two would deadlock. (The delete does not take the
	// advisory lock, so that lock alone does not order them.) NO KEY UPDATE is
	// enough and does not block inserts that reference the user.
	var exists bool
	err = tx.QueryRowContext(ctx,
		`SELECT true FROM users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&exists)
	if err != nil {
		return mapNoRows(fmt.Errorf("failed to lock the user for account token %s: %w", op, err),
			repositories.ErrUserNotFound)
	}
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
func (r *AccountTokenRepository) Replace(
	ctx context.Context, token *models.AccountToken, unlessIssuedWithin time.Duration,
) error {
	// Under the user's lock: two requests issuing the same kind of link for one
	// user would otherwise both delete, both insert, and the second would fail
	// on the one-live-link index; queued on the lock, the second simply
	// replaces the first. It is the lock Complete and ClearForClaim take, so a
	// link is never issued into the middle of either.
	return r.inUserTx(ctx, "replace", token.UserID, func(tx *sql.Tx) error {
		if err := refuseSelfServiceResetWhileDisabled(ctx, tx, token); err != nil {
			return err
		}
		if err := refuseWhileLiveLinkIsRecent(ctx, tx, token, unlessIssuedWithin); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM account_tokens
			WHERE user_id = $1 AND purpose = $2 AND delivery = $3 AND consumed_at IS NULL`,
			token.UserID, token.Purpose, token.Delivery,
		); err != nil {
			return fmt.Errorf("failed to remove the previous account token: %w", err)
		}

		err := tx.QueryRowContext(ctx, `
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
	})
}

// refuseSelfServiceResetWhileDisabled refuses a reset the owner asked for (no
// administrator behind it) while an administrator has the password disabled:
// completing it would lift the mark. It runs under the lock DisablePassword
// takes, so a disable that commits after the request was screened still wins.
func refuseSelfServiceResetWhileDisabled(ctx context.Context, tx *sql.Tx, token *models.AccountToken) error {
	if token.CreatedBy != nil || token.Purpose != models.AccountTokenPurposeReset {
		return nil
	}
	var disabled bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM user_credentials WHERE user_id = $1 AND disabled_at IS NOT NULL)`,
		token.UserID).Scan(&disabled)
	if err != nil {
		return fmt.Errorf("failed to read whether the password is disabled: %w", err)
	}
	if disabled {
		return repositories.ErrUserCredentialDisabled
	}
	return nil
}

// refuseWhileLiveLinkIsRecent is Replace's age check: it runs under the user's
// lock, in the transaction that replaces, so of simultaneous requests the
// first stores its link and every later one sees it here. Both sides of the
// comparison are the database's clock.
func refuseWhileLiveLinkIsRecent(
	ctx context.Context, tx *sql.Tx, token *models.AccountToken, unlessIssuedWithin time.Duration,
) error {
	if unlessIssuedWithin <= 0 {
		return nil
	}
	var tooRecent bool
	err := tx.QueryRowContext(ctx, recentAccountLinkExists,
		token.UserID, token.Purpose, token.Delivery, unlessIssuedWithin.Seconds()).Scan(&tooRecent)
	if err != nil {
		return fmt.Errorf("failed to read the age of the live account token: %w", err)
	}
	if tooRecent {
		return repositories.ErrAccountTokenTooRecent
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

// completeAccountLinkUser is the one statement that changes the user row when
// a link is completed: every earlier session ends, an emailed link verifies an
// address that was not verified yet, and a setup may set the display name.
// The row's version moves, so a stale UserRepository.Update running beside it
// fails instead of writing the old name or an unverified address back.
const completeAccountLinkUser = `
	UPDATE users
	SET session_epoch = session_epoch + 1,
	    email_verified_at = CASE WHEN $2 THEN COALESCE(email_verified_at, $3) ELSE email_verified_at END,
	    name = CASE WHEN $4 <> '' THEN $4 ELSE name END,
	    updated_at = CURRENT_TIMESTAMP,
	    version = version + 1
	WHERE id = $1
	RETURNING session_epoch, name, email_verified_at, updated_at, version`

// Complete completes the link in one transaction; see the interface.
//
// The transaction takes the user's sign-in method lock before anything else
// and does every read and write on its own connection. That is what makes a
// completion exclude an account claim (ClearForClaim takes the same lock), the
// issue of a new link (Replace) and another completion of any link of the same
// user: they run one at a time, and each later one sees what the earlier one
// committed. A completion that runs after a claim finds its link deleted and
// writes nothing; a claim that runs after a completion removes the password it
// stored.
//
// The lock's key needs the user, which only the link knows, so the link is read
// once without the lock (and outside the transaction) to find its user, and
// then consumed under the lock by a statement that matches it only if it is
// still live.
func (r *AccountTokenRepository) Complete(
	ctx context.Context, completion repositories.AccountLinkCompletion,
) (*repositories.CompletedAccountLink, error) {
	var userID string
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id FROM account_tokens WHERE token_hash = $1`, completion.TokenHash,
	).Scan(&userID)
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to find account token: %w", err),
			repositories.ErrAccountTokenNotFound)
	}

	var done *repositories.CompletedAccountLink
	err = r.inUserTx(ctx, "complete", userID, func(tx *sql.Tx) error {
		var txErr error
		done, txErr = completeAccountLinkTx(ctx, tx, userID, completion)
		return txErr
	})
	if errors.Is(err, repositories.ErrUserNotFound) {
		// The user was deleted after the link was looked up, and the link
		// with them.
		return nil, repositories.ErrAccountTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return done, nil
}

// completeAccountLinkTx is the body of Complete, run under the user's lock.
func completeAccountLinkTx(
	ctx context.Context, tx *sql.Tx, userID string, completion repositories.AccountLinkCompletion,
) (*repositories.CompletedAccountLink, error) {
	consumed, err := scanAccountToken(
		tx.QueryRowContext(ctx, consumeAccountLink, completion.TokenHash, completion.Now))
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

	if _, err = tx.ExecContext(ctx, storeUserHashQuery, userID, completion.PasswordHash); err != nil {
		// The driver's error names the constraint, never the hash.
		return nil, fmt.Errorf("failed to store the password of a completed account link: %w", err)
	}

	done := &repositories.CompletedAccountLink{Token: consumed}
	err = tx.QueryRowContext(ctx, completeAccountLinkUser,
		userID, consumed.ProvesMailbox(), completion.Now, completion.Name,
	).Scan(&done.SessionEpoch, &done.Name, &done.EmailVerifiedAt, &done.UpdatedAt, &done.Version)
	if err != nil {
		// No row means the user was deleted meanwhile, and the link with them.
		return nil, mapNoRows(fmt.Errorf("failed to update the user of a completed account link: %w", err),
			repositories.ErrAccountTokenNotFound)
	}
	return done, nil
}

// ClearForClaim removes what a link's holder could have set up on the account;
// see the interface. One transaction under the user's sign-in method lock, the
// lock Complete takes: a completion cannot land between the deletes, and one
// that queued behind this finds its link gone.
func (r *AccountTokenRepository) ClearForClaim(
	ctx context.Context, userID string,
) (*repositories.ClearedAccountClaim, error) {
	cleared := &repositories.ClearedAccountClaim{}
	err := r.inUserTx(ctx, "clear for claim", userID, func(tx *sql.Tx) error {
		links, err := tx.ExecContext(ctx, `DELETE FROM account_tokens WHERE user_id = $1`, userID)
		if err != nil {
			return fmt.Errorf("failed to delete the account tokens of a claimed account: %w", err)
		}
		if cleared.Links, err = links.RowsAffected(); err != nil {
			return fmt.Errorf("failed to count deleted account tokens: %w", err)
		}

		credentials, err := tx.ExecContext(ctx, `DELETE FROM user_credentials WHERE user_id = $1`, userID)
		if err != nil {
			return fmt.Errorf("failed to delete the password of a claimed account: %w", err)
		}
		removed, err := credentials.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to count deleted credentials: %w", err)
		}
		cleared.HadPassword = removed > 0

		err = tx.QueryRowContext(ctx, `
			UPDATE users SET session_epoch = session_epoch + 1 WHERE id = $1 RETURNING session_epoch`, userID,
		).Scan(&cleared.SessionEpoch)
		if err != nil {
			return mapNoRows(fmt.Errorf("failed to end the sessions of a claimed account: %w", err),
				repositories.ErrUserNotFound)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cleared, nil
}

// DisablePassword is an administrator's "invalidate now"; see the interface.
// One transaction under the user's sign-in method lock: a link issued, or
// completed, concurrently lands wholly before (and is deleted or overridden
// here) or wholly after.
func (r *AccountTokenRepository) DisablePassword(
	ctx context.Context, userID string,
) (*repositories.DisabledAccountPassword, error) {
	disabled := &repositories.DisabledAccountPassword{}
	err := r.inUserTx(ctx, "disable password", userID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			UPDATE user_credentials SET disabled_at = COALESCE(disabled_at, CURRENT_TIMESTAMP),
				updated_at = CURRENT_TIMESTAMP
			WHERE user_id = $1 RETURNING true`, userID).Scan(&disabled.HadPassword)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to disable the password: %w", err)
		}

		links, err := tx.ExecContext(ctx,
			`DELETE FROM account_tokens WHERE user_id = $1 AND consumed_at IS NULL`, userID)
		if err != nil {
			return fmt.Errorf("failed to delete the live account tokens of a disabled password: %w", err)
		}
		if disabled.Links, err = links.RowsAffected(); err != nil {
			return fmt.Errorf("failed to count deleted account tokens: %w", err)
		}

		err = tx.QueryRowContext(ctx, `
			UPDATE users SET session_epoch = session_epoch + 1 WHERE id = $1 RETURNING session_epoch`, userID,
		).Scan(&disabled.SessionEpoch)
		if err != nil {
			return mapNoRows(fmt.Errorf("failed to end the sessions of a disabled password: %w", err),
				repositories.ErrUserNotFound)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return disabled, nil
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
