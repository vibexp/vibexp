package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// resourceFreshnessColumns is the canonical column list for
// resource_freshness SELECT/RETURNING clauses; scanResourceFreshnessDest
// reads them in this order.
const resourceFreshnessColumns = "id, team_id, project_id, resource_type, resource_id, " +
	"status, matched_rule_ids, since, reason, created_at, updated_at"

// ResourceFreshnessRepository implements
// repositories.ResourceFreshnessRepository for PostgreSQL.
type ResourceFreshnessRepository struct {
	db *database.DB
}

// NewResourceFreshnessRepository creates a new ResourceFreshnessRepository.
func NewResourceFreshnessRepository(db *database.DB) repositories.ResourceFreshnessRepository {
	return &ResourceFreshnessRepository{db: db}
}

// Upsert marks a resource stale, or refreshes an already-stale one, keyed on
// the (resource_type, resource_id) unique index.
//
// `since` is NOT overwritten on conflict: it must keep meaning "first marked
// stale at" for a resource that stays stale across successive evaluations,
// otherwise every run would reset the age the UI reports.
func (r *ResourceFreshnessRepository) Upsert(ctx context.Context, f *models.ResourceFreshness) error {
	query := `
		INSERT INTO resource_freshness
			(team_id, project_id, resource_type, resource_id, status, matched_rule_ids, since, reason)
		VALUES ($1, $2, $3, $4, $5, $6::uuid[], COALESCE($7, now()), $8)
		ON CONFLICT (resource_type, resource_id) DO UPDATE
		SET team_id          = EXCLUDED.team_id,
		    project_id       = EXCLUDED.project_id,
		    status           = EXCLUDED.status,
		    matched_rule_ids = EXCLUDED.matched_rule_ids,
		    reason           = EXCLUDED.reason,
		    updated_at       = now()
		RETURNING ` + resourceFreshnessColumns

	// A zero Since defers to the database clock, so a caller that does not
	// care about the exact instant cannot write a zero timestamp.
	since := &f.Since
	if f.Since.IsZero() {
		since = nil
	}

	if err := r.db.QueryRowContext(
		ctx, query,
		f.TeamID, f.ProjectID, f.ResourceType, f.ResourceID,
		f.Status, pq.Array(f.MatchedRuleIDs), since, f.Reason,
	).Scan(scanResourceFreshnessDest(f)...); err != nil {
		return fmt.Errorf("failed to upsert resource freshness: %w", err)
	}
	return nil
}

// GetByResource returns the freshness state of one resource, or (nil, nil)
// when the resource is not stale -- absence is the normal case, not an error.
func (r *ResourceFreshnessRepository) GetByResource(
	ctx context.Context, resourceType, resourceID string,
) (*models.ResourceFreshness, error) {
	query := `
		SELECT ` + resourceFreshnessColumns + `
		FROM resource_freshness
		WHERE resource_type = $1 AND resource_id = $2
	`

	var f models.ResourceFreshness
	err := r.db.QueryRowContext(ctx, query, resourceType, resourceID).
		Scan(scanResourceFreshnessDest(&f)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get resource freshness: %w", err)
	}
	return &f, nil
}

