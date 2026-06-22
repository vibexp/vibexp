package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// APIKeyRepository implements the repositories.APIKeyRepository interface for PostgreSQL
type APIKeyRepository struct {
	db *database.DB
}

// NewAPIKeyRepository creates a new APIKeyRepository
func NewAPIKeyRepository(db *database.DB) repositories.APIKeyRepository {
	return &APIKeyRepository{
		db: db,
	}
}

// Create creates a new API key with integration permissions
func (r *APIKeyRepository) Create(ctx context.Context, apiKey *models.APIKey) error {
	// Start transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
			slog.Error("Failed to rollback transaction", "error", rollbackErr)
		}
	}()

	// Insert API key with new unified prefix (is_legacy = false for new keys)
	query := `
		INSERT INTO api_keys (name, user_id, key_hash, key_prefix, is_legacy, created_at, updated_at)
		VALUES ($1, $2, $3, $4, false, $5, $6)
		RETURNING id, created_at, updated_at, version
	`

	err = tx.QueryRowContext(ctx, query,
		apiKey.Name, apiKey.UserID, apiKey.KeyHash, apiKey.KeyPrefix,
		apiKey.CreatedAt, apiKey.UpdatedAt,
	).Scan(&apiKey.ID, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.Version)

	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	// Insert integration permissions
	if len(apiKey.Integrations) > 0 {
		for _, integrationCode := range apiKey.Integrations {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO api_key_integration_permissions (api_key_id, integration_code)
				 VALUES ($1, $2)`,
				apiKey.ID, integrationCode,
			)
			if err != nil {
				return fmt.Errorf("failed to insert integration permission: %w", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetByUserID retrieves all API keys for a specific user
func (r *APIKeyRepository) GetByUserID(ctx context.Context, userID string) ([]models.APIKey, error) {
	query := `
		SELECT id, name, user_id, key_hash, key_prefix, usage_type, is_legacy, migration_notes,
		       last_used_at, expires_at, created_at, updated_at, version
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys by user ID: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	apiKeys := make([]models.APIKey, 0)
	for rows.Next() {
		var apiKey models.APIKey
		scanErr := rows.Scan(
			&apiKey.ID, &apiKey.Name, &apiKey.UserID, &apiKey.KeyHash,
			&apiKey.KeyPrefix, &apiKey.UsageType, &apiKey.IsLegacy, &apiKey.MigrationNotes,
			&apiKey.LastUsedAt, &apiKey.ExpiresAt, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.Version,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", scanErr)
		}

		// Load integrations for this key
		integrations, intErr := r.GetIntegrationsByAPIKeyID(ctx, apiKey.ID)
		if intErr == nil {
			apiKey.Integrations = integrations
		}

		apiKeys = append(apiKeys, apiKey)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate API keys: %w", err)
	}

	return apiKeys, nil
}

// GetByKeyHash retrieves an API key by its hash
func (r *APIKeyRepository) GetByKeyHash(ctx context.Context, keyHash string) (*models.APIKey, error) {
	// Reject expired keys at the query layer: a key is valid only while it has no
	// expiry (expires_at IS NULL) or its expiry is still in the future.
	query := `
		SELECT id, name, user_id, key_hash, key_prefix, usage_type, is_legacy, migration_notes,
		       last_used_at, expires_at, created_at, updated_at, version
		FROM api_keys
		WHERE key_hash = $1
		  AND (expires_at IS NULL OR expires_at > NOW())
	`

	var apiKey models.APIKey
	err := r.db.QueryRowContext(ctx, query, keyHash).Scan(
		&apiKey.ID, &apiKey.Name, &apiKey.UserID, &apiKey.KeyHash,
		&apiKey.KeyPrefix, &apiKey.UsageType, &apiKey.IsLegacy, &apiKey.MigrationNotes,
		&apiKey.LastUsedAt, &apiKey.ExpiresAt, &apiKey.CreatedAt, &apiKey.UpdatedAt, &apiKey.Version,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get API key by hash: %w", err), repositories.ErrAPIKeyNotFound)
	}

	// Load integrations
	integrations, intErr := r.GetIntegrationsByAPIKeyID(ctx, apiKey.ID)
	if intErr != nil {
		return nil, fmt.Errorf("failed to load integrations: %w", intErr)
	}
	apiKey.Integrations = integrations

	return &apiKey, nil
}

// Delete deletes an API key
func (r *APIKeyRepository) Delete(ctx context.Context, userID, keyID string) error {
	query := `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, keyID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrAPIKeyNotFound
	}

	return nil
}

// UpdateLastUsed updates the last used timestamp for an API key
func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, keyID string, lastUsedAt time.Time) error {
	query := `
		UPDATE api_keys
		SET last_used_at = $2, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, keyID, lastUsedAt)
	if err != nil {
		return fmt.Errorf("failed to update last used timestamp: %w", err)
	}

	return nil
}

// GetIntegrationsByAPIKeyID retrieves all integration codes for a specific API key
func (r *APIKeyRepository) GetIntegrationsByAPIKeyID(ctx context.Context, apiKeyID string) ([]string, error) {
	query := `
		SELECT integration_code
		FROM api_key_integration_permissions
		WHERE api_key_id = $1
		ORDER BY granted_at
	`

	rows, err := r.db.QueryContext(ctx, query, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get integrations: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	integrations := make([]string, 0)
	for rows.Next() {
		var code string
		if scanErr := rows.Scan(&code); scanErr != nil {
			return nil, fmt.Errorf("failed to scan integration code: %w", scanErr)
		}
		integrations = append(integrations, code)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate integrations: %w", err)
	}

	return integrations, nil
}

// HasIntegrationPermission checks if an API key has permission for a specific integration
func (r *APIKeyRepository) HasIntegrationPermission(
	ctx context.Context, apiKeyID, integrationCode string,
) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM api_key_integration_permissions
		WHERE api_key_id = $1 AND integration_code = $2
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, apiKeyID, integrationCode).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check integration permission: %w", err)
	}

	return count > 0, nil
}

// GetNamesByIDs returns a map of apiKeyID → name for the given IDs owned by userID.
// Unknown or inaccessible IDs are omitted from the result.
func (r *APIKeyRepository) GetNamesByIDs(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	// $1 is userID; api key IDs start at $2
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = userID
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(
		`SELECT id, name FROM api_keys WHERE id IN (%s) AND user_id = $1`,
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get api key names by ids: %w", err)
	}

	result, scanErr := scanIDNameRows(rows, len(ids), "api_key")
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close api key name rows: %w", closeErr)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}

// GetValidIntegrationCodes retrieves all active integration codes from catalog
func (r *APIKeyRepository) GetValidIntegrationCodes(ctx context.Context) ([]string, error) {
	query := `
		SELECT integration_code
		FROM api_key_integrations_catalog
		WHERE is_active = true
		ORDER BY integration_code
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get valid integration codes: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	codes := make([]string, 0)
	for rows.Next() {
		var code string
		if scanErr := rows.Scan(&code); scanErr != nil {
			return nil, fmt.Errorf("failed to scan integration code: %w", scanErr)
		}
		codes = append(codes, code)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate integration codes: %w", err)
	}

	return codes, nil
}
