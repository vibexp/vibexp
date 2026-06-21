package postgres

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// EmbeddingProviderRepository implements the repositories.EmbeddingProviderRepository interface for PostgreSQL
type EmbeddingProviderRepository struct {
	db *database.DB
}

// NewEmbeddingProviderRepository creates a new EmbeddingProviderRepository
func NewEmbeddingProviderRepository(db *database.DB) repositories.EmbeddingProviderRepository {
	return &EmbeddingProviderRepository{
		db: db,
	}
}

// Create creates a new embedding provider
func (r *EmbeddingProviderRepository) Create(ctx context.Context, provider *models.EmbeddingProvider) error {
	query := `
		INSERT INTO embedding_providers
		(user_id, name, provider_type, is_default, base_url, api_key_encrypted, configuration, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		provider.UserID, provider.Name, provider.ProviderType, provider.IsDefault,
		provider.BaseURL, provider.APIKeyEncrypted, provider.Configuration,
		provider.CreatedAt, provider.UpdatedAt,
	).Scan(&provider.ID, &provider.CreatedAt, &provider.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create embedding provider: %w", err)
	}

	return nil
}

// GetByID retrieves an embedding provider by its ID
func (r *EmbeddingProviderRepository) GetByID(
	ctx context.Context, userID, providerID string,
) (*models.EmbeddingProvider, error) {
	query := `
		SELECT id, user_id, name, provider_type, is_default, base_url,
		api_key_encrypted, configuration, created_at, updated_at, version
		FROM embedding_providers
		WHERE id = $1 AND user_id = $2
	`

	var provider models.EmbeddingProvider
	err := r.db.QueryRowContext(ctx, query, providerID, userID).Scan(
		&provider.ID, &provider.UserID, &provider.Name, &provider.ProviderType,
		&provider.IsDefault, &provider.BaseURL, &provider.APIKeyEncrypted,
		&provider.Configuration, &provider.CreatedAt, &provider.UpdatedAt, &provider.Version,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get embedding provider by ID: %w", err),
			repositories.ErrEmbeddingProviderNotFound,
		)
	}

	return &provider, nil
}

// List retrieves embedding providers with filtering and pagination
func (r *EmbeddingProviderRepository) List(
	ctx context.Context, userID string, filters repositories.EmbeddingProviderFilters,
) ([]models.EmbeddingProvider, int, error) {
	// Shared WHERE conditions guarantee the count and page queries can never diverge.
	where := squirrel.And{squirrel.Eq{"user_id": userID}}
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
func (r *EmbeddingProviderRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.EmbeddingProviderFilters,
) ([]models.EmbeddingProvider, error) {
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
			"id", "user_id", "name", "provider_type", "is_default", "base_url",
			"api_key_encrypted", "configuration", "created_at", "updated_at",
		).
		From("embedding_providers").
		Where(where).
		OrderBy("is_default DESC", "created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build embedding providers list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list embedding providers: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	providers := make([]models.EmbeddingProvider, 0)
	for rows.Next() {
		var provider models.EmbeddingProvider
		scanErr := rows.Scan(
			&provider.ID, &provider.UserID, &provider.Name, &provider.ProviderType,
			&provider.IsDefault, &provider.BaseURL, &provider.APIKeyEncrypted,
			&provider.Configuration, &provider.CreatedAt, &provider.UpdatedAt,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan embedding provider: %w", scanErr)
		}
		providers = append(providers, provider)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate embedding providers: %w", err)
	}

	return providers, nil
}

// countList counts embedding providers matching the shared WHERE conditions
// used by List, so the count and page queries can never diverge.
func (r *EmbeddingProviderRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("embedding_providers").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build embedding providers count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count embedding providers: %w", err)
	}

	return totalCount, nil
}

// Update updates an existing embedding provider
func (r *EmbeddingProviderRepository) Update(ctx context.Context, provider *models.EmbeddingProvider) error {
	query := `
		UPDATE embedding_providers
		SET name = $2, provider_type = $3, is_default = $4, base_url = $5,
		api_key_encrypted = $6, configuration = $7, updated_at = $8, version = version + 1
		WHERE id = $1 AND user_id = $9 AND version = $10
		RETURNING updated_at, version
	`

	err := r.db.QueryRowContext(ctx, query,
		provider.ID, provider.Name, provider.ProviderType, provider.IsDefault,
		provider.BaseURL, provider.APIKeyEncrypted, provider.Configuration,
		provider.UpdatedAt, provider.UserID, provider.Version,
	).Scan(&provider.UpdatedAt, &provider.Version)

	if err != nil {
		return mapNoRows(
			fmt.Errorf("failed to update embedding provider: %w", err),
			fmt.Errorf("embedding provider not found or version mismatch"),
		)
	}

	return nil
}

// Delete deletes an embedding provider
func (r *EmbeddingProviderRepository) Delete(ctx context.Context, userID, providerID string) error {
	query := `DELETE FROM embedding_providers WHERE id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, providerID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete embedding provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrEmbeddingProviderNotFound
	}

	return nil
}

// GetDefault retrieves the default embedding provider for a user
func (r *EmbeddingProviderRepository) GetDefault(
	ctx context.Context, userID string,
) (*models.EmbeddingProvider, error) {
	query := `
		SELECT id, user_id, name, provider_type, is_default, base_url,
		api_key_encrypted, configuration, created_at, updated_at
		FROM embedding_providers
		WHERE user_id = $1 AND is_default = true
		LIMIT 1
	`

	var provider models.EmbeddingProvider
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&provider.ID, &provider.UserID, &provider.Name, &provider.ProviderType,
		&provider.IsDefault, &provider.BaseURL, &provider.APIKeyEncrypted,
		&provider.Configuration, &provider.CreatedAt, &provider.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get default embedding provider: %w", err),
			repositories.ErrDefaultEmbeddingProviderNotFound,
		)
	}

	return &provider, nil
}

// SetDefault sets an embedding provider as the default for a user
func (r *EmbeddingProviderRepository) SetDefault(ctx context.Context, userID, providerID string) error {
	// Start a transaction to ensure atomicity
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			logrus.WithError(rollbackErr).Error("Failed to rollback transaction")
		}
	}()

	// First, unset all defaults for the user
	_, err = tx.ExecContext(
		ctx,
		"UPDATE embedding_providers SET is_default = false WHERE user_id = $1",
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to unset default providers: %w", err)
	}

	// Then set the specified provider as default
	result, err := tx.ExecContext(
		ctx,
		"UPDATE embedding_providers SET is_default = true WHERE id = $1 AND user_id = $2",
		providerID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to set default provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrEmbeddingProviderNotFound
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UnsetAllDefaults unsets all default embedding providers for a user
func (r *EmbeddingProviderRepository) UnsetAllDefaults(ctx context.Context, userID string) error {
	query := `UPDATE embedding_providers SET is_default = false WHERE user_id = $1`

	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to unset all default providers: %w", err)
	}

	return nil
}

// Count returns the total number of embedding providers for a user
func (r *EmbeddingProviderRepository) Count(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM embedding_providers WHERE user_id = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count embedding providers: %w", err)
	}

	return count, nil
}
