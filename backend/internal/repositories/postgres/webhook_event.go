package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// WebhookEventRepository implements the WebhookEventRepository interface
type WebhookEventRepository struct {
	db *sql.DB
}

// Ensure WebhookEventRepository implements the interface
var _ repositories.WebhookEventRepository = (*WebhookEventRepository)(nil)

// NewWebhookEventRepository creates a new WebhookEventRepository
func NewWebhookEventRepository(db *sql.DB) *WebhookEventRepository {
	return &WebhookEventRepository{db: db}
}

// IsProcessed checks if a webhook event has already been processed
func (r *WebhookEventRepository) IsProcessed(ctx context.Context, eventID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM webhook_events WHERE event_id = $1)`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, eventID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check webhook event: %w", err)
	}

	return exists, nil
}

// MarkProcessed records a webhook event as processed
func (r *WebhookEventRepository) MarkProcessed(ctx context.Context, eventID, eventType string, teamID *string) error {
	query := `
		INSERT INTO webhook_events (event_id, event_type, team_id, processed_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (event_id) DO NOTHING
	`

	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, eventID, eventType, teamID, now, now)
	if err != nil {
		return fmt.Errorf("failed to mark webhook event as processed: %w", err)
	}

	return nil
}

// GetByEventID retrieves a webhook event by its Stripe event ID
func (r *WebhookEventRepository) GetByEventID(ctx context.Context, eventID string) (*models.WebhookEvent, error) {
	query := `
		SELECT id, event_id, event_type, processed_at, team_id, created_at
		FROM webhook_events
		WHERE event_id = $1
	`

	var event models.WebhookEvent
	err := r.db.QueryRowContext(ctx, query, eventID).Scan(
		&event.ID,
		&event.EventID,
		&event.EventType,
		&event.ProcessedAt,
		&event.TeamID,
		&event.CreatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get webhook event: %w", err), repositories.ErrWebhookEventNotFound)
	}

	return &event, nil
}
