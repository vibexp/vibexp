package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// AdminRepository implements the repositories.AdminRepository interface for
// PostgreSQL.
type AdminRepository struct {
	db *database.DB
}

// NewAdminRepository creates a new AdminRepository.
func NewAdminRepository(db *database.DB) repositories.AdminRepository {
	return &AdminRepository{
		db: db,
	}
}

// instanceCountsQuery gathers every instance-wide total in a single round-trip
// via correlated scalar subqueries. Table names are hardcoded (not user input),
// so there is no injection surface.
const instanceCountsQuery = `
SELECT
	(SELECT COUNT(*) FROM users)     AS users,
	(SELECT COUNT(*) FROM teams)     AS teams,
	(SELECT COUNT(*) FROM prompts)   AS prompts,
	(SELECT COUNT(*) FROM artifacts) AS artifacts,
	(SELECT COUNT(*) FROM memories)  AS memories
`

// GetInstanceCounts returns unscoped totals for the top-level entities.
func (r *AdminRepository) GetInstanceCounts(ctx context.Context) (models.InstanceCounts, error) {
	var counts models.InstanceCounts
	err := r.db.QueryRowContext(ctx, instanceCountsQuery).Scan(
		&counts.Users,
		&counts.Teams,
		&counts.Prompts,
		&counts.Artifacts,
		&counts.Memories,
	)
	if err != nil {
		return models.InstanceCounts{}, fmt.Errorf("failed to query instance counts: %w", err)
	}
	return counts, nil
}

// adminUserListSelectColumns is the projection for the admin user listing. The
// team count comes from a LEFT JOIN aggregate over team_members; no role
// predicate (decision D3) — the join is on user_id only.
var adminUserListSelectColumns = []string{
	"u.id", "u.email", "u.name", "u.idp_provider", "u.status", "u.created_at",
	"COUNT(tm.team_id) AS team_count",
}

// adminUserListGroupByColumns are the non-aggregated projection columns, which
// must all appear in GROUP BY alongside the team_count aggregate.
var adminUserListGroupByColumns = []string{
	"u.id", "u.email", "u.name", "u.idp_provider", "u.status", "u.created_at",
}

// applyAdminWhere attaches the shared conditions to a select builder, skipping
// an empty conjunction (squirrel would emit a dangling "WHERE" for it).
func applyAdminWhere(sb squirrel.SelectBuilder, where squirrel.And) squirrel.SelectBuilder {
	if len(where) == 0 {
		return sb
	}
	return sb.Where(where)
}

// adminSortDirection maps the allowlisted sort_order enum to SQL. Anything
// other than "asc" (including the empty default) sorts descending.
func adminSortDirection(sortOrder string) string {
	if strings.EqualFold(sortOrder, "asc") {
		return "ASC"
	}
	return "DESC"
}

// adminPageBounds converts page/limit into the unsigned LIMIT/OFFSET squirrel
// requires, clamping first (the same guard as prompt.go queryList): negative
// paging inputs would otherwise wrap to huge values, and two negative factors
// would multiply into a bogus positive offset. Contract: a non-positive limit
// emits LIMIT 0 (empty page); the service clamps to page>=1, limit in [1,100].
func adminPageBounds(page, limit int) (boundedLimit, offset uint64) {
	if limit > 0 {
		boundedLimit = uint64(limit)
	}
	if page > 1 && limit > 0 {
		if raw := (page - 1) * limit; raw > 0 {
			offset = uint64(raw)
		}
	}
	return boundedLimit, offset
}

// buildAdminUserWhere builds the shared WHERE conditions for the admin user
// listing. The count and page queries consume the same conditions, so the
// pagination envelope can never diverge from the returned rows. Every predicate
// here references only columns of `users`, which is why the count query can skip
// the team_members join entirely.
func buildAdminUserWhere(filters repositories.AdminUserFilters) squirrel.And {
	where := squirrel.And{}

	if filters.Search != nil && *filters.Search != "" {
		term := "%" + *filters.Search + "%"
		where = append(where, squirrel.Expr("(u.email ILIKE ? OR u.name ILIKE ?)", term, term))
	}
	if filters.IDPProvider != nil && *filters.IDPProvider != "" {
		where = append(where, squirrel.Eq{"u.idp_provider": *filters.IDPProvider})
	}
	if filters.Status != nil && *filters.Status != "" {
		where = append(where, squirrel.Eq{"u.status": *filters.Status})
	}
	if filters.CreatedFrom != nil {
		where = append(where, squirrel.GtOrEq{"u.created_at": *filters.CreatedFrom})
	}
	if filters.CreatedTo != nil {
		where = append(where, squirrel.LtOrEq{"u.created_at": *filters.CreatedTo})
	}

	return where
}

