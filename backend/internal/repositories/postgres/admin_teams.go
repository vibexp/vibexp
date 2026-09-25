package postgres

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// adminTeamStatsCTE applies the admin list count-aggregate pattern (see the
// comment above adminUserStatsCTE) to the team listing (#1138): one CTE per
// source, each unique on team_id and LEFT JOINed 1:1 onto teams.
//
// Resource tables are counted by team_id (every row, regardless of status or
// archive state, matching #1133). Multi-row "configured" sources are reduced
// to DISTINCT team_id, so their joins cannot fan out either. The member/role
// aggregate (mc) lives in admin_team_role_counts.go — see that file for why.
//
// "Configured" means the team's OWN row, never instance-level fallback (epic
// #1131 decision 11). embedding_providers.team_id is nullable (legacy rows the
// 003 backfill could not attribute), and those rows count for no team.
const adminTeamStatsCTE = "WITH " + adminTeamRoleCountsCTE + `,
	pj AS (SELECT team_id, COUNT(*) AS n FROM projects GROUP BY team_id),
	pr AS (SELECT team_id, COUNT(*) AS n FROM prompts GROUP BY team_id),
	me AS (SELECT team_id, COUNT(*) AS n FROM memories GROUP BY team_id),
	ar AS (SELECT team_id, COUNT(*) AS n FROM artifacts GROUP BY team_id),
	bp AS (SELECT team_id, COUNT(*) AS n FROM blueprints GROUP BY team_id),
	ag AS (SELECT team_id, COUNT(*) AS n FROM agents GROUP BY team_id),
	fd AS (SELECT team_id, COUNT(*) AS n FROM feeds GROUP BY team_id),
	fi AS (SELECT team_id, COUNT(*) AS n FROM feed_items GROUP BY team_id),
	cm AS (SELECT team_id, COUNT(*) AS n FROM comments GROUP BY team_id),
	att AS (SELECT team_id, COUNT(*) AS n FROM attachments GROUP BY team_id),
	ep AS (SELECT DISTINCT team_id FROM embedding_providers WHERE team_id IS NOT NULL),
	mp AS (SELECT DISTINCT team_id FROM model_providers),
	gi AS (SELECT DISTINCT team_id FROM github_installations),
	fr AS (SELECT DISTINCT team_id FROM freshness_rules WHERE enabled)`

// adminTeamStatsAliases are the CTE aliases of adminTeamStatsCTE, each LEFT
// JOINed onto teams on team_id.
var adminTeamStatsAliases = []string{
	"mc", "pj", "pr", "me", "ar", "bp", "ag", "fd", "fi", "cm", "att", "ep", "mp", "gi", "fr",
}

// adminTeamSettingsJoins are the per-team singleton settings tables (PRIMARY KEY
// or UNIQUE on team_id), joined directly — at most one row each, so no fan-out.
var adminTeamSettingsJoins = []string{
	"team_ai_summary_settings ais ON ais.team_id = t.id",
	"team_email_providers tep ON tep.team_id = t.id",
	"github_app_configs gac ON gac.team_id = t.id",
	"team_search_settings tss ON tss.team_id = t.id",
}

