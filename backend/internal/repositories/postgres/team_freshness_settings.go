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

// teamFreshnessSettingsColumns is the canonical column list for
// team_freshness_settings SELECT/RETURNING clauses; scanTeamFreshnessSettings
// reads them in this order.
const teamFreshnessSettingsColumns = "team_id, interval_seconds, reversibility_enabled, " +
	"created_at, updated_at, version"

// TeamFreshnessSettingsRepository implements
// repositories.TeamFreshnessSettingsRepository for PostgreSQL.
type TeamFreshnessSettingsRepository struct {
	db *database.DB
}

// NewTeamFreshnessSettingsRepository creates a new
// TeamFreshnessSettingsRepository.
func NewTeamFreshnessSettingsRepository(db *database.DB) repositories.TeamFreshnessSettingsRepository {
	return &TeamFreshnessSettingsRepository{db: db}
}

// Get returns the team's stored settings, or (nil, nil) when the team has none
// and therefore inherits the defaults. Absence is the common case, so it is
// deliberately not an error.
func (r *TeamFreshnessSettingsRepository) Get(
	ctx context.Context, teamID string,
) (*models.TeamFreshnessSettings, error) {
	query := `
		SELECT ` + teamFreshnessSettingsColumns + `
		FROM team_freshness_settings
		WHERE team_id = $1
	`

	var s models.TeamFreshnessSettings
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(scanTeamFreshnessSettingsDest(&s)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get team freshness settings: %w", err)
	}
	return &s, nil
}

// Upsert writes the whole settings row and increments its version.
//
// The write is unconditional, matching team_search_settings: version is a
// monotonic counter callers can compare against, not a compare-and-swap
// performed here. A caller wanting to reject a stale write reads the version
// first and compares before calling.
func (r *TeamFreshnessSettingsRepository) Upsert(
	ctx context.Context, settings *models.TeamFreshnessSettings,
) error {
	query := `
		INSERT INTO team_freshness_settings
			(team_id, interval_seconds, reversibility_enabled)
		VALUES ($1, $2, $3)
		ON CONFLICT (team_id) DO UPDATE
		SET interval_seconds      = EXCLUDED.interval_seconds,
		    reversibility_enabled = EXCLUDED.reversibility_enabled,
		    updated_at            = now(),
		    version               = team_freshness_settings.version + 1
		RETURNING ` + teamFreshnessSettingsColumns

	if err := r.db.QueryRowContext(
		ctx, query, settings.TeamID, settings.IntervalSeconds, settings.ReversibilityEnabled,
	).Scan(scanTeamFreshnessSettingsDest(settings)...); err != nil {
		return fmt.Errorf("failed to upsert team freshness settings: %w", err)
	}
	return nil
}

// Delete removes the team's settings row, reverting it to the defaults.
// Deleting when no row exists is a no-op.
func (r *TeamFreshnessSettingsRepository) Delete(ctx context.Context, teamID string) error {
	query := `DELETE FROM team_freshness_settings WHERE team_id = $1`

	if _, err := r.db.ExecContext(ctx, query, teamID); err != nil {
		return fmt.Errorf("failed to delete team freshness settings: %w", err)
	}
	return nil
}

// scanTeamFreshnessSettingsDest returns the scan targets for
// teamFreshnessSettingsColumns, in order.
func scanTeamFreshnessSettingsDest(s *models.TeamFreshnessSettings) []interface{} {
	return []interface{}{
		&s.TeamID, &s.IntervalSeconds, &s.ReversibilityEnabled,
		&s.CreatedAt, &s.UpdatedAt, &s.Version,
	}
}
