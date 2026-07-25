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
			id, team_id, app_config_id, installation_id, account_login, account_type, target_type,
			encrypted_access_token, token_expires_at, permissions, events, suspended_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at
	`

	err = r.db.QueryRowContext(
		ctx, query,
		installation.ID,
		installation.TeamID,
		installation.AppConfigID,
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

// githubInstallationColumnList is the projection every installation read uses.
// One definition keeps the three readers from drifting apart — a column added
// to one and forgotten in another scans into the wrong field.
const githubInstallationColumnList = `id, team_id, app_config_id, installation_id, account_login,
	account_type, target_type, encrypted_access_token, token_expires_at, permissions, events,
	suspended_at, created_at, updated_at`

// The three read queries are package-level CONSTANTS, not strings assembled at
// call time. Every part is a compile-time constant, so no caller input can
// reach the SQL text — and building them here rather than inline keeps that
// obvious to a reader (and to static analysis) instead of looking like dynamic
// SQL construction.
const (
	getInstallationByTeamQuery = `SELECT ` + githubInstallationColumnList + `
		FROM github_installations
		WHERE team_id = $1`

	getInstallationByIDQuery = `SELECT ` + githubInstallationColumnList + `
		FROM github_installations
		WHERE installation_id = $1`

	getInstallationByAppConfigQuery = `SELECT ` + githubInstallationColumnList + `
		FROM github_installations
		WHERE app_config_id = $1 AND installation_id = $2`
)

// scanInstallation reads one row in githubInstallationColumnList order and
// unmarshals the JSONB permissions blob.
func scanInstallation(row rowScanner) (*models.GitHubInstallation, error) {
	installation := &models.GitHubInstallation{}
	var permissionsJSON []byte

	err := row.Scan(
		&installation.ID,
		&installation.TeamID,
		&installation.AppConfigID,
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

// GetByTeamID retrieves a GitHub installation by team ID
func (r *GitHubInstallationRepository) GetByTeamID(
	ctx context.Context,
	teamID string,
) (*models.GitHubInstallation, error) {
	return scanInstallation(r.db.QueryRowContext(ctx, getInstallationByTeamQuery, teamID))
}

// GetByInstallationID retrieves a GitHub installation by installation ID.
//
// This read is NOT scoped to an App, and installation_id stopped being globally
// unique in #477 — two teams may install their own Apps on the same GitHub org.
// Use GetByAppConfigAndInstallationID wherever the App is known; this one
// remains for the reconnection check, which deliberately looks across Apps.
func (r *GitHubInstallationRepository) GetByInstallationID(
	ctx context.Context,
	installationID int64,
) (*models.GitHubInstallation, error) {
	return scanInstallation(r.db.QueryRowContext(ctx, getInstallationByIDQuery, installationID))
}

// GetByAppConfigAndInstallationID retrieves an installation scoped to one App,
// matching the UNIQUE (app_config_id, installation_id) constraint. This is what
// keeps a webhook delivery for one team from resolving another team's
// installation when the numeric ids collide.
func (r *GitHubInstallationRepository) GetByAppConfigAndInstallationID(
	ctx context.Context,
	appConfigID string,
	installationID int64,
) (*models.GitHubInstallation, error) {
	return scanInstallation(
		r.db.QueryRowContext(ctx, getInstallationByAppConfigQuery, appConfigID, installationID))
}

// Update updates a GitHub installation
func (r *GitHubInstallationRepository) Update(ctx context.Context, installation *models.GitHubInstallation) error {
	permissionsJSON, err := json.Marshal(installation.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %w", err)
	}

	query := `
		UPDATE github_installations
		SET app_config_id = $1, installation_id = $2, account_login = $3, account_type = $4,
			target_type = $5, encrypted_access_token = $6, token_expires_at = $7,
			permissions = $8, events = $9, suspended_at = $10, updated_at = NOW()
		WHERE id = $11
	`

	result, err := r.db.ExecContext(
		ctx, query,
		installation.AppConfigID,
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
