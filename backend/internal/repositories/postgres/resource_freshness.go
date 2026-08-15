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
//
// It reports whether the row was INSERTED, as opposed to updated on conflict --
// the same "say what actually happened" contract DeleteByResource has, and for
// the same reason. `xmax = 0` is the standard Postgres spelling: xmax is the
// deleting/locking transaction id on the tuple, which is zero for a freshly
// inserted row and non-zero for the row DO UPDATE rewrote. Without it a caller
// can only guess from state it read earlier, and a concurrent delete in between
// makes that guess wrong (#771).
func (r *ResourceFreshnessRepository) Upsert(ctx context.Context, f *models.ResourceFreshness) (bool, error) {
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
		RETURNING ` + resourceFreshnessColumns + `, (xmax = 0) AS inserted`

	// A zero Since defers to the database clock, so a caller that does not
	// care about the exact instant cannot write a zero timestamp.
	since := &f.Since
	if f.Since.IsZero() {
		since = nil
	}

	var inserted bool
	if err := r.db.QueryRowContext(
		ctx, query,
		f.TeamID, f.ProjectID, f.ResourceType, f.ResourceID,
		f.Status, pq.Array(f.MatchedRuleIDs), since, f.Reason,
	).Scan(append(scanResourceFreshnessDest(f), &inserted)...); err != nil {
		return false, fmt.Errorf("failed to upsert resource freshness: %w", err)
	}
	return inserted, nil
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

// ListAllByTeam returns every freshness row of one team, unpaginated.
//
// Rule evaluation reconciles a team's whole stale set against what its rules
// currently match, so it needs the complete stored state: a page of it would
// make "no rule matches this any more, clear it" undecidable for the rows
// outside the page. The result is bounded by how many resources the team has
// actually been marked stale on, not by its total resource count.
func (r *ResourceFreshnessRepository) ListAllByTeam(
	ctx context.Context, teamID string,
) ([]*models.ResourceFreshness, error) {
	query := `SELECT ` + resourceFreshnessColumns + ` FROM resource_freshness WHERE team_id = $1`

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to list team resource freshness: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close team resource freshness rows", "error", closeErr)
		}
	}()

	items := make([]*models.ResourceFreshness, 0)
	for rows.Next() {
		var f models.ResourceFreshness
		if err := rows.Scan(scanResourceFreshnessDest(&f)...); err != nil {
			return nil, fmt.Errorf("failed to scan team resource freshness: %w", err)
		}
		items = append(items, &f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate team resource freshness: %w", err)
	}
	return items, nil
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
//
// The delete is keyed on the ids the update just emptied rather than on
// `cardinality(matched_rule_ids) = 0`. That predicate would match EVERY
// empty-array row in the table, in any team -- and an empty array is a value
// Upsert accepts -- so removing one team's rule could clear freshness state
// this call never touched.
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

	orphaned, err := stripFreshnessRule(ctx, tx, ruleID)
	if err != nil {
		return 0, err
	}

	// With nothing emptied there is nothing to delete, and the statement is
	// skipped rather than widened: any predicate broad enough to run here
	// without a list of ids would reach rows this call never touched.
	var deleted int64
	if len(orphaned) > 0 {
		var res sql.Result
		res, err = tx.ExecContext(ctx,
			`DELETE FROM resource_freshness WHERE id = ANY($1::uuid[])`, pq.Array(orphaned))
		if err != nil {
			return 0, fmt.Errorf("failed to delete unmatched resource freshness: %w", err)
		}
		deleted, err = res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("failed to read resource freshness rule-removal result: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit resource freshness rule-removal: %w", err)
	}
	return deleted, nil
}

// stripFreshnessRule removes ruleID from every row that matches it and returns
// the ids of the rows left matching no rule at all.
//
// The predicate uses the containment operator rather than the more obvious
// `$1 = ANY (matched_rule_ids)`: ANY over an array is not an indexable
// operator, so that form seq-scans the whole table, while `@>` is served by
// idx_resource_freshness_matched_rules (GIN, array_ops). Both placeholders are
// the same uuid; the explicit casts keep lib/pq from having to infer two
// different types for one parameter.
//
// The rows are fully drained before the caller issues its DELETE: a
// database/sql transaction allows only one active query at a time.
func stripFreshnessRule(ctx context.Context, tx *sql.Tx, ruleID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		UPDATE resource_freshness
		SET matched_rule_ids = array_remove(matched_rule_ids, $1::uuid),
		    updated_at = now()
		WHERE matched_rule_ids @> ARRAY[$1::uuid]
		RETURNING id, cardinality(matched_rule_ids)
	`, ruleID)
	if err != nil {
		return nil, fmt.Errorf("failed to strip rule from resource freshness: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close resource freshness rule-removal rows", "error", closeErr)
		}
	}()

	orphaned := make([]string, 0)
	for rows.Next() {
		var id string
		var remaining int
		if err := rows.Scan(&id, &remaining); err != nil {
			return nil, fmt.Errorf("failed to scan stripped resource freshness: %w", err)
		}
		if remaining == 0 {
			orphaned = append(orphaned, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate stripped resource freshness: %w", err)
	}
	return orphaned, nil
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

// scanBucketCounts drains a two-column (key, count) result set. Grouped counts
// all have the same shape, so they share one scanner rather than three
// near-identical loops.
func scanBucketCounts(rows *sql.Rows, what string) ([]models.FreshnessBucketCount, error) {
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close freshness bucket rows", "grouping", what, "error", closeErr)
		}
	}()

	counts := make([]models.FreshnessBucketCount, 0)
	for rows.Next() {
		var c models.FreshnessBucketCount
		if err := rows.Scan(&c.Key, &c.Count); err != nil {
			return nil, fmt.Errorf("failed to scan stale count by %s: %w", what, err)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate stale counts by %s: %w", what, err)
	}
	return counts, nil
}

// CountStaleByType returns the team's stale counts grouped by resource type.
//
// Served by idx_resource_freshness_team_type, whose leading column is team_id
// and whose second is resource_type — so the group is an index scan rather than
// a heap scan of the team's rows.
func (r *ResourceFreshnessRepository) CountStaleByType(
	ctx context.Context, teamID string,
) ([]models.FreshnessBucketCount, error) {
	query := `
		SELECT resource_type, COUNT(*)
		FROM resource_freshness
		WHERE team_id = $1
		GROUP BY resource_type
		ORDER BY resource_type
	`

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to count stale resources by type: %w", err)
	}
	return scanBucketCounts(rows, "type")
}

