package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// blueprintSlugConflictError builds a slug-collision error that names the scope
// that ACTUALLY collided. Blueprints carry two UNIQUE slug keys — the team-wide
// blueprints_slug_team_id_key (slug, team_id) and the stricter, subsumed
// blueprints_project_id_slug_unique (project_id, slug) — so a slug that is free
// in the target project can still collide elsewhere in the team. Reporting
// "for this project" unconditionally sent callers to search the wrong scope
// (#282, mirroring the artifact fix in #256); branching on the constraint that
// fired names the right one.
func blueprintSlugConflictError(pqErr *pq.Error, slug string) error {
	switch pqErr.Constraint {
	case "blueprints_slug_team_id_key":
		return fmt.Errorf("blueprint with slug '%s' already exists in this team", slug)
	case "blueprints_project_id_slug_unique":
		return fmt.Errorf("blueprint with slug '%s' already exists in this project", slug)
	default:
		return fmt.Errorf("blueprint with slug '%s' already exists", slug)
	}
}

// blueprintUniqueConflictError names the constraint that actually fired: the new
// (project_id, path) uniqueness (#339) gets a path-specific message, everything
// else falls through to the slug-scoped messages.
func blueprintUniqueConflictError(pqErr *pq.Error, bp *models.Blueprint) error {
	if pqErr.Constraint == "blueprints_project_id_path_unique" {
		return fmt.Errorf("blueprint with path '%s' already exists in this project", bp.Path)
	}
	return blueprintSlugConflictError(pqErr, bp.Slug)
}

// BlueprintRepository implements the repositories.BlueprintRepository interface for PostgreSQL
type BlueprintRepository struct {
	db *database.DB
}

// metadataKeyPattern validates metadata keys to prevent SQL injection.
// The pattern allows only alphanumeric characters, underscores, and hyphens,
// explicitly excluding SQL special characters (quotes, semicolons, etc.).
// This makes it safe to use the key in fmt.Sprintf for JSON field access.
var metadataKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// isValidMetadataKey validates that a metadata key only contains safe characters.
// Returns true only if the key:
// - Is non-empty and max 255 chars (to prevent DoS)
// - Matches the strict alphanumeric pattern (prevents SQL injection)
func isValidMetadataKey(key string) bool {
	return len(key) > 0 && len(key) <= 255 && metadataKeyPattern.MatchString(key)
}

// NewBlueprintRepository creates a new BlueprintRepository
func NewBlueprintRepository(db *database.DB) repositories.BlueprintRepository {
	return &BlueprintRepository{
		db: db,
	}
}

// blueprintSyncScan holds the nullable sync columns (epic #334) while scanning a
// blueprint row, before they are applied onto the model.
type blueprintSyncScan struct {
	rawContent   sql.NullString
	contentSHA   sql.NullString
	sourceRepo   sql.NullString
	sourceCommit sql.NullString
	sourceBlob   sql.NullString
	importedAt   sql.NullTime
}

// apply copies the scanned nullable sync columns onto the blueprint, assembling
// the provenance Source object (nil when no provenance is set).
func (s blueprintSyncScan) apply(bp *models.Blueprint) {
	bp.RawContent = s.rawContent.String
	bp.ContentSHA = s.contentSHA.String
	bp.Source = assembleBlueprintSource(s.sourceRepo, s.sourceCommit, s.sourceBlob, s.importedAt)
}

// assembleBlueprintSource builds the read-only provenance object, or nil when no
// provenance column is set (a VibeXP-authored blueprint).
func assembleBlueprintSource(repo, commit, blob sql.NullString, importedAt sql.NullTime) *models.BlueprintSource {
	if !repo.Valid && !commit.Valid && !blob.Valid && !importedAt.Valid {
		return nil
	}
	src := &models.BlueprintSource{
		Repo:      repo.String,
		CommitSHA: commit.String,
		BlobSHA:   blob.String,
	}
	if importedAt.Valid {
		t := importedAt.Time
		src.ImportedAt = &t
	}
	return src
}

