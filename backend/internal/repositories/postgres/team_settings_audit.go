package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// teamSettingsAuditColumns is the canonical column list for team_settings_audit
// projections; scanTeamSettingsAuditDest reads them in this order.
//
// Unlike the freshness audit (#789) there is no second, wider list: nothing on
// this entry is resolved by joining a live row. The copied resources are
// polymorphic across the provider and type tables, and an entry must stay
// readable after its source row is deleted or rotated away — so the names that
// make it legible are snapshotted into `detail` at write time instead.
const teamSettingsAuditColumns = "id, team_id, actor_user_id, surface, source_team_id, " +
	"source_resource_id, created_resource_id, detail, created_at"

// TeamSettingsAuditRepository implements
// repositories.TeamSettingsAuditRepository for PostgreSQL. The table is
// append-only: this repository deliberately exposes no update or delete.
type TeamSettingsAuditRepository struct {
	db *database.DB
}

// NewTeamSettingsAuditRepository creates a new TeamSettingsAuditRepository.
func NewTeamSettingsAuditRepository(db *database.DB) repositories.TeamSettingsAuditRepository {
	return &TeamSettingsAuditRepository{db: db}
}

// Append records one settings-copy event and populates the model from the
// persisted row.
func (r *TeamSettingsAuditRepository) Append(
	ctx context.Context, entry *models.TeamSettingsAudit,
) error {
	// An absent Detail is substituted with an empty OBJECT rather than left
	// nil. The column is `jsonb NOT NULL DEFAULT '{}'`, and a nil
	// json.RawMessage binds as SQL NULL rather than falling back to that
	// default, so the INSERT would be rejected outright. Substituting keeps
	// every stored row a readable object and spares #832's reader a NULL case.
	detail := entry.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}

	query := `
		INSERT INTO team_settings_audit
			(team_id, actor_user_id, surface, source_team_id,
			 source_resource_id, created_resource_id, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + teamSettingsAuditColumns

	if err := r.db.QueryRowContext(
		ctx, query,
		entry.TeamID, entry.ActorUserID, entry.Surface, entry.SourceTeamID,
		entry.SourceResourceID, entry.CreatedResourceID, detail,
	).Scan(scanTeamSettingsAuditDest(entry)...); err != nil {
		return fmt.Errorf("failed to append team settings audit entry: %w", err)
	}
	return nil
}

// ListByTeam returns a team's audit entries newest first, plus the total entry
// count for pagination.
//
// The ordering breaks created_at ties on id, and that tiebreaker is
// load-bearing rather than cosmetic: `now()` is transaction-start time, so a
// single copy action writing several entries stamps an identical created_at on
// every one. Without the tiebreaker their relative order is undefined between
// queries and pagination can repeat or skip entries.
//
// Tenancy-only predicate, no role predicates (authz decision D3).
func (r *TeamSettingsAuditRepository) ListByTeam(
	ctx context.Context, teamID string, limit, offset int,
) ([]*models.TeamSettingsAudit, int, error) {
	total, err := r.countByTeam(ctx, teamID)
	if err != nil {
		return nil, 0, err
	}

	// Negative paging values are clamped to 0: Postgres rejects a negative
	// LIMIT/OFFSET outright, so the clamp only changes an already-invalid call
	// and keeps the query static. A zero/absent limit means "no limit", which
	// NULL expresses without a second query shape. Mirrors
	// FreshnessAuditRepository.ListByTeam.
	var limitArg *int
	if limit > 0 {
		limitArg = &limit
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT ` + teamSettingsAuditColumns + `
		FROM team_settings_audit
		WHERE team_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, teamID, limitArg, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list team settings audit entries: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close team settings audit rows", "error", closeErr)
		}
	}()

	entries := make([]*models.TeamSettingsAudit, 0)
	for rows.Next() {
		var entry models.TeamSettingsAudit
		if err := rows.Scan(scanTeamSettingsAuditDest(&entry)...); err != nil {
			return nil, 0, fmt.Errorf("failed to scan team settings audit entry: %w", err)
		}
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate team settings audit entries: %w", err)
	}
	return entries, total, nil
}

// countByTeam returns how many audit entries the team has, ignoring pagination.
func (r *TeamSettingsAuditRepository) countByTeam(ctx context.Context, teamID string) (int, error) {
	query := `SELECT COUNT(*) FROM team_settings_audit WHERE team_id = $1`

	var total int
	if err := r.db.QueryRowContext(ctx, query, teamID).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count team settings audit entries: %w", err)
	}
	return total, nil
}

// scanTeamSettingsAuditDest returns the scan targets for
// teamSettingsAuditColumns, in order.
func scanTeamSettingsAuditDest(entry *models.TeamSettingsAudit) []interface{} {
	return []interface{}{
		&entry.ID, &entry.TeamID, &entry.ActorUserID, &entry.Surface,
		&entry.SourceTeamID, &entry.SourceResourceID, &entry.CreatedResourceID,
		&entry.Detail, &entry.CreatedAt,
	}
}
