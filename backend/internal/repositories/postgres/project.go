package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ProjectRepository implements the repositories.ProjectRepository interface for PostgreSQL
type ProjectRepository struct {
	db *database.DB
}

// NewProjectRepository creates a new ProjectRepository
func NewProjectRepository(db *database.DB) repositories.ProjectRepository {
	return &ProjectRepository{
		db: db,
	}
}

// Create creates a new project
func (r *ProjectRepository) Create(ctx context.Context, project *models.Project) error {
	query := `
		INSERT INTO projects
		(user_id, team_id, name, slug, description, git_url, homepage, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at, version
	`

	err := r.db.QueryRowContext(ctx, query,
		project.UserID, project.TeamID, project.Name, project.Slug,
		project.Description, project.GitURL, project.Homepage,
		project.CreatedAt, project.UpdatedAt,
	).Scan(&project.ID, &project.CreatedAt, &project.UpdatedAt, &project.Version)

	if err != nil {
		if pqErr := uniqueViolation(err); pqErr != nil {
			return mapProjectUniqueViolation(pqErr, project)
		}
		return fmt.Errorf("failed to create project: %w", err)
	}

	return nil
}

// mapProjectUniqueViolation translates a Postgres 23505 unique-violation on the projects
// table into the matching domain sentinel. The git_url and slug constraints are distinct
// (idx_projects_team_id_git_url_unique vs projects_slug_team_id_key), so callers can route
// a collision to the correct recovery path via errors.Is.
func mapProjectUniqueViolation(pqErr *pq.Error, project *models.Project) error {
	if strings.Contains(pqErr.Constraint, "git_url") {
		return fmt.Errorf("project with git_url '%s' already exists: %w", project.GitURL, repositories.ErrProjectGitURLExists)
	}
	return fmt.Errorf("project with slug '%s' already exists: %w", project.Slug, repositories.ErrProjectSlugExists)
}

// GetBySlug retrieves a project by user ID and slug
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ProjectRepository) GetBySlug(ctx context.Context, teamID, userID, slug string) (*models.Project, error) {
	query := `
		SELECT p.id, p.user_id, p.team_id, p.name, p.slug, p.description, p.git_url, p.homepage,
		p.created_at, p.updated_at, p.version
		FROM projects p
		WHERE p.slug = $1
			AND p.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = p.team_id AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = p.team_id AND user_id = $3)
			)
	`

	var project models.Project
	err := r.db.QueryRowContext(ctx, query, slug, teamID, userID).Scan(
		&project.ID, &project.UserID, &project.TeamID, &project.Name, &project.Slug,
		&project.Description, &project.GitURL, &project.Homepage,
		&project.CreatedAt, &project.UpdatedAt, &project.Version,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get project by slug: %w", err),
			fmt.Errorf("%w: slug=%s team=%s", repositories.ErrProjectNotFoundForRepo, slug, teamID),
		)
	}

	return &project, nil
}

// GetByGitURL retrieves a project by team ID and git_url
// Uses EXISTS subqueries to verify user authorization via team ownership or membership
func (r *ProjectRepository) GetByGitURL(ctx context.Context, teamID, userID, gitURL string) (*models.Project, error) {
	query := `
		SELECT p.id, p.user_id, p.team_id, p.name, p.slug, p.description, p.git_url, p.homepage,
		p.created_at, p.updated_at, p.version
		FROM projects p
		WHERE p.team_id = $1 AND p.git_url = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = p.team_id AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = p.team_id AND user_id = $3)
			)
	`

	var project models.Project
	err := r.db.QueryRowContext(ctx, query, teamID, gitURL, userID).Scan(
		&project.ID, &project.UserID, &project.TeamID, &project.Name, &project.Slug,
		&project.Description, &project.GitURL, &project.Homepage,
		&project.CreatedAt, &project.UpdatedAt, &project.Version,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get project by git_url: %w", err),
			fmt.Errorf("%w: git_url=%s team=%s", repositories.ErrProjectNotFoundForRepo, gitURL, teamID),
		)
	}

	return &project, nil
}

// GetByID retrieves a project by user ID and project ID
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ProjectRepository) GetByID(ctx context.Context, userID, projectID string) (*models.Project, error) {
	query := `
		SELECT p.id, p.user_id, p.team_id, p.name, p.slug, p.description, p.git_url, p.homepage,
		p.created_at, p.updated_at, p.version
		FROM projects p
		WHERE p.id = $1
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = p.team_id AND owner_id = $2)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = p.team_id AND user_id = $2)
			)
	`

	var project models.Project
	err := r.db.QueryRowContext(ctx, query, projectID, userID).Scan(
		&project.ID, &project.UserID, &project.TeamID, &project.Name, &project.Slug,
		&project.Description, &project.GitURL, &project.Homepage,
		&project.CreatedAt, &project.UpdatedAt, &project.Version,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get project by ID: %w", err),
			fmt.Errorf("%w: id=%s", repositories.ErrProjectNotFoundForRepo, projectID),
		)
	}

	return &project, nil
}