// Expressions over adminTeamStatsCTE and the settings joins, shared by the
// projection, the filters and the sort allowlist so the three can never
// disagree.
const (
	colTeamMemberCount     = "COALESCE(mc.n, 0)"
	colTeamOwnerCount      = "COALESCE(mc.owners, 0)"
	colTeamAdminCount      = "COALESCE(mc.admins, 0)"
	colTeamProjectCount    = "COALESCE(pj.n, 0)"
	colTeamPromptCount     = "COALESCE(pr.n, 0)"
	colTeamMemoryCount     = "COALESCE(me.n, 0)"
	colTeamArtifactCount   = "COALESCE(ar.n, 0)"
	colTeamBlueprintCount  = "COALESCE(bp.n, 0)"
	colTeamAgentCount      = "COALESCE(ag.n, 0)"
	colTeamFeedCount       = "COALESCE(fd.n, 0)"
	colTeamFeedItemCount   = "COALESCE(fi.n, 0)"
	colTeamCommentCount    = "COALESCE(cm.n, 0)"
	colTeamAttachmentCount = "COALESCE(att.n, 0)"

	colTeamTotalResourceCount = "(" + colTeamPromptCount + " + " + colTeamMemoryCount + " + " +
		colTeamArtifactCount + " + " + colTeamBlueprintCount + " + " + colTeamAgentCount + " + " +
		colTeamFeedCount + " + " + colTeamFeedItemCount + " + " + colTeamCommentCount + " + " +
		colTeamAttachmentCount + ")"

	colTeamEmbeddingConfigured      = "(ep.team_id IS NOT NULL)"
	colTeamLLMConfigured            = "(mp.team_id IS NOT NULL)"
	colTeamAISummaryEnabled         = "COALESCE(ais.enabled, false)"
	colTeamEmailConfigured          = "(tep.team_id IS NOT NULL)"
	colTeamGitHubConfigured         = "(gac.team_id IS NOT NULL OR gi.team_id IS NOT NULL)"
	colTeamSearchSettingsCustomized = "(tss.team_id IS NOT NULL)"
	colTeamFreshnessEnabled         = "(fr.team_id IS NOT NULL)"
)

// adminTeamListSelectColumns is the projection for the admin team listing, in
// the order queryAdminTeams scans it. The owner join is inner (teams.owner_id ->
// users, ON DELETE CASCADE, so an existing team always has an owner).
var adminTeamListSelectColumns = []string{
	"t.id", "t.name", "t.slug", "t.is_personal", colTeamCreatedAt,
	"u.id", colUserEmail, colUserName,
	colTeamMemberCount + " AS member_count",
	colTeamOwnerCount + " AS owner_count",
	colTeamAdminCount + " AS admin_count",
	colTeamProjectCount + " AS project_count",
	colTeamPromptCount + " AS prompt_count",
	colTeamMemoryCount + " AS memory_count",
	colTeamArtifactCount + " AS artifact_count",
	colTeamBlueprintCount + " AS blueprint_count",
	colTeamAgentCount + " AS agent_count",
	colTeamFeedCount + " AS feed_count",
	colTeamFeedItemCount + " AS feed_item_count",
	colTeamCommentCount + " AS comment_count",
	colTeamAttachmentCount + " AS attachment_count",
	colTeamTotalResourceCount + " AS total_resource_count",
	colTeamEmbeddingConfigured + " AS embedding_configured",
	colTeamLLMConfigured + " AS llm_configured",
	colTeamAISummaryEnabled + " AS ai_summary_enabled",
	colTeamEmailConfigured + " AS email_configured",
	colTeamGitHubConfigured + " AS github_configured",
	colTeamSearchSettingsCustomized + " AS search_settings_customized",
	colTeamFreshnessEnabled + " AS freshness_enabled",
}

// adminTeamSortColumns is the ORDER BY allowlist: sort_by enum value -> fixed
// expression. Anything absent falls back to t.created_at.
var adminTeamSortColumns = map[string]string{
	"name":                 "t.name",
	"created_at":           colTeamCreatedAt,
	"member_count":         colTeamMemberCount,
	"owner_count":          colTeamOwnerCount,
	"admin_count":          colTeamAdminCount,
	"project_count":        colTeamProjectCount,
	"prompt_count":         colTeamPromptCount,
	"memory_count":         colTeamMemoryCount,
	"artifact_count":       colTeamArtifactCount,
	"blueprint_count":      colTeamBlueprintCount,
	"agent_count":          colTeamAgentCount,
	"feed_count":           colTeamFeedCount,
	"feed_item_count":      colTeamFeedItemCount,
	"comment_count":        colTeamCommentCount,
	"attachment_count":     colTeamAttachmentCount,
	"total_resource_count": colTeamTotalResourceCount,
}

