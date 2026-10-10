package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamStatusRepository is the Postgres implementation of
// repositories.TeamStatusRepository. Every statement carries team_id: the
// repository is tenancy-only and decides nothing about roles (epic #220 D3).
type TeamStatusRepository struct {
	db *database.DB
}

// NewTeamStatusRepository creates a new TeamStatusRepository.
func NewTeamStatusRepository(db *database.DB) repositories.TeamStatusRepository {
	return &TeamStatusRepository{db: db}
}

// The statements are whole literals rather than a shared column list
// concatenated in, so each is a compile-time constant a reader (and a static
// analyser) can take in at a glance. Their column order matches scanTeamStatus.
const (
	teamStatusInsertSQL = `
		INSERT INTO team_statuses (team_id, resource_type, slug, name, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	teamStatusGetByIDSQL = `
		SELECT id, team_id, resource_type, slug, name, created_by, created_at, updated_at
		  FROM team_statuses
		 WHERE team_id = $1 AND id = $2`

	teamStatusListSQL = `
		SELECT id, team_id, resource_type, slug, name, created_by, created_at, updated_at
		  FROM team_statuses
		 WHERE team_id = $1 AND resource_type = $2
		 ORDER BY name ASC, slug ASC`

	teamStatusCountSQL = `
		SELECT count(*) FROM team_statuses WHERE team_id = $1 AND resource_type = $2`

	teamStatusUpdateNameSQL = `
		UPDATE team_statuses
		   SET name = $3, updated_at = now()
		 WHERE team_id = $1 AND id = $2
		RETURNING id, team_id, resource_type, slug, name, created_by, created_at, updated_at`

	teamStatusDeleteSQL = `
		DELETE FROM team_statuses WHERE team_id = $1 AND id = $2`
)

func (r *TeamStatusRepository) Create(ctx context.Context, status *models.TeamStatus) error {
	err := r.db.QueryRowContext(ctx, teamStatusInsertSQL,
		status.TeamID, status.ResourceType, status.Slug, status.Name, status.CreatedBy,
	).Scan(&status.ID, &status.CreatedAt, &status.UpdatedAt)
	if err != nil {
		if uniqueViolation(err) != nil {
			return repositories.ErrTeamStatusExists
		}
		if isFKViolation(err) {
			return fmt.Errorf("team or creator not found for team status: %w", err)
		}
		return fmt.Errorf("failed to create team status: %w", err)
	}
	return nil
}

func (r *TeamStatusRepository) GetByID(
	ctx context.Context, teamID, id string,
) (*models.TeamStatus, error) {
	status, err := scanTeamStatus(r.db.QueryRowContext(ctx, teamStatusGetByIDSQL, teamID, id))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get team status: %w", err),
			repositories.ErrTeamStatusNotFound,
		)
	}
	return &status, nil
}

func (r *TeamStatusRepository) List(
	ctx context.Context, teamID, resourceType string,
) ([]models.TeamStatus, error) {
	return queryAdminRows(ctx, r.db, "team statuses",
		func(rows *sql.Rows) (models.TeamStatus, error) { return scanTeamStatus(rows) },
		teamStatusListSQL, teamID, resourceType)
}

func (r *TeamStatusRepository) Count(
	ctx context.Context, teamID, resourceType string,
) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, teamStatusCountSQL, teamID, resourceType).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count team statuses: %w", err)
	}
	return count, nil
}

func (r *TeamStatusRepository) UpdateName(
	ctx context.Context, teamID, id, name string,
) (*models.TeamStatus, error) {
	status, err := scanTeamStatus(r.db.QueryRowContext(ctx, teamStatusUpdateNameSQL, teamID, id, name))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to rename team status: %w", err),
			repositories.ErrTeamStatusNotFound,
		)
	}
	return &status, nil
}

func (r *TeamStatusRepository) Delete(ctx context.Context, teamID, id string) error {
	result, err := r.db.ExecContext(ctx, teamStatusDeleteSQL, teamID, id)
	if err != nil {
		return fmt.Errorf("failed to delete team status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read team status delete result: %w", err)
	}
	if affected == 0 {
		return repositories.ErrTeamStatusNotFound
	}
	return nil
}

// scanTeamStatus scans one team_statuses row, from a single-row lookup or a
// result set. created_by scans straight into the model's *string: NULL is nil.
func scanTeamStatus(s rowScanner) (models.TeamStatus, error) {
	var status models.TeamStatus
	err := s.Scan(
		&status.ID, &status.TeamID, &status.ResourceType, &status.Slug, &status.Name,
		&status.CreatedBy, &status.CreatedAt, &status.UpdatedAt,
	)
	return status, err
}
