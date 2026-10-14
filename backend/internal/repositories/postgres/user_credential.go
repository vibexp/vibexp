package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// UserCredentialRepository implements repositories.UserCredentialRepository
// for PostgreSQL. Every statement is keyed by the user id alone: a credential
// belongs to one user and to no team, so there is no tenancy or role predicate.
type UserCredentialRepository struct {
	db *database.DB
}

// NewUserCredentialRepository creates a new UserCredentialRepository.
func NewUserCredentialRepository(db *database.DB) repositories.UserCredentialRepository {
	return &UserCredentialRepository{db: db}
}

const userCredentialColumns = `user_id, password_hash, failed_attempts, locked_until, disabled_at,
		password_changed_at, created_at, updated_at`

func scanUserCredential(row rowScanner) (*models.UserCredential, error) {
	var c models.UserCredential
	if err := row.Scan(
		&c.UserID, &c.PasswordHash, &c.FailedAttempts, &c.LockedUntil, &c.DisabledAt,
		&c.PasswordChangedAt, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// Get returns the user's credential, or ErrUserCredentialNotFound.
func (r *UserCredentialRepository) Get(ctx context.Context, userID string) (*models.UserCredential, error) {
	query := `SELECT ` + userCredentialColumns + ` FROM user_credentials WHERE user_id = $1`

	c, err := scanUserCredential(r.db.QueryRowContext(ctx, query, userID))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get user credential: %w", err),
			repositories.ErrUserCredentialNotFound)
	}
	return c, nil
}

// Upsert sets the user's password hash; see the interface. A new password is a
// fresh start, so the throttle and a disabled mark are cleared with it. It is
// the one statement that moves password_changed_at (the column default covers
// the insert): attempts, successes and a rehash leave it alone.
func (r *UserCredentialRepository) Upsert(ctx context.Context, userID, passwordHash string) error {
	query := `
		INSERT INTO user_credentials (user_id, password_hash)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			failed_attempts = 0,
			locked_until = NULL,
			disabled_at = NULL,
			password_changed_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`

	if _, err := r.db.ExecContext(ctx, query, userID, passwordHash); err != nil {
		if isFKViolation(err) {
			return fmt.Errorf("%w: %s", repositories.ErrUserNotFound, userID)
		}
		// The error is the driver's and names the constraint, never the hash.
		return fmt.Errorf("failed to store user credential: %w", err)
	}
	return nil
}

// UpdateHash replaces the hash only while it still equals currentHash, and
// leaves the throttle alone; see the interface. The comparison is in the WHERE
// clause, so a password changed between the caller's check and this write is
// never overwritten.
func (r *UserCredentialRepository) UpdateHash(
	ctx context.Context, userID, currentHash, newHash string,
) (bool, error) {
	query := `
		UPDATE user_credentials
		SET password_hash = $3, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND password_hash = $2
	`

	result, err := r.db.ExecContext(ctx, query, userID, currentHash, newHash)
	if err != nil {
		return false, fmt.Errorf("failed to update user credential hash: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to update user credential hash: %w", err)
	}
	return affected > 0, nil
}

// maxLockoutDoublings bounds the exponent of the lock computation in SQL. The
// result is capped by the schedule's maximum long before this many doublings;
// the bound only keeps power() from overflowing on a very large failure count.
const maxLockoutDoublings = 30