// buildProjectOrderByClause builds the ORDER BY clause for the project list
// query using an allowlist of sortable columns. Any SortBy outside the allowlist
// (including injection attempts) falls back to the default, so the value never
// reaches the query unvalidated. The switch already emits the `p.`-prefixed
// column, so the caller passes the result straight to OrderBy without a prefix.
func buildProjectOrderByClause(filters repositories.ProjectListFilters) string {
	orderBy := "p.created_at DESC"
	if filters.SortBy == "" {
		return orderBy
	}
	direction := "DESC"
	if filters.SortOrder == "asc" {
		direction = "ASC"
	}
	switch filters.SortBy {
	case "created_at", "updated_at", "name", "slug":
		return fmt.Sprintf("p.%s %s", filters.SortBy, direction)
	}
	return orderBy
}

// buildProjectListWhereClause builds the shared WHERE conditions for the project
// list query. Count and page queries consume the same conditions, so they can
// never diverge. EXISTS subqueries avoid a Cartesian product with multi-member
// teams.
func buildProjectListWhereClause(userID string, filters repositories.ProjectListFilters) squirrel.Sqlizer {
	teamID := filters.TeamID
	where := squirrel.And{
		squirrel.Eq{"p.team_id": teamID},
		teamReadAccess(teamID, userID),
	}

	if filters.Search != "" {
		searchTerm := "%" + filters.Search + "%"
		where = append(where, squirrel.Expr(
			"(p.name ILIKE ? OR p.description ILIKE ? OR p.slug ILIKE ?)",
			searchTerm, searchTerm, searchTerm,
		))
	}

	return where
}

// List retrieves projects with filtering and pagination
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ProjectRepository) List(
	ctx context.Context, userID string, filters repositories.ProjectListFilters,
) ([]models.Project, int, error) {
	// Validate required TeamID
	if filters.TeamID == "" {
		return nil, 0, fmt.Errorf("TeamID is required but was empty")
	}

	// Shared WHERE conditions guarantee the count and page queries can never diverge.
	where := buildProjectListWhereClause(userID, filters)

	totalCount, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	projects, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return projects, totalCount, nil
}

// countList counts projects matching the shared WHERE conditions used by List,
// so the count and page queries can never diverge. COUNT(*) is safe because the
// EXISTS subqueries (rather than a JOIN) eliminate multi-member duplicates.
func (r *ProjectRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("projects p").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build projects count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count projects: %w", err)
	}

	return totalCount, nil
}

// queryList runs the paginated page query for List using the shared WHERE
// conditions, so it can never diverge from the count query.
func (r *ProjectRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.ProjectListFilters,
) ([]models.Project, error) {
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

	query, args, err := psql.
		Select(
			"p.id", "p.user_id", "p.team_id", "p.name", "p.slug", "p.description",
			"p.git_url", "p.homepage", "p.created_at", "p.updated_at", "p.version",
		).
		From("projects p").
		Where(where).
		OrderBy(buildProjectOrderByClause(filters)).
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build projects list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	projects := make([]models.Project, 0)
	for rows.Next() {
		var project models.Project
		scanErr := rows.Scan(
			&project.ID, &project.UserID, &project.TeamID, &project.Name, &project.Slug,
			&project.Description, &project.GitURL, &project.Homepage,
			&project.CreatedAt, &project.UpdatedAt, &project.Version,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan project: %w", scanErr)
		}
		projects = append(projects, project)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate projects: %w", err)
	}

	return projects, nil
}

