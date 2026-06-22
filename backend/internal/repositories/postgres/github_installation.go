package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// GitHubInstallationRepository is the PostgreSQL implementation of repositories.GitHubInstallationRepository
type GitHubInstallationRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewGitHubInstallationRepository creates a new GitHub installation repository
func NewGitHubInstallationRepository(db *sql.DB, logger *slog.Logger) repositories.GitHubInstallationRepository {
	return &GitHubInstallationRepository{db: db, logger: logger}
}

// Create inserts a new GitHub installation
func (r *GitHubInstallationRepository) Create(ctx context.Context, installation *models.GitHubInstallation) error {
	permissionsJSON, err := json.Marshal(installation.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %w", err)
	}

	query := `
		INSERT INTO github_installations (
			id, team_id, installation_id, account_login, account_type, target_type,
			encrypted_access_token, token_expires_at, permissions, events, suspended_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at
	`

	err = r.db.QueryRowContext(
		ctx, query,
		installation.ID,
		installation.TeamID,
		installation.InstallationID,
		installation.AccountLogin,
		installation.AccountType,
		installation.TargetType,
		installation.EncryptedAccessToken,
		installation.TokenExpiresAt,
		permissionsJSON,
		pq.Array(installation.Events),
		installation.SuspendedAt,
	).Scan(&installation.CreatedAt, &installation.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create GitHub installation: %w", err)
	}

	return nil
}

// GetByTeamID retrieves a GitHub installation by team ID
func (r *GitHubInstallationRepository) GetByTeamID(
	ctx context.Context,
	teamID string,
) (*models.GitHubInstallation, error) {
	installation := &models.GitHubInstallation{}
	var permissionsJSON []byte

	query := `
		SELECT id, team_id, installation_id, account_login, account_type, target_type,
			   encrypted_access_token, token_expires_at, permissions, events, suspended_at,
			   created_at, updated_at
		FROM github_installations
		WHERE team_id = $1
	`

	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&installation.ID,
		&installation.TeamID,
		&installation.InstallationID,
		&installation.AccountLogin,
		&installation.AccountType,
		&installation.TargetType,
		&installation.EncryptedAccessToken,
		&installation.TokenExpiresAt,
		&permissionsJSON,
		pq.Array(&installation.Events),
		&installation.SuspendedAt,
		&installation.CreatedAt,
		&installation.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get GitHub installation: %w", err),
			repositories.ErrGitHubInstallationNotFound,
		)
	}

	if err := json.Unmarshal(permissionsJSON, &installation.Permissions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
	}

	return installation, nil
}

// GetByInstallationID retrieves a GitHub installation by installation ID
func (r *GitHubInstallationRepository) GetByInstallationID(
	ctx context.Context,
	installationID int64,
) (*models.GitHubInstallation, error) {
	installation := &models.GitHubInstallation{}
	var permissionsJSON []byte

	query := `
		SELECT id, team_id, installation_id, account_login, account_type, target_type,
			   encrypted_access_token, token_expires_at, permissions, events, suspended_at,
			   created_at, updated_at
		FROM github_installations
		WHERE installation_id = $1
	`

	err := r.db.QueryRowContext(ctx, query, installationID).Scan(
		&installation.ID,
		&installation.TeamID,
		&installation.InstallationID,
		&installation.AccountLogin,
		&installation.AccountType,
		&installation.TargetType,
		&installation.EncryptedAccessToken,
		&installation.TokenExpiresAt,
		&permissionsJSON,
		pq.Array(&installation.Events),
		&installation.SuspendedAt,
		&installation.CreatedAt,
		&installation.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get GitHub installation: %w", err),
			repositories.ErrGitHubInstallationNotFound,
		)
	}

	if err := json.Unmarshal(permissionsJSON, &installation.Permissions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
	}

	return installation, nil
}

// Update updates a GitHub installation
func (r *GitHubInstallationRepository) Update(ctx context.Context, installation *models.GitHubInstallation) error {
	permissionsJSON, err := json.Marshal(installation.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %w", err)
	}

	query := `
		UPDATE github_installations
		SET installation_id = $1, account_login = $2, account_type = $3, target_type = $4,
			encrypted_access_token = $5, token_expires_at = $6, permissions = $7,
			events = $8, suspended_at = $9, updated_at = NOW()
		WHERE id = $10
	`

	result, err := r.db.ExecContext(
		ctx, query,
		installation.InstallationID,
		installation.AccountLogin,
		installation.AccountType,
		installation.TargetType,
		installation.EncryptedAccessToken,
		installation.TokenExpiresAt,
		permissionsJSON,
		pq.Array(installation.Events),
		installation.SuspendedAt,
		installation.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update GitHub installation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrGitHubInstallationNotFound
	}

	return nil
}

// Delete removes a GitHub installation
func (r *GitHubInstallationRepository) Delete(ctx context.Context, teamID string) error {
	query := `DELETE FROM github_installations WHERE team_id = $1`

	result, err := r.db.ExecContext(ctx, query, teamID)
	if err != nil {
		return fmt.Errorf("failed to delete GitHub installation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrGitHubInstallationNotFound
	}

	return nil
}
