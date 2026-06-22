package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// artifactListColumns is the 12-column projection shared by List and
// ListCrossTeam. The content column is deliberately excluded from list
// operations to keep payloads small.
var artifactListColumns = []string{
	"a.id", "a.project_id", "a.slug", "a.user_id", "a.team_id",
	"a.title", "a.description", "a.status", "a.type", "a.metadata",
	"a.created_at", "a.updated_at",
}

// ArtifactRepository implements the repositories.ArtifactRepository interface for PostgreSQL
type ArtifactRepository struct {
	db *database.DB
}

// NewArtifactRepository creates a new ArtifactRepository
func NewArtifactRepository(db *database.DB) repositories.ArtifactRepository {
	return &ArtifactRepository{
		db: db,
	}
}

// Create creates a new artifact
func (r *ArtifactRepository) Create(ctx context.Context, artifact *models.Artifact) error {
	metadataJSON, err := json.Marshal(artifact.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO artifacts
		(project_id, slug, user_id, team_id, title, description, content, status, type, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`

	err = r.db.QueryRowContext(ctx, query,
		artifact.ProjectID, artifact.Slug, artifact.UserID, artifact.TeamID,
		artifact.Title, artifact.Description, artifact.Content,
		artifact.Status, artifact.Type, metadataJSON,
		artifact.CreatedAt, artifact.UpdatedAt,
	).Scan(&artifact.ID, &artifact.CreatedAt, &artifact.UpdatedAt)

	if err != nil {
		if uniqueViolation(err) != nil {
			return fmt.Errorf("artifact with slug '%s' already exists for this project", artifact.Slug)
		}
		if isFKViolation(err) {
			return fmt.Errorf("project not found or does not belong to user")
		}
		return fmt.Errorf("failed to create artifact: %w", err)
	}

	return nil
}

// GetByID retrieves an artifact by its ID
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ArtifactRepository) GetByID(ctx context.Context, userID, teamID, artifactID string) (*models.Artifact, error) {
	query := `
		SELECT a.id, a.project_id, a.slug, a.user_id, a.team_id, a.title, a.description, a.content,
		a.status, a.type, a.metadata, a.created_at, a.updated_at, a.version
		FROM artifacts a
		WHERE a.id = $1
			AND a.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	var artifact models.Artifact
	var metadataJSON []byte
	err := r.db.QueryRowContext(ctx, query, artifactID, teamID, userID).Scan(
		&artifact.ID, &artifact.ProjectID, &artifact.Slug,
		&artifact.UserID, &artifact.TeamID, &artifact.Title, &artifact.Description,
		&artifact.Content, &artifact.Status, &artifact.Type,
		&metadataJSON, &artifact.CreatedAt, &artifact.UpdatedAt, &artifact.Version,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get artifact by ID: %w", err), repositories.ErrArtifactNotFound)
	}

	if err := json.Unmarshal(metadataJSON, &artifact.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &artifact, nil
}

// GetByProjectIDAndSlug retrieves an artifact by project ID and slug
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ArtifactRepository) GetByProjectIDAndSlug(
	ctx context.Context, userID, teamID, projectID, slug string,
) (*models.Artifact, error) {
	query := `
		SELECT a.id, a.project_id, a.slug, a.user_id, a.team_id, a.title, a.description, a.content,
		a.status, a.type, a.metadata, a.created_at, a.updated_at, a.version
		FROM artifacts a
		WHERE a.project_id = $1
			AND a.slug = $2
			AND a.team_id = $3
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $3 AND owner_id = $4)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $3 AND user_id = $4)
			)
	`

	var artifact models.Artifact
	var metadataJSON []byte
	err := r.db.QueryRowContext(ctx, query, projectID, slug, teamID, userID).Scan(
		&artifact.ID, &artifact.ProjectID, &artifact.Slug,
		&artifact.UserID, &artifact.TeamID, &artifact.Title, &artifact.Description,
		&artifact.Content, &artifact.Status, &artifact.Type,
		&metadataJSON, &artifact.CreatedAt, &artifact.UpdatedAt, &artifact.Version,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get artifact by project and slug: %w", err),
			repositories.ErrArtifactNotFound,
		)
	}

	if err := json.Unmarshal(metadataJSON, &artifact.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &artifact, nil
}

// GetByIDCrossTeam retrieves an artifact by ID across all user's teams
func (r *ArtifactRepository) GetByIDCrossTeam(
	ctx context.Context, userID, artifactID string,
) (*models.Artifact, error) {
	query := `
		SELECT id, project_id, slug, user_id, team_id, title, description, content, status, type, metadata,
		created_at, updated_at, version
		FROM artifacts
		WHERE id = $1 AND user_id = $2
	`

	var artifact models.Artifact
	var metadataJSON []byte
	err := r.db.QueryRowContext(ctx, query, artifactID, userID).Scan(
		&artifact.ID, &artifact.ProjectID, &artifact.Slug,
		&artifact.UserID, &artifact.TeamID, &artifact.Title, &artifact.Description,
		&artifact.Content, &artifact.Status, &artifact.Type,
		&metadataJSON, &artifact.CreatedAt, &artifact.UpdatedAt, &artifact.Version,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get artifact by ID (cross-team): %w", err),
			repositories.ErrArtifactNotFound,
		)
	}

	if err := json.Unmarshal(metadataJSON, &artifact.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &artifact, nil
}

// GetByProjectIDAndSlugCrossTeam retrieves an artifact by project ID and slug across all user's teams
func (r *ArtifactRepository) GetByProjectIDAndSlugCrossTeam(
	ctx context.Context, userID, projectID, slug string,
) (*models.Artifact, error) {
	query := `
		SELECT id, project_id, slug, user_id, team_id, title, description, content, status, type, metadata,
		created_at, updated_at, version
		FROM artifacts
		WHERE project_id = $1 AND slug = $2 AND user_id = $3
	`

	var artifact models.Artifact
	var metadataJSON []byte
	err := r.db.QueryRowContext(ctx, query, projectID, slug, userID).Scan(
		&artifact.ID, &artifact.ProjectID, &artifact.Slug,
		&artifact.UserID, &artifact.TeamID, &artifact.Title, &artifact.Description,
		&artifact.Content, &artifact.Status, &artifact.Type,
		&metadataJSON, &artifact.CreatedAt, &artifact.UpdatedAt, &artifact.Version,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get artifact by project and slug (cross-team): %w", err),
			repositories.ErrArtifactNotFound,
		)
	}

	if err := json.Unmarshal(metadataJSON, &artifact.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &artifact, nil
}

// buildArtifactOrderByClause builds the ORDER BY clause for the artifact list
// queries using an allowlist of sortable columns. Any value outside the
// allowlist (including injection attempts) falls back to the default. The
// returned string is always `a.`-prefixed, matching the historical template.
func buildArtifactOrderByClause(filters repositories.ArtifactFilters) string {
	orderBy := "a.created_at DESC"
	if filters.SortBy == "" {
		return orderBy
	}
	direction := "DESC"
	if filters.SortOrder == "asc" {
		direction = "ASC"
	}
	switch filters.SortBy {
	case "created_at", "updated_at":
		return fmt.Sprintf("a.%s %s", filters.SortBy, direction)
	}
	return orderBy
}

// applyArtifactFilters appends the optional filter conditions shared by List
// and ListCrossTeam to the WHERE clause. The conditions and their triggers are
// identical for both methods; only the base WHERE differs. Metadata keys are
// validated against the allowlist and iterated in sorted order so multi-key
// filters produce a deterministic, parameter-bound query.
func applyArtifactFilters(where squirrel.And, filters repositories.ArtifactFilters) (squirrel.And, error) {
	if filters.ProjectID != nil && *filters.ProjectID != "" {
		where = append(where, squirrel.Eq{"a.project_id": *filters.ProjectID})
	}

	if filters.Type != nil && *filters.Type != "" {
		where = append(where, squirrel.Eq{"a.type": *filters.Type})
	}

	where = applyArtifactStatusVisibility(where, filters)

	for _, key := range sortedMetadataKeys(filters.Metadata) {
		if !isValidMetadataKey(key) {
			return nil, fmt.Errorf("invalid metadata key: %s (must contain only alphanumeric, underscore, or hyphen)", key)
		}
		where = append(where, squirrel.Expr("a.metadata->>? = ?", key, filters.Metadata[key]))
	}

	return where, nil
}

// applyArtifactStatusVisibility encodes the artifact lifecycle visibility rules
// shared by List and ListCrossTeam, in priority order:
//   - A substring search only ever returns active artifacts; drafts and archived
//     content are never surfaced through search, so any explicit status filter is
//     ignored while searching.
//   - Otherwise an explicit status filter selects exactly that status (the way to
//     reach draft or archived artifacts deliberately).
//   - The default list (no search, no explicit status) hides archived artifacts
//     while keeping drafts visible to their owner.
func applyArtifactStatusVisibility(
	where squirrel.And, filters repositories.ArtifactFilters,
) squirrel.And {
	switch {
	case filters.Search != "":
		term := "%" + filters.Search + "%"
		return append(where,
			squirrel.Eq{"a.status": models.ArtifactStatusActive},
			squirrel.Expr(
				"(a.title ILIKE ? OR a.description ILIKE ? OR a.content ILIKE ?)", term, term, term,
			),
		)
	case filters.Status != nil && *filters.Status != "":
		return append(where, squirrel.Eq{"a.status": *filters.Status})
	default:
		return append(where, squirrel.NotEq{"a.status": models.ArtifactStatusArchived})
	}
}

// sortedMetadataKeys returns the metadata keys in sorted order so that
// multi-key filters generate a deterministic query (the previous map-range
// iteration was nondeterministic). The result set is identical regardless of
// ordering, so this is a behaviour-preserving improvement.
func sortedMetadataKeys(metadata map[string]string) []string {
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// clampArtifactPaging converts the page/limit filters into squirrel's unsigned
// LIMIT/OFFSET values. Contract: a non-positive Limit emits LIMIT 0 (empty
// page) and only valid paging inputs derive an offset, so negative inputs can
// never wrap into a bogus positive offset.
func clampArtifactPaging(filters repositories.ArtifactFilters) (limit, offset uint64) {
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	if filters.Page > 1 && filters.Limit > 0 {
		if rawOffset := (filters.Page - 1) * filters.Limit; rawOffset > 0 {
			offset = uint64(rawOffset)
		}
	}
	return limit, offset
}

// countArtifacts counts artifacts matching the shared WHERE conditions used by
// the corresponding list query, so the count and page queries can never
// diverge. COUNT(*) is safe because the EXISTS subqueries (rather than a JOIN)
// eliminate multi-member duplicates. crossTeam selects the error-string suffix.
func (r *ArtifactRepository) countArtifacts(ctx context.Context, where squirrel.Sqlizer, crossTeam bool) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("artifacts a").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build artifacts count query%s: %w", artifactErrSuffix(crossTeam), err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count artifacts%s: %w", artifactErrSuffix(crossTeam), err)
	}

	return totalCount, nil
}

