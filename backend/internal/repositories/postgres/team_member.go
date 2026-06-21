package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamMemberRepository implements the repositories.TeamMemberRepository interface for PostgreSQL
type TeamMemberRepository struct {
	db *database.DB
}

// NewTeamMemberRepository creates a new TeamMemberRepository
func NewTeamMemberRepository(db *database.DB) repositories.TeamMemberRepository {
	return &TeamMemberRepository{
		db: db,
	}
}

// Create creates a new team member
func (r *TeamMemberRepository) Create(ctx context.Context, member *models.TeamMember) error {
	query := `
		INSERT INTO team_members (team_id, user_id, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		member.TeamID, member.UserID, member.Role,
		member.CreatedAt, member.UpdatedAt,
	).Scan(&member.ID, &member.CreatedAt, &member.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create team member: %w", err)
	}

	return nil
}

// GetByTeamAndUser retrieves a team member by team ID and user ID
func (r *TeamMemberRepository) GetByTeamAndUser(
	ctx context.Context, teamID, userID string,
) (*models.TeamMember, error) {
	query := `
		SELECT id, team_id, user_id, role, created_at, updated_at
		FROM team_members WHERE team_id = $1 AND user_id = $2
	`

	var member models.TeamMember
	err := r.db.QueryRowContext(ctx, query, teamID, userID).Scan(
		&member.ID, &member.TeamID, &member.UserID, &member.Role,
		&member.CreatedAt, &member.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get team member: %w", err), repositories.ErrTeamMemberNotFound)
	}

	return &member, nil
}

// GetByTeamID retrieves all members of a team
func (r *TeamMemberRepository) GetByTeamID(ctx context.Context, teamID string) ([]models.TeamMember, error) {
	query := `
		SELECT id, team_id, user_id, role, created_at, updated_at
		FROM team_members WHERE team_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to query team members: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var members []models.TeamMember
	for rows.Next() {
		var member models.TeamMember
		if err := rows.Scan(
			&member.ID, &member.TeamID, &member.UserID, &member.Role,
			&member.CreatedAt, &member.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team member: %w", err)
		}
		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating team members: %w", err)
	}

	return members, nil
}

// GetByUserID retrieves all team memberships for a user
func (r *TeamMemberRepository) GetByUserID(ctx context.Context, userID string) ([]models.TeamMember, error) {
	query := `
		SELECT id, team_id, user_id, role, created_at, updated_at
		FROM team_members WHERE user_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query team memberships: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var members []models.TeamMember
	for rows.Next() {
		var member models.TeamMember
		if err := rows.Scan(
			&member.ID, &member.TeamID, &member.UserID, &member.Role,
			&member.CreatedAt, &member.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team membership: %w", err)
		}
		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating team memberships: %w", err)
	}

	return members, nil
}

// UpdateRole updates a team member's role
func (r *TeamMemberRepository) UpdateRole(
	ctx context.Context,
	teamID, userID string,
	role models.TeamMemberRole,
) error {
	query := `
		UPDATE team_members
		SET role = $1, updated_at = $2
		WHERE team_id = $3 AND user_id = $4
	`

	result, err := r.db.ExecContext(ctx, query, role, time.Now(), teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to update team member role: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrTeamMemberNotFound
	}

	return nil
}

// Delete removes a team member
func (r *TeamMemberRepository) Delete(ctx context.Context, teamID, userID string) error {
	query := `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete team member: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrTeamMemberNotFound
	}

	return nil
}
