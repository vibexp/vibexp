package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// promptListSelectColumns is the 13-column projection shared by both variants
// of the prompt list SELECT clause; each variant appends its own is_shared
// expression (see buildListSelectClause).
var promptListSelectColumns = []string{
	"p.id", "p.name", "p.slug", "p.description", "p.body", "p.user_id", "p.team_id",
	"p.project_id", "p.status", "p.mcp_expose", "p.labels", "p.created_at", "p.updated_at",
}

// PromptRepository implements the repositories.PromptRepository interface for PostgreSQL
type PromptRepository struct {
	db *database.DB
}

// NewPromptRepository creates a new PromptRepository
func NewPromptRepository(db *database.DB) repositories.PromptRepository {
	return &PromptRepository{
		db: db,
	}
}

// Create creates a new prompt
func (r *PromptRepository) Create(ctx context.Context, prompt *models.Prompt) error {
	query := `
		INSERT INTO prompts (name, slug, description, body, user_id, team_id, project_id,
			status, mcp_expose, labels, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		prompt.Name, prompt.Slug, prompt.Description, prompt.Body, prompt.UserID,
		prompt.TeamID, prompt.ProjectID, prompt.Status, prompt.MCPExpose, pq.Array(prompt.Labels),
		prompt.CreatedAt, prompt.UpdatedAt,
	).Scan(&prompt.ID, &prompt.CreatedAt, &prompt.UpdatedAt)

	if err != nil {
		if pqErr := uniqueViolation(err); pqErr != nil && strings.Contains(pqErr.Detail, "slug") {
			return fmt.Errorf("prompt with slug '%s' already exists for this user", prompt.Slug)
		}
		return fmt.Errorf("failed to create prompt: %w", err)
	}

	return nil
}

// GetByID retrieves a prompt by its ID
func (r *PromptRepository) GetByID(ctx context.Context, userID, teamID, promptID string) (*models.Prompt, error) {
	var query string
	var args []interface{}

	if userID == "" {
		// For shared prompts, retrieve without user check
		query = `
			SELECT
				p.id, p.name, p.slug, p.description, p.body, p.user_id, p.team_id, p.project_id,
				p.status, p.mcp_expose, p.labels, p.created_at, p.updated_at, p.version,
				CASE WHEN ps.id IS NOT NULL THEN true ELSE false END as is_shared
			FROM prompts p
			LEFT JOIN prompt_shares ps ON p.id = ps.prompt_id
				AND ps.is_active = true
				AND (ps.expires_at IS NULL OR ps.expires_at > NOW())
			WHERE p.id = $1
		`
		args = []interface{}{promptID}
	} else {
		// Normal case: check team membership (user is team owner OR member)
		// Use EXISTS subqueries to avoid Cartesian product with multi-member teams
		query = `
			SELECT
				p.id, p.name, p.slug, p.description, p.body, p.user_id, p.team_id, p.project_id,
				p.status, p.mcp_expose, p.labels, p.created_at, p.updated_at, p.version,
				CASE WHEN ps.id IS NOT NULL THEN true ELSE false END as is_shared
			FROM prompts p
			LEFT JOIN prompt_shares ps ON p.id = ps.prompt_id
				AND ps.is_active = true
				AND (ps.expires_at IS NULL OR ps.expires_at > NOW())
			WHERE p.id = $1
				AND p.team_id = $2
				AND (
					EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
					OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
				)
		`
		args = []interface{}{promptID, teamID, userID}
	}

	var prompt models.Prompt
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&prompt.ID, &prompt.Name, &prompt.Slug, &prompt.Description, &prompt.Body,
		&prompt.UserID, &prompt.TeamID, &prompt.ProjectID, &prompt.Status, &prompt.MCPExpose,
		&prompt.Labels, &prompt.CreatedAt, &prompt.UpdatedAt, &prompt.Version, &prompt.IsShared,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get prompt by ID: %w", err), repositories.ErrPromptNotFound)
	}

	return &prompt, nil
}

// GetBySlug retrieves a prompt by its slug
func (r *PromptRepository) GetBySlug(ctx context.Context, userID, teamID, slug string) (*models.Prompt, error) {
	// Use EXISTS subqueries to avoid Cartesian product with multi-member teams
	query := `
		SELECT
			p.id, p.name, p.slug, p.description, p.body, p.user_id, p.team_id, p.project_id,
			p.status, p.mcp_expose, p.labels, p.created_at, p.updated_at,
			p.version, CASE WHEN ps.id IS NOT NULL THEN true ELSE false END as is_shared
		FROM prompts p
		LEFT JOIN prompt_shares ps ON p.id = ps.prompt_id
			AND ps.is_active = true
			AND (ps.expires_at IS NULL OR ps.expires_at > NOW())
		WHERE p.slug = $1
			AND p.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	var prompt models.Prompt
	err := r.db.QueryRowContext(ctx, query, slug, teamID, userID).Scan(
		&prompt.ID, &prompt.Name, &prompt.Slug, &prompt.Description, &prompt.Body,
		&prompt.UserID, &prompt.TeamID, &prompt.ProjectID, &prompt.Status, &prompt.MCPExpose,
		&prompt.Labels, &prompt.CreatedAt, &prompt.UpdatedAt, &prompt.Version, &prompt.IsShared,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get prompt by slug: %w", err), repositories.ErrPromptNotFound)
	}

	return &prompt, nil
}

