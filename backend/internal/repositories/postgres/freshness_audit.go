package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// freshnessAuditColumns is the canonical column list for
// resource_freshness_audit SELECT/RETURNING clauses; scanFreshnessAuditDest
// reads them in this order.
const freshnessAuditColumns = "id, team_id, resource_type, resource_id, rule_id, " +
	"action, reason, created_at"

// FreshnessAuditRepository implements repositories.FreshnessAuditRepository
// for PostgreSQL. The table is append-only: this repository deliberately
// exposes no update or delete.
type FreshnessAuditRepository struct {
	db *database.DB
}

// NewFreshnessAuditRepository creates a new FreshnessAuditRepository.
func NewFreshnessAuditRepository(db *database.DB) repositories.FreshnessAuditRepository {
	return &FreshnessAuditRepository{db: db}
}

// Create appends one entry and populates the model from the persisted row.
func (r *FreshnessAuditRepository) Create(
	ctx context.Context, entry *models.ResourceFreshnessAudit,
) error {
	query := `
		INSERT INTO resource_freshness_audit
			(team_id, resource_type, resource_id, rule_id, action, reason)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + freshnessAuditColumns

	if err := r.db.QueryRowContext(
		ctx, query,
		entry.TeamID, entry.ResourceType, entry.ResourceID,
		entry.RuleID, entry.Action, entry.Reason,
	).Scan(scanFreshnessAuditDest(entry)...); err != nil {
		return fmt.Errorf("failed to create freshness audit entry: %w", err)
	}
	return nil
}

// ListByTeam returns a team's audit entries newest first, plus the total entry
// count for pagination.
//
// The ordering breaks created_at ties on id, and that tiebreaker is
// load-bearing rather than cosmetic: `now()` is transaction-start time, so a
// single rule run marking many resources writes an identical created_at to
// every row it inserts. Without the tiebreaker their relative order is
// undefined between queries and pagination can repeat or skip entries.
func (r *FreshnessAuditRepository) ListByTeam(
	ctx context.Context, teamID string, limit, offset int,
) ([]*models.ResourceFreshnessAudit, int, error) {
	total, err := r.countByTeam(ctx, teamID)
	if err != nil {
		return nil, 0, err
	}

	// Negative paging values are clamped to 0: Postgres rejects a negative
	// LIMIT/OFFSET outright, so the clamp only changes an already-invalid call
	// and keeps the query static. A zero/absent limit means "no limit", which
	// NULL expresses without a second query shape.
	var limitArg *int
	if limit > 0 {
		limitArg = &limit
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT ` + freshnessAuditColumns + `
		FROM resource_freshness_audit
		WHERE team_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, teamID, limitArg, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list freshness audit entries: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close freshness audit rows", "error", closeErr)
		}
	}()

	entries := make([]*models.ResourceFreshnessAudit, 0)
	for rows.Next() {
		var entry models.ResourceFreshnessAudit
		if err := rows.Scan(scanFreshnessAuditDest(&entry)...); err != nil {
			return nil, 0, fmt.Errorf("failed to scan freshness audit entry: %w", err)
		}
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate freshness audit entries: %w", err)
	}
	return entries, total, nil
}

// countByTeam returns how many audit entries the team has, ignoring
// pagination.
func (r *FreshnessAuditRepository) countByTeam(ctx context.Context, teamID string) (int, error) {
	query := `SELECT COUNT(*) FROM resource_freshness_audit WHERE team_id = $1`

	var total int
	if err := r.db.QueryRowContext(ctx, query, teamID).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count freshness audit entries: %w", err)
	}
	return total, nil
}

// scanFreshnessAuditDest returns the scan targets for freshnessAuditColumns,
// in order.
func scanFreshnessAuditDest(entry *models.ResourceFreshnessAudit) []interface{} {
	return []interface{}{
		&entry.ID, &entry.TeamID, &entry.ResourceType, &entry.ResourceID,
		&entry.RuleID, &entry.Action, &entry.Reason, &entry.CreatedAt,
	}
}

// CountTransitionsByDay returns the team's marked/cleared counts per UTC day
// from `since` onwards, sparse.
//
// The bucket is rendered as text in exactly the layout the zero-filled series
// uses, so the two key sets align without the caller parsing anything back.
//
// The explicit `AT TIME ZONE 'UTC'` is load-bearing and deliberately differs
// from the older analytics queries, which use a bare `DATE(created_at)`:
// created_at is timestamptz, so a bare DATE() truncates in the SESSION
// timezone, and the DSN sets none. The caller builds its keys in UTC, so on a
// server running any other zone a transition would land on a key outside the
// window — dropping it from the totals AND shifting every earlier day's
// reconstructed level, which is a wrong line on a chart rather than a missing
// bar. (The pre-existing queries have the same latent bug; tracked separately.)
func (r *FreshnessAuditRepository) CountTransitionsByDay(
	ctx context.Context, teamID string, since time.Time,
) ([]models.FreshnessTransitionCount, error) {
	query := `
		SELECT TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS date, action, COUNT(*)
		FROM resource_freshness_audit
		WHERE team_id = $1 AND created_at >= $2
		GROUP BY (created_at AT TIME ZONE 'UTC')::date, action
		ORDER BY (created_at AT TIME ZONE 'UTC')::date, action
	`

	rows, err := r.db.QueryContext(ctx, query, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to count freshness transitions by day: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close freshness transition rows", "error", closeErr)
		}
	}()

	counts := make([]models.FreshnessTransitionCount, 0)
	for rows.Next() {
		var c models.FreshnessTransitionCount
		if err := rows.Scan(&c.Date, &c.Action, &c.Count); err != nil {
			return nil, fmt.Errorf("failed to scan freshness transition count: %w", err)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate freshness transition counts: %w", err)
	}
	return counts, nil
}