// adminTeamListFrom is the FROM/JOIN shared by the count and page queries, so
// both see exactly the same row set before filtering: teams, the inner owner
// join, the aggregate CTEs and the settings singletons, all 1:1.
func adminTeamListFrom(sb squirrel.SelectBuilder) squirrel.SelectBuilder {
	sb = sb.Prefix(adminTeamStatsCTE).From("teams t").Join("users u ON u.id = t.owner_id")
	for _, alias := range adminTeamStatsAliases {
		sb = sb.LeftJoin(alias + " ON " + alias + ".team_id = t.id")
	}
	for _, join := range adminTeamSettingsJoins {
		sb = sb.LeftJoin(join)
	}
	return sb
}

// buildAdminTeamWhere builds the shared WHERE conditions for the admin team
// listing, consumed by both the count and the page query so they can never
// diverge.
func buildAdminTeamWhere(filters repositories.AdminTeamFilters) squirrel.And {
	where := squirrel.And{}

	if filters.Search != nil && *filters.Search != "" {
		term := "%" + *filters.Search + "%"
		where = append(where, squirrel.Expr(
			"(t.name ILIKE ? OR t.slug ILIKE ? OR u.email ILIKE ?)", term, term, term,
		))
	}
	if filters.IsPersonal != nil {
		where = append(where, squirrel.Eq{"t.is_personal": *filters.IsPersonal})
	}
	if filters.CreatedFrom != nil {
		where = append(where, squirrel.GtOrEq{colTeamCreatedAt: *filters.CreatedFrom})
	}
	if filters.CreatedTo != nil {
		where = append(where, squirrel.LtOrEq{colTeamCreatedAt: *filters.CreatedTo})
	}
	// teams.owner_id is authoritative for who the owner is; emails created via
	// an identity provider may carry mixed case, hence lower() on both sides.
	if filters.OwnerEmail != nil && *filters.OwnerEmail != "" {
		where = append(where, squirrel.Expr("lower(u.email) = lower(?)", *filters.OwnerEmail))
	}

	for _, cr := range []struct {
		col string
		r   repositories.AdminCountRange
	}{
		{colTeamMemberCount, filters.MemberCount},
		{colTeamOwnerCount, filters.OwnerCount},
		{colTeamAdminCount, filters.AdminCount},
		{colTeamProjectCount, filters.ProjectCount},
		{colTeamPromptCount, filters.PromptCount},
		{colTeamMemoryCount, filters.MemoryCount},
		{colTeamArtifactCount, filters.ArtifactCount},
		{colTeamBlueprintCount, filters.BlueprintCount},
		{colTeamAgentCount, filters.AgentCount},
		{colTeamFeedCount, filters.FeedCount},
		{colTeamFeedItemCount, filters.FeedItemCount},
		{colTeamCommentCount, filters.CommentCount},
		{colTeamAttachmentCount, filters.AttachmentCount},
		{colTeamTotalResourceCount, filters.TotalResourceCount},
	} {
		where = appendAdminCountRange(where, cr.col, cr.r)
	}

	return appendAdminTeamTriStates(where, filters)
}

// appendAdminTeamTriStates appends the configured tri-states. Each compares a
// never-NULL boolean expression with the bound value; nil adds nothing, so an
// absent param never narrows the list.
func appendAdminTeamTriStates(where squirrel.And, filters repositories.AdminTeamFilters) squirrel.And {
	for _, ts := range []struct {
		col string
		v   *bool
	}{
		{colTeamEmbeddingConfigured, filters.EmbeddingConfigured},
		{colTeamLLMConfigured, filters.LLMConfigured},
		{colTeamAISummaryEnabled, filters.AISummaryEnabled},
		{colTeamEmailConfigured, filters.EmailConfigured},
		{colTeamGitHubConfigured, filters.GitHubConfigured},
		{colTeamSearchSettingsCustomized, filters.SearchSettingsCustomized},
		{colTeamFreshnessEnabled, filters.FreshnessEnabled},
	} {
		if ts.v != nil {
			where = append(where, squirrel.Eq{ts.col: *ts.v})
		}
	}

	return where
}