// queryArtifacts runs the paginated page query using the shared WHERE
// conditions, so it can never diverge from the count query. crossTeam selects
// the error-string suffix.
func (r *ArtifactRepository) queryArtifacts(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.ArtifactFilters, crossTeam bool,
) ([]models.Artifact, error) {
	limit, offset := clampArtifactPaging(filters)

	query, args, err := psql.
		Select(artifactListColumns...).
		From("artifacts a").
		Where(where).
		OrderBy(buildArtifactOrderByClause(filters)).
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build artifacts list query%s: %w", artifactErrSuffix(crossTeam), err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts%s: %w", artifactErrSuffix(crossTeam), err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	artifacts, err := scanArtifactListRows(rows, crossTeam)
	if err != nil {
		return nil, err
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate artifacts%s: %w", artifactErrSuffix(crossTeam), err)
	}

	return artifacts, nil
}

// scanArtifactListRows scans the 12-column artifact projection shared by List
// and ListCrossTeam (content excluded), unmarshalling the JSON metadata column
// per row. A malformed metadata payload returns an error. crossTeam selects the
// error-string suffix.
func scanArtifactListRows(rows *sql.Rows, crossTeam bool) ([]models.Artifact, error) {
	artifacts := make([]models.Artifact, 0)
	for rows.Next() {
		var artifact models.Artifact
		var metadataJSON []byte
		scanErr := rows.Scan(
			&artifact.ID, &artifact.ProjectID, &artifact.Slug,
			&artifact.UserID, &artifact.TeamID, &artifact.Title, &artifact.Description,
			&artifact.Status, &artifact.Type,
			&metadataJSON, &artifact.CreatedAt, &artifact.UpdatedAt,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan artifact%s: %w", artifactErrSuffix(crossTeam), scanErr)
		}

		if jsonErr := json.Unmarshal(metadataJSON, &artifact.Metadata); jsonErr != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata%s: %w", artifactErrSuffix(crossTeam), jsonErr)
		}

		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

// artifactErrSuffix returns the " (cross-team)" error-string suffix used by the
// cross-team list path so both list methods can share the same query helpers.
func artifactErrSuffix(crossTeam bool) string {
	if crossTeam {
		return " (cross-team)"
	}
	return ""
}

// ListCrossTeam retrieves artifacts across all teams the user owns, filtering by user_id ownership.
// Unlike List, it does not require a TeamID and performs no team membership check.
func (r *ArtifactRepository) ListCrossTeam(
	ctx context.Context, userID string, filters repositories.ArtifactFilters,
) ([]models.Artifact, int, error) {
	// Build WHERE clause using user_id ownership plus a team-membership guard so a
	// stale/cross-team artifact row can only be returned to a user who is still an
	// owner or member of the artifact's team (defense-in-depth on top of user_id).
	where := squirrel.And{
		squirrel.Eq{"a.user_id": userID},
		teamRowReadAccess("a.team_id", userID),
	}

	where, err := applyArtifactFilters(where, filters)
	if err != nil {
		return nil, 0, err
	}

	totalCount, err := r.countArtifacts(ctx, where, true)
	if err != nil {
		return nil, 0, err
	}

	artifacts, err := r.queryArtifacts(ctx, where, filters, true)
	if err != nil {
		return nil, 0, err
	}

	return artifacts, totalCount, nil
}

// List retrieves artifacts with filtering and pagination
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ArtifactRepository) List(
	ctx context.Context, userID string, filters repositories.ArtifactFilters,
) ([]models.Artifact, int, error) {
	// Validate required TeamID
	if filters.TeamID == "" {
		return nil, 0, fmt.Errorf("TeamID is required but was empty")
	}

	teamID := filters.TeamID

	// Build the WHERE clause with team membership check using EXISTS subqueries.
	where := squirrel.And{
		squirrel.Eq{"a.team_id": teamID},
		teamReadAccess(teamID, userID),
	}

	where, err := applyArtifactFilters(where, filters)
	if err != nil {
		return nil, 0, err
	}

	totalCount, err := r.countArtifacts(ctx, where, false)
	if err != nil {
		return nil, 0, err
	}

	artifacts, err := r.queryArtifacts(ctx, where, filters, false)
	if err != nil {
		return nil, 0, err
	}

	return artifacts, totalCount, nil
}