// userCredentialBeginAttempt admits and counts one sign-in attempt in one
// statement.
//
// $2 now, $3 threshold, $4 base seconds, $5 max seconds, $6 doubling bound.
//
// The WHERE clause is the lock: a credential that is disabled, or whose lock is
// still running at $2, matches no row and nothing is written. Otherwise the
// count goes up and, from the threshold-th attempt on, the lock is set to
// base * 2^(count - threshold) capped at max, the same schedule as
// models.PasswordLockout.LockFor (pinned against it by
// TestIntegrationUserCredential_LockoutLadderMatchesTheSchedule).
//
// Because the UPDATE takes the row lock, attempts arriving together are
// admitted one at a time, and each re-evaluates the WHERE clause against the
// row the previous one committed: once the threshold-th has set the lock, the
// rest match nothing. That is what bounds a burst to the threshold.
const userCredentialBeginAttempt = `
	UPDATE user_credentials
	SET failed_attempts = failed_attempts + 1,
		locked_until = CASE
			WHEN failed_attempts + 1 >= $3::integer THEN
				$2::timestamptz + make_interval(secs => LEAST(
					$4::double precision
						* power(2::double precision, LEAST(failed_attempts + 1 - $3::integer, $6::integer)),
					$5::double precision))
			ELSE locked_until
		END,
		updated_at = CURRENT_TIMESTAMP
	WHERE user_id = $1
		AND disabled_at IS NULL
		AND (locked_until IS NULL OR locked_until <= $2::timestamptz)
	RETURNING ` + userCredentialColumns

// BeginAttempt admits and counts one sign-in attempt; see the interface.
func (r *UserCredentialRepository) BeginAttempt(
	ctx context.Context, userID string, now time.Time, lockout models.PasswordLockout,
) (*models.UserCredential, error) {
	c, err := scanUserCredential(r.db.QueryRowContext(ctx, userCredentialBeginAttempt,
		userID, now, lockout.Threshold, lockout.Base.Seconds(), lockout.Max.Seconds(), maxLockoutDoublings))
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to begin a sign-in attempt: %w", err)
	}
	// No row was updated: there is no credential, or it is disabled, or it is
	// locked. One read tells the three apart.
	stored, getErr := r.Get(ctx, userID)
	if getErr != nil {
		return nil, getErr
	}
	if stored.IsDisabled() {
		return nil, repositories.ErrUserCredentialDisabled
	}
	return nil, repositories.ErrUserCredentialLocked
}

// RecordSuccess clears the failure count and any lock; see the interface. The
// WHERE clause skips the write on the common path, a credential with nothing to
// clear.
func (r *UserCredentialRepository) RecordSuccess(ctx context.Context, userID string) error {
	query := `
		UPDATE user_credentials
		SET failed_attempts = 0, locked_until = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND (failed_attempts <> 0 OR locked_until IS NOT NULL)
	`

	if _, err := r.db.ExecContext(ctx, query, userID); err != nil {
		return fmt.Errorf("failed to reset sign-in throttle: %w", err)
	}
	return nil
}

// Delete removes the user's credential; see the interface.
func (r *UserCredentialRepository) Delete(ctx context.Context, userID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM user_credentials WHERE user_id = $1`, userID)
	if err != nil {
		return false, fmt.Errorf("failed to delete user credential: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to delete user credential: %w", err)
	}
	return affected > 0, nil
}

// CountUsableForAdmins counts instance admins with a usable password; see the
// interface. Admin-ness is passed in (the config addresses) or read from the
// instance_admins grants; nothing here decides what an admin may do.
func (r *UserCredentialRepository) CountUsableForAdmins(ctx context.Context, rootEmails []string) (int, error) {
	query := `
		SELECT count(*)
		FROM user_credentials c
		JOIN users u ON u.id = c.user_id
		WHERE c.disabled_at IS NULL
		  AND u.status IS DISTINCT FROM $1
		  AND (u.email = ANY($2) OR EXISTS (SELECT 1 FROM instance_admins a WHERE a.user_id = u.id))`

	var count int
	err := r.db.QueryRowContext(ctx, query, models.UserStatusSuspended, pq.Array(rootEmails)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count admins with a usable password: %w", err)
	}
	return count, nil
}

// CountPasswordOnly counts accounts whose only sign-in method is a password;
// see the interface.
func (r *UserCredentialRepository) CountPasswordOnly(ctx context.Context) (int, error) {
	query := `
		SELECT count(*)
		FROM user_credentials c
		WHERE NOT EXISTS (
			SELECT 1 FROM user_identities i WHERE i.user_id = c.user_id AND i.provider <> $1)`

	var count int
	if err := r.db.QueryRowContext(ctx, query, models.DevIdentityProvider).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count password-only accounts: %w", err)
	}
	return count, nil
}
