package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamEmailProviderRepository implements repositories.TeamEmailProviderRepository
// for PostgreSQL.
//
// Every statement is keyed on team_id: the table holds at most one row per team
// (unique_team_email_provider), so a row is never addressed independently of its
// team and tenancy is enforced by construction rather than by the caller.
type TeamEmailProviderRepository struct {
	db *database.DB
}

// NewTeamEmailProviderRepository creates a new TeamEmailProviderRepository.
func NewTeamEmailProviderRepository(db *database.DB) repositories.TeamEmailProviderRepository {
	return &TeamEmailProviderRepository{db: db}
}

// teamEmailProviderColumns is the full projection, shared by every read so a
// column added to one query can never be forgotten in another.
const teamEmailProviderColumns = `id, team_id, user_id, provider_type, settings,
	secret_encrypted, from_address, from_name, reply_to,
	last_success_at, last_error, last_error_at, created_at, updated_at, version`

// scanTeamEmailProvider reads one row in teamEmailProviderColumns order.
func scanTeamEmailProvider(row rowScanner) (*models.TeamEmailProvider, error) {
	var provider models.TeamEmailProvider
	err := row.Scan(
		&provider.ID, &provider.TeamID, &provider.UserID, &provider.ProviderType,
		&provider.Settings, &provider.SecretEncrypted, &provider.FromAddress,
		&provider.FromName, &provider.ReplyTo,
		&provider.LastSuccessAt, &provider.LastError, &provider.LastErrorAt,
		&provider.CreatedAt, &provider.UpdatedAt, &provider.Version,
	)
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// GetByTeamID retrieves the team's email provider.
//
// ErrTeamEmailProviderNotFound is the ordinary result for a team that has not
// configured its own provider — the caller falls back to the instance provider.
func (r *TeamEmailProviderRepository) GetByTeamID(
	ctx context.Context, teamID string,
) (*models.TeamEmailProvider, error) {
	query := `SELECT ` + teamEmailProviderColumns + `
		FROM team_email_providers
		WHERE team_id = $1`

	provider, err := scanTeamEmailProvider(r.db.QueryRowContext(ctx, query, teamID))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get team email provider: %w", err),
			repositories.ErrTeamEmailProviderNotFound,
		)
	}

	return provider, nil
}

// Upsert creates or replaces the team's email provider.
//
// A single INSERT ... ON CONFLICT (team_id) rather than a read-then-write: the
// unique constraint is the arbiter, so two concurrent writers cannot both decide
// the row is absent and race to insert it. The health columns are deliberately
// left out of the DO UPDATE set — reconfiguring a provider must not erase the
// delivery history that explains why it was reconfigured.
func (r *TeamEmailProviderRepository) Upsert(
	ctx context.Context, provider *models.TeamEmailProvider,
) error {
	// The column is NOT NULL with a '{}' default, but a default only fires when
	// the column is omitted from the INSERT — and this statement names it. So an
	// absent Settings has to be normalised here or it reaches Postgres as NULL
	// and fails the constraint. Providers whose only configuration is their
	// secret (SendGrid) legitimately have no settings at all.
	if len(provider.Settings) == 0 {
		provider.Settings = json.RawMessage(`{}`)
	}

	query := `
		INSERT INTO team_email_providers
		(team_id, user_id, provider_type, settings, secret_encrypted,
		from_address, from_name, reply_to)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (team_id)
		DO UPDATE SET
			user_id = EXCLUDED.user_id,
			provider_type = EXCLUDED.provider_type,
			settings = EXCLUDED.settings,
			secret_encrypted = EXCLUDED.secret_encrypted,
			from_address = EXCLUDED.from_address,
			from_name = EXCLUDED.from_name,
			reply_to = EXCLUDED.reply_to,
			updated_at = CURRENT_TIMESTAMP,
			version = team_email_providers.version + 1
		RETURNING id, created_at, updated_at, version
	`

	err := r.db.QueryRowContext(ctx, query,
		provider.TeamID, provider.UserID, provider.ProviderType, provider.Settings,
		provider.SecretEncrypted, provider.FromAddress, provider.FromName, provider.ReplyTo,
	).Scan(&provider.ID, &provider.CreatedAt, &provider.UpdatedAt, &provider.Version)

	if err != nil {
		return fmt.Errorf("failed to upsert team email provider: %w", err)
	}

	return nil
}

// Delete removes the team's email provider, reverting the team to the instance
// provider.
func (r *TeamEmailProviderRepository) Delete(ctx context.Context, teamID string) error {
	query := `DELETE FROM team_email_providers WHERE team_id = $1`

	result, err := r.db.ExecContext(ctx, query, teamID)
	if err != nil {
		return fmt.Errorf("failed to delete team email provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrTeamEmailProviderNotFound
	}

	return nil
}

// RecordSendResult stamps the outcome of one send attempt on the team's provider.
//
// One targeted UPDATE of only the health columns, never a read-modify-write, so
// it cannot clobber a configuration change made between the send and this call.
// It does not bump version for the same reason: a delivery result is not a
// configuration edit, and bumping it would make an unrelated optimistic-locked
// update fail. A missing row is not an error — a team can have its provider
// deleted while a send is in flight, and failing here would turn that race into
// a spurious error on an already-completed send.
func (r *TeamEmailProviderRepository) RecordSendResult(
	ctx context.Context, teamID string, sendErr error, at time.Time,
) error {
	// A success does not clear last_error: current health comes from comparing
	// the two timestamps (models.TeamEmailProvider.IsHealthy), which keeps the
	// last failure readable for diagnosis after the provider recovers.
	query := `
		UPDATE team_email_providers
		SET last_success_at = $2
		WHERE team_id = $1`
	args := []any{teamID, at}

	if sendErr != nil {
		query = `
		UPDATE team_email_providers
		SET last_error = $2, last_error_at = $3
		WHERE team_id = $1`
		args = []any{teamID, sendErr.Error(), at}
	}

	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to record team email provider send result: %w", err)
	}

	return nil
}
