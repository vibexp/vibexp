package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

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

// Create creates a new team invitation. The invitee's address is stored
// normalized (models.NormalizeEmail; team_invitations_invitee_email_lowercase
// rejects anything else) and invitation.InviteeEmail is updated to that form.
func (r *TeamInvitationRepository) Create(ctx context.Context, invitation *models.TeamInvitation) error {
	invitation.InviteeEmail = models.NormalizeEmail(invitation.InviteeEmail)
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

// GetPendingByEmail retrieves pending, unexpired invitations for an email
// address. Stored addresses are always normalized (#1434), so normalizing the
// argument makes the match case-insensitive while still using
// idx_team_invitations_invitee_email.
func (r *TeamInvitationRepository) GetPendingByEmail(
	ctx context.Context, email string,
) ([]models.TeamInvitation, error) {
	query := `
		SELECT id, team_id, inviter_id, invitee_email, role, token, status, expires_at, created_at, updated_at
		FROM team_invitations
		WHERE invitee_email = $1 AND status = $2 AND expires_at > NOW()
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, models.NormalizeEmail(email), models.InvitationStatusPending)
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

// Accept implements repositories.TeamInvitationRepository. The status update
// carries the "pending and unexpired" condition itself, so it is the lock: of
// two simultaneous accepts only one updates a row.
func (r *TeamInvitationRepository) Accept(
	ctx context.Context, invitationID string, member *models.TeamMember,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin invitation acceptance: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back an invitation acceptance", "error", rollbackErr)
		}
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE team_invitations
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3 AND expires_at > NOW()`,
		invitationID, models.InvitationStatusAccepted, models.InvitationStatusPending)
	if err != nil {
		return fmt.Errorf("failed to accept invitation: %w", err)
	}
	accepted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to accept invitation: %w", err)
	}
	if accepted == 0 {
		return repositories.ErrTeamInvitationNotPending
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO team_members (team_id, user_id, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`,
		member.TeamID, member.UserID, member.Role, member.CreatedAt, member.UpdatedAt,
	).Scan(&member.ID, &member.CreatedAt, &member.UpdatedAt)
	if pqErr := uniqueViolation(err); pqErr != nil && pqErr.Constraint == "team_members_unique" {
		return repositories.ErrTeamMemberExists
	}
	if err != nil {
		return fmt.Errorf("failed to create team member: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit invitation acceptance: %w", err)
	}
	return nil
}
