package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// MemoryRepository implements the repositories.MemoryRepository interface for PostgreSQL
type MemoryRepository struct {
	db *database.DB
}

// NewMemoryRepository creates a new MemoryRepository
func NewMemoryRepository(db *database.DB) repositories.MemoryRepository {
	return &MemoryRepository{
		db: db,
	}
}

// Create creates a new memory
func (r *MemoryRepository) Create(ctx context.Context, memory *models.Memory) error {
	metadataJSON, err := json.Marshal(memory.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO memories (user_id, team_id, project_id, text, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`

	err = r.db.QueryRowContext(ctx, query,
		memory.UserID, memory.TeamID, memory.ProjectID, memory.Text, metadataJSON,
		memory.CreatedAt, memory.UpdatedAt,
	).Scan(&memory.ID, &memory.CreatedAt, &memory.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create memory: %w", err)
	}

	return nil
}

// GetByID retrieves a memory by its ID
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *MemoryRepository) GetByID(ctx context.Context, userID, teamID, memoryID string) (*models.Memory, error) {
	query := `
		SELECT m.id, m.user_id, m.team_id, m.project_id, m.text, m.metadata,
		       m.created_at, m.updated_at, m.version
		FROM memories m
		WHERE m.id = $1
			AND m.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	var memory models.Memory
	var metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, memoryID, teamID, userID).Scan(
		&memory.ID, &memory.UserID, &memory.TeamID, &memory.ProjectID, &memory.Text, &metadataJSON,
		&memory.CreatedAt, &memory.UpdatedAt, &memory.Version,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get memory by ID: %w", err), repositories.ErrMemoryNotFound)
	}

	if err := json.Unmarshal(metadataJSON, &memory.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &memory, nil
}

// GetByIDCrossTeam retrieves a memory by ID across all user's teams
func (r *MemoryRepository) GetByIDCrossTeam(ctx context.Context, userID, memoryID string) (*models.Memory, error) {
	query := `
		SELECT id, user_id, team_id, project_id, text, metadata, created_at, updated_at, version
		FROM memories
		WHERE id = $1 AND user_id = $2
	`

	var memory models.Memory
	var metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, memoryID, userID).Scan(
		&memory.ID, &memory.UserID, &memory.TeamID, &memory.ProjectID, &memory.Text, &metadataJSON,
		&memory.CreatedAt, &memory.UpdatedAt, &memory.Version,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get memory by ID (cross-team): %w", err), repositories.ErrMemoryNotFound)
	}

	if err := json.Unmarshal(metadataJSON, &memory.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &memory, nil
}

// buildMemoryOrderByClause builds the ORDER BY clause for memory list query using allowlisted fields
func buildMemoryOrderByClause(filters repositories.MemoryFilters) string {
	orderBy := "m.updated_at DESC"
	if filters.SortBy == "" {
		return orderBy
	}
	direction := "DESC"
	if filters.SortOrder == "asc" {
		direction = "ASC"
	}
	switch filters.SortBy {
	case "text", "updated_at", "created_at":
		return fmt.Sprintf("m.%s %s", filters.SortBy, direction)
	}
	return orderBy
}

// buildMemoryListWhereClause builds the shared WHERE conditions for the memory
// list query. Count and page queries consume the same conditions, so they can
// never diverge. EXISTS subqueries avoid a Cartesian product with multi-member
// teams.
func buildMemoryListWhereClause(userID string, filters repositories.MemoryFilters) squirrel.Sqlizer {
	teamID := filters.TeamID
	where := squirrel.And{
		squirrel.Eq{"m.team_id": teamID},
		teamReadAccess(teamID, userID),
	}

	if filters.ProjectID != nil {
		where = append(where, squirrel.Eq{"m.project_id": *filters.ProjectID})
	}

	if filters.Search != "" {
		where = append(where, squirrel.Expr("m.text ILIKE ?", "%"+filters.Search+"%"))
	}

	if filters.MetadataKey != nil && filters.MetadataValue != nil {
		where = append(where, squirrel.Expr(
			"m.metadata ->> ? = ?", *filters.MetadataKey, *filters.MetadataValue,
		))
	}

	return where
}

// List retrieves memories with filtering and pagination
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *MemoryRepository) List(
	ctx context.Context, userID string, filters repositories.MemoryFilters,
) ([]models.Memory, int, error) {
	// Validate required TeamID
	if filters.TeamID == "" {
		return nil, 0, fmt.Errorf("TeamID is required but was empty")
	}

	// Shared WHERE conditions guarantee the count and page queries can never diverge.
	where := buildMemoryListWhereClause(userID, filters)

	totalCount, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	memories, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return memories, totalCount, nil
}

// countList counts memories matching the shared WHERE conditions used by List,
// so the count and page queries can never diverge. COUNT(*) is safe because the
// EXISTS subqueries (rather than a JOIN) eliminate multi-member duplicates.
func (r *MemoryRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("memories m").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build memories count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count memories: %w", err)
	}

	return totalCount, nil
}

