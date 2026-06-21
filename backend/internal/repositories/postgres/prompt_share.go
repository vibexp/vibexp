package postgres

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// PromptShareRepository implements the repositories.PromptShareRepository interface for PostgreSQL
type PromptShareRepository struct {
	db *database.DB
}

// NewPromptShareRepository creates a new PromptShareRepository
func NewPromptShareRepository(db *database.DB) repositories.PromptShareRepository {
	return &PromptShareRepository{
		db: db,
	}
}

// Create creates a new prompt share
func (r *PromptShareRepository) Create(ctx context.Context, share *models.PromptShare) error {
	query := `
		INSERT INTO prompt_shares (
			prompt_id, share_token, share_type, created_by,
			created_at, expires_at, is_active, access_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`

	err := r.db.QueryRowContext(ctx, query,
		share.PromptID, share.ShareToken, share.ShareType, share.CreatedBy,
		share.CreatedAt, share.ExpiresAt, share.IsActive, share.AccessCount,
	).Scan(&share.ID, &share.CreatedAt)

	if err != nil {
		if uniqueViolation(err) != nil {
			return fmt.Errorf("share already exists for this prompt")
		}
		return fmt.Errorf("failed to create prompt share: %w", err)
	}

	return nil
}

// GetByToken retrieves a prompt share by its token
func (r *PromptShareRepository) GetByToken(ctx context.Context, token string) (*models.PromptShare, error) {
	query := `
		SELECT id, prompt_id, share_token, share_type, created_by, created_at, expires_at, is_active, access_count, version
		FROM prompt_shares
		WHERE share_token = $1
	`

	var share models.PromptShare
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&share.ID, &share.PromptID, &share.ShareToken, &share.ShareType,
		&share.CreatedBy, &share.CreatedAt, &share.ExpiresAt, &share.IsActive, &share.AccessCount, &share.Version,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get share by token: %w", err), repositories.ErrPromptShareNotFound)
	}

	return &share, nil
}

// GetByPromptID retrieves a prompt share by prompt ID
func (r *PromptShareRepository) GetByPromptID(ctx context.Context, promptID string) (*models.PromptShare, error) {
	query := `
		SELECT id, prompt_id, share_token, share_type, created_by, created_at, expires_at, is_active, access_count, version
		FROM prompt_shares
		WHERE prompt_id = $1
	`

	var share models.PromptShare
	err := r.db.QueryRowContext(ctx, query, promptID).Scan(
		&share.ID, &share.PromptID, &share.ShareToken, &share.ShareType,
		&share.CreatedBy, &share.CreatedAt, &share.ExpiresAt, &share.IsActive, &share.AccessCount, &share.Version,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get share by prompt ID: %w", err), repositories.ErrPromptShareNotFound)
	}

	return &share, nil
}

// Update updates a prompt share
func (r *PromptShareRepository) Update(ctx context.Context, share *models.PromptShare) error {
	query := `
		UPDATE prompt_shares
		SET share_type = $1, expires_at = $2, is_active = $3, version = version + 1
		WHERE id = $4 AND version = $5
		RETURNING version
	`

	err := r.db.QueryRowContext(ctx, query,
		share.ShareType, share.ExpiresAt, share.IsActive, share.ID, share.Version,
	).Scan(&share.Version)

	if err != nil {
		return mapNoRows(
			fmt.Errorf("failed to update prompt share: %w", err),
			fmt.Errorf("share not found or version mismatch"),
		)
	}

	return nil
}

// Delete deletes a prompt share
func (r *PromptShareRepository) Delete(ctx context.Context, shareID string) error {
	query := `DELETE FROM prompt_shares WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, shareID)
	if err != nil {
		return fmt.Errorf("failed to delete prompt share: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrPromptShareNotFound
	}

	return nil
}

// IncrementAccessCount increments the access count for a share
func (r *PromptShareRepository) IncrementAccessCount(ctx context.Context, shareID string) error {
	query := `UPDATE prompt_shares SET access_count = access_count + 1 WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, shareID)
	if err != nil {
		return fmt.Errorf("failed to increment access count: %w", err)
	}

	return nil
}

// AddAccessEmails adds email addresses to the share access list
func (r *PromptShareRepository) AddAccessEmails(ctx context.Context, shareID string, emails []string) error {
	if len(emails) == 0 {
		return nil
	}

	// Start transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		//nolint:errcheck // Rollback is safe to ignore if Commit succeeds
		_ = tx.Rollback()
	}()

	// First, delete existing access entries for this share
	deleteQuery := `DELETE FROM prompt_share_access WHERE share_id = $1`
	_, err = tx.ExecContext(ctx, deleteQuery, shareID)
	if err != nil {
		return fmt.Errorf("failed to delete existing access entries: %w", err)
	}

	// Insert new access entries
	insertQuery := `
		INSERT INTO prompt_share_access (share_id, email, granted_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (share_id, email) DO NOTHING
	`

	for _, email := range emails {
		_, err := tx.ExecContext(ctx, insertQuery, shareID, email)
		if err != nil {
			return fmt.Errorf("failed to add access email: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RemoveAccessEmail removes an email address from the share access list
func (r *PromptShareRepository) RemoveAccessEmail(ctx context.Context, shareID, email string) error {
	query := `DELETE FROM prompt_share_access WHERE share_id = $1 AND email = $2`

	_, err := r.db.ExecContext(ctx, query, shareID, email)
	if err != nil {
		return fmt.Errorf("failed to remove access email: %w", err)
	}

	return nil
}

// GetAccessEmails retrieves all email addresses with access to a share
func (r *PromptShareRepository) GetAccessEmails(ctx context.Context, shareID string) ([]string, error) {
	query := `SELECT email FROM prompt_share_access WHERE share_id = $1 ORDER BY email`

	rows, err := r.db.QueryContext(ctx, query, shareID)
	if err != nil {
		return nil, fmt.Errorf("failed to get access emails: %w", err)
	}
	defer func() {
		//nolint:errcheck // Close is safe to ignore
		_ = rows.Close()
	}()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		emails = append(emails, email)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over emails: %w", err)
	}

	return emails, nil
}

// HasAccess checks if an email has access to a share
func (r *PromptShareRepository) HasAccess(ctx context.Context, shareID, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM prompt_share_access WHERE share_id = $1 AND email = $2)`

	var hasAccess bool
	err := r.db.QueryRowContext(ctx, query, shareID, email).Scan(&hasAccess)
	if err != nil {
		return false, fmt.Errorf("failed to check access: %w", err)
	}

	return hasAccess, nil
}