// buildAdminTeamOrderBy builds the ORDER BY clause from an allowlist (the same
// SQL-injection control as buildAdminUserOrderBy). No team sort key is
// nullable. The t.id tie-breaker keeps paging stable.
func buildAdminTeamOrderBy(filters repositories.AdminTeamFilters) string {
	column, ok := adminTeamSortColumns[filters.SortBy]
	if !ok {
		column = colTeamCreatedAt
	}
	return column + " " + adminSortDirection(filters.SortOrder) + ", t.id"
}

// ListTeams returns a page of teams matching the filters with owner, counts and
// configuration state, plus the total count of the filtered set.
func (r *AdminRepository) ListTeams(
	ctx context.Context, filters repositories.AdminTeamFilters,
) ([]models.AdminTeamListItem, int, error) {
	where := buildAdminTeamWhere(filters)

	totalCount, err := r.countAdminTeams(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	teams, err := r.queryAdminTeams(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return teams, totalCount, nil
}

// countAdminTeams counts teams matching the shared WHERE conditions over the
// same FROM/JOIN as the page query. The owner join is inner on a NOT NULL FK and
// every other join is 1:1, so no team row is duplicated or dropped and COUNT(*)
// is exact. Joins no predicate references are removed by the planner (rule 5 of
// the pattern above adminUserStatsCTE).
func (r *AdminRepository) countAdminTeams(ctx context.Context, where squirrel.And) (int, error) {
	query, args, err := applyAdminWhere(adminTeamListFrom(psql.Select("COUNT(*)")), where).ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build admin team count query: %w", err)
	}

	var totalCount int
	if scanErr := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); scanErr != nil {
		return 0, fmt.Errorf("failed to count teams: %w", scanErr)
	}
	return totalCount, nil
}

// queryAdminTeams runs the paginated page query using the same FROM and WHERE
// conditions as countAdminTeams.
func (r *AdminRepository) queryAdminTeams(
	ctx context.Context, where squirrel.And, filters repositories.AdminTeamFilters,
) ([]models.AdminTeamListItem, error) {
	limit, offset := adminPageBounds(filters.Page, filters.Limit)
	sb := applyAdminWhere(
		adminTeamListFrom(psql.Select(adminTeamListSelectColumns...)), where,
	).
		OrderBy(buildAdminTeamOrderBy(filters)).
		Limit(limit).
		Offset(offset)

	rows, err := r.runAdminListQuery(ctx, sb, "team")
	if err != nil {
		return nil, err
	}
	defer closeAdminListRows(rows, "team")

	teams := make([]models.AdminTeamListItem, 0)
	for rows.Next() {
		var t models.AdminTeamListItem
		rc, cfg := &t.ResourceCounts, &t.Configuration
		if scanErr := rows.Scan(
			&t.ID, &t.Name, &t.Slug, &t.IsPersonal, &t.CreatedAt,
			&t.Owner.ID, &t.Owner.Email, &t.Owner.Name,
			&t.MemberCount, &t.OwnerCount, &t.AdminCount, &t.ProjectCount,
			&rc.Prompts, &rc.Memories, &rc.Artifacts, &rc.Blueprints, &rc.Agents,
			&rc.Feeds, &rc.FeedItems, &rc.Comments, &rc.Attachments, &rc.Total,
			&cfg.EmbeddingConfigured, &cfg.LLMConfigured, &cfg.AISummaryEnabled, &cfg.EmailConfigured,
			&cfg.GitHubConfigured, &cfg.SearchSettingsCustomized, &cfg.FreshnessEnabled,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan admin team: %w", scanErr)
		}
		teams = append(teams, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate admin teams: %w", err)
	}
	return teams, nil
}
