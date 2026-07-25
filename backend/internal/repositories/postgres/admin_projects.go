package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Instance-wide project reads for the admin surface (#453).
//
// Both joins are many-to-one (`projects` → its one `team`, → its one owning
// `user`), so no row can fan out and COUNT(*) stays exact. Deliberately no
// LEFT JOIN onto a resource table here: that would multiply rows per project.
// Resource counts live on the DETAIL endpoint only, as correlated subqueries.
//
// Cross-tenant reads with no role predicate (decision D3) — the only
// authorization is instanceAdminMiddleware at the transport layer.

// adminProjectListSelectColumns is the projection for the project listing.
// `owner` is projects.user_id, the project's creator — NOT the team's owner_id.
var adminProjectListSelectColumns = []string{
	"p.id", "p.name", "p.slug", "p.created_at", "p.updated_at",
	"t.id", "t.name", "t.slug",
	"u.id", "u.email", "u.name",
}

// adminProjectListFrom is the FROM/JOIN shared by the count and page queries, so
// both see the same row set before filtering.
func adminProjectListFrom(sb squirrel.SelectBuilder) squirrel.SelectBuilder {
	return sb.
		From("projects p").
		Join("teams t ON t.id = p.team_id").
		Join("users u ON u.id = p.user_id")
}

// buildAdminProjectWhere builds the shared WHERE conditions, consumed by BOTH
// the count and the page query so the envelope can never diverge from the rows.
func buildAdminProjectWhere(filters repositories.AdminProjectFilters) squirrel.And {
	where := squirrel.And{}

	if filters.Search != nil && *filters.Search != "" {
		term := "%" + *filters.Search + "%"
		where = append(where, squirrel.Expr("(p.name ILIKE ? OR p.slug ILIKE ?)", term, term))
	}
	if filters.TeamID != nil && *filters.TeamID != "" {
		where = append(where, squirrel.Eq{"p.team_id": *filters.TeamID})
	}
	if filters.CreatedFrom != nil {
		where = append(where, squirrel.GtOrEq{"p.created_at": *filters.CreatedFrom})
	}
	if filters.CreatedTo != nil {
		where = append(where, squirrel.LtOrEq{"p.created_at": *filters.CreatedTo})
	}

	return where
}

// buildAdminProjectOrderBy builds the ORDER BY from an allowlist — the same
// SQL-injection control as the users/teams listings. The p.id tie-breaker keeps
// paging stable when the sort column has duplicates (project names are not
// unique across teams).
func buildAdminProjectOrderBy(filters repositories.AdminProjectFilters) string {
	column := "p.created_at"
	if filters.SortBy == "name" {
		column = "p.name"
	}
	return column + " " + adminSortDirection(filters.SortOrder) + ", p.id"
}

// ListProjects returns a page of projects matching the filters, plus the total
// count of the filtered set.
func (r *AdminRepository) ListProjects(
	ctx context.Context, filters repositories.AdminProjectFilters,
) ([]models.AdminProjectListItem, int, error) {
	where := buildAdminProjectWhere(filters)

	totalCount, err := r.countAdminProjects(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	projects, err := r.queryAdminProjects(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return projects, totalCount, nil
}

// countAdminProjects counts projects matching the shared WHERE over the same
// FROM/JOIN as the page query. Both joins are inner on NOT NULL FKs, so COUNT(*)
// neither drops nor duplicates a project.
func (r *AdminRepository) countAdminProjects(ctx context.Context, where squirrel.And) (int, error) {
	query, args, err := applyAdminWhere(adminProjectListFrom(psql.Select("COUNT(*)")), where).ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build admin project count query: %w", err)
	}

	var totalCount int
	if scanErr := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); scanErr != nil {
		return 0, fmt.Errorf("failed to count projects: %w", scanErr)
	}
	return totalCount, nil
}

// queryAdminProjects runs the paginated page query using the same WHERE as the
// count query.
func (r *AdminRepository) queryAdminProjects(
	ctx context.Context, where squirrel.And, filters repositories.AdminProjectFilters,
) ([]models.AdminProjectListItem, error) {
	limit, offset := adminPageBounds(filters.Page, filters.Limit)
	sb := applyAdminWhere(
		adminProjectListFrom(psql.Select(adminProjectListSelectColumns...)), where,
	).
		OrderBy(buildAdminProjectOrderBy(filters)).
		Limit(limit).
		Offset(offset)

	query, args, err := sb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build admin project list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer closeAdminRows(rows, "admin project")

	projects := make([]models.AdminProjectListItem, 0)
	for rows.Next() {
		var p models.AdminProjectListItem
		if scanErr := rows.Scan(
			&p.ID, &p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt,
			&p.Team.ID, &p.Team.Name, &p.Team.Slug,
			&p.Owner.ID, &p.Owner.Email, &p.Owner.Name,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan admin project: %w", scanErr)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate admin projects: %w", err)
	}
	return projects, nil
}

// adminProjectDetailQuery reads one project with its team and owner. Nullable
// text columns carry a DEFAULT ” in the schema, but COALESCE keeps the scan safe
// against a row written before those defaults existed.
const adminProjectDetailQuery = `
SELECT p.id, p.name, p.slug,
	COALESCE(p.description, ''), COALESCE(p.git_url, ''), COALESCE(p.homepage, ''),
	p.created_at, p.updated_at,
	t.id, t.name, t.slug,
	u.id, u.email, u.name
FROM projects p
JOIN teams t ON t.id = p.team_id
JOIN users u ON u.id = p.user_id
WHERE p.id = $1
`

// adminProjectResourceCountsQuery counts the project-scoped resource types in one
// round-trip.
//
// It covers exactly the four tables that HAVE a project_id column, verified
// against migrations/001_baseline.up.sql. `agents` and `feeds` are team-scoped
// and have no such column, so they are absent rather than reported as zero —
// a zero would read as "this project has no agents" instead of "agents do not
// belong to projects".
const adminProjectResourceCountsQuery = `
SELECT
	(SELECT COUNT(*) FROM prompts    WHERE project_id = $1) AS prompts,
	(SELECT COUNT(*) FROM artifacts  WHERE project_id = $1) AS artifacts,
	(SELECT COUNT(*) FROM memories   WHERE project_id = $1) AS memories,
	(SELECT COUNT(*) FROM blueprints WHERE project_id = $1) AS blueprints
`

// GetProjectDetail returns one project with its team, owner and resource counts,
// or (nil, nil) when no project with that id exists — the convention the handler
// maps to 404.
func (r *AdminRepository) GetProjectDetail(
	ctx context.Context, id string,
) (*models.AdminProjectDetail, error) {
	var detail models.AdminProjectDetail
	err := r.db.QueryRowContext(ctx, adminProjectDetailQuery, id).Scan(
		&detail.ID, &detail.Name, &detail.Slug,
		&detail.Description, &detail.GitURL, &detail.Homepage,
		&detail.CreatedAt, &detail.UpdatedAt,
		&detail.Team.ID, &detail.Team.Name, &detail.Team.Slug,
		&detail.Owner.ID, &detail.Owner.Email, &detail.Owner.Name,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin project: %w", err)
	}

	counts, err := r.projectResourceCounts(ctx, id)
	if err != nil {
		return nil, err
	}
	detail.ResourceCounts = counts

	return &detail, nil
}

// projectResourceCounts runs the per-type counts for one project.
func (r *AdminRepository) projectResourceCounts(
	ctx context.Context, projectID string,
) (models.AdminProjectResourceCounts, error) {
	var counts models.AdminProjectResourceCounts
	err := r.db.QueryRowContext(ctx, adminProjectResourceCountsQuery, projectID).Scan(
		&counts.Prompts, &counts.Artifacts, &counts.Memories, &counts.Blueprints,
	)
	if err != nil {
		return models.AdminProjectResourceCounts{}, fmt.Errorf("failed to count project resources: %w", err)
	}
	return counts, nil
}