// validateArtifactOwnership checks if user has access to the artifact
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ArtifactRepository) validateArtifactOwnership(
	ctx context.Context, artifactID, teamID, userID string,
) error {
	var exists bool
	query := `
		SELECT EXISTS(
			SELECT 1 FROM artifacts a
			WHERE a.id = $1
				AND a.team_id = $2
				AND (
					EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
					OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
				)
		)
	`
	err := r.db.QueryRowContext(ctx, query, artifactID, teamID, userID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to validate artifact ownership: %w", err)
	}
	if !exists {
		return repositories.ErrArtifactNotFound
	}
	return nil
}

// Update updates an existing artifact with optimistic locking
func (r *ArtifactRepository) Update(ctx context.Context, artifact *models.Artifact) error {
	if err := r.validateArtifactOwnership(ctx, artifact.ID, artifact.TeamID, artifact.UserID); err != nil {
		return err
	}

	metadataJSON, err := json.Marshal(artifact.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Use EXISTS subqueries to avoid Cartesian product with multi-member teams
	query := `
		UPDATE artifacts
		SET project_id = $2, slug = $3, title = $4, description = $5, content = $6,
		status = $7, type = $8, metadata = $9, team_id = $10, updated_at = $11, version = version + 1
		WHERE id = $1
			AND team_id = $12
			AND version = $13
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $12 AND owner_id = $14)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $12 AND user_id = $14)
			)
		RETURNING updated_at, version
	`

	err = r.db.QueryRowContext(ctx, query,
		artifact.ID, artifact.ProjectID, artifact.Slug,
		artifact.Title, artifact.Description, artifact.Content,
		artifact.Status, artifact.Type, metadataJSON,
		artifact.TeamID, artifact.UpdatedAt, artifact.TeamID, artifact.Version, artifact.UserID,
	).Scan(&artifact.UpdatedAt, &artifact.Version)

	if err != nil {
		if uniqueViolation(err) != nil {
			return fmt.Errorf("artifact with slug '%s' already exists for this project", artifact.Slug)
		}
		if isFKViolation(err) {
			return fmt.Errorf("project not found or does not belong to user")
		}
		return mapNoRows(
			fmt.Errorf("failed to update artifact: %w", err),
			fmt.Errorf("version conflict: resource was modified by another request"),
		)
	}

	return nil
}

// Delete deletes an artifact
// Allows deletion if user is: resource owner, team owner, or team admin
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *ArtifactRepository) Delete(ctx context.Context, userID, teamID, artifactID string) error {
	query := `
		DELETE FROM artifacts
		WHERE id = $1
			AND team_id = $2
			AND (
				user_id = $3
				OR EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3 AND role IN ('owner', 'admin'))
			)
	`

	result, err := r.db.ExecContext(ctx, query, artifactID, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete artifact: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrArtifactNotFound
	}

	return nil
}

