package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// DigestJobAdvisoryKey is the PostgreSQL advisory lock key reserved for the digest runner.
// Only one instance may hold this lock at a time, preventing concurrent runs from
// double-sending emails. The value is arbitrary but must be stable across deploys.
const DigestJobAdvisoryKey int64 = 7_481_293_001

// NotificationDigestQueueRepository implements repositories.NotificationDigestQueueRepository for PostgreSQL
type NotificationDigestQueueRepository struct {
	db *database.DB
}

// NewNotificationDigestQueueRepository creates a new NotificationDigestQueueRepository
func NewNotificationDigestQueueRepository(db *database.DB) repositories.NotificationDigestQueueRepository {
	return &NotificationDigestQueueRepository{db: db}
}

// Enqueue adds a notification to the digest delivery queue scheduled for the given time
func (r *NotificationDigestQueueRepository) Enqueue(
	ctx context.Context, userID, notifID string, scheduledFor time.Time,
) error {
	query := `
		INSERT INTO notification_digest_queue
			(user_id, notification_id, scheduled_for, created_at)
		VALUES ($1, $2, $3, NOW())
	`

	_, err := r.db.ExecContext(ctx, query, userID, notifID, scheduledFor)
	if err != nil {
		return fmt.Errorf("enqueue notification for digest: %w", err)
	}

	return nil
}

// FetchPending returns all rows whose scheduled_for is before the given time and sent_at is NULL.
func (r *NotificationDigestQueueRepository) FetchPending(
	ctx context.Context, before time.Time,
) ([]*models.NotificationDigestQueueRow, error) {
	query := `
		SELECT id, user_id, notification_id, scheduled_for, sent_at, created_at
		FROM notification_digest_queue
		WHERE scheduled_for < $1
		  AND sent_at IS NULL
		ORDER BY user_id, created_at
	`

	rows, err := r.db.QueryContext(ctx, query, before)
	if err != nil {
		return nil, fmt.Errorf("fetch pending digest queue rows: %w", err)
	}

	result, scanErr := r.scanDigestQueueRows(rows)
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close digest queue rows: %w", closeErr)
	}

	return result, scanErr
}

func (r *NotificationDigestQueueRepository) scanDigestQueueRows(
	rows interface {
		Next() bool
		Scan(...interface{}) error
		Err() error
	},
) ([]*models.NotificationDigestQueueRow, error) {
	var result []*models.NotificationDigestQueueRow
	for rows.Next() {
		row := &models.NotificationDigestQueueRow{}
		if err := rows.Scan(
			&row.ID, &row.UserID, &row.NotificationID,
			&row.ScheduledFor, &row.SentAt, &row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan digest queue row: %w", err)
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate digest queue rows: %w", err)
	}

	if result == nil {
		result = []*models.NotificationDigestQueueRow{}
	}

	return result, nil
}

// MarkSent sets sent_at = sentAt for the given row IDs that have not yet been marked sent.
// Passing an explicit sentAt makes the operation deterministic and safe for testing.
// The AND sent_at IS NULL guard prevents overwriting an existing timestamp in a concurrent race.
func (r *NotificationDigestQueueRepository) MarkSent(ctx context.Context, ids []string, sentAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = sentAt
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(
		`UPDATE notification_digest_queue SET sent_at = $1 WHERE id IN (%s) AND sent_at IS NULL`,
		strings.Join(placeholders, ", "),
	)

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("mark digest queue rows sent: %w", err)
	}

	return nil
}

// TryAdvisoryLock attempts to acquire a PostgreSQL session-level advisory lock.
// Returns (true, nil) when the lock is acquired, (false, nil) when already held
// by another session (non-blocking — pg_try_advisory_lock returns false immediately),
// or (false, err) on a database error.
func (r *NotificationDigestQueueRepository) TryAdvisoryLock(ctx context.Context, key int64) (bool, error) {
	var locked bool
	if err := r.db.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil {
		return false, fmt.Errorf("acquire digest advisory lock: %w", err)
	}
	return locked, nil
}

// ReleaseAdvisoryLock releases a previously acquired session-level advisory lock.
func (r *NotificationDigestQueueRepository) ReleaseAdvisoryLock(ctx context.Context, key int64) error {
	if _, err := r.db.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, key); err != nil {
		return fmt.Errorf("release digest advisory lock: %w", err)
	}
	return nil
}
