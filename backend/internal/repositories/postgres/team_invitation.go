package postgres

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamInvitationRepository implements the repositories.TeamInvitationRepository interface for PostgreSQL
type TeamInvitationRepository struct {
	db *database.DB
}

// NewTeamInvitationRepository creates a new TeamInvitationRepository
func NewTeamInvitationRepository(db *database.DB) repositories.TeamInvitationRepository {
	return &TeamInvitationRepository{
		db: db,
	}
}

// Create creates a new team invitation
func (r *TeamInvitationRepository) Create(ctx context.Context, invitation *models.TeamInvitation) error {
	query := `
		INSERT INTO team_invitations (
			team_id, inviter_id, invitee_email, role, token,
			status, expires_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		invitation.TeamID, invitation.InviterID, invitation.InviteeEmail,
		invitation.Role, invitation.Token, invitation.Status,
		invitation.ExpiresAt, invitation.CreatedAt, invitation.UpdatedAt,
	).Scan(&invitation.ID, &invitation.CreatedAt, &invitation.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create team invitation: %w", err)
	}

	return nil
}

// GetByID retrieves a team invitation by ID
func (r *TeamInvitationRepository) GetByID(ctx context.Context, invitationID string) (*models.TeamInvitation, error) {
	query := `
		SELECT id, team_id, inviter_id, invitee_email, role, token, status, expires_at, created_at, updated_at
		FROM team_invitations WHERE id = $1
	`

	var invitation models.TeamInvitation
	err := r.db.QueryRowContext(ctx, query, invitationID).Scan(
		&invitation.ID, &invitation.TeamID, &invitation.InviterID, &invitation.InviteeEmail,
		&invitation.Role, &invitation.Token, &invitation.Status, &invitation.ExpiresAt,
		&invitation.CreatedAt, &invitation.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get team invitation: %w", err), repositories.ErrTeamInvitationNotFound)
	}

	return &invitation, nil
}

// GetByToken retrieves a team invitation by token
func (r *TeamInvitationRepository) GetByToken(ctx context.Context, token string) (*models.TeamInvitation, error) {
	query := `
		SELECT id, team_id, inviter_id, invitee_email, role, token, status, expires_at, created_at, updated_at
		FROM team_invitations WHERE token = $1
	`

	var invitation models.TeamInvitation
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&invitation.ID, &invitation.TeamID, &invitation.InviterID, &invitation.InviteeEmail,
		&invitation.Role, &invitation.Token, &invitation.Status, &invitation.ExpiresAt,
		&invitation.CreatedAt, &invitation.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get team invitation: %w", err), repositories.ErrTeamInvitationNotFound)
	}

	return &invitation, nil
}

// GetByTeamID retrieves all invitations for a team
func (r *TeamInvitationRepository) GetByTeamID(ctx context.Context, teamID string) ([]models.TeamInvitation, error) {
	query := `
		SELECT id, team_id, inviter_id, invitee_email, role, token, status, expires_at, created_at, updated_at
		FROM team_invitations WHERE team_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to query team invitations: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var invitations []models.TeamInvitation
	for rows.Next() {
		var invitation models.TeamInvitation
		if err := rows.Scan(
			&invitation.ID, &invitation.TeamID, &invitation.InviterID, &invitation.InviteeEmail,
			&invitation.Role, &invitation.Token, &invitation.Status, &invitation.ExpiresAt,
			&invitation.CreatedAt, &invitation.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team invitation: %w", err)
		}
		invitations = append(invitations, invitation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating team invitations: %w", err)
	}

	return invitations, nil
}

// GetPendingByEmail retrieves pending invitations for an email address
func (r *TeamInvitationRepository) GetPendingByEmail(
	ctx context.Context, email string,
) ([]models.TeamInvitation, error) {
	query := `
		SELECT id, team_id, inviter_id, invitee_email, role, token, status, expires_at, created_at, updated_at
		FROM team_invitations
		WHERE invitee_email = $1 AND status = $2 AND expires_at > NOW()
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, email, models.InvitationStatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending invitations: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var invitations []models.TeamInvitation
	for rows.Next() {
		var invitation models.TeamInvitation
		if err := rows.Scan(
			&invitation.ID, &invitation.TeamID, &invitation.InviterID, &invitation.InviteeEmail,
			&invitation.Role, &invitation.Token, &invitation.Status, &invitation.ExpiresAt,
			&invitation.CreatedAt, &invitation.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team invitation: %w", err)
		}
		invitations = append(invitations, invitation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pending invitations: %w", err)
	}

	return invitations, nil
}

// UpdateStatus updates the status of a team invitation
func (r *TeamInvitationRepository) UpdateStatus(
	ctx context.Context,
	invitationID string,
	status models.InvitationStatus,
) error {
	query := `
		UPDATE team_invitations
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, status, invitationID)
	if err != nil {
		return fmt.Errorf("failed to update invitation status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrTeamInvitationNotFound
	}

	return nil
}

// Delete removes a team invitation
func (r *TeamInvitationRepository) Delete(ctx context.Context, invitationID string) error {
	query := `DELETE FROM team_invitations WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, invitationID)
	if err != nil {
		return fmt.Errorf("failed to delete team invitation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrTeamInvitationNotFound
	}

	return nil
}