// List returns a team's stale resources, most recently marked first, plus the
// total count matching the filters so callers can paginate. The predicate
// shape varies with the optional filters, so the query is built with
// squirrel.
func (r *ResourceFreshnessRepository) List(
	ctx context.Context, filters models.ResourceFreshnessFilters,
) ([]*models.ResourceFreshness, int, error) {
	where := squirrel.Eq{"team_id": filters.TeamID}
	if filters.ResourceType != "" {
		where["resource_type"] = filters.ResourceType
	}
	if filters.ProjectID != "" {
		where["project_id"] = filters.ProjectID
	}

	total, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	// `since DESC, id DESC` matches the listing indexes and breaks ties
	// deterministically: one rule run marks every resource it touches with the
	// same transaction timestamp, so without the tiebreaker pagination could
	// repeat or skip rows.
	builder := psql.Select(resourceFreshnessColumns).
		From("resource_freshness").
		Where(where).
		OrderBy("since DESC", "id DESC")
	if filters.Limit > 0 {
		builder = builder.Limit(uint64(filters.Limit))
	}
	if filters.Offset > 0 {
		builder = builder.Offset(uint64(filters.Offset))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build resource freshness list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list resource freshness: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close resource freshness rows", "error", closeErr)
		}
	}()

	items := make([]*models.ResourceFreshness, 0)
	for rows.Next() {
		var f models.ResourceFreshness
		if err := rows.Scan(scanResourceFreshnessDest(&f)...); err != nil {
			return nil, 0, fmt.Errorf("failed to scan resource freshness: %w", err)
		}
		items = append(items, &f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate resource freshness: %w", err)
	}
	return items, total, nil
}

// countList returns how many rows match the listing predicate, ignoring
// pagination.
func (r *ResourceFreshnessRepository) countList(
	ctx context.Context, where squirrel.Eq,
) (int, error) {
	query, args, err := psql.Select("COUNT(*)").
		From("resource_freshness").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build resource freshness count query: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count resource freshness: %w", err)
	}
	return total, nil
}

// DeleteByResource clears one resource's freshness state, reporting whether a
// row was removed. Clearing a resource that is not stale is a no-op.
func (r *ResourceFreshnessRepository) DeleteByResource(
	ctx context.Context, resourceType, resourceID string,
) (bool, error) {
	query := `DELETE FROM resource_freshness WHERE resource_type = $1 AND resource_id = $2`

	res, err := r.db.ExecContext(ctx, query, resourceType, resourceID)
	if err != nil {
		return false, fmt.Errorf("failed to delete resource freshness: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read resource freshness delete result: %w", err)
	}
	return affected > 0, nil
}

// RemoveRule strips ruleID from every row's matched_rule_ids, then deletes the
// rows left matching no rule at all, returning how many were deleted.
//
// The two statements run in one transaction because they are one logical
// operation: between them, rows exist that are stale for no reason. They
// cannot be merged into a single statement -- Postgres does not support
// updating and then deleting the same row within one statement, since a
// data-modifying CTE and the outer statement share a snapshot.
func (r *ResourceFreshnessRepository) RemoveRule(ctx context.Context, ruleID string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin resource freshness rule-removal: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back resource freshness rule-removal", "error", rbErr)
		}
	}()

	// The predicate is spelled with the containment operator rather than the
	// more obvious `$1 = ANY (matched_rule_ids)`: ANY over an array is not an
	// indexable operator, so that form seq-scans the whole table, while `@>`
	// is served by idx_resource_freshness_matched_rules (GIN, array_ops).
	// Both placeholders are the same uuid; the explicit casts keep lib/pq from
	// having to infer two different types for one parameter.
	if _, err = tx.ExecContext(ctx, `
		UPDATE resource_freshness
		SET matched_rule_ids = array_remove(matched_rule_ids, $1::uuid),
		    updated_at = now()
		WHERE matched_rule_ids @> ARRAY[$1::uuid]
	`, ruleID); err != nil {
		return 0, fmt.Errorf("failed to strip rule from resource freshness: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		DELETE FROM resource_freshness WHERE cardinality(matched_rule_ids) = 0
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to delete unmatched resource freshness: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read resource freshness rule-removal result: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit resource freshness rule-removal: %w", err)
	}
	return deleted, nil
}

// scanResourceFreshnessDest returns the scan targets for
// resourceFreshnessColumns, in order.
func scanResourceFreshnessDest(f *models.ResourceFreshness) []interface{} {
	return []interface{}{
		&f.ID, &f.TeamID, &f.ProjectID, &f.ResourceType, &f.ResourceID,
		&f.Status, pq.Array(&f.MatchedRuleIDs), &f.Since, &f.Reason,
		&f.CreatedAt, &f.UpdatedAt,
	}
}