// buildAdminUserOrderBy builds the ORDER BY clause from an allowlist. This is an
// SQL-injection control: the request's sort_by never reaches the query text, only
// a column name selected by the switch. The u.id tie-breaker keeps paging stable
// when the sort column has duplicates.
func buildAdminUserOrderBy(filters repositories.AdminUserFilters) string {
	column := "u.created_at"
	switch filters.SortBy {
	case "email", "name", "created_at":
		column = "u." + filters.SortBy
	case "team_count":
		column = "COUNT(tm.team_id)"
	}
	return column + " " + adminSortDirection(filters.SortOrder) + ", u.id"
}

// ListUsers returns a page of users matching the filters with team counts, plus
// the total count of the filtered set.
func (r *AdminRepository) ListUsers(
	ctx context.Context, filters repositories.AdminUserFilters,
) ([]models.AdminUserListItem, int, error) {
	where := buildAdminUserWhere(filters)

	totalCount, err := r.countAdminUsers(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	users, err := r.queryAdminUsers(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return users, totalCount, nil
}

// countAdminUsers counts users matching the shared WHERE conditions. The
// team_members join is deliberately absent: no filter predicate references it,
// and joining would require a COUNT(DISTINCT) over every membership row.
func (r *AdminRepository) countAdminUsers(ctx context.Context, where squirrel.And) (int, error) {
	query, args, err := applyAdminWhere(psql.Select("COUNT(*)").From("users u"), where).ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build admin user count query: %w", err)
	}

	var totalCount int
	if scanErr := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); scanErr != nil {
		return 0, fmt.Errorf("failed to count users: %w", scanErr)
	}
	return totalCount, nil
}

// queryAdminUsers runs the paginated page query using the same WHERE conditions
// as countAdminUsers.
func (r *AdminRepository) queryAdminUsers(
	ctx context.Context, where squirrel.And, filters repositories.AdminUserFilters,
) ([]models.AdminUserListItem, error) {
	limit, offset := adminPageBounds(filters.Page, filters.Limit)
	sb := applyAdminWhere(
		psql.Select(adminUserListSelectColumns...).
			From("users u").
			LeftJoin("team_members tm ON tm.user_id = u.id"),
		where,
	).
		GroupBy(adminUserListGroupByColumns...).
		OrderBy(buildAdminUserOrderBy(filters)).
		Limit(limit).
		Offset(offset)

	query, args, err := sb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build admin user list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close admin user rows", "error", closeErr)
		}
	}()

	users := make([]models.AdminUserListItem, 0)
	for rows.Next() {
		var u models.AdminUserListItem
		if scanErr := rows.Scan(
			&u.ID, &u.Email, &u.Name, &u.IDPProvider, &u.Status, &u.CreatedAt, &u.TeamCount,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan admin user: %w", scanErr)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate admin users: %w", err)
	}
	return users, nil
}

// adminUserMembershipsQuery returns the teams a user belongs to with the user's
// role as a plain column (no role predicate — decision D3).
const adminUserMembershipsQuery = `
SELECT t.id, t.name, tm.role
FROM team_members tm
JOIN teams t ON t.id = tm.team_id
WHERE tm.user_id = $1
ORDER BY t.name, t.id
`

// GetUserDetail returns one user with their team memberships, or (nil, nil) when
// no user with that id exists.
func (r *AdminRepository) GetUserDetail(
	ctx context.Context, id string,
) (*models.AdminUserDetail, error) {
	var detail models.AdminUserDetail
	err := r.db.QueryRowContext(ctx,
		"SELECT id, email, name, idp_provider, status, created_at FROM users WHERE id = $1", id,
	).Scan(&detail.ID, &detail.Email, &detail.Name, &detail.IDPProvider, &detail.Status, &detail.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin user: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, adminUserMembershipsQuery, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query user memberships: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close membership rows", "error", closeErr)
		}
	}()

	detail.Memberships = make([]models.AdminTeamMembership, 0)
	for rows.Next() {
		var m models.AdminTeamMembership
		if scanErr := rows.Scan(&m.TeamID, &m.TeamName, &m.Role); scanErr != nil {
			return nil, fmt.Errorf("failed to scan membership: %w", scanErr)
		}
		detail.Memberships = append(detail.Memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate memberships: %w", err)
	}
	return &detail, nil
}