// Update updates an existing project with optimistic locking
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ProjectRepository) Update(ctx context.Context, project *models.Project) error {
	// Validate team membership before update
	var exists bool
	ownershipQuery := `
		SELECT EXISTS(
			SELECT 1 FROM projects p
			WHERE p.id = $1
				AND (
					EXISTS (SELECT 1 FROM teams WHERE id = p.team_id AND owner_id = $2)
					OR EXISTS (SELECT 1 FROM team_members WHERE team_id = p.team_id AND user_id = $2)
				)
		)
	`
	err := r.db.QueryRowContext(ctx, ownershipQuery, project.ID, project.UserID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to validate project ownership: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: id=%s", repositories.ErrProjectNotFoundForRepo, project.ID)
	}

	// Use EXISTS subqueries to avoid Cartesian product with multi-member teams.
	// The team_members EXISTS qualifies the outer column as projects.team_id so
	// the membership re-check correlates to *this* project's team (team_members
	// has its own team_id column, so an unqualified reference would self-compare
	// and always be true). The teams EXISTS needs no qualifier: teams has no
	// team_id column, so its unqualified team_id resolves to the outer row.
	query := `
		UPDATE projects
		SET name = $2, slug = $3, description = $4, git_url = $5, homepage = $6,
		updated_at = $7, version = version + 1
		WHERE id = $1
			AND version = $8
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = team_id AND owner_id = $9)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = projects.team_id AND user_id = $9)
			)
		RETURNING updated_at, version
	`

	err = r.db.QueryRowContext(ctx, query,
		project.ID, project.Name, project.Slug,
		project.Description, project.GitURL, project.Homepage,
		project.UpdatedAt, project.Version, project.UserID,
	).Scan(&project.UpdatedAt, &project.Version)

	if err != nil {
		if pqErr := uniqueViolation(err); pqErr != nil {
			return mapProjectUniqueViolation(pqErr, project)
		}
		return mapNoRows(
			fmt.Errorf("failed to update project: %w", err),
			fmt.Errorf("%w: id=%s (version mismatch)", repositories.ErrProjectNotFoundForRepo, project.ID),
		)
	}

	return nil
}

// Delete deletes a project by team ID, user ID, and slug
// Allows deletion if user is: resource owner, team owner, or team admin
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ProjectRepository) Delete(ctx context.Context, teamID, userID, slug string) error {
	query := `
		DELETE FROM projects
		WHERE slug = $1
			AND team_id = $2
			AND (
				user_id = $3
				OR EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3 AND role IN ('owner', 'admin'))
			)
	`

	result, err := r.db.ExecContext(ctx, query, slug, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%w: slug=%s team=%s", repositories.ErrProjectNotFoundForRepo, slug, teamID)
	}

	return nil
}