// GetByIDCrossTeam retrieves a prompt by ID across all user's teams
func (r *PromptRepository) GetByIDCrossTeam(ctx context.Context, userID, promptID string) (*models.Prompt, error) {
	query := `
		SELECT
			p.id, p.name, p.slug, p.description, p.body, p.user_id, p.team_id, p.project_id,
			p.status, p.mcp_expose, p.labels, p.created_at, p.updated_at, p.version,
			CASE WHEN ps.id IS NOT NULL THEN true ELSE false END as is_shared
		FROM prompts p
		LEFT JOIN prompt_shares ps ON p.id = ps.prompt_id
			AND ps.is_active = true
			AND (ps.expires_at IS NULL OR ps.expires_at > NOW())
		WHERE p.id = $1 AND p.user_id = $2
	`

	var prompt models.Prompt
	err := r.db.QueryRowContext(ctx, query, promptID, userID).Scan(
		&prompt.ID, &prompt.Name, &prompt.Slug, &prompt.Description, &prompt.Body,
		&prompt.UserID, &prompt.TeamID, &prompt.ProjectID, &prompt.Status, &prompt.MCPExpose,
		&prompt.Labels, &prompt.CreatedAt, &prompt.UpdatedAt, &prompt.Version, &prompt.IsShared,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get prompt by ID (cross-team): %w", err), repositories.ErrPromptNotFound)
	}

	return &prompt, nil
}

// GetBySlugCrossTeam retrieves a prompt by slug across all user's teams
func (r *PromptRepository) GetBySlugCrossTeam(ctx context.Context, userID, slug string) (*models.Prompt, error) {
	query := `
		SELECT
			p.id, p.name, p.slug, p.description, p.body, p.user_id, p.team_id, p.project_id,
			p.status, p.mcp_expose, p.labels, p.created_at, p.updated_at, p.version,
			CASE WHEN ps.id IS NOT NULL THEN true ELSE false END as is_shared
		FROM prompts p
		LEFT JOIN prompt_shares ps ON p.id = ps.prompt_id
			AND ps.is_active = true
			AND (ps.expires_at IS NULL OR ps.expires_at > NOW())
		WHERE p.slug = $1 AND p.user_id = $2
	`

	var prompt models.Prompt
	err := r.db.QueryRowContext(ctx, query, slug, userID).Scan(
		&prompt.ID, &prompt.Name, &prompt.Slug, &prompt.Description, &prompt.Body,
		&prompt.UserID, &prompt.TeamID, &prompt.ProjectID, &prompt.Status, &prompt.MCPExpose,
		&prompt.Labels, &prompt.CreatedAt, &prompt.UpdatedAt, &prompt.Version, &prompt.IsShared,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get prompt by slug (cross-team): %w", err),
			repositories.ErrPromptNotFound,
		)
	}

	return &prompt, nil
}