// adminTeamListSelectColumns is the projection for the admin team listing. The
// owner join is inner (teams.owner_id -> users, ON DELETE CASCADE, so an
// existing team always has an owner). No role predicate (decision D3).
var adminTeamListSelectColumns = []string{
	"t.id", "t.name", "t.slug", "t.is_personal", "t.created_at",
	"u.id", "u.email", "u.name",
	"(SELECT COUNT(*) FROM team_members tm WHERE tm.team_id = t.id) AS member_count",
}

// adminTeamListFrom is the FROM/JOIN shared by the count and page queries, so
// both see exactly the same row set before filtering.
func adminTeamListFrom(sb squirrel.SelectBuilder) squirrel.SelectBuilder {
	return sb.From("teams t").Join("users u ON u.id = t.owner_id")
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
		where = append(where, squirrel.GtOrEq{"t.created_at": *filters.CreatedFrom})
	}
	if filters.CreatedTo != nil {
		where = append(where, squirrel.LtOrEq{"t.created_at": *filters.CreatedTo})
	}

	return where
}

// buildAdminTeamOrderBy builds the ORDER BY clause from an allowlist (the same
// SQL-injection control as buildAdminUserOrderBy). member_count is the SELECT
// alias of the correlated subquery. The t.id tie-breaker keeps paging stable.
func buildAdminTeamOrderBy(filters repositories.AdminTeamFilters) string {
	column := "t.created_at"
	switch filters.SortBy {
	case "name", "created_at":
		column = "t." + filters.SortBy
	case "member_count":
		column = "member_count"
	}
	return column + " " + adminSortDirection(filters.SortOrder) + ", t.id"
}

// ListTeams returns a page of teams matching the filters with owner and member
// count, plus the total count of the filtered set.
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
// same FROM/JOIN as the page query. The owner join is inner on a NOT NULL FK, so
// it never duplicates or drops a team row and COUNT(*) is exact.
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

// queryAdminTeams runs the paginated page query using the same WHERE conditions
// as countAdminTeams.
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

	query, args, err := sb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build admin team list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close admin team rows", "error", closeErr)
		}
	}()

	teams := make([]models.AdminTeamListItem, 0)
	for rows.Next() {
		var t models.AdminTeamListItem
		if scanErr := rows.Scan(
			&t.ID, &t.Name, &t.Slug, &t.IsPersonal, &t.CreatedAt,
			&t.Owner.ID, &t.Owner.Email, &t.Owner.Name,
			&t.MemberCount,
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

// adminTeamMembersQuery returns a team's members with the member's role (plain
// column — no role predicate, decision D3) and join time.
const adminTeamMembersQuery = `
SELECT u.id, u.email, u.name, tm.role, tm.created_at
FROM team_members tm
JOIN users u ON u.id = tm.user_id
WHERE tm.team_id = $1
ORDER BY u.name, u.id
`

// GetTeamDetail returns one team with owner and member list, or (nil, nil) when
// no team with that id exists.
func (r *AdminRepository) GetTeamDetail(
	ctx context.Context, id string,
) (*models.AdminTeamDetail, error) {
	var detail models.AdminTeamDetail
	err := r.db.QueryRowContext(ctx,
		`SELECT t.id, t.name, t.slug, t.is_personal, t.created_at, u.id, u.email, u.name
		 FROM teams t JOIN users u ON u.id = t.owner_id WHERE t.id = $1`, id,
	).Scan(
		&detail.ID, &detail.Name, &detail.Slug, &detail.IsPersonal, &detail.CreatedAt,
		&detail.Owner.ID, &detail.Owner.Email, &detail.Owner.Name,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin team: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, adminTeamMembersQuery, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query team members: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close team member rows", "error", closeErr)
		}
	}()

	detail.Members = make([]models.AdminTeamMember, 0)
	for rows.Next() {
		var m models.AdminTeamMember
		if scanErr := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.JoinedAt); scanErr != nil {
			return nil, fmt.Errorf("failed to scan team member: %w", scanErr)
		}
		detail.Members = append(detail.Members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate team members: %w", err)
	}
	return &detail, nil
}

// UpdateUserStatus sets a user's lifecycle status, reporting whether a row was
// actually matched so the caller can 404 an unknown id without a second query.
// The status value is written as a bound parameter and additionally constrained
// by the users_status_check CHECK constraint, so an unexpected value fails at
// the database rather than being stored.
func (r *AdminRepository) UpdateUserStatus(ctx context.Context, id, status string) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		"UPDATE users SET status = $1, updated_at = NOW() WHERE id = $2", status, id)
	if err != nil {
		return false, fmt.Errorf("failed to update user status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read affected rows for user status update: %w", err)
	}
	return affected > 0, nil
}
