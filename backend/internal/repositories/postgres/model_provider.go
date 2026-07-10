package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ModelProviderRepository implements the repositories.ModelProviderRepository interface for PostgreSQL
type ModelProviderRepository struct {
	db *database.DB
}

// NewModelProviderRepository creates a new ModelProviderRepository
func NewModelProviderRepository(db *database.DB) repositories.ModelProviderRepository {
	return &ModelProviderRepository{
		db: db,
	}
}

// Create creates a new model provider
func (r *ModelProviderRepository) Create(ctx context.Context, provider *models.ModelProvider) error {
	query := `
		INSERT INTO model_providers
		(user_id, team_id, name, provider_type, model,
		is_default, base_url, api_key_encrypted, configuration, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		provider.UserID, provider.TeamID, provider.Name, provider.ProviderType,
		provider.Model, provider.IsDefault, provider.BaseURL, provider.APIKeyEncrypted,
		provider.Configuration, provider.CreatedAt, provider.UpdatedAt,
	).Scan(&provider.ID, &provider.CreatedAt, &provider.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create model provider: %w", err)
	}

	return nil
}

// GetByID retrieves a model provider by its ID
func (r *ModelProviderRepository) GetByID(
	ctx context.Context, teamID, providerID string,
) (*models.ModelProvider, error) {
	query := `
		SELECT id, user_id, team_id, name, provider_type, model,
		is_default, base_url, api_key_encrypted, configuration, created_at, updated_at, version
		FROM model_providers
		WHERE id = $1 AND team_id = $2
	`

	var provider models.ModelProvider
	err := r.db.QueryRowContext(ctx, query, providerID, teamID).Scan(
		&provider.ID, &provider.UserID, &provider.TeamID, &provider.Name, &provider.ProviderType,
		&provider.Model, &provider.IsDefault, &provider.BaseURL, &provider.APIKeyEncrypted,
		&provider.Configuration, &provider.CreatedAt, &provider.UpdatedAt, &provider.Version,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get model provider by ID: %w", err),
			repositories.ErrModelProviderNotFound,
		)
	}

	return &provider, nil
}

// List retrieves model providers with filtering and pagination
func (r *ModelProviderRepository) List(
	ctx context.Context, teamID string, filters repositories.ModelProviderFilters,
) ([]models.ModelProvider, int, error) {
	// Shared WHERE conditions guarantee the count and page queries can never diverge.
	where := squirrel.And{squirrel.Eq{"team_id": teamID}}
	if filters.ProviderType != nil && *filters.ProviderType != "" {
		where = append(where, squirrel.Eq{"provider_type": *filters.ProviderType})
	}

	totalCount, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	providers, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return providers, totalCount, nil
}

// queryList runs the paginated page query for List using the shared WHERE
// conditions, so it can never diverge from the count query.
func (r *ModelProviderRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.ModelProviderFilters,
) ([]models.ModelProvider, error) {
	// Clamp to non-negative before the unsigned conversion squirrel requires;
	// negative paging inputs would otherwise wrap to huge offsets/limits.
	// Contract: a non-positive Limit emits LIMIT 0 (empty page). Callers must
	// pass Limit >= 1 and Page >= 1.
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
			"id", "user_id", "team_id", "name", "provider_type", "model",
			"is_default", "base_url", "api_key_encrypted", "configuration",
			"created_at", "updated_at",
		).
		From("model_providers").
		Where(where).
		OrderBy("is_default DESC", "created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build model providers list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list model providers: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	providers := make([]models.ModelProvider, 0)
	for rows.Next() {
		var provider models.ModelProvider
		scanErr := rows.Scan(
			&provider.ID, &provider.UserID, &provider.TeamID, &provider.Name, &provider.ProviderType,
			&provider.Model, &provider.IsDefault, &provider.BaseURL, &provider.APIKeyEncrypted,
			&provider.Configuration, &provider.CreatedAt, &provider.UpdatedAt,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan model provider: %w", scanErr)
		}
		providers = append(providers, provider)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate model providers: %w", err)
	}

	return providers, nil
}

// countList counts model providers matching the shared WHERE conditions used by
// List, so the count and page queries can never diverge.
func (r *ModelProviderRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("model_providers").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build model providers count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count model providers: %w", err)
	}

	return totalCount, nil
}

// Update updates an existing model provider using optimistic locking on version.
func (r *ModelProviderRepository) Update(ctx context.Context, provider *models.ModelProvider) error {
	query := `
		UPDATE model_providers
		SET name = $2, provider_type = $3, model = $4, is_default = $5, base_url = $6,
		api_key_encrypted = $7, configuration = $8, updated_at = $9, version = version + 1
		WHERE id = $1 AND team_id = $10 AND version = $11
		RETURNING updated_at, version
	`

	err := r.db.QueryRowContext(ctx, query,
		provider.ID, provider.Name, provider.ProviderType, provider.Model,
		provider.IsDefault, provider.BaseURL, provider.APIKeyEncrypted, provider.Configuration,
		provider.UpdatedAt, provider.TeamID, provider.Version,
	).Scan(&provider.UpdatedAt, &provider.Version)

	if err != nil {
		return mapNoRows(
			fmt.Errorf("failed to update model provider: %w", err),
			fmt.Errorf("model provider not found or version mismatch"),
		)
	}

	return nil
}

// Delete deletes a model provider
func (r *ModelProviderRepository) Delete(ctx context.Context, teamID, providerID string) error {
	query := `DELETE FROM model_providers WHERE id = $1 AND team_id = $2`

	result, err := r.db.ExecContext(ctx, query, providerID, teamID)
	if err != nil {
		return fmt.Errorf("failed to delete model provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrModelProviderNotFound
	}

	return nil
}

// GetDefault retrieves the default model provider for a team
func (r *ModelProviderRepository) GetDefault(
	ctx context.Context, teamID string,
) (*models.ModelProvider, error) {
	query := `
		SELECT id, user_id, team_id, name, provider_type, model,
		is_default, base_url, api_key_encrypted, configuration, created_at, updated_at
		FROM model_providers
		WHERE team_id = $1 AND is_default = true
		LIMIT 1
	`

	var provider models.ModelProvider
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&provider.ID, &provider.UserID, &provider.TeamID, &provider.Name, &provider.ProviderType,
		&provider.Model, &provider.IsDefault, &provider.BaseURL, &provider.APIKeyEncrypted,
		&provider.Configuration, &provider.CreatedAt, &provider.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get default model provider: %w", err),
			repositories.ErrDefaultModelProviderNotFound,
		)
	}

	return &provider, nil
}

// SetDefault sets a model provider as the default for a team
func (r *ModelProviderRepository) SetDefault(ctx context.Context, teamID, providerID string) error {
	// Start a transaction to ensure atomicity
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			slog.Error("Failed to rollback transaction", "error", rollbackErr)
		}
	}()

	// First, unset all defaults for the team
	_, err = tx.ExecContext(
		ctx,
		"UPDATE model_providers SET is_default = false WHERE team_id = $1",
		teamID,
	)
	if err != nil {
		return fmt.Errorf("failed to unset default providers: %w", err)
	}

	// Then set the specified provider as default
	result, err := tx.ExecContext(
		ctx,
		"UPDATE model_providers SET is_default = true WHERE id = $1 AND team_id = $2",
		providerID,
		teamID,
	)
	if err != nil {
		return fmt.Errorf("failed to set default provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrModelProviderNotFound
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UnsetAllDefaults unsets all default model providers for a team
func (r *ModelProviderRepository) UnsetAllDefaults(ctx context.Context, teamID string) error {
	query := `UPDATE model_providers SET is_default = false WHERE team_id = $1`

	_, err := r.db.ExecContext(ctx, query, teamID)
	if err != nil {
		return fmt.Errorf("failed to unset all default providers: %w", err)
	}

	return nil
}

// Count returns the total number of model providers for a team
func (r *ModelProviderRepository) Count(ctx context.Context, teamID string) (int, error) {
	query := `SELECT COUNT(*) FROM model_providers WHERE team_id = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count model providers: %w", err)
	}

	return count, nil
}
