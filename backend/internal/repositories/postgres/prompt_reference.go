package postgres

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// PromptReferenceRepository implements the repositories.PromptReferenceRepository interface for PostgreSQL
type PromptReferenceRepository struct {
	db *database.DB
}

// NewPromptReferenceRepository creates a new PromptReferenceRepository
func NewPromptReferenceRepository(db *database.DB) repositories.PromptReferenceRepository {
	return &PromptReferenceRepository{
		db: db,
	}
}

// CreateBatch creates multiple prompt references
func (r *PromptReferenceRepository) CreateBatch(ctx context.Context, references []models.PromptReference) error {
	if len(references) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err.Error() != "sql: transaction has already been committed or rolled back" {
			logrus.WithError(err).Error("Failed to rollback transaction")
		}
	}()

	query := `
		INSERT INTO prompt_references (prompt_id, referenced_prompt_id)
		VALUES ($1, $2)
		ON CONFLICT (prompt_id, referenced_prompt_id) DO NOTHING
	`

	for _, ref := range references {
		_, err := tx.ExecContext(ctx, query, ref.PromptID, ref.ReferencedPromptID)
		if err != nil {
			return fmt.Errorf("failed to create prompt reference: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteByPromptID deletes all references for a prompt
func (r *PromptReferenceRepository) DeleteByPromptID(ctx context.Context, promptID string) error {
	query := `DELETE FROM prompt_references WHERE prompt_id = $1`

	_, err := r.db.ExecContext(ctx, query, promptID)
	if err != nil {
		return fmt.Errorf("failed to delete prompt references: %w", err)
	}

	return nil
}

// GetPromptsUsingPrompt returns prompts that reference the given prompt (used by)
func (r *PromptReferenceRepository) GetPromptsUsingPrompt(
	ctx context.Context,
	userID, promptID string,
) ([]models.PromptDependencyInfo, error) {
	query := `
		SELECT p.id, p.slug, p.name
		FROM prompt_references pr
		INNER JOIN prompts p ON pr.prompt_id = p.id
		WHERE pr.referenced_prompt_id = $1 AND p.user_id = $2
		ORDER BY p.name
	`

	rows, err := r.db.QueryContext(ctx, query, promptID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompts using prompt: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	// Initialize as empty array instead of nil to ensure JSON serialization as [] not null
	results := make([]models.PromptDependencyInfo, 0)
	for rows.Next() {
		var info models.PromptDependencyInfo
		if scanErr := rows.Scan(&info.ID, &info.Slug, &info.Name); scanErr != nil {
			return nil, fmt.Errorf("failed to scan prompt dependency info: %w", scanErr)
		}
		results = append(results, info)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate prompts using prompt: %w", err)
	}

	return results, nil
}

// GetPromptsUsedByPrompt returns prompts that are referenced by the given prompt (uses)
func (r *PromptReferenceRepository) GetPromptsUsedByPrompt(
	ctx context.Context,
	userID, promptID string,
) ([]models.PromptDependencyInfo, error) {
	query := `
		SELECT p.id, p.slug, p.name
		FROM prompt_references pr
		INNER JOIN prompts p ON pr.referenced_prompt_id = p.id
		WHERE pr.prompt_id = $1 AND p.user_id = $2
		ORDER BY p.name
	`

	rows, err := r.db.QueryContext(ctx, query, promptID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompts used by prompt: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	// Initialize as empty array instead of nil to ensure JSON serialization as [] not null
	results := make([]models.PromptDependencyInfo, 0)
	for rows.Next() {
		var info models.PromptDependencyInfo
		if scanErr := rows.Scan(&info.ID, &info.Slug, &info.Name); scanErr != nil {
			return nil, fmt.Errorf("failed to scan prompt dependency info: %w", scanErr)
		}
		results = append(results, info)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate prompts used by prompt: %w", err)
	}

	return results, nil
}

// HasDependents checks if a prompt is referenced by any other prompts
func (r *PromptReferenceRepository) HasDependents(ctx context.Context, promptID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM prompt_references WHERE referenced_prompt_id = $1)`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, promptID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if prompt has dependents: %w", err)
	}

	return exists, nil
}
