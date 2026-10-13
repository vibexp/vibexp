package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// UserIdentityRepository implements repositories.UserIdentityRepository for
// PostgreSQL.
type UserIdentityRepository struct {
	db *database.DB
}

// NewUserIdentityRepository creates a new UserIdentityRepository.
func NewUserIdentityRepository(db *database.DB) repositories.UserIdentityRepository {
	return &UserIdentityRepository{db: db}
}

// Upsert creates the identity or, when the user already holds it, stamps
// last_used_at. The conflict update is restricted to the owning user, so a pair
// held by someone else updates nothing, returns no row, and is reported as
// ErrUserIdentityTaken instead of being silently reassigned.
func (r *UserIdentityRepository) Upsert(ctx context.Context, userID, provider, subject string) error {
	query := `
		INSERT INTO user_identities (user_id, provider, subject, last_used_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (provider, subject) DO UPDATE
		    SET last_used_at = EXCLUDED.last_used_at
		  WHERE user_identities.user_id = EXCLUDED.user_id
		RETURNING id
	`

	var id string
	err := r.db.QueryRowContext(ctx, query, userID, provider, subject).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: provider %q", repositories.ErrUserIdentityTaken, provider)
	}
	if err != nil {
		return fmt.Errorf("failed to upsert user identity: %w", err)
	}
	return nil
}

// ListByUserID returns the user's identities, oldest first.
func (r *UserIdentityRepository) ListByUserID(ctx context.Context, userID string) ([]models.UserIdentity, error) {
	query := `
		SELECT id, user_id, provider, subject, last_used_at, created_at
		FROM user_identities
		WHERE user_id = $1
		ORDER BY created_at, id
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user identities: %w", err)
	}

	defer rows.Close() //nolint:errcheck

	identities := []models.UserIdentity{}
	for rows.Next() {
		var identity models.UserIdentity
		if scanErr := rows.Scan(
			&identity.ID, &identity.UserID, &identity.Provider, &identity.Subject,
			&identity.LastUsedAt, &identity.CreatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan user identity: %w", scanErr)
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate user identities: %w", err)
	}
	return identities, nil
}
