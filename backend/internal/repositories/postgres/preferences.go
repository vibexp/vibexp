package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// UserPreferencesRepository handles user preferences database operations
type UserPreferencesRepository struct {
	db *database.DB
}

// NewUserPreferencesRepository creates a new UserPreferencesRepository
func NewUserPreferencesRepository(db *database.DB) *UserPreferencesRepository {
	return &UserPreferencesRepository{db: db}
}

// GetByUserID retrieves user preferences by user ID.
//
// When the user has no preferences row yet it returns (nil, nil) — not an
// error — so callers can fall back to default preferences.
func (r *UserPreferencesRepository) GetByUserID(
	ctx context.Context, userID string,
) (*models.UserPreferences, error) {
	query := `
		SELECT id, user_id, preferences, created_at, updated_at, version
		FROM user_preferences
		WHERE user_id = $1
	`

	var prefs models.UserPreferences
	var prefsJSON []byte

	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&prefs.ID,
		&prefs.UserID,
		&prefsJSON,
		&prefs.CreatedAt,
		&prefs.UpdatedAt,
		&prefs.Version,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(prefsJSON, &prefs.Preferences); err != nil {
		return nil, err
	}

	return &prefs, nil
}

// Upsert creates or updates user preferences
func (r *UserPreferencesRepository) Upsert(
	ctx context.Context, prefs *models.UserPreferences,
) error {
	prefsJSON, err := json.Marshal(prefs.Preferences)
	if err != nil {
		return err
	}

	if prefs.ID == "" {
		prefs.ID = uuid.New().String()
	}

	now := time.Now().UTC()
	prefs.UpdatedAt = now

	query := `
		INSERT INTO user_preferences (id, user_id, preferences, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, 1)
		ON CONFLICT (user_id)
		DO UPDATE SET
			preferences = EXCLUDED.preferences,
			updated_at = EXCLUDED.updated_at,
			version = user_preferences.version + 1
		RETURNING id, created_at, version
	`

	err = r.db.QueryRowContext(ctx, query,
		prefs.ID,
		prefs.UserID,
		prefsJSON,
		now,
		now,
	).Scan(&prefs.ID, &prefs.CreatedAt, &prefs.Version)

	return err
}