// queryList runs the paginated page query for List using the shared WHERE
// conditions, so it can never diverge from the count query.
func (r *MemoryRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.MemoryFilters,
) ([]models.Memory, error) {
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
			"m.id", "m.user_id", "m.team_id", "m.project_id",
			"m.text", "m.metadata", "m.created_at", "m.updated_at",
		).
		From("memories m").
		Where(where).
		OrderBy(buildMemoryOrderByClause(filters)).
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build memories list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	memories, err := scanMemoryRows(rows)
	if err != nil {
		return nil, err
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate memories: %w", err)
	}

	return memories, nil
}

// scanMemoryRows scans the 8-column memory projection shared by the list and
// metadata-search queries, unmarshalling the JSON metadata column per row.
func scanMemoryRows(rows *sql.Rows) ([]models.Memory, error) {
	memories := make([]models.Memory, 0)
	for rows.Next() {
		var memory models.Memory
		var metadataJSON []byte

		scanErr := rows.Scan(
			&memory.ID, &memory.UserID, &memory.TeamID, &memory.ProjectID, &memory.Text, &metadataJSON,
			&memory.CreatedAt, &memory.UpdatedAt,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan memory: %w", scanErr)
		}

		if jsonErr := json.Unmarshal(metadataJSON, &memory.Metadata); jsonErr != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", jsonErr)
		}

		memories = append(memories, memory)
	}

	return memories, nil
}

// Update updates an existing memory with optimistic locking
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *MemoryRepository) Update(ctx context.Context, memory *models.Memory) error {
	// Validate team membership BEFORE checking version to distinguish errors
	// This prevents conflating authorization failures with optimistic lock conflicts
	var exists bool
	ownershipQuery := `
		SELECT EXISTS(
			SELECT 1 FROM memories m
			WHERE m.id = $1
				AND m.team_id = $2
				AND (
					EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
					OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
				)
		)
	`
	err := r.db.QueryRowContext(ctx, ownershipQuery, memory.ID, memory.TeamID, memory.UserID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to validate memory ownership: %w", err)
	}
	if !exists {
		// Memory doesn't exist for this user/team - authorization failure
		return repositories.ErrMemoryNotFound
	}

	metadataJSON, err := json.Marshal(memory.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Use EXISTS subqueries to avoid Cartesian product with multi-member teams
	query := `
		UPDATE memories
		SET text = $2, metadata = $3, project_id = $4, team_id = $5, updated_at = $6, version = version + 1
		WHERE id = $1
			AND team_id = $7
			AND version = $8
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $7 AND owner_id = $9)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $7 AND user_id = $9)
			)
		RETURNING updated_at, version
	`

	err = r.db.QueryRowContext(ctx, query,
		memory.ID, memory.Text, metadataJSON, memory.ProjectID, memory.TeamID, memory.UpdatedAt,
		memory.TeamID, memory.Version, memory.UserID,
	).Scan(&memory.UpdatedAt, &memory.Version)

	if err != nil {
		// No rows: ownership was already validated, so this must be a version mismatch.
		return mapNoRows(
			fmt.Errorf("failed to update memory: %w", err),
			fmt.Errorf("version conflict: resource was modified by another request"),
		)
	}

	return nil
}

// Delete deletes a memory
// Allows deletion if user is: resource owner, team owner, or team admin
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *MemoryRepository) Delete(ctx context.Context, userID, teamID, memoryID string) error {
	query := `
		DELETE FROM memories
		WHERE id = $1
			AND team_id = $2
			AND (
				user_id = $3
				OR EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3 AND role IN ('owner', 'admin'))
			)
	`

	result, err := r.db.ExecContext(ctx, query, memoryID, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrMemoryNotFound
	}

	return nil
}