// GetStats retrieves statistics for artifacts
//
//nolint:funlen // Repository code with necessary complexity
func (r *ArtifactRepository) GetStats(
	ctx context.Context, userID, teamID string,
) (*models.ArtifactStatsResponse, error) {
	query := `
		SELECT
			COUNT(DISTINCT project_id) as total_projects,
			COUNT(*) as total_artifacts,
			COUNT(CASE WHEN created_at >= NOW() - INTERVAL '7 days' THEN 1 END) as added_this_week
		FROM artifacts
		WHERE user_id = $1 AND team_id = $2
	`

	var stats models.ArtifactStatsResponse
	err := r.db.QueryRowContext(ctx, query, userID, teamID).Scan(
		&stats.TotalProjects, &stats.TotalArtifacts, &stats.AddedThisWeek,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get artifact stats: %w", err)
	}

	// Get stats by type
	typeQuery := `
		SELECT type, COUNT(*)
		FROM artifacts
		WHERE user_id = $1 AND team_id = $2
		GROUP BY type
	`

	typeRows, err := r.db.QueryContext(ctx, typeQuery, userID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get artifact type stats: %w", err)
	}
	defer func() {
		if closeErr := typeRows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	stats.TotalByType = make(map[string]int)
	for typeRows.Next() {
		var artifactType string
		var count int
		if scanErr := typeRows.Scan(&artifactType, &count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan type stats: %w", scanErr)
		}
		stats.TotalByType[artifactType] = count
	}

	// Get stats by status
	statusQuery := `
		SELECT status, COUNT(*)
		FROM artifacts
		WHERE user_id = $1 AND team_id = $2
		GROUP BY status
	`

	statusRows, err := r.db.QueryContext(ctx, statusQuery, userID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get artifact status stats: %w", err)
	}
	defer func() {
		if closeErr := statusRows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	stats.TotalByStatus = make(map[string]int)
	for statusRows.Next() {
		var status string
		var count int
		if err := statusRows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan status stats: %w", err)
		}
		stats.TotalByStatus[status] = count
	}

	return &stats, nil
}

// CountAll counts all artifacts for a user across all teams
func (r *ArtifactRepository) CountAll(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM artifacts WHERE user_id = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count artifacts: %w", err)
	}

	return count, nil
}

// GetNamesByIDsCrossTeam returns a map of artifactID → title for the given IDs visible to userID
// across all teams the user belongs to (owner or member).
// Unknown or inaccessible IDs are omitted from the result map.
func (r *ArtifactRepository) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	// $1 is userID; artifact IDs start at $2
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = userID
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	// Artifacts use "title" instead of "name" — scanIDNameRows still applies because
	// the alias is just the second column, irrespective of source column.
	query := fmt.Sprintf(
		`SELECT a.id, a.title FROM artifacts a
		WHERE a.id IN (%s)
			AND (
				EXISTS (SELECT 1 FROM teams t WHERE t.id = a.team_id AND t.owner_id = $1)
				OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = a.team_id AND tm.user_id = $1)
			)`,
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get artifact names by ids: %w", err)
	}

	result, scanErr := scanIDNameRows(rows, len(ids), "artifact")
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close artifact name rows: %w", closeErr)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}
