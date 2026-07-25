package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// GitHubAppConfigRepository implements repositories.GitHubAppConfigRepository for PostgreSQL.
type GitHubAppConfigRepository struct {
	db *database.DB
}

// NewGitHubAppConfigRepository creates a new GitHubAppConfigRepository.
func NewGitHubAppConfigRepository(db *database.DB) repositories.GitHubAppConfigRepository {
	return &GitHubAppConfigRepository{db: db}
}

// githubAppConfigColumns is the full projection, shared by every read so a
// column added to one query can never be forgotten in another.
const githubAppConfigColumns = `id, team_id, user_id, app_id, app_slug, client_id,
	private_key_encrypted, client_secret_encrypted, webhook_secret_encrypted,
	webhook_token, created_at, updated_at, version`

// scanGitHubAppConfig reads one row in githubAppConfigColumns order.
func scanGitHubAppConfig(row rowScanner) (*models.GitHubAppConfig, error) {
	var config models.GitHubAppConfig
	err := row.Scan(
		&config.ID, &config.TeamID, &config.UserID, &config.AppID, &config.AppSlug,
		&config.ClientID, &config.PrivateKeyEncrypted, &config.ClientSecretEncrypted,
		&config.WebhookSecretEncrypted, &config.WebhookToken,
		&config.CreatedAt, &config.UpdatedAt, &config.Version,
	)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// mapGitHubAppConfigUniqueViolation translates a Postgres unique violation into
// the matching domain sentinel. It dispatches on the CONSTRAINT NAME, not on the
// message text, so a Postgres locale or wording change cannot silently reroute a
// 409 into a 500.
func mapGitHubAppConfigUniqueViolation(err error) error {
	pqErr := uniqueViolation(err)
	if pqErr == nil {
		return err
	}

	switch {
	case strings.Contains(pqErr.Constraint, "unique_github_app_id"):
		return repositories.ErrGitHubAppAlreadyRegistered
	case strings.Contains(pqErr.Constraint, "unique_team_github_app"):
		return repositories.ErrGitHubAppConfigTeamTaken
	case strings.Contains(pqErr.Constraint, "idx_github_app_configs_webhook_token"):
		return repositories.ErrGitHubAppWebhookTokenTaken
	default:
		return err
	}
}

// Create inserts a new GitHub App config for a team.
func (r *GitHubAppConfigRepository) Create(ctx context.Context, config *models.GitHubAppConfig) error {
	query := `
		INSERT INTO github_app_configs
		(team_id, user_id, app_id, app_slug, client_id,
		private_key_encrypted, client_secret_encrypted, webhook_secret_encrypted, webhook_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at, version
	`

	err := r.db.QueryRowContext(ctx, query,
		config.TeamID, config.UserID, config.AppID, config.AppSlug, config.ClientID,
		config.PrivateKeyEncrypted, config.ClientSecretEncrypted,
		config.WebhookSecretEncrypted, config.WebhookToken,
	).Scan(&config.ID, &config.CreatedAt, &config.UpdatedAt, &config.Version)

	if err != nil {
		if mapped := mapGitHubAppConfigUniqueViolation(err); mapped != err {
			return mapped
		}
		return fmt.Errorf("failed to create GitHub App config: %w", err)
	}

	return nil
}

// GetByTeamID retrieves the GitHub App config registered by a team.
func (r *GitHubAppConfigRepository) GetByTeamID(
	ctx context.Context, teamID string,
) (*models.GitHubAppConfig, error) {
	query := `SELECT ` + githubAppConfigColumns + `
		FROM github_app_configs
		WHERE team_id = $1`

	config, err := scanGitHubAppConfig(r.db.QueryRowContext(ctx, query, teamID))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get GitHub App config by team: %w", err),
			repositories.ErrGitHubAppConfigNotFound,
		)
	}

	return config, nil
}

// GetByID retrieves a GitHub App config by its ID, scoped to the owning team.
func (r *GitHubAppConfigRepository) GetByID(
	ctx context.Context, teamID, configID string,
) (*models.GitHubAppConfig, error) {
	query := `SELECT ` + githubAppConfigColumns + `
		FROM github_app_configs
		WHERE id = $1 AND team_id = $2`

	config, err := scanGitHubAppConfig(r.db.QueryRowContext(ctx, query, configID, teamID))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get GitHub App config by ID: %w", err),
			repositories.ErrGitHubAppConfigNotFound,
		)
	}

	return config, nil
}

// GetByWebhookToken resolves a GitHub App config from the opaque token embedded
// in its webhook URL.
//
// This is the only read in this repository without a team_id predicate, and
// deliberately so: GitHub posts deliveries to a public route that carries no
// team context, so the token IS the team resolution. The unique index on
// webhook_token makes the lookup exact, and the token is 32 bytes of
// crypto/rand, so it is not guessable.
func (r *GitHubAppConfigRepository) GetByWebhookToken(
	ctx context.Context, token string,
) (*models.GitHubAppConfig, error) {
	query := `SELECT ` + githubAppConfigColumns + `
		FROM github_app_configs
		WHERE webhook_token = $1`

	config, err := scanGitHubAppConfig(r.db.QueryRowContext(ctx, query, token))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get GitHub App config by webhook token: %w", err),
			repositories.ErrGitHubAppConfigNotFound,
		)
	}

	return config, nil
}

// Update applies an optimistic-locked update to a team's GitHub App config.
//
// The version predicate collapses three failure modes into one conflict
// sentinel -- stale version, wrong team, missing row -- because none of them
// should tell the caller anything about a config they may not own.
func (r *GitHubAppConfigRepository) Update(ctx context.Context, config *models.GitHubAppConfig) error {
	query := `
		UPDATE github_app_configs
		SET app_id = $4, app_slug = $5, client_id = $6,
		private_key_encrypted = $7, client_secret_encrypted = $8,
		webhook_secret_encrypted = $9, webhook_token = $10,
		updated_at = CURRENT_TIMESTAMP, version = version + 1
		WHERE id = $1 AND team_id = $2 AND version = $3
		RETURNING updated_at, version
	`

	err := r.db.QueryRowContext(ctx, query,
		config.ID, config.TeamID, config.Version,
		config.AppID, config.AppSlug, config.ClientID,
		config.PrivateKeyEncrypted, config.ClientSecretEncrypted,
		config.WebhookSecretEncrypted, config.WebhookToken,
	).Scan(&config.UpdatedAt, &config.Version)

	if err != nil {
		if mapped := mapGitHubAppConfigUniqueViolation(err); mapped != err {
			return mapped
		}
		return mapNoRows(
			fmt.Errorf("failed to update GitHub App config: %w", err),
			repositories.ErrGitHubAppConfigVersionConflict,
		)
	}

	return nil
}

// Delete removes a team's GitHub App config. The FK from github_installations
// is ON DELETE CASCADE, so this also disconnects every installation made
// through the App.
func (r *GitHubAppConfigRepository) Delete(ctx context.Context, teamID, configID string) error {
	query := `DELETE FROM github_app_configs WHERE id = $1 AND team_id = $2`

	result, err := r.db.ExecContext(ctx, query, configID, teamID)
	if err != nil {
		return fmt.Errorf("failed to delete GitHub App config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrGitHubAppConfigNotFound
	}

	return nil
}
