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

// Instance-wide project reads for the admin surface (#453, #1143).
//
// The listing applies the admin list count-aggregate pattern (see the comment
// above adminUserStatsCTE in admin.go): one pre-aggregated CTE per
// project-scoped resource table, each unique on project_id and LEFT JOINed 1:1
// onto projects. The team and creator joins are many-to-one. No join can fan
// out, so COUNT(*) stays exact and the count and page queries share one FROM
// and one WHERE.
//
// Project-scoped means a direct project_id column: prompts, memories,
// artifacts, blueprints and feed_items (nullable there, so a feed item posted
// without a project counts for none). agents and feeds are team-scoped;
// comments and attachments reach a project only through the resource they
// belong to. All four are excluded rather than reported as zero.
//
// Cross-tenant reads with no role predicate (decision D3) — the only
// authorization is instanceAdminMiddleware at the transport layer.

// colProjectCreatedAt is the project creation timestamp, used by the
// projection, the date-range filters and the default sort.
const colProjectCreatedAt = "p.created_at"

// adminProjectStatsCTE holds one aggregate per project-scoped table. The naive
// memories.created_at is normalized inside its CTE (rule 4 of the pattern), and
// feed_items has no created_at: its creation time is posted_at.
const adminProjectStatsCTE = `WITH
	pr AS (SELECT project_id, COUNT(*) AS n, MAX(created_at) AS last_at FROM prompts GROUP BY project_id),
	me AS (SELECT project_id, COUNT(*) AS n, MAX(created_at) AT TIME ZONE 'UTC' AS last_at
		FROM memories GROUP BY project_id),
	ar AS (SELECT project_id, COUNT(*) AS n, MAX(created_at) AS last_at FROM artifacts GROUP BY project_id),
	bp AS (SELECT project_id, COUNT(*) AS n, MAX(created_at) AS last_at FROM blueprints GROUP BY project_id),
	fi AS (SELECT project_id, COUNT(*) AS n, MAX(posted_at) AS last_at
		FROM feed_items WHERE project_id IS NOT NULL GROUP BY project_id)`

// adminProjectStatsAliases are the CTE aliases of adminProjectStatsCTE, each
// LEFT JOINed onto projects on project_id.
var adminProjectStatsAliases = []string{"pr", "me", "ar", "bp", "fi"}

// Aggregate expressions over adminProjectStatsCTE, shared by the projection, the
// filters and the sort allowlist so the three can never disagree.
const (
	colProjectPromptCount    = "COALESCE(pr.n, 0)"
	colProjectMemoryCount    = "COALESCE(me.n, 0)"
	colProjectArtifactCount  = "COALESCE(ar.n, 0)"
	colProjectBlueprintCount = "COALESCE(bp.n, 0)"
	colProjectFeedItemCount  = "COALESCE(fi.n, 0)"

	colProjectTotalResourceCount = "(" + colProjectPromptCount + " + " + colProjectMemoryCount + " + " +
		colProjectArtifactCount + " + " + colProjectBlueprintCount + " + " + colProjectFeedItemCount + ")"

	// GREATEST ignores NULLs, so this is NULL only for a project with no resources.
	colProjectLastResourceCreatedAt = "GREATEST(pr.last_at, me.last_at, ar.last_at, bp.last_at, fi.last_at)"
)

// adminProjectListSelectColumns is the projection for the project listing, in
// the order queryAdminProjects scans it.
// `owner` is projects.user_id, the project's creator — NOT the team's owner_id.
var adminProjectListSelectColumns = []string{
	"p.id", "p.name", "p.slug", colProjectCreatedAt, "p.updated_at",
	"t.id", "t.name", "t.slug",
	"u.id", "u.email", "u.name",
	colProjectPromptCount + " AS prompt_count",
	colProjectMemoryCount + " AS memory_count",
	colProjectArtifactCount + " AS artifact_count",
	colProjectBlueprintCount + " AS blueprint_count",
	colProjectFeedItemCount + " AS feed_item_count",
	colProjectTotalResourceCount + " AS total_resource_count",
	colProjectLastResourceCreatedAt + " AS last_resource_created_at",
}

// adminProjectSortColumns is the ORDER BY allowlist: sort_by enum value -> fixed
// expression. Anything absent falls back to p.created_at.
var adminProjectSortColumns = map[string]string{
	"name":                     "p.name",
	"created_at":               colProjectCreatedAt,
	"prompt_count":             colProjectPromptCount,
	"memory_count":             colProjectMemoryCount,
	"artifact_count":           colProjectArtifactCount,
	"blueprint_count":          colProjectBlueprintCount,
	"feed_item_count":          colProjectFeedItemCount,
	"total_resource_count":     colProjectTotalResourceCount,
	"last_resource_created_at": colProjectLastResourceCreatedAt,
}

