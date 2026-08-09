package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// freshnessRuleColumns is the canonical column list for freshness_rules
// SELECT/RETURNING clauses; scanFreshnessRuleDest reads them in this order.
const freshnessRuleColumns = "id, team_id, project_id, resource_types, mediums, " +
	"threshold_days, enabled, created_at, updated_at"

// FreshnessRuleRepository implements repositories.FreshnessRuleRepository for
// PostgreSQL.
type FreshnessRuleRepository struct {
	db *database.DB
}

// NewFreshnessRuleRepository creates a new FreshnessRuleRepository.
func NewFreshnessRuleRepository(db *database.DB) repositories.FreshnessRuleRepository {
	return &FreshnessRuleRepository{db: db}
}

// Create inserts a rule and populates the model from the persisted row.
func (r *FreshnessRuleRepository) Create(ctx context.Context, rule *models.FreshnessRule) error {
	query := `
		INSERT INTO freshness_rules
			(team_id, project_id, resource_types, mediums, threshold_days, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + freshnessRuleColumns

	if err := r.db.QueryRowContext(
		ctx, query,
		rule.TeamID, rule.ProjectID,
		pq.Array(rule.ResourceTypes), pq.Array(freshnessMediums(rule.Mediums)),
		rule.ThresholdDays, rule.Enabled,
	).Scan(scanFreshnessRuleDest(rule)...); err != nil {
		return fmt.Errorf("failed to create freshness rule: %w", err)
	}
	return nil
}

// GetByID returns one rule scoped to its team, or (nil, nil) when no such rule
// exists there. Scoping the lookup by team_id is the tenancy boundary: another
// team's rule id must be indistinguishable from a non-existent one.
func (r *FreshnessRuleRepository) GetByID(
	ctx context.Context, teamID, ruleID string,
) (*models.FreshnessRule, error) {
	query := `
		SELECT ` + freshnessRuleColumns + `
		FROM freshness_rules
		WHERE team_id = $1 AND id = $2
	`

	var rule models.FreshnessRule
	err := r.db.QueryRowContext(ctx, query, teamID, ruleID).Scan(scanFreshnessRuleDest(&rule)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get freshness rule: %w", err)
	}
	return &rule, nil
}

// ListByTeam returns the team's rules oldest first. enabledOnly narrows to the
// rules an evaluation run should actually apply.
//
// The two query shapes are spelled out rather than built dynamically: the
// predicate varies by a single boolean, so a static string per branch stays
// clearer than a builder and keeps the SQL greppable.
func (r *FreshnessRuleRepository) ListByTeam(
	ctx context.Context, teamID string, enabledOnly bool,
) ([]*models.FreshnessRule, error) {
	query := `
		SELECT ` + freshnessRuleColumns + `
		FROM freshness_rules
		WHERE team_id = $1
		ORDER BY created_at ASC, id ASC
	`
	if enabledOnly {
		query = `
			SELECT ` + freshnessRuleColumns + `
			FROM freshness_rules
			WHERE team_id = $1 AND enabled = true
			ORDER BY created_at ASC, id ASC
		`
	}

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to list freshness rules: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close freshness rule rows", "error", closeErr)
		}
	}()

	rules := make([]*models.FreshnessRule, 0)
	for rows.Next() {
		var rule models.FreshnessRule
		if err := rows.Scan(scanFreshnessRuleDest(&rule)...); err != nil {
			return nil, fmt.Errorf("failed to scan freshness rule: %w", err)
		}
		rules = append(rules, &rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate freshness rules: %w", err)
	}
	return rules, nil
}

// Update replaces the rule's mutable fields, scoped to its team, and refreshes
// UpdatedAt from the persisted row. It returns
// repositories.ErrFreshnessRuleNotFound when no row matched.
func (r *FreshnessRuleRepository) Update(ctx context.Context, rule *models.FreshnessRule) error {
	query := `
		UPDATE freshness_rules
		SET project_id     = $3,
		    resource_types = $4,
		    mediums        = $5,
		    threshold_days = $6,
		    enabled        = $7,
		    updated_at     = now()
		WHERE team_id = $1 AND id = $2
		RETURNING ` + freshnessRuleColumns

	err := r.db.QueryRowContext(
		ctx, query,
		rule.TeamID, rule.ID, rule.ProjectID,
		pq.Array(rule.ResourceTypes), pq.Array(freshnessMediums(rule.Mediums)),
		rule.ThresholdDays, rule.Enabled,
	).Scan(scanFreshnessRuleDest(rule)...)
	if err != nil {
		return mapNoRows(
			fmt.Errorf("failed to update freshness rule: %w", err),
			repositories.ErrFreshnessRuleNotFound,
		)
	}
	return nil
}

// Delete removes the rule, reporting whether a row was removed. Callers must
// also run ResourceFreshnessRepository.RemoveRule so no freshness state keeps
// referencing the deleted rule.
func (r *FreshnessRuleRepository) Delete(ctx context.Context, teamID, ruleID string) (bool, error) {
	query := `DELETE FROM freshness_rules WHERE team_id = $1 AND id = $2`

	res, err := r.db.ExecContext(ctx, query, teamID, ruleID)
	if err != nil {
		return false, fmt.Errorf("failed to delete freshness rule: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read freshness rule delete result: %w", err)
	}
	return affected > 0, nil
}

// freshnessMediums normalizes a nil Mediums to an empty slice. The column is
// NOT NULL and an empty array carries the meaning "any medium", so a nil slice
// must persist as `{}` rather than NULL -- the two would otherwise be
// indistinguishable to a reader but differ in SQL.
func freshnessMediums(mediums []string) []string {
	if mediums == nil {
		return []string{}
	}
	return mediums
}

// scanFreshnessRuleDest returns the scan targets for freshnessRuleColumns, in
// order.
func scanFreshnessRuleDest(rule *models.FreshnessRule) []interface{} {
	return []interface{}{
		&rule.ID, &rule.TeamID, &rule.ProjectID,
		pq.Array(&rule.ResourceTypes), pq.Array(&rule.Mediums),
		&rule.ThresholdDays, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
	}
}
