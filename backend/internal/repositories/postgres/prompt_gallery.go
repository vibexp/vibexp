package postgres

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// PromptGalleryRepository implements the repositories.PromptGalleryRepository interface for PostgreSQL
type PromptGalleryRepository struct {
	db *database.DB
}

// NewPromptGalleryRepository creates a new PromptGalleryRepository
func NewPromptGalleryRepository(db *database.DB) repositories.PromptGalleryRepository {
	return &PromptGalleryRepository{
		db: db,
	}
}

// GetCategories retrieves all categories with prompt counts
func (r *PromptGalleryRepository) GetCategories(ctx context.Context) ([]models.PromptGalleryCategory, error) {
	query := `
		SELECT category, COUNT(*) as count
		FROM prompt_gallery_templates
		GROUP BY category
		ORDER BY category
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	// Initialize as empty slice to ensure JSON encodes as [] not null
	categories := make([]models.PromptGalleryCategory, 0)
	for rows.Next() {
		var category models.PromptGalleryCategory
		if err := rows.Scan(&category.Category, &category.Count); err != nil {
			return nil, fmt.Errorf("failed to scan category: %w", err)
		}
		categories = append(categories, category)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating categories: %w", err)
	}

	return categories, nil
}

// buildPromptGalleryConditions builds the shared, all-optional WHERE conditions
// for the List count and page queries, so the two can never diverge. Predicate
// set and trigger conditions match the prior hand-built builder exactly; when no
// filter is set the slice is empty and callers must omit the WHERE clause.
//
// The JSONB `?|` operator is written as `??|` inside squirrel.Expr: squirrel
// treats every `?` as a placeholder and unescapes a doubled `??` back to a
// literal `?`, so the emitted SQL is the intended `tags ?| $N` (verified by
// TestPromptGalleryRepository_List_JSONBTagsOperator).
func buildPromptGalleryConditions(filters repositories.PromptGalleryFilters) squirrel.And {
	conditions := squirrel.And{}

	if filters.Category != "" {
		conditions = append(conditions, squirrel.Eq{"category": filters.Category})
	}
	if filters.Search != "" {
		pattern := "%" + filters.Search + "%"
		conditions = append(conditions, squirrel.Or{
			squirrel.ILike{"title": pattern},
			squirrel.ILike{"description": pattern},
		})
	}
	if len(filters.Tags) > 0 {
		conditions = append(conditions, squirrel.Expr("tags ??| ?", pq.Array(filters.Tags)))
	}

	return conditions
}

// List retrieves prompts based on filters
func (r *PromptGalleryRepository) List(
	ctx context.Context, filters repositories.PromptGalleryFilters,
) ([]models.PromptGalleryTemplate, int, error) {
	conditions := buildPromptGalleryConditions(filters)

	total, err := r.countList(ctx, conditions)
	if err != nil {
		return nil, 0, err
	}

	prompts, err := r.queryList(ctx, conditions, filters)
	if err != nil {
		return nil, 0, err
	}

	return prompts, total, nil
}

// countList counts prompts matching the shared filter conditions used by List,
// so the count and page queries can never diverge.
func (r *PromptGalleryRepository) countList(ctx context.Context, conditions squirrel.And) (int, error) {
	builder := psql.Select("COUNT(*)").From("prompt_gallery_templates")
	if len(conditions) > 0 {
		builder = builder.Where(conditions)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build prompts count query: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count prompts: %w", err)
	}

	return total, nil
}

// queryList runs the paginated page query for List using the same shared filter
// conditions as the count query. The returned slice is always non-nil so JSON
// encodes as [] not null.
func (r *PromptGalleryRepository) queryList(
	ctx context.Context, conditions squirrel.And, filters repositories.PromptGalleryFilters,
) ([]models.PromptGalleryTemplate, error) {
	builder := psql.
		Select("id", "title", "description", "content", "category", "tags", "metadata", "created_at", "updated_at").
		From("prompt_gallery_templates")
	if len(conditions) > 0 {
		builder = builder.Where(conditions)
	}

	limit, offset := promptGalleryPaging(filters)
	query, args, err := builder.
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build prompts list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list prompts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	// Initialize as empty slice to ensure JSON encodes as [] not null
	prompts := make([]models.PromptGalleryTemplate, 0)
	for rows.Next() {
		var prompt models.PromptGalleryTemplate
		if err := rows.Scan(
			&prompt.ID, &prompt.Title, &prompt.Description, &prompt.Content,
			&prompt.Category, &prompt.Tags, &prompt.Metadata, &prompt.CreatedAt, &prompt.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan prompt: %w", err)
		}
		prompts = append(prompts, prompt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating prompts: %w", err)
	}

	return prompts, nil
}

// promptGalleryPaging resolves the LIMIT/OFFSET for the List page query,
// preserving the prior contract: offset = (Page-1)*Limit. Negative results are
// clamped to 0 (provably non-wrapping for gosec G115); Postgres rejects negative
// LIMIT/OFFSET regardless.
func promptGalleryPaging(filters repositories.PromptGalleryFilters) (limit, offset uint64) {
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	if rawOffset := (filters.Page - 1) * filters.Limit; rawOffset > 0 {
		offset = uint64(rawOffset)
	}
	return limit, offset
}

// GetByID retrieves a prompt by its ID
func (r *PromptGalleryRepository) GetByID(ctx context.Context, promptID string) (*models.PromptGalleryTemplate, error) {
	query := `
		SELECT id, title, description, content, category, tags, metadata, created_at, updated_at
		FROM prompt_gallery_templates
		WHERE id = $1
	`

	var prompt models.PromptGalleryTemplate
	err := r.db.QueryRowContext(ctx, query, promptID).Scan(
		&prompt.ID, &prompt.Title, &prompt.Description, &prompt.Content,
		&prompt.Category, &prompt.Tags, &prompt.Metadata, &prompt.CreatedAt, &prompt.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get prompt by ID: %w", err), repositories.ErrPromptNotFound)
	}

	return &prompt, nil
}