// buildMemorySearchWhereClause builds the shared WHERE conditions for
// SearchByMetadata. Count and page queries consume the same conditions, so they
// can never diverge. Columns are unqualified (no m. alias) because this path
// queries the memories table without an alias.
func buildMemorySearchWhereClause(
	userID, metadataKey, metadataValue string, filters repositories.MemoryFilters,
) squirrel.Sqlizer {
	where := squirrel.And{
		squirrel.Eq{"user_id": userID},
		// metadata ->> ? has no `?` operator, so no `??` escaping is needed.
		squirrel.Expr("metadata ->> ? = ?", metadataKey, metadataValue),
	}

	if filters.ProjectID != nil {
		where = append(where, squirrel.Eq{"project_id": *filters.ProjectID})
	}

	if filters.Search != "" {
		where = append(where, squirrel.Expr("text ILIKE ?", "%"+filters.Search+"%"))
	}

	return where
}

// SearchByMetadata searches memories by metadata key-value pairs
func (r *MemoryRepository) SearchByMetadata(
	ctx context.Context, userID string, metadataKey, metadataValue string, filters repositories.MemoryFilters,
) ([]models.Memory, int, error) {
	where := buildMemorySearchWhereClause(userID, metadataKey, metadataValue, filters)

	totalCount, err := r.countSearchByMetadata(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	memories, err := r.querySearchByMetadata(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return memories, totalCount, nil
}

// countSearchByMetadata counts memories matching the shared WHERE conditions
// used by SearchByMetadata, so the count and page queries can never diverge.
func (r *MemoryRepository) countSearchByMetadata(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("memories").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build memories count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count memories: %w", err)
	}

	return totalCount, nil
}

// querySearchByMetadata runs the paginated page query for SearchByMetadata using
// the shared WHERE conditions, so it can never diverge from the count query.
func (r *MemoryRepository) querySearchByMetadata(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.MemoryFilters,
) ([]models.Memory, error) {
	// Clamp to non-negative before the unsigned conversion squirrel requires;
	// negative paging inputs would otherwise wrap to huge offsets/limits.
	// Note: invalid paging now silently clamps to LIMIT 0 (empty page) instead
	// of erroring at Postgres — callers bypassing service-layer defaults get an
	// empty result, not an error.
	limit := uint64(0)
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	offset := uint64(0)
	if filters.Page > 1 && filters.Limit > 0 {
		if rawOffset := (filters.Page - 1) * filters.Limit; rawOffset > 0 {
			offset = uint64(rawOffset)
		}
	}

	query, args, err := psql.
		Select("id", "user_id", "team_id", "project_id", "text", "metadata", "created_at", "updated_at").
		From("memories").
		Where(where).
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build memories search query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search memories by metadata: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	memories, err := scanMemoryRows(rows)
	if err != nil {
		return nil, err
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate memories: %w", err)
	}

	return memories, nil
}

// GetNamesByIDsCrossTeam returns a map of memoryID → truncated text (first 60 chars) for the given
// IDs visible to userID across all teams the user belongs to (owner or member).
// Unknown or inaccessible IDs are omitted from the result map.
func (r *MemoryRepository) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	// Memories have no name column; use the first 60 characters of text as display value.
	// squirrel.Eq{"mem.id": ids} expands the IN list and binds each id.
	query, args, err := psql.
		Select("mem.id", "LEFT(mem.text, 60)").
		From("memories mem").
		Where(squirrel.Eq{"mem.id": ids}).
		Where(teamRowReadAccess("mem.team_id", userID)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build memory names query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get memory names by ids: %w", err)
	}

	result, scanErr := scanIDNameRows(rows, len(ids), "memory")
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close memory name rows: %w", closeErr)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}

// CountAll counts all memories for a user across all teams they have access to
func (r *MemoryRepository) CountAll(ctx context.Context, userID string) (int, error) {
	// Count memories in teams where user is owner or member (same logic as List)
	query := `
		SELECT COUNT(DISTINCT m.id)
		FROM memories m
		WHERE (
			EXISTS (SELECT 1 FROM teams WHERE id = m.team_id AND owner_id = $1)
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = m.team_id AND user_id = $1)
		)
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count memories: %w", err)
	}

	return count, nil
}
