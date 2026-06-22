package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TypeRepository is the Postgres implementation of repositories.TypeRepository.
// Global system defaults (team_id NULL) and a team's own custom types live in
// the same table; reads union the two and writes are scoped to the team.
type TypeRepository struct {
	db *database.DB
}

// NewTypeRepository creates a new TypeRepository.
func NewTypeRepository(db *database.DB) repositories.TypeRepository {
	return &TypeRepository{db: db}
}

// typeColumns is the shared SELECT column list, ordered to match scanType.
const typeColumns = "id, team_id, resource_type, slug, name, is_system, created_by, created_at, updated_at"

func (r *TypeRepository) Create(ctx context.Context, t *models.Type) error {
	query := `
		INSERT INTO types (team_id, resource_type, slug, name, is_system, created_by)
		VALUES ($1, $2, $3, $4, FALSE, $5)
		RETURNING id, created_at, updated_at
	`
	// created_by is nullable (ON DELETE SET NULL); store NULL for an empty creator.
	var createdBy interface{}
	if t.CreatedBy != "" {
		createdBy = t.CreatedBy
	}

	err := r.db.QueryRowContext(ctx, query,
		t.TeamID, t.ResourceType, t.Slug, t.Name, createdBy,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if uniqueViolation(err) != nil {
			return repositories.ErrTypeAlreadyExists
		}
		if isFKViolation(err) {
			return fmt.Errorf("team not found for type: %w", err)
		}
		return fmt.Errorf("failed to create type: %w", err)
	}
	t.IsSystem = false
	return nil
}

func (r *TypeRepository) GetBySlug(
	ctx context.Context, teamID, resourceType, slug string,
) (*models.Type, error) {
	// Match a global default (team_id IS NULL) or one of this team's own rows.
	query := "SELECT " + typeColumns + " FROM types " +
		"WHERE resource_type = $1 AND slug = $2 AND (team_id IS NULL OR team_id = $3) " +
		"LIMIT 1"

	t, err := scanType(r.db.QueryRowContext(ctx, query, resourceType, slug, teamID))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get type by slug: %w", err),
			repositories.ErrTypeNotFound,
		)
	}
	return t, nil
}

func (r *TypeRepository) List(
	ctx context.Context, teamID, resourceType string,
) ([]models.Type, error) {
	query := "SELECT " + typeColumns + " FROM types " +
		"WHERE resource_type = $1 AND (team_id IS NULL OR team_id = $2) " +
		"ORDER BY is_system DESC, name ASC"

	rows, err := r.db.QueryContext(ctx, query, resourceType, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to list types: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	types := make([]models.Type, 0)
	for rows.Next() {
		t, scanErr := scanType(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan type: %w", scanErr)
		}
		types = append(types, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate types: %w", err)
	}
	return types, nil
}

func (r *TypeRepository) DeleteCustom(
	ctx context.Context, teamID, id, fallbackSlug string,
) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil &&
			rbErr.Error() != "sql: transaction has already been committed or rolled back" {
			slog.Error("Failed to rollback transaction", "error", rbErr)
		}
	}()

	// Delete only a non-system row owned by this team; capture its slug and
	// resource_type so we can reassign the resources that referenced it. A
	// system default or another team's row matches nothing here.
	var deletedSlug, resourceType string
	delErr := tx.QueryRowContext(ctx,
		"DELETE FROM types WHERE id = $1 AND team_id = $2 AND is_system = FALSE "+
			"RETURNING slug, resource_type",
		id, teamID,
	).Scan(&deletedSlug, &resourceType)
	if delErr != nil {
		return mapNoRows(
			fmt.Errorf("failed to delete custom type: %w", delErr),
			repositories.ErrTypeNotFound,
		)
	}

	// Reassign resources that referenced the deleted type to the fallback. Only
	// artifacts consume types today; other resource types will be added here as
	// they adopt the customizable-types system.
	if resourceType == "artifacts" {
		if _, err := tx.ExecContext(ctx,
			"UPDATE artifacts SET type = $1, updated_at = now() WHERE team_id = $2 AND type = $3",
			fallbackSlug, teamID, deletedSlug,
		); err != nil {
			return fmt.Errorf("failed to reassign artifacts after type delete: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true
	return nil
}

// scanType scans one type row (from *sql.Row or *sql.Rows via rowScanner),
// mapping nullable team_id/created_by to empty strings on the model (global
// defaults have a NULL team_id).
func scanType(s rowScanner) (*models.Type, error) {
	var (
		t         models.Type
		teamID    sql.NullString
		createdBy sql.NullString
	)
	if err := s.Scan(
		&t.ID, &teamID, &t.ResourceType, &t.Slug, &t.Name,
		&t.IsSystem, &createdBy, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return nil, err
	}
	t.TeamID = teamID.String
	t.CreatedBy = createdBy.String
	return &t, nil
}
