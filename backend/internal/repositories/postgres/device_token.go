package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// DeviceTokenRepository implements repositories.DeviceTokenRepository for PostgreSQL
type DeviceTokenRepository struct {
	db *database.DB
}

// NewDeviceTokenRepository creates a new DeviceTokenRepository
func NewDeviceTokenRepository(db *database.DB) repositories.DeviceTokenRepository {
	return &DeviceTokenRepository{db: db}
}

// Upsert inserts or updates a device token. On conflict the UPDATE is only applied
// when the existing row belongs to the same user (device_tokens.user_id = EXCLUDED.user_id).
// If the token is already claimed by a different user the conditional WHERE clause causes
// no row to be updated; Upsert detects the zero-row result and returns
// repositories.ErrDeviceTokenConflict so the caller can surface a 409 response.
func (r *DeviceTokenRepository) Upsert(ctx context.Context, token *models.DeviceToken) error {
	query := `
		INSERT INTO device_tokens (user_id, token, platform, user_agent, last_used_at, created_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (token) DO UPDATE SET
			last_used_at = NOW(),
			user_agent   = EXCLUDED.user_agent
		WHERE device_tokens.user_id = EXCLUDED.user_id
		RETURNING id
	`

	var id string
	err := r.db.QueryRowContext(ctx, query,
		token.UserID, token.Token, token.Platform, token.UserAgent,
	).Scan(&id)

	if err != nil {
		// No rows: the conflict guard (WHERE device_tokens.user_id = EXCLUDED.user_id)
		// was not satisfied — the token exists but belongs to a different user.
		return mapNoRows(fmt.Errorf("upsert device token: %w", err), repositories.ErrDeviceTokenConflict)
	}

	return nil
}

// ListByUserID returns all device tokens registered for the given user
func (r *DeviceTokenRepository) ListByUserID(ctx context.Context, userID string) ([]*models.DeviceToken, error) {
	query := `
		SELECT id, user_id, token, platform, COALESCE(user_agent,''), last_used_at, created_at
		FROM device_tokens
		WHERE user_id = $1
		ORDER BY last_used_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list device tokens by user_id: %w", err)
	}

	var tokens []*models.DeviceToken

	for rows.Next() {
		var t models.DeviceToken
		if scanErr := rows.Scan(
			&t.ID, &t.UserID, &t.Token, &t.Platform, &t.UserAgent,
			&t.LastUsedAt, &t.CreatedAt,
		); scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("scan device token row (close: %w): %w", closeErr, scanErr)
			}
			return nil, fmt.Errorf("scan device token row: %w", scanErr)
		}

		tokens = append(tokens, &t)
	}

	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close device token rows: %w", closeErr)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device token rows: %w", err)
	}

	if tokens == nil {
		tokens = []*models.DeviceToken{}
	}

	return tokens, nil
}

// Delete removes a single device token scoped to the given user
func (r *DeviceTokenRepository) Delete(ctx context.Context, token string, userID string) error {
	query := `DELETE FROM device_tokens WHERE token = $1 AND user_id = $2`

	_, err := r.db.ExecContext(ctx, query, token, userID)
	if err != nil {
		return fmt.Errorf("delete device token: %w", err)
	}

	return nil
}

// DeleteByTokens removes multiple device tokens by their token strings.
// Used to prune expired or invalid FCM registration tokens.
func (r *DeviceTokenRepository) DeleteByTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}

	query := `DELETE FROM device_tokens WHERE token = ANY($1)`

	_, err := r.db.ExecContext(ctx, query, pq.Array(tokens))
	if err != nil {
		return fmt.Errorf("delete device tokens by token list: %w", err)
	}

	return nil
}