// nullableString maps "" to a NULL parameter, any other value to itself, so an
// empty provenance/hash column is stored as NULL rather than an empty string.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// blueprintSourceInsertValues extracts the four provenance parameters (NULL when
// unset) for INSERT.
func blueprintSourceInsertValues(src *models.BlueprintSource) (repo, commit, blob, importedAt any) {
	if src == nil {
		return nil, nil, nil, nil
	}
	var at any
	if src.ImportedAt != nil {
		at = *src.ImportedAt
	}
	return nullableString(src.Repo), nullableString(src.CommitSHA), nullableString(src.BlobSHA), at
}

// Create creates a new blueprint entry
func (r *BlueprintRepository) Create(ctx context.Context, blueprint *models.Blueprint) error {
	metadataJSON, err := json.Marshal(blueprint.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	srcRepo, srcCommit, srcBlob, importedAt := blueprintSourceInsertValues(blueprint.Source)

	query := `
		INSERT INTO blueprints
		(project_id, slug, user_id, team_id, title, description, content,
		status, type, subtype, metadata, created_at, updated_at,
		path, path_derived, raw_content, content_sha,
		source_repo, source_commit_sha, source_blob_sha, imported_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
		$14, $15, $16, $17, $18, $19, $20, $21)
		RETURNING id, created_at, updated_at
	`

	err = r.db.QueryRowContext(ctx, query,
		blueprint.ProjectID, blueprint.Slug, blueprint.UserID, blueprint.TeamID,
		blueprint.Title, blueprint.Description, blueprint.Content,
		blueprint.Status, blueprint.Type, blueprint.Subtype, metadataJSON,
		blueprint.CreatedAt, blueprint.UpdatedAt,
		blueprint.Path, blueprint.PathDerived, nullableString(blueprint.RawContent), nullableString(blueprint.ContentSHA),
		srcRepo, srcCommit, srcBlob, importedAt,
	).Scan(&blueprint.ID, &blueprint.CreatedAt, &blueprint.UpdatedAt)

	if err != nil {
		if pqErr := uniqueViolation(err); pqErr != nil {
			return blueprintUniqueConflictError(pqErr, blueprint)
		}
		if isFKViolation(err) {
			return fmt.Errorf("project not found or does not belong to user")
		}
		return fmt.Errorf("failed to create blueprint: %w", err)
	}

	return nil
}

// GetByID retrieves a blueprint entry by its ID
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *BlueprintRepository) GetByID(
	ctx context.Context, userID, teamID, blueprintID string,
) (*models.Blueprint, error) {
	query := `
		SELECT s.id, s.project_id, s.slug, s.user_id, s.team_id, s.title, s.description, s.content, s.status,
		s.type, s.subtype, s.metadata, s.created_at, s.updated_at, s.version,
		s.path, s.path_derived, s.raw_content, s.content_sha,
		s.source_repo, s.source_commit_sha, s.source_blob_sha, s.imported_at
		FROM blueprints s
		WHERE s.id = $1
			AND s.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	var blueprint models.Blueprint
	var metadataJSON []byte
	var sync blueprintSyncScan
	err := r.db.QueryRowContext(ctx, query, blueprintID, teamID, userID).Scan(
		&blueprint.ID, &blueprint.ProjectID, &blueprint.Slug,
		&blueprint.UserID, &blueprint.TeamID, &blueprint.Title, &blueprint.Description,
		&blueprint.Content, &blueprint.Status, &blueprint.Type,
		&blueprint.Subtype, &metadataJSON, &blueprint.CreatedAt, &blueprint.UpdatedAt, &blueprint.Version,
		&blueprint.Path, &blueprint.PathDerived, &sync.rawContent, &sync.contentSHA,
		&sync.sourceRepo, &sync.sourceCommit, &sync.sourceBlob, &sync.importedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get blueprint by ID: %w", err), repositories.ErrBlueprintNotFound)
	}
	sync.apply(&blueprint)

	// Initialize metadata if JSON is nil or empty
	if len(metadataJSON) == 0 {
		blueprint.Metadata = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(metadataJSON, &blueprint.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &blueprint, nil
}

// GetByProjectIDAndPath retrieves a blueprint by project ID and canonical path,
// team-scoped by membership (mirrors GetByProjectIDAndSlug). Used by #341's
// path-first re-import matching.
func (r *BlueprintRepository) GetByProjectIDAndPath(
	ctx context.Context, userID, teamID, projectID, path string,
) (*models.Blueprint, error) {
	query := `
		SELECT s.id, s.project_id, s.slug, s.user_id, s.team_id, s.title, s.description, s.content, s.status,
		s.type, s.subtype, s.metadata, s.created_at, s.updated_at, s.version,
		s.path, s.path_derived, s.raw_content, s.content_sha,
		s.source_repo, s.source_commit_sha, s.source_blob_sha, s.imported_at
		FROM blueprints s
		WHERE s.project_id = $1
			AND s.path = $2
			AND s.team_id = $3
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $3 AND owner_id = $4)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $3 AND user_id = $4)
			)
	`

	var blueprint models.Blueprint
	var metadataJSON []byte
	var sync blueprintSyncScan
	err := r.db.QueryRowContext(ctx, query, projectID, path, teamID, userID).Scan(
		&blueprint.ID, &blueprint.ProjectID, &blueprint.Slug,
		&blueprint.UserID, &blueprint.TeamID, &blueprint.Title, &blueprint.Description,
		&blueprint.Content, &blueprint.Status, &blueprint.Type,
		&blueprint.Subtype, &metadataJSON, &blueprint.CreatedAt, &blueprint.UpdatedAt, &blueprint.Version,
		&blueprint.Path, &blueprint.PathDerived, &sync.rawContent, &sync.contentSHA,
		&sync.sourceRepo, &sync.sourceCommit, &sync.sourceBlob, &sync.importedAt,
	)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get blueprint by project and path: %w", err),
			repositories.ErrBlueprintNotFound,
		)
	}
	sync.apply(&blueprint)
	if len(metadataJSON) == 0 {
		blueprint.Metadata = make(map[string]interface{})
	} else if err := json.Unmarshal(metadataJSON, &blueprint.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &blueprint, nil
}

// GetByProjectIDAndSlug retrieves a blueprint entry by project ID and slug
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *BlueprintRepository) GetByProjectIDAndSlug(
	ctx context.Context, userID, teamID, projectID, slug string,
) (*models.Blueprint, error) {
	query := `
		SELECT s.id, s.project_id, s.slug, s.user_id, s.team_id, s.title, s.description, s.content, s.status,
		s.type, s.subtype, s.metadata, s.created_at, s.updated_at, s.version,
		s.path, s.path_derived, s.raw_content, s.content_sha,
		s.source_repo, s.source_commit_sha, s.source_blob_sha, s.imported_at
		FROM blueprints s
		WHERE s.project_id = $1
			AND s.slug = $2
			AND s.team_id = $3
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $3 AND owner_id = $4)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $3 AND user_id = $4)
			)
	`

	var blueprint models.Blueprint
	var metadataJSON []byte
	var sync blueprintSyncScan
	err := r.db.QueryRowContext(ctx, query, projectID, slug, teamID, userID).Scan(
		&blueprint.ID, &blueprint.ProjectID, &blueprint.Slug,
		&blueprint.UserID, &blueprint.TeamID, &blueprint.Title, &blueprint.Description,
		&blueprint.Content, &blueprint.Status, &blueprint.Type,
		&blueprint.Subtype, &metadataJSON, &blueprint.CreatedAt, &blueprint.UpdatedAt, &blueprint.Version,
		&blueprint.Path, &blueprint.PathDerived, &sync.rawContent, &sync.contentSHA,
		&sync.sourceRepo, &sync.sourceCommit, &sync.sourceBlob, &sync.importedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get blueprint by project and slug: %w", err),
			repositories.ErrBlueprintNotFound,
		)
	}
	sync.apply(&blueprint)

	// Initialize metadata if JSON is nil or empty
	if len(metadataJSON) == 0 {
		blueprint.Metadata = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(metadataJSON, &blueprint.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &blueprint, nil
}

// GetByIDCrossTeam retrieves a blueprint by ID across all user's teams
func (r *BlueprintRepository) GetByIDCrossTeam(
	ctx context.Context, userID, blueprintID string,
) (*models.Blueprint, error) {
	query := `
		SELECT id, project_id, slug, user_id, team_id, title, description, content, status,
		type, subtype, metadata, created_at, updated_at, version,
		path, path_derived, raw_content, content_sha,
		source_repo, source_commit_sha, source_blob_sha, imported_at
		FROM blueprints
		WHERE id = $1 AND user_id = $2
	`

	var blueprint models.Blueprint
	var metadataJSON []byte
	var sync blueprintSyncScan
	err := r.db.QueryRowContext(ctx, query, blueprintID, userID).Scan(
		&blueprint.ID, &blueprint.ProjectID, &blueprint.Slug,
		&blueprint.UserID, &blueprint.TeamID, &blueprint.Title, &blueprint.Description,
		&blueprint.Content, &blueprint.Status, &blueprint.Type,
		&blueprint.Subtype, &metadataJSON, &blueprint.CreatedAt, &blueprint.UpdatedAt, &blueprint.Version,
		&blueprint.Path, &blueprint.PathDerived, &sync.rawContent, &sync.contentSHA,
		&sync.sourceRepo, &sync.sourceCommit, &sync.sourceBlob, &sync.importedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get blueprint by ID (cross-team): %w", err),
			repositories.ErrBlueprintNotFound,
		)
	}
	sync.apply(&blueprint)

	// Initialize metadata if JSON is nil or empty
	if len(metadataJSON) == 0 {
		blueprint.Metadata = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(metadataJSON, &blueprint.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &blueprint, nil
}

// GetByProjectIDAndSlugCrossTeam retrieves a blueprint by project ID and slug across all user's teams
func (r *BlueprintRepository) GetByProjectIDAndSlugCrossTeam(
	ctx context.Context, userID, projectID, slug string,
) (*models.Blueprint, error) {
	query := `
		SELECT id, project_id, slug, user_id, team_id, title, description, content, status,
		type, subtype, metadata, created_at, updated_at, version,
		path, path_derived, raw_content, content_sha,
		source_repo, source_commit_sha, source_blob_sha, imported_at
		FROM blueprints
		WHERE project_id = $1 AND slug = $2 AND user_id = $3
	`

	var blueprint models.Blueprint
	var metadataJSON []byte
	var sync blueprintSyncScan
	err := r.db.QueryRowContext(ctx, query, projectID, slug, userID).Scan(
		&blueprint.ID, &blueprint.ProjectID, &blueprint.Slug,
		&blueprint.UserID, &blueprint.TeamID, &blueprint.Title, &blueprint.Description,
		&blueprint.Content, &blueprint.Status, &blueprint.Type,
		&blueprint.Subtype, &metadataJSON, &blueprint.CreatedAt, &blueprint.UpdatedAt, &blueprint.Version,
		&blueprint.Path, &blueprint.PathDerived, &sync.rawContent, &sync.contentSHA,
		&sync.sourceRepo, &sync.sourceCommit, &sync.sourceBlob, &sync.importedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get blueprint by project and slug (cross-team): %w", err),
			repositories.ErrBlueprintNotFound,
		)
	}
	sync.apply(&blueprint)

	// Initialize metadata if JSON is nil or empty
	if len(metadataJSON) == 0 {
		blueprint.Metadata = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(metadataJSON, &blueprint.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &blueprint, nil
}

// blueprintListColumns is the projection used by List. The content and
// raw_content columns are deliberately excluded from list operations to keep
// payloads small (raw_content is returned only on the detail GET). path is
// required on the Blueprint response, so it is included.
var blueprintListColumns = []string{
	"s.id", "s.project_id", "s.slug", "s.user_id", "s.team_id",
	"s.title", "s.description", "s.status", "s.type", "s.subtype",
	"s.metadata", "s.created_at", "s.updated_at",
	"s.path", "s.path_derived", "s.content_sha",
	"s.source_repo", "s.source_commit_sha", "s.source_blob_sha", "s.imported_at",
}

// buildBlueprintListOrderByClause builds the ORDER BY clause for the blueprint
// list query using an allowlist of sortable columns. Any value outside the
// allowlist (including injection attempts) falls back to the default. The
// returned string is always `s.`-prefixed, matching the historical template.
func buildBlueprintListOrderByClause(filters repositories.BlueprintFilters) string {
	orderBy := "s.created_at DESC"
	if filters.SortBy == "" {
		return orderBy
	}
	direction := "DESC"
	if filters.SortOrder == "asc" {
		direction = "ASC"
	}
	switch filters.SortBy {
	case "created_at", "updated_at":
		return fmt.Sprintf("s.%s %s", filters.SortBy, direction)
	}
	return orderBy
}

// applyBlueprintFilters appends the optional filter conditions to the WHERE
// clause. Metadata keys are validated against the allowlist and iterated in
// sorted order so multi-key filters produce a deterministic, parameter-bound
// query.
func applyBlueprintFilters(
	where squirrel.And, filters repositories.BlueprintFilters,
) (squirrel.And, error) {
	if filters.ProjectID != nil && *filters.ProjectID != "" {
		where = append(where, squirrel.Eq{"s.project_id": *filters.ProjectID})
	}

	if filters.Status != nil && *filters.Status != "" {
		where = append(where, squirrel.Eq{"s.status": *filters.Status})
	}

	if filters.Type != nil && *filters.Type != "" {
		where = append(where, squirrel.Eq{"s.type": *filters.Type})
	}

	if filters.Subtype != nil && *filters.Subtype != "" {
		where = append(where, squirrel.Eq{"s.subtype": *filters.Subtype})
	}

	if filters.Search != "" {
		term := "%" + filters.Search + "%"
		where = append(where, squirrel.Expr(
			"(s.title ILIKE ? OR s.description ILIKE ? OR s.content ILIKE ?)", term, term, term,
		))
	}

	for _, key := range sortedMetadataKeys(filters.Metadata) {
		if !isValidMetadataKey(key) {
			return nil, fmt.Errorf("invalid metadata key: %s (must contain only alphanumeric, underscore, or hyphen)", key)
		}
		where = append(where, squirrel.Expr("s.metadata->>? = ?", key, filters.Metadata[key]))
	}

	return where, nil
}

// clampBlueprintPaging converts the page/limit filters into squirrel's unsigned
// LIMIT/OFFSET values. Contract: a non-positive Limit emits LIMIT 0 (empty page)
// and only valid paging inputs derive an offset, so negative inputs can never
// wrap into a bogus positive offset.
func clampBlueprintPaging(filters repositories.BlueprintFilters) (limit, offset uint64) {
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

// List retrieves blueprint entries with filtering and pagination
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *BlueprintRepository) List(
	ctx context.Context, userID string, filters repositories.BlueprintFilters,
) ([]models.Blueprint, int, error) {
	// Validate required TeamID
	if filters.TeamID == "" {
		return nil, 0, fmt.Errorf("TeamID is required but was empty")
	}

	teamID := filters.TeamID

	// Build the WHERE clause with team membership check using EXISTS subqueries.
	where := squirrel.And{
		squirrel.Eq{"s.team_id": teamID},
		teamReadAccess(teamID, userID),
	}

	where, err := applyBlueprintFilters(where, filters)
	if err != nil {
		return nil, 0, err
	}

	totalCount, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	blueprints, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return blueprints, totalCount, nil
}

// countList counts blueprint entries matching the shared WHERE conditions used
// by queryList, so the count and page queries can never diverge. COUNT(*) is
// safe because the EXISTS subqueries (rather than a JOIN) eliminate multi-member
// duplicates.
func (r *BlueprintRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("blueprints s").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build blueprints count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count blueprints: %w", err)
	}

	return totalCount, nil
}

// queryList runs the paginated page query using the shared WHERE conditions, so
// it can never diverge from the count query. The content column is excluded for
// list performance.
func (r *BlueprintRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.BlueprintFilters,
) ([]models.Blueprint, error) {
	limit, offset := clampBlueprintPaging(filters)

	query, args, err := psql.
		Select(blueprintListColumns...).
		From("blueprints s").
		Where(where).
		OrderBy(buildBlueprintListOrderByClause(filters)).
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build blueprints list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list blueprint entries: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	blueprints, err := scanBlueprintListRows(rows)
	if err != nil {
		return nil, err
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate blueprint entries: %w", err)
	}

	return blueprints, nil
}

// scanBlueprintListRows scans the blueprint list projection (content and
// raw_content excluded; see blueprintListColumns). An empty/NULL metadata column
// initializes an empty map without unmarshalling; a non-empty malformed payload
// returns an error.
func scanBlueprintListRows(rows *sql.Rows) ([]models.Blueprint, error) {
	blueprints := make([]models.Blueprint, 0)
	for rows.Next() {
		var blueprint models.Blueprint
		var metadataJSON []byte
		var sync blueprintSyncScan
		scanErr := rows.Scan(
			&blueprint.ID, &blueprint.ProjectID, &blueprint.Slug,
			&blueprint.UserID, &blueprint.TeamID, &blueprint.Title, &blueprint.Description,
			&blueprint.Status, &blueprint.Type, &blueprint.Subtype,
			&metadataJSON, &blueprint.CreatedAt, &blueprint.UpdatedAt,
			&blueprint.Path, &blueprint.PathDerived, &sync.contentSHA,
			&sync.sourceRepo, &sync.sourceCommit, &sync.sourceBlob, &sync.importedAt,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan blueprint: %w", scanErr)
		}

		// Initialize metadata if JSON is nil or empty
		if len(metadataJSON) == 0 {
			blueprint.Metadata = make(map[string]interface{})
		} else {
			if jsonErr := json.Unmarshal(metadataJSON, &blueprint.Metadata); jsonErr != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", jsonErr)
			}
		}

		// raw_content is not selected by List; apply the rest (content_sha,
		// provenance). rawContent stays empty and is omitted from the response.
		sync.apply(&blueprint)
		blueprints = append(blueprints, blueprint)
	}

	return blueprints, nil
}

// validateBlueprintInTeam proves the blueprint exists within the team. It is a TENANCY
// check only (epic #220 decision D3): whether the caller's ROLE permits the
// update is decided by BlueprintService via the authz matrix before this is reached.
// It is kept because Update relies on ErrBlueprintNotFound to tell a missing blueprint
// apart from an optimistic-lock version conflict.
func (r *BlueprintRepository) validateBlueprintInTeam(ctx context.Context, blueprintID, teamID string) error {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM blueprints s WHERE s.id = $1 AND s.team_id = $2)`
	if err := r.db.QueryRowContext(ctx, query, blueprintID, teamID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to validate blueprint: %w", err)
	}
	if !exists {
		return repositories.ErrBlueprintNotFound
	}
	return nil
}