// buildListFromClause applies the FROM/JOIN for the prompt list query to a
// squirrel select builder. Team membership is checked via EXISTS subqueries in
// the WHERE clause, so the base FROM is just prompts; the prompt_shares LEFT
// JOIN is added only when filtering by IsShared.
func buildListFromClause(
	sb squirrel.SelectBuilder, filters repositories.PromptFilters,
) squirrel.SelectBuilder {
	sb = sb.From("prompts p")
	if filters.IsShared != nil {
		sb = sb.LeftJoin("prompt_shares ps ON p.id = ps.prompt_id " +
			"AND ps.is_active = true " +
			"AND (ps.expires_at IS NULL OR ps.expires_at > NOW())")
	}
	return sb
}

// buildListWhereClause builds the shared WHERE conditions for the prompt list
// query. Count and page queries consume the same conditions, so they can never
// diverge. EXISTS subqueries avoid a Cartesian product with multi-member teams.
func buildListWhereClause(userID string, filters repositories.PromptFilters) squirrel.Sqlizer {
	teamID := filters.TeamID
	where := squirrel.And{
		squirrel.Eq{"p.team_id": teamID},
		teamReadAccess(teamID, userID),
	}

	if filters.Status != "" {
		where = append(where, squirrel.Eq{"p.status": filters.Status})
	}

	if filters.MCPExpose != nil {
		where = append(where, squirrel.Eq{"p.mcp_expose": *filters.MCPExpose})
	}

	if filters.IsShared != nil {
		if *filters.IsShared {
			where = append(where, squirrel.Expr("ps.id IS NOT NULL"))
		} else {
			where = append(where, squirrel.Expr("ps.id IS NULL"))
		}
	}

	if filters.Search != "" {
		term := "%" + filters.Search + "%"
		where = append(where, squirrel.Expr(
			"(p.name ILIKE ? OR p.description ILIKE ? OR p.body ILIKE ?)",
			term, term, term,
		))
	}

	if len(filters.Labels) > 0 {
		where = append(where, squirrel.Expr("p.labels @> ?", pq.Array(filters.Labels)))
	}

	if filters.ProjectID != nil && *filters.ProjectID != "" {
		where = append(where, squirrel.Eq{"p.project_id": *filters.ProjectID})
	}

	return where
}

// buildListSelectClause returns the SELECT projection columns for the prompt
// list query. DISTINCT (applied separately) is needed only when filtering by
// IsShared due to the prompt_shares LEFT JOIN, which can otherwise duplicate
// rows; the is_shared column is then computed from the join. Without the join,
// EXISTS subqueries eliminate team_members duplicates and is_shared is computed
// via a correlated EXISTS subquery.
func buildListSelectClause(filters repositories.PromptFilters) []string {
	isShared := "EXISTS(SELECT 1 FROM prompt_shares ps2 WHERE ps2.prompt_id = p.id AND ps2.is_active = true " +
		"AND (ps2.expires_at IS NULL OR ps2.expires_at > NOW())) as is_shared"
	if filters.IsShared != nil {
		isShared = "CASE WHEN ps.id IS NOT NULL THEN true ELSE false END as is_shared"
	}
	return append(append([]string{}, promptListSelectColumns...), isShared)
}

// buildListOrderByClause builds the ORDER BY clause for prompt list query using allowlisted fields.
// This is an SQL-injection control: only allowlisted column names reach the query.
func buildListOrderByClause(filters repositories.PromptFilters) string {
	orderBy := "p.updated_at DESC"
	if filters.SortBy != "" {
		direction := "DESC"
		if filters.SortOrder == "asc" {
			direction = "ASC"
		}
		switch filters.SortBy {
		case "name", "status", "updated_at", "created_at":
			orderBy = fmt.Sprintf("p.%s %s", filters.SortBy, direction)
		}
	}
	return orderBy
}

