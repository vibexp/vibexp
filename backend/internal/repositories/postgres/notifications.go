package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// NotificationRepository implements repositories.NotificationRepository for PostgreSQL
type NotificationRepository struct {
	db *database.DB
}

// NewNotificationRepository creates a new NotificationRepository
func NewNotificationRepository(db *database.DB) repositories.NotificationRepository {
	return &NotificationRepository{db: db}
}

// Insert persists a notification. When DedupeKey is set the INSERT uses
// ON CONFLICT (recipient_user_id, dedupe_key) DO NOTHING so duplicate
// calls with the same key are silently ignored: the RETURNING clause then
// yields no row and Insert returns nil without populating n.ID/n.CreatedAt.
func (r *NotificationRepository) Insert(ctx context.Context, n *models.Notification) error {
	var entityRefJSON []byte
	if n.EntityRef != nil {
		entityRefJSON = n.EntityRef
	}

	var dedupeKey *string
	if n.DedupeKey != "" {
		dedupeKey = &n.DedupeKey
	}

	var teamID *string
	if n.TeamID != "" {
		teamID = &n.TeamID
	}

	query := `
		INSERT INTO notifications
			(recipient_user_id, team_id, type, category, title, body, action_url,
			 entity_ref, dedupe_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (recipient_user_id, dedupe_key)
		WHERE dedupe_key IS NOT NULL
		DO NOTHING
		RETURNING id, created_at
	`

	row := r.db.QueryRowContext(ctx, query,
		n.RecipientUserID, teamID, n.Type, n.Category,
		n.Title, n.Body, n.ActionURL, entityRefJSON, dedupeKey,
	)

	err := row.Scan(&n.ID, &n.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// ON CONFLICT DO NOTHING — dedupe key already exists, return silently
		return nil
	}

	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}

	return nil
}

// notificationColumns is the projection shared by ListForUser and
// GetByIDsForUser, scanned by scanNotificationRows.
var notificationColumns = []string{
	"id", "recipient_user_id", "COALESCE(team_id::text,'')", "type", "category",
	"title", "COALESCE(body,'')", "COALESCE(action_url,'')",
	"entity_ref", "COALESCE(dedupe_key,'')",
	"read_at", "dismissed_at", "created_at",
}

// ListForUser returns paginated notifications for a user, optionally filtering unread-only
func (r *NotificationRepository) ListForUser(
	ctx context.Context, userID string, f repositories.NotificationListFilters,
) ([]*models.Notification, error) {
	// Clamp paging inputs before the unsigned conversion squirrel requires; the
	// guards also prove non-negativity to the overflow linter. Invalid paging
	// silently clamps to the defaults instead of erroring at Postgres.
	limit := uint64(20)
	if f.Limit > 0 {
		limit = uint64(f.Limit)
	}
	offset := uint64(0)
	if f.Offset > 0 {
		offset = uint64(f.Offset)
	}

	builder := psql.
		Select(notificationColumns...).
		From("notifications").
		Where(squirrel.Eq{"recipient_user_id": userID})

	if f.UnreadOnly {
		builder = builder.Where(squirrel.Expr("read_at IS NULL AND dismissed_at IS NULL"))
	}

	query, args, err := builder.
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list notifications for user: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list notifications for user: %w", err)
	}

	result, scanErr := r.scanNotificationRows(rows)
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close notification rows: %w", closeErr)
	}

	return result, scanErr
}

func (r *NotificationRepository) scanNotificationRows(
	rows interface {
		Next() bool
		Scan(...interface{}) error
		Err() error
	},
) ([]*models.Notification, error) {
	var notifications []*models.Notification

	for rows.Next() {
		var n models.Notification
		var entityRefRaw []byte

		if err := rows.Scan(
			&n.ID, &n.RecipientUserID, &n.TeamID, &n.Type, &n.Category,
			&n.Title, &n.Body, &n.ActionURL,
			&entityRefRaw, &n.DedupeKey,
			&n.ReadAt, &n.DismissedAt, &n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification row: %w", err)
		}

		if len(entityRefRaw) > 0 {
			n.EntityRef = json.RawMessage(entityRefRaw)
		}

		notifications = append(notifications, &n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification rows: %w", err)
	}

	if notifications == nil {
		notifications = []*models.Notification{}
	}

	return notifications, nil
}

// GetByIDsForUser returns the notifications matching ids that belong to the given userID.
// The AND recipient_user_id = $1 filter is defence-in-depth: it ensures a future bug
// that enqueues user B's notification ID for user A cannot leak B's content to A's digest.
// Unknown IDs or IDs belonging to other users are silently skipped.
func (r *NotificationRepository) GetByIDsForUser(
	ctx context.Context, userID string, ids []string,
) ([]*models.Notification, error) {
	if len(ids) == 0 {
		return []*models.Notification{}, nil
	}

	// recipient_user_id is bound first so it stays $1; squirrel.Eq{"id": ids}
	// expands the IN list and binds each id after it.
	query, args, err := psql.
		Select(notificationColumns...).
		From("notifications").
		Where(squirrel.Eq{"recipient_user_id": userID}).
		Where(squirrel.Eq{"id": ids}).
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get notifications by ids for user: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get notifications by ids for user: %w", err)
	}

	result, scanErr := r.scanNotificationRows(rows)
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close notification rows: %w", closeErr)
	}

	return result, scanErr
}

// GetUnreadCount returns the count of unread, non-dismissed notifications for a user
func (r *NotificationRepository) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM notifications
		WHERE recipient_user_id = $1
		  AND read_at IS NULL
		  AND dismissed_at IS NULL
	`

	var count int
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("get unread notification count: %w", err)
	}

	return count, nil
}

// MarkRead marks a single notification as read for the given user
func (r *NotificationRepository) MarkRead(ctx context.Context, userID, notifID string) error {
	query := `
		UPDATE notifications
		SET read_at = NOW()
		WHERE id = $1
		  AND recipient_user_id = $2
		  AND read_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, notifID, userID)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}

	return nil
}

// MarkAllRead marks all unread notifications as read for the given user
func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID string) error {
	query := `
		UPDATE notifications
		SET read_at = NOW()
		WHERE recipient_user_id = $1
		  AND read_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}

	return nil
}

// DeleteOlderThan deletes notifications created before the given time.
// CASCADE constraints handle notification_deliveries and notification_digest_queue rows.
func (r *NotificationRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	query := `DELETE FROM notifications WHERE created_at < $1`

	result, err := r.db.ExecContext(ctx, query, before)
	if err != nil {
		return 0, fmt.Errorf("delete old notifications: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}

	return count, nil
}
