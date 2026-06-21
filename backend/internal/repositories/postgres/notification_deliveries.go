package postgres

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// NotificationDeliveryRepository implements repositories.NotificationDeliveryRepository for PostgreSQL
type NotificationDeliveryRepository struct {
	db *database.DB
}

// NewNotificationDeliveryRepository creates a new NotificationDeliveryRepository
func NewNotificationDeliveryRepository(db *database.DB) repositories.NotificationDeliveryRepository {
	return &NotificationDeliveryRepository{db: db}
}

// Insert persists a notification delivery record
func (r *NotificationDeliveryRepository) Insert(ctx context.Context, d *models.NotificationDelivery) error {
	query := `
		INSERT INTO notification_deliveries
			(notification_id, channel, status, reason, attempts, delivered_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id, created_at
	`

	err := r.db.QueryRowContext(ctx, query,
		d.NotificationID, d.Channel, d.Status, d.Reason, d.Attempts, d.DeliveredAt,
	).Scan(&d.ID, &d.CreatedAt)

	if err != nil {
		return fmt.Errorf("insert notification delivery: %w", err)
	}

	return nil
}