// List retrieves prompts with filtering and pagination
func (r *PromptRepository) List(
	ctx context.Context, userID string, filters repositories.PromptFilters,
) ([]models.Prompt, int, error) {
	// Validate required TeamID
	if filters.TeamID == "" {
		return nil, 0, fmt.Errorf("TeamID is required but was empty")
	}

	// Shared WHERE conditions guarantee the count and page queries can never diverge.
	where := buildListWhereClause(userID, filters)

	totalCount, err := r.countList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	prompts, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return prompts, totalCount, nil
}

// countList counts prompts matching the shared WHERE conditions used by List,
// so the count and page queries can never diverge. COUNT(DISTINCT p.id) is used
// when filtering by IsShared because the prompt_shares LEFT JOIN can duplicate
// rows; otherwise the EXISTS subqueries make COUNT(*) safe.
func (r *PromptRepository) countList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.PromptFilters,
) (int, error) {
	countColumn := "COUNT(*)"
	if filters.IsShared != nil {
		countColumn = "COUNT(DISTINCT p.id)"
	}

	sb := buildListFromClause(psql.Select(countColumn), filters).Where(where)
	query, args, err := sb.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build prompts count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count prompts: %w", err)
	}

	return totalCount, nil
}

// queryList runs the paginated page query for List using the shared WHERE
// conditions, so it can never diverge from the count query.
func (r *PromptRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.PromptFilters,
) ([]models.Prompt, error) {
	// Clamp to non-negative before the unsigned conversion squirrel requires;
	// negative paging inputs would otherwise wrap to huge offsets/limits.
	// Contract: a non-positive Limit emits LIMIT 0 (empty page) — identical to
	// the previous hand-built query. Callers must pass Limit >= 1 and Page >= 1.
	limit := uint64(0)
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	// Only derive an offset from valid paging inputs: two negative factors
	// would otherwise multiply into a bogus positive offset.
	offset := uint64(0)
	if filters.Page > 1 && filters.Limit > 0 {
		if rawOffset := (filters.Page - 1) * filters.Limit; rawOffset > 0 {
			offset = uint64(rawOffset)
		}
	}

	sb := psql.Select(buildListSelectClause(filters)...)
	if filters.IsShared != nil {
		// DISTINCT needed due to prompt_shares LEFT JOIN which can produce duplicates.
		sb = sb.Distinct()
	}
	sb = buildListFromClause(sb, filters).
		Where(where).
		OrderBy(buildListOrderByClause(filters)).
		Limit(limit).
		Offset(offset)

	query, args, err := sb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build prompts list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list prompts: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	prompts := make([]models.Prompt, 0)
	for rows.Next() {
		var prompt models.Prompt
		scanErr := rows.Scan(
			&prompt.ID, &prompt.Name, &prompt.Slug, &prompt.Description,
			&prompt.Body, &prompt.UserID, &prompt.TeamID, &prompt.ProjectID, &prompt.Status, &prompt.MCPExpose, &prompt.Labels,
			&prompt.CreatedAt, &prompt.UpdatedAt, &prompt.IsShared,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan prompt: %w", scanErr)
		}
		prompts = append(prompts, prompt)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate prompts: %w", err)
	}

	return prompts, nil
}