// CountByTeamID counts the total number of projects for a team
func (r *ProjectRepository) CountByTeamID(ctx context.Context, teamID string) (int, error) {
	query := `SELECT COUNT(*) FROM projects WHERE team_id = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count projects: %w", err)
	}

	return count, nil
}

// GetNamesByIDs returns a map of projectID → name for the given IDs visible to userID
// across all teams the user belongs to (owner or member).
// Unknown or inaccessible IDs are omitted from the result map.
func (r *ProjectRepository) GetNamesByIDs(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	// $1 is userID; project IDs start at $2
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = userID
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(
		`SELECT p.id, p.name FROM projects p
		WHERE p.id IN (%s)
			AND (
				EXISTS (SELECT 1 FROM teams t WHERE t.id = p.team_id AND t.owner_id = $1)
				OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = p.team_id AND tm.user_id = $1)
			)`,
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get project names by ids: %w", err)
	}

	result, scanErr := scanIDNameRows(rows, len(ids), "project")
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close project name rows: %w", closeErr)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}

// GetProjectStats returns resource counts for the project identified by teamID + slug.
// A single query first resolves the project ID (authorising via team ownership or membership)
// then counts rows in all five resource tables with a matching project_id.
func (r *ProjectRepository) GetProjectStats(
	ctx context.Context, teamID, userID, projectSlug string,
) (*models.ProjectStatsResponse, error) {
	query := `
		WITH proj AS (
			SELECT p.id
			FROM projects p
			WHERE p.slug = $1
				AND p.team_id = $2
				AND (
					EXISTS (SELECT 1 FROM teams WHERE id = p.team_id AND owner_id = $3)
					OR EXISTS (SELECT 1 FROM team_members WHERE team_id = p.team_id AND user_id = $3)
				)
		)
		SELECT
			(SELECT COUNT(*) FROM prompts    WHERE project_id = (SELECT id FROM proj)),
			(SELECT COUNT(*) FROM artifacts  WHERE project_id = (SELECT id FROM proj)),
			(SELECT COUNT(*) FROM blueprints WHERE project_id = (SELECT id FROM proj)),
			(SELECT COUNT(*) FROM memories   WHERE project_id = (SELECT id FROM proj)),
			(SELECT COUNT(*) FROM feed_items WHERE project_id = (SELECT id FROM proj))
		WHERE EXISTS (SELECT 1 FROM proj)
	`

	var stats models.ProjectStatsResponse
	err := r.db.QueryRowContext(ctx, query, projectSlug, teamID, userID).Scan(
		&stats.TotalPrompts,
		&stats.TotalArtifacts,
		&stats.TotalBlueprints,
		&stats.TotalMemories,
		&stats.TotalFeedItems,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get project stats: %w", err),
			fmt.Errorf("%w: slug=%s team=%s", repositories.ErrProjectNotFoundForRepo, projectSlug, teamID),
		)
	}

	return &stats, nil
}

// GetProjectResourceCreationMetrics returns sparse per-day creation counts per
// resource type (prompts, artifacts, blueprints, memories) for the project
// identified by teamID + slug, counting rows created at or after `since`. Days
// with no creations for a type are omitted — the caller zero-fills them into a
// continuous daily series. Returns ErrProjectNotFoundForRepo when the project
// does not exist or is not accessible to userID.
func (r *ProjectRepository) GetProjectResourceCreationMetrics(
	ctx context.Context, teamID, userID, projectSlug string, since time.Time,
) ([]models.ProjectResourceCreationCount, error) {
	// Resolve and authorize the project first so an unknown or inaccessible
	// project is a 404 rather than an empty (but 200) series — the aggregate
	// query below would otherwise match zero rows for project_id = NULL. Mirrors
	// the owner/member auth CTE used by GetProjectStats.
	var projectID string
	err := r.db.QueryRowContext(ctx, `
		SELECT p.id
		FROM projects p
		WHERE p.slug = $1
			AND p.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = p.team_id AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = p.team_id AND user_id = $3)
			)
	`, projectSlug, teamID, userID).Scan(&projectID)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to resolve project for creation metrics: %w", err),
			fmt.Errorf("%w: slug=%s team=%s", repositories.ErrProjectNotFoundForRepo, projectSlug, teamID),
		)
	}

	// TO_CHAR(DATE(created_at)) keys match the zero-fill series keys the handler
	// builds. memories.created_at is a plain TIMESTAMP and the others are
	// TIMESTAMPTZ; DATE() buckets both in the server's (UTC) timezone.
	const query = `
		SELECT TO_CHAR(DATE(created_at), 'YYYY-MM-DD') AS date, 'prompts' AS resource_type, COUNT(*) AS count
		FROM prompts WHERE project_id = $1 AND created_at >= $2 GROUP BY DATE(created_at)
		UNION ALL
		SELECT TO_CHAR(DATE(created_at), 'YYYY-MM-DD'), 'artifacts', COUNT(*)
		FROM artifacts WHERE project_id = $1 AND created_at >= $2 GROUP BY DATE(created_at)
		UNION ALL
		SELECT TO_CHAR(DATE(created_at), 'YYYY-MM-DD'), 'blueprints', COUNT(*)
		FROM blueprints WHERE project_id = $1 AND created_at >= $2 GROUP BY DATE(created_at)
		UNION ALL
		SELECT TO_CHAR(DATE(created_at), 'YYYY-MM-DD'), 'memories', COUNT(*)
		FROM memories WHERE project_id = $1 AND created_at >= $2 GROUP BY DATE(created_at)
		ORDER BY date, resource_type`

	rows, err := r.db.QueryContext(ctx, query, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query project resource creation metrics: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	counts := []models.ProjectResourceCreationCount{}
	for rows.Next() {
		var c models.ProjectResourceCreationCount
		if scanErr := rows.Scan(&c.Date, &c.ResourceType, &c.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan project resource creation metric: %w", scanErr)
		}
		counts = append(counts, c)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("failed to iterate project resource creation metrics: %w", rowsErr)
	}

	return counts, nil
}

// listGitURLToSlugQuery selects the (git_url, slug) tuples for every project in a team
// whose git_url is non-empty and that the requesting user can access via team ownership
// or membership. The EXISTS clauses match the scoping used by GetByGitURL/GetBySlug so
// the enrichment honours the same authorisation boundary as direct project lookups.
const listGitURLToSlugQuery = `
	SELECT p.git_url, p.slug
	FROM projects p
	WHERE p.team_id = $1
		AND p.git_url <> ''
		AND (
			EXISTS (SELECT 1 FROM teams WHERE id = p.team_id AND owner_id = $2)
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = p.team_id AND user_id = $2)
		)
`

// ListGitURLToSlugByTeam returns a map of git_url → slug for every project in teamID
// that has a non-empty git_url and is visible to userID. Used by the GitHub
// integration listing endpoint to enrich each repository with the slug of an
// already-imported project (if any).
func (r *ProjectRepository) ListGitURLToSlugByTeam(
	ctx context.Context, teamID, userID string,
) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, listGitURLToSlugQuery, teamID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list git_url->slug for team: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	result := make(map[string]string)
	for rows.Next() {
		var gitURL, slug string
		if scanErr := rows.Scan(&gitURL, &slug); scanErr != nil {
			return nil, fmt.Errorf("failed to scan git_url->slug row: %w", scanErr)
		}
		result[gitURL] = slug
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate git_url->slug rows: %w", err)
	}
	return result, nil
}

// scanIDNameRows iterates rows yielding (id, name) pairs into a map.
// label is used in error messages, e.g. "project" -> "scan project name row: ..."
func scanIDNameRows(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
}, capacity int, label string) (map[string]string, error) {
	result := make(map[string]string, capacity)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan %s name row: %w", label, err)
		}
		result[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s name rows: %w", label, err)
	}
	return result, nil
}
