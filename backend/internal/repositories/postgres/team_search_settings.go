package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// TeamSearchSettingsRepository handles per-team search ranking override storage.
//
// The table holds at most one row per team (team_id is its primary key), so
// this is a settings-singleton repository in the shape of
// UserPreferencesRepository: a miss is an absence, not an error.
type TeamSearchSettingsRepository struct {
	db *database.DB
}

// NewTeamSearchSettingsRepository creates a new TeamSearchSettingsRepository
func NewTeamSearchSettingsRepository(db *database.DB) *TeamSearchSettingsRepository {
	return &TeamSearchSettingsRepository{db: db}
}

// Get retrieves a team's search settings.
//
// When the team has no override row it returns (nil, nil) — not an error — so
// callers can fall back to the instance defaults.
func (r *TeamSearchSettingsRepository) Get(
	ctx context.Context, teamID string,
) (*models.TeamSearchSettings, error) {
	query := `
		SELECT team_id, recency_ranking_enabled, rank_weight_relevance, rank_weight_created,
			rank_weight_updated, rank_half_life_days, created_at, updated_at, version
		FROM team_search_settings
		WHERE team_id = $1
	`

	var settings models.TeamSearchSettings

	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&settings.TeamID,
		&settings.RecencyRankingEnabled,
		&settings.RankWeightRelevance,
		&settings.RankWeightCreated,
		&settings.RankWeightUpdated,
		&settings.RankHalfLifeDays,
		&settings.CreatedAt,
		&settings.UpdatedAt,
		&settings.Version,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &settings, nil
}

// Upsert creates or replaces a team's search settings, bumping version on
// every update. The whole profile is written at once — there is no partial
// update, matching the whole-row override model.
//
// The table's CHECK constraints mirror validateSearchRankingConfig, so a
// degenerate profile (negative or all-zero weights, a non-positive or absurd
// half-life) is rejected here by Postgres rather than silently stored.
func (r *TeamSearchSettingsRepository) Upsert(
	ctx context.Context, settings *models.TeamSearchSettings,
) error {
	now := time.Now().UTC()
	settings.UpdatedAt = now

	query := `
		INSERT INTO team_search_settings (team_id, recency_ranking_enabled, rank_weight_relevance,
			rank_weight_created, rank_weight_updated, rank_half_life_days, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, 1)
		ON CONFLICT (team_id)
		DO UPDATE SET
			recency_ranking_enabled = EXCLUDED.recency_ranking_enabled,
			rank_weight_relevance = EXCLUDED.rank_weight_relevance,
			rank_weight_created = EXCLUDED.rank_weight_created,
			rank_weight_updated = EXCLUDED.rank_weight_updated,
			rank_half_life_days = EXCLUDED.rank_half_life_days,
			updated_at = EXCLUDED.updated_at,
			version = team_search_settings.version + 1
		RETURNING created_at, version
	`

	return r.db.QueryRowContext(ctx, query,
		settings.TeamID,
		settings.RecencyRankingEnabled,
		settings.RankWeightRelevance,
		settings.RankWeightCreated,
		settings.RankWeightUpdated,
		settings.RankHalfLifeDays,
		now,
	).Scan(&settings.CreatedAt, &settings.Version)
}

// Delete removes a team's override row, reverting it to the instance defaults.
// Deleting when no row exists is a no-op, not an error.
func (r *TeamSearchSettingsRepository) Delete(ctx context.Context, teamID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM team_search_settings WHERE team_id = $1`, teamID)
	return err
}
