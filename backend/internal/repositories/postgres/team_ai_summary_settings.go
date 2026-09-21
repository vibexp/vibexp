package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// TeamAISummarySettingsRepository handles per-team AI summary override storage.
//
// The table holds at most one row per team (team_id is its primary key), so
// this is a settings-singleton repository in the shape of
// TeamSearchSettingsRepository: a miss is an absence, not an error.
type TeamAISummarySettingsRepository struct {
	db *database.DB
}

// NewTeamAISummarySettingsRepository creates a new TeamAISummarySettingsRepository
func NewTeamAISummarySettingsRepository(db *database.DB) *TeamAISummarySettingsRepository {
	return &TeamAISummarySettingsRepository{db: db}
}

// Get retrieves a team's AI summary settings.
//
// When the team has no override row it returns (nil, nil) — not an error — so
// callers can fall back to the instance defaults.
func (r *TeamAISummarySettingsRepository) Get(
	ctx context.Context, teamID string,
) (*models.TeamAISummarySettings, error) {
	query := `
		SELECT team_id, enabled, model_provider_id, top_n, style, max_output_tokens,
			created_at, updated_at, version
		FROM team_ai_summary_settings
		WHERE team_id = $1
	`

	var settings models.TeamAISummarySettings

	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&settings.TeamID,
		&settings.Enabled,
		// model_provider_id is nullable — its NULL is the value "use the team
		// default provider", so it scans into the pointer field directly.
		&settings.ModelProviderID,
		&settings.TopN,
		&settings.Style,
		&settings.MaxOutputTokens,
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

// Upsert creates or replaces a team's AI summary settings, bumping version on
// every update. The whole profile is written at once — there is no partial
// update, matching the whole-row override model.
//
// The table's CHECK constraints bound top_n, style and max_output_tokens, so a
// profile the service failed to reject is still rejected by Postgres rather
// than silently stored.
func (r *TeamAISummarySettingsRepository) Upsert(
	ctx context.Context, settings *models.TeamAISummarySettings,
) error {
	now := time.Now().UTC()
	settings.UpdatedAt = now

	query := `
		INSERT INTO team_ai_summary_settings (team_id, enabled, model_provider_id, top_n, style,
			max_output_tokens, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, 1)
		ON CONFLICT (team_id)
		DO UPDATE SET
			enabled = EXCLUDED.enabled,
			model_provider_id = EXCLUDED.model_provider_id,
			top_n = EXCLUDED.top_n,
			style = EXCLUDED.style,
			max_output_tokens = EXCLUDED.max_output_tokens,
			updated_at = EXCLUDED.updated_at,
			version = team_ai_summary_settings.version + 1
		RETURNING created_at, version
	`

	return r.db.QueryRowContext(ctx, query,
		settings.TeamID,
		settings.Enabled,
		settings.ModelProviderID,
		settings.TopN,
		settings.Style,
		settings.MaxOutputTokens,
		now,
	).Scan(&settings.CreatedAt, &settings.Version)
}

// Delete removes a team's override row, reverting it to the instance defaults.
// Deleting when no row exists is a no-op, not an error.
func (r *TeamAISummarySettingsRepository) Delete(ctx context.Context, teamID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM team_ai_summary_settings WHERE team_id = $1`, teamID)
	return err
}