// CountStaleByProject returns the team's stale counts grouped by project.
//
// The result is keyed by project id only; the service joins the names, because
// enumerating the team's projects is also what lets it report the projects with
// nothing stale, which no grouping over this table can produce.
func (r *ResourceFreshnessRepository) CountStaleByProject(
	ctx context.Context, teamID string,
) ([]models.FreshnessBucketCount, error) {
	query := `
		SELECT project_id::text, COUNT(*)
		FROM resource_freshness
		WHERE team_id = $1
		GROUP BY project_id
		ORDER BY project_id
	`

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to count stale resources by project: %w", err)
	}
	return scanBucketCounts(rows, "project")
}

// CountStaleByRule returns the team's stale counts grouped by matching rule.
//
// `unnest` is what makes the union semantics visible: a resource matched by two
// rules contributes one row to each, so the counts answer "how many resources
// does this rule mark" rather than "how many are stale because of it alone".
// The sum across rules therefore exceeds the number of stale resources whenever
// any resource matches more than one — which is why CountStale exists.
func (r *ResourceFreshnessRepository) CountStaleByRule(
	ctx context.Context, teamID string,
) ([]models.FreshnessBucketCount, error) {
	query := `
		SELECT rule_id::text, COUNT(*)
		FROM resource_freshness, unnest(matched_rule_ids) AS rule_id
		WHERE team_id = $1
		GROUP BY rule_id
		ORDER BY rule_id
	`

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to count stale resources by rule: %w", err)
	}
	return scanBucketCounts(rows, "rule")
}

// CountStale returns how many distinct resources are stale in the team.
func (r *ResourceFreshnessRepository) CountStale(ctx context.Context, teamID string) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM resource_freshness WHERE team_id = $1`, teamID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to count stale resources: %w", err)
	}
	return total, nil
}

// ListByResources returns the freshness rows for a page of resources of one
// type, keyed by resource id.
//
// The predicate is `resource_type = $1 AND resource_id = ANY($2)`, which the
// unique index on (resource_type, resource_id) serves: the leading column is
// an equality and the second is a list of equalities. (`= ANY (array)` is only
// unindexable when the ARRAY is the column -- see the rule-cleanup query, which
// has to use the containment operator instead.)
func (r *ResourceFreshnessRepository) ListByResources(
	ctx context.Context, resourceType string, resourceIDs []string,
) (map[string]*models.ResourceFreshness, error) {
	// An empty page must not issue a query, and must still return a usable map
	// so the caller can look ids up unconditionally.
	if len(resourceIDs) == 0 {
		return map[string]*models.ResourceFreshness{}, nil
	}

	query := `
		SELECT ` + resourceFreshnessColumns + `
		FROM resource_freshness
		WHERE resource_type = $1 AND resource_id = ANY($2::uuid[])
	`

	rows, err := r.db.QueryContext(ctx, query, resourceType, pq.Array(resourceIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to list freshness for %s resources: %w", resourceType, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close resource freshness page rows", "error", closeErr)
		}
	}()

	byResource := make(map[string]*models.ResourceFreshness, len(resourceIDs))
	for rows.Next() {
		var f models.ResourceFreshness
		if err := rows.Scan(scanResourceFreshnessDest(&f)...); err != nil {
			return nil, fmt.Errorf("failed to scan freshness for %s resource: %w", resourceType, err)
		}
		byResource[f.ResourceID] = &f
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate freshness for %s resources: %w", resourceType, err)
	}
	return byResource, nil
}