// validatePromptOwnerOrAdmin checks if user can modify the prompt (owner, team owner, or team admin)
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
// validatePromptInTeam proves the prompt exists within the team. It is a
// TENANCY check only (epic #220 decision D3): whether the caller's ROLE permits
// the update is decided by PromptService via the authz matrix before this is
// reached. It is kept because the caller relies on ErrPromptNotFound to tell a
// missing prompt apart from an optimistic-lock version conflict.
func (r *PromptRepository) validatePromptInTeam(ctx context.Context, promptID, teamID string) error {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM prompts p WHERE p.id = $1 AND p.team_id = $2)`
	if err := r.db.QueryRowContext(ctx, query, promptID, teamID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to validate prompt: %w", err)
	}
	if !exists {
		return repositories.ErrPromptNotFound
	}
	return nil
}

// Update updates an existing prompt
func (r *PromptRepository) Update(ctx context.Context, prompt *models.Prompt) error {
	// Prove the prompt exists in the team before checking version, so a missing
	// prompt is not reported as a version conflict. Role is NOT checked here —
	// PromptService authorized the caller already.
	if err := r.validatePromptInTeam(ctx, prompt.ID, prompt.TeamID); err != nil {
		return err
	}

	// Use EXISTS subqueries to avoid multiple row matches from JOIN
	query := `UPDATE prompts
		SET name = $2, slug = $3, description = $4, body = $5, project_id = $6,
		status = $7, mcp_expose = $8, labels = $9, team_id = $10, updated_at = $11, version = version + 1
		WHERE id = $1 AND team_id = $12 AND version = $13
		RETURNING updated_at, version`

	err := r.db.QueryRowContext(ctx, query,
		prompt.ID, prompt.Name, prompt.Slug, prompt.Description, prompt.Body,
		prompt.ProjectID, prompt.Status, prompt.MCPExpose, pq.Array(prompt.Labels),
		prompt.TeamID, prompt.UpdatedAt, prompt.TeamID, prompt.Version,
	).Scan(&prompt.UpdatedAt, &prompt.Version)

	if err != nil {
		if pqErr := uniqueViolation(err); pqErr != nil && strings.Contains(pqErr.Detail, "slug") {
			return fmt.Errorf("prompt with slug '%s' already exists for this user", prompt.Slug)
		}
		return mapNoRows(
			fmt.Errorf("failed to update prompt: %w", err),
			fmt.Errorf("version conflict: resource was modified by another request"),
		)
	}
	return nil
}

// Delete deletes a prompt
// Allows deletion if user is: resource owner, team owner, or team admin
func (r *PromptRepository) Delete(ctx context.Context, userID, teamID, promptID string) error {
	// Use EXISTS subqueries to avoid multiple row matches from JOIN
	query := `
		DELETE FROM prompts
		WHERE id = $1
			AND team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	result, err := r.db.ExecContext(ctx, query, promptID, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete prompt: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrPromptNotFound
	}

	return nil
}

// CountByStatus counts prompts by status for a user
func (r *PromptRepository) CountByStatus(ctx context.Context, userID, status string) (int, error) {
	query := `SELECT COUNT(*) FROM prompts WHERE user_id = $1 AND status = $2`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count prompts by status: %w", err)
	}

	return count, nil
}

// GetUserLabels retrieves all distinct labels used by a user's prompts
func (r *PromptRepository) GetUserLabels(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT DISTINCT unnest(labels) as label
		FROM prompts
		WHERE user_id = $1 AND labels IS NOT NULL AND array_length(labels, 1) > 0
		ORDER BY label
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user labels: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	labels := make([]string, 0)
	for rows.Next() {
		var label string
		if scanErr := rows.Scan(&label); scanErr != nil {
			return nil, fmt.Errorf("failed to scan label: %w", scanErr)
		}
		labels = append(labels, label)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate labels: %w", err)
	}

	return labels, nil
}

// GetNamesByIDsCrossTeam returns a map of promptID → name for the given IDs visible to userID
// across all teams the user belongs to (owner or member).
// Unknown or inaccessible IDs are omitted from the result map.
func (r *PromptRepository) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	// $1 is userID; prompt IDs start at $2
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = userID
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(
		`SELECT pr.id, pr.name FROM prompts pr
		WHERE pr.id IN (%s)
			AND (
				EXISTS (SELECT 1 FROM teams t WHERE t.id = pr.team_id AND t.owner_id = $1)
				OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = pr.team_id AND tm.user_id = $1)
			)`,
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get prompt names by ids: %w", err)
	}

	result, scanErr := scanIDNameRows(rows, len(ids), "prompt")
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close prompt name rows: %w", closeErr)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}