func (r *BlueprintRepository) Update(ctx context.Context, blueprint *models.Blueprint) error {
	if err := r.validateBlueprintInTeam(ctx, blueprint.ID, blueprint.TeamID); err != nil {
		return err
	}

	metadataJSON, err := json.Marshal(blueprint.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Provenance columns (source_*/imported_at) are intentionally NOT in the SET
	// list: they are server-set on import and must survive a VibeXP edit — a
	// changed content_sha (below) is the "edited in VibeXP" signal (#341), while
	// source_blob_sha stays the imported reference.
	query := `
		UPDATE blueprints
		SET project_id = $2, slug = $3, title = $4, description = $5, content = $6,
		status = $7, type = $8, subtype = $9, metadata = $10, team_id = $11, updated_at = $12,
		path = $13, path_derived = $14, raw_content = $15, content_sha = $16, version = version + 1
		WHERE id = $1
			AND team_id = $17
			AND version = $18
		RETURNING updated_at, version
	`

	err = r.db.QueryRowContext(ctx, query,
		blueprint.ID, blueprint.ProjectID, blueprint.Slug,
		blueprint.Title, blueprint.Description, blueprint.Content,
		blueprint.Status, blueprint.Type, blueprint.Subtype, metadataJSON,
		blueprint.TeamID, blueprint.UpdatedAt,
		blueprint.Path, blueprint.PathDerived, nullableString(blueprint.RawContent), nullableString(blueprint.ContentSHA),
		blueprint.TeamID, blueprint.Version,
	).Scan(&blueprint.UpdatedAt, &blueprint.Version)

	if err != nil {
		if pqErr := uniqueViolation(err); pqErr != nil {
			return blueprintUniqueConflictError(pqErr, blueprint)
		}
		if isFKViolation(err) {
			return fmt.Errorf("project not found or does not belong to user")
		}
		return mapNoRows(
			fmt.Errorf("failed to update blueprint: %w", err),
			fmt.Errorf("version conflict: resource was modified by another request"),
		)
	}

	return nil
}

// UpdateOnReimport refreshes an existing blueprint from a changed repo file. It
// rewrites the parsed + raw content, the frozen source path, AND the provenance
// columns (unlike Update, which preserves provenance). Import is server-
// authoritative, so it does not take an optimistic-lock version — it matches by
// (id, team_id) and bumps the version.
func (r *BlueprintRepository) UpdateOnReimport(ctx context.Context, blueprint *models.Blueprint) error {
	metadataJSON, err := json.Marshal(blueprint.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	srcRepo, srcCommit, srcBlob, importedAt := blueprintSourceInsertValues(blueprint.Source)

	query := `
		UPDATE blueprints
		SET title = $2, description = $3, content = $4, type = $5, subtype = $6, metadata = $7,
		path = $8, path_derived = $9, raw_content = $10, content_sha = $11,
		source_repo = $12, source_commit_sha = $13, source_blob_sha = $14, imported_at = $15,
		updated_at = $16, version = version + 1
		WHERE id = $1 AND team_id = $17
		RETURNING updated_at, version
	`
	err = r.db.QueryRowContext(ctx, query,
		blueprint.ID, blueprint.Title, blueprint.Description, blueprint.Content,
		blueprint.Type, blueprint.Subtype, metadataJSON,
		blueprint.Path, blueprint.PathDerived, nullableString(blueprint.RawContent), nullableString(blueprint.ContentSHA),
		srcRepo, srcCommit, srcBlob, importedAt,
		blueprint.UpdatedAt, blueprint.TeamID,
	).Scan(&blueprint.UpdatedAt, &blueprint.Version)
	if err != nil {
		if pqErr := uniqueViolation(err); pqErr != nil {
			return blueprintUniqueConflictError(pqErr, blueprint)
		}
		return mapNoRows(
			fmt.Errorf("failed to re-import blueprint: %w", err),
			repositories.ErrBlueprintNotFound,
		)
	}
	return nil
}

// Delete deletes a blueprint entry
// Allows deletion if user is: resource owner, team owner, or team admin
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *BlueprintRepository) Delete(ctx context.Context, userID, teamID, blueprintID string) error {
	query := `
		DELETE FROM blueprints
		WHERE id = $1
			AND team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	result, err := r.db.ExecContext(ctx, query, blueprintID, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete blueprint: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrBlueprintNotFound
	}

	return nil
}

// GetStats retrieves statistics for blueprints.
//
// When the user has no blueprint data it returns a zero-valued
// models.BlueprintStatsResponse — not an error.
//
//nolint:funlen // Repository code with necessary complexity
func (r *BlueprintRepository) GetStats(ctx context.Context, userID string) (*models.BlueprintStatsResponse, error) {
	// Optimized single query using JSON aggregation to avoid N+1 queries
	query := `
		WITH base_stats AS (
			SELECT
				COUNT(DISTINCT project_id) as total_projects,
				COUNT(*) as total_blueprints,
				COUNT(CASE WHEN created_at >= NOW() - INTERVAL '7 days' THEN 1 END) as added_this_week
			FROM blueprints
			WHERE user_id = $1
		),
		type_stats AS (
			SELECT json_object_agg(type, count) as type_counts
			FROM (
				SELECT type, COUNT(*) as count
				FROM blueprints
				WHERE user_id = $1
				GROUP BY type
			) t
		),
		status_stats AS (
			SELECT json_object_agg(status, count) as status_counts
			FROM (
				SELECT status, COUNT(*) as count
				FROM blueprints
				WHERE user_id = $1
				GROUP BY status
			) s
		)
		SELECT
			b.total_projects,
			b.total_blueprints,
			b.added_this_week,
			COALESCE(t.type_counts, '{}'::json),
			COALESCE(s.status_counts, '{}'::json)
		FROM base_stats b
		CROSS JOIN type_stats t
		CROSS JOIN status_stats s
	`

	var stats models.BlueprintStatsResponse
	var typeCountsJSON []byte
	var statusCountsJSON []byte

	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&stats.TotalProjects,
		&stats.TotalBlueprints,
		&stats.AddedThisWeek,
		&typeCountsJSON,
		&statusCountsJSON,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No data for this user, return empty stats
			return &models.BlueprintStatsResponse{
				TotalProjects:   0,
				TotalBlueprints: 0,
				AddedThisWeek:   0,
				TotalByType:     make(map[string]int),
				TotalByStatus:   make(map[string]int),
			}, nil
		}
		return nil, fmt.Errorf("failed to get blueprints stats: %w", err)
	}

	// Unmarshal JSON aggregated results
	stats.TotalByType = make(map[string]int)
	if len(typeCountsJSON) > 0 && string(typeCountsJSON) != "{}" {
		if err := json.Unmarshal(typeCountsJSON, &stats.TotalByType); err != nil {
			return nil, fmt.Errorf("failed to unmarshal type stats: %w", err)
		}
	}

	stats.TotalByStatus = make(map[string]int)
	if len(statusCountsJSON) > 0 && string(statusCountsJSON) != "{}" {
		if err := json.Unmarshal(statusCountsJSON, &stats.TotalByStatus); err != nil {
			return nil, fmt.Errorf("failed to unmarshal status stats: %w", err)
		}
	}

	return &stats, nil
}

// GetNamesByIDsCrossTeam returns a map of blueprintID → title for the given IDs visible to userID
// across all teams the user belongs to (owner or member).
// Unknown or inaccessible IDs are omitted from the result map.
func (r *BlueprintRepository) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	// $1 is userID; blueprint IDs start at $2
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = userID
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	// Blueprints use "title" instead of "name" — scanIDNameRows still applies because
	// the alias is just the second column, irrespective of source column.
	query := fmt.Sprintf(
		`SELECT b.id, b.title FROM blueprints b
		WHERE b.id IN (%s)
			AND (
				EXISTS (SELECT 1 FROM teams t WHERE t.id = b.team_id AND t.owner_id = $1)
				OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = b.team_id AND tm.user_id = $1)
			)`,
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get blueprint names by ids: %w", err)
	}

	result, scanErr := scanIDNameRows(rows, len(ids), "blueprint")
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close blueprint name rows: %w", closeErr)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}
