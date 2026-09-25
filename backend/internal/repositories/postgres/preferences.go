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
	"github.com/vibexp/vibexp/internal/repositories"
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

// Upsert creates or updates user preferences.
//
// The update MERGES the typed document into the stored one at the top level
// (`||`) instead of replacing it: the typed struct always carries its own keys
// in full, so those are still fully replaced, while keys it does not model —
// the admin's saved filter presets under `admin` (#1147) — survive a
// PUT /api/v1/preferences.
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
			preferences = user_preferences.preferences || EXCLUDED.preferences,
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

// GetAdminSavedFilters returns the presets stored under
// admin.saved_filters.<list> together with the row version. A user without a
// preferences row gets (empty, 0, nil); a legacy row with a NULL version reads
// as version 1, matching what the next write compares against.
func (r *UserPreferencesRepository) GetAdminSavedFilters(
	ctx context.Context, userID, list string,
) ([]models.AdminSavedFilterPreset, int64, error) {
	query := `
		SELECT COALESCE(preferences #> ARRAY['admin', 'saved_filters', $2::text], '[]'::jsonb),
		       COALESCE(version, 1)
		FROM user_preferences
		WHERE user_id = $1
	`

	var presetsJSON []byte
	var version int64
	err := r.db.QueryRowContext(ctx, query, userID, list).Scan(&presetsJSON, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return []models.AdminSavedFilterPreset{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	presets := []models.AdminSavedFilterPreset{}
	if err := json.Unmarshal(presetsJSON, &presets); err != nil {
		return nil, 0, err
	}
	if presets == nil {
		presets = []models.AdminSavedFilterPreset{}
	}
	return presets, version, nil
}

// ReplaceAdminSavedFilters replaces admin.saved_filters.<list> — and only that
// path — in one version-checked statement, returning the new version.
//
// expectedVersion 0 means "no row yet": the row is inserted with seed plus the
// presets, and an existing row makes that a conflict. Any other value updates
// the row only when its version still matches. Either way a lost race returns
// repositories.ErrUserPreferencesVersionConflict with nothing written.
func (r *UserPreferencesRepository) ReplaceAdminSavedFilters(
	ctx context.Context, userID, list string, presets []models.AdminSavedFilterPreset,
	seed models.Preferences, expectedVersion int64,
) (int64, error) {
	if presets == nil {
		presets = []models.AdminSavedFilterPreset{}
	}
	presetsJSON, err := json.Marshal(presets)
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	var version int64
	if expectedVersion == 0 {
		version, err = r.insertAdminSavedFilters(ctx, userID, list, presets, seed, now)
	} else {
		version, err = r.updateAdminSavedFilters(ctx, userID, list, presetsJSON, expectedVersion, now)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, repositories.ErrUserPreferencesVersionConflict
	}
	return version, err
}

// insertAdminSavedFilters creates the preferences row, seeded with the default
// preferences so creating it through the admin path cannot silently switch off
// the user's notifications. DO NOTHING on an existing row yields sql.ErrNoRows,
// which the caller reports as a version conflict.
func (r *UserPreferencesRepository) insertAdminSavedFilters(
	ctx context.Context, userID, list string, presets []models.AdminSavedFilterPreset,
	seed models.Preferences, now time.Time,
) (int64, error) {
	seedJSON, err := json.Marshal(seed)
	if err != nil {
		return 0, err
	}
	doc := map[string]any{}
	if err = json.Unmarshal(seedJSON, &doc); err != nil {
		return 0, err
	}
	doc["admin"] = map[string]any{
		"saved_filters": map[string]any{list: presets},
	}
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return 0, err
	}

	query := `
		INSERT INTO user_preferences (id, user_id, preferences, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, 1)
		ON CONFLICT (user_id) DO NOTHING
		RETURNING version
	`
	var version int64
	err = r.db.QueryRowContext(ctx, query, uuid.New().String(), userID, string(docJSON), now, now).Scan(&version)
	return version, err
}

// updateAdminSavedFilters merges the presets into the stored document at
// admin.saved_filters.<list>. Every expression reads the OLD row
// (user_preferences.*), so sibling lists and every other key are kept.
func (r *UserPreferencesRepository) updateAdminSavedFilters(
	ctx context.Context, userID, list string, presetsJSON []byte, expectedVersion int64, now time.Time,
) (int64, error) {
	query := `
		UPDATE user_preferences
		SET preferences = user_preferences.preferences || jsonb_build_object(
				'admin',
				COALESCE(user_preferences.preferences -> 'admin', '{}'::jsonb) || jsonb_build_object(
					'saved_filters',
					COALESCE(user_preferences.preferences #> '{admin,saved_filters}', '{}'::jsonb)
						|| jsonb_build_object($2::text, $3::jsonb)
				)
			),
			updated_at = $4,
			version = COALESCE(user_preferences.version, 1) + 1
		WHERE user_id = $1 AND COALESCE(user_preferences.version, 1) = $5::bigint
		RETURNING version
	`
	var version int64
	err := r.db.QueryRowContext(ctx, query, userID, list, string(presetsJSON), now, expectedVersion).Scan(&version)
	return version, err
}