// adminProjectListFrom is the FROM/JOIN shared by the count and page queries, so
// both see the same row set before filtering: projects, its team and creator
// (inner, NOT NULL FKs) and the aggregate CTEs, all at most one row per project.
func adminProjectListFrom(sb squirrel.SelectBuilder) squirrel.SelectBuilder {
	sb = sb.Prefix(adminProjectStatsCTE).
		From("projects p").
		Join("teams t ON t.id = p.team_id").
		Join("users u ON u.id = p.user_id")
	for _, alias := range adminProjectStatsAliases {
		sb = sb.LeftJoin(alias + " ON " + alias + ".project_id = p.id")
	}
	return sb
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
		where = append(where, squirrel.GtOrEq{colProjectCreatedAt: *filters.CreatedFrom})
	}
	if filters.CreatedTo != nil {
		where = append(where, squirrel.LtOrEq{colProjectCreatedAt: *filters.CreatedTo})
	}
	// The creator (projects.user_id), matching the `owner` column; emails
	// created via an identity provider may carry mixed case.
	if filters.OwnerEmail != nil && *filters.OwnerEmail != "" {
		where = append(where, squirrel.Expr("lower(u.email) = lower(?)", *filters.OwnerEmail))
	}

	for _, cr := range []struct {
		col string
		r   repositories.AdminCountRange
	}{
		{colProjectPromptCount, filters.PromptCount},
		{colProjectMemoryCount, filters.MemoryCount},
		{colProjectArtifactCount, filters.ArtifactCount},
		{colProjectBlueprintCount, filters.BlueprintCount},
		{colProjectFeedItemCount, filters.FeedItemCount},
		{colProjectTotalResourceCount, filters.TotalResourceCount},
	} {
		where = appendAdminCountRange(where, cr.col, cr.r)
	}

	// A NULL GREATEST (no resources) fails both comparisons, so such projects
	// never match a last-resource bound.
	if filters.LastResourceCreatedFrom != nil {
		where = append(where, squirrel.GtOrEq{colProjectLastResourceCreatedAt: *filters.LastResourceCreatedFrom})
	}
	if filters.LastResourceCreatedTo != nil {
		where = append(where, squirrel.LtOrEq{colProjectLastResourceCreatedAt: *filters.LastResourceCreatedTo})
	}

	return where
}

// buildAdminProjectOrderBy builds the ORDER BY from an allowlist — the same
// SQL-injection control as the users/teams listings. The p.id tie-breaker keeps
// paging stable when the sort column has duplicates (project names are not
// unique across teams). last_resource_created_at is NULL for projects with no
// resources; NULLS LAST keeps them at the end in both directions.
func buildAdminProjectOrderBy(filters repositories.AdminProjectFilters) string {
	column, ok := adminProjectSortColumns[filters.SortBy]
	if !ok {
		column = colProjectCreatedAt
	}
	clause := column + " " + adminSortDirection(filters.SortOrder)
	if column == colProjectLastResourceCreatedAt {
		clause += " NULLS LAST"
	}
	return clause + ", p.id"
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
// FROM/JOIN as the page query. The team and creator joins are inner on NOT NULL
// FKs and every aggregate join is 1:1, so COUNT(*) neither drops nor duplicates
// a project. Aggregates no predicate references are removed by the planner
// (rule 5 of the pattern above adminUserStatsCTE).
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

	return collectAdminListRows(ctx, r, sb, "project", scanAdminProjectListItem)
}

// scanAdminProjectListItem scans one row of adminProjectListSelectColumns,
// shared by the page query and the export stream.
func scanAdminProjectListItem(rows *sql.Rows) (models.AdminProjectListItem, error) {
	var p models.AdminProjectListItem
	rc := &p.ResourceCounts
	err := rows.Scan(
		&p.ID, &p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt,
		&p.Team.ID, &p.Team.Name, &p.Team.Slug,
		&p.Owner.ID, &p.Owner.Email, &p.Owner.Name,
		&rc.Prompts, &rc.Memories, &rc.Artifacts, &rc.Blueprints, &rc.FeedItems, &rc.Total,
		&p.LastResourceCreatedAt,
	)
	return p, err
}

// CountProjects returns the size of the filtered project set, from the same
// count query the listing's pagination envelope uses.
func (r *AdminRepository) CountProjects(ctx context.Context, filters repositories.AdminProjectFilters) (int, error) {
	return r.countAdminProjects(ctx, buildAdminProjectWhere(filters))
}

// StreamProjects calls fn for each project matching the filters, in the
// listing's order, up to limit rows (#1149).
func (r *AdminRepository) StreamProjects(
	ctx context.Context, filters repositories.AdminProjectFilters, limit int,
	fn func(models.AdminProjectListItem) error,
) error {
	sb := applyAdminWhere(
		adminProjectListFrom(psql.Select(adminProjectListSelectColumns...)), buildAdminProjectWhere(filters),
	).
		OrderBy(buildAdminProjectOrderBy(filters)).
		Limit(adminStreamLimit(limit))
	return eachAdminListRow(ctx, r, sb, "project", scanAdminProjectListItem, fn)
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
// round-trip: exactly the five tables with a direct project_id, the same set the
// listing aggregates (see the header of this file), so list and detail agree.
const adminProjectResourceCountsQuery = `
SELECT
	(SELECT COUNT(*) FROM prompts    WHERE project_id = $1) AS prompts,
	(SELECT COUNT(*) FROM artifacts  WHERE project_id = $1) AS artifacts,
	(SELECT COUNT(*) FROM memories   WHERE project_id = $1) AS memories,
	(SELECT COUNT(*) FROM blueprints WHERE project_id = $1) AS blueprints,
	(SELECT COUNT(*) FROM feed_items WHERE project_id = $1) AS feed_items
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
		&counts.Prompts, &counts.Artifacts, &counts.Memories, &counts.Blueprints, &counts.FeedItems,
	)
	if err != nil {
		return models.AdminProjectResourceCounts{}, fmt.Errorf("failed to count project resources: %w", err)
	}
	counts.Total = counts.Prompts + counts.Artifacts + counts.Memories + counts.Blueprints + counts.FeedItems
	return counts, nil
}
