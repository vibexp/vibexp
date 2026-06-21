package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// contentVersionRepository implements the ContentVersionRepository interface using PostgreSQL.
type contentVersionRepository struct {
	db *database.DB
}

// NewContentVersionRepository creates a new PostgreSQL content version repository.
func NewContentVersionRepository(db *database.DB) repositories.ContentVersionRepository {
	return &contentVersionRepository{db: db}
}

// Create persists a new content version snapshot. The version_number is computed in
// SQL as MAX(version_number)+1 for the (resource_type, resource_id) pair, and the
// generated id, version_number, and created_at are back-filled onto v.
func (r *contentVersionRepository) Create(ctx context.Context, v *models.ContentVersion) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	var createdBy sql.NullString
	if v.CreatedBy != nil && *v.CreatedBy != "" {
		createdBy = sql.NullString{String: *v.CreatedBy, Valid: true}
	}

	var changeSummary sql.NullString
	if v.ChangeSummary != nil {
		changeSummary = sql.NullString{String: *v.ChangeSummary, Valid: true}
	}

	actorType := v.ActorType
	if actorType == "" {
		actorType = models.ActorTypeHuman
	}

	query := `
		INSERT INTO content_versions
			(team_id, resource_type, resource_id, version_number, content, change_summary, actor_type, created_by)
		VALUES (
			$1, $2, $3,
			(SELECT COALESCE(MAX(version_number), 0) + 1 FROM content_versions
				WHERE resource_type = $2 AND resource_id = $3),
			$4, $5, $6, $7
		)
		RETURNING id, version_number, created_at`

	err := r.db.QueryRowContext(ctx, query,
		v.TeamID,
		v.ResourceType,
		v.ResourceID,
		v.Content,
		changeSummary,
		actorType,
		createdBy,
	).Scan(&v.ID, &v.VersionNumber, &v.CreatedAt)
	if err != nil {
		logrus.WithError(err).Error("Failed to create content version")
		return fmt.Errorf("failed to create content version: %w", err)
	}

	v.ActorType = actorType

	return nil
}

// ListByResource returns all versions for the resource, newest version first.
func (r *contentVersionRepository) ListByResource(
	ctx context.Context, teamID, resourceType, resourceID string,
) ([]*models.ContentVersion, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, team_id, resource_type, resource_id, version_number, content,
			change_summary, actor_type, created_by, created_at
		FROM content_versions
		WHERE team_id = $1 AND resource_type = $2 AND resource_id = $3
		ORDER BY version_number DESC`

	rows, err := r.db.QueryContext(ctx, query, teamID, resourceType, resourceID)
	if err != nil {
		logrus.WithError(err).Error("Failed to query content versions")
		return nil, fmt.Errorf("failed to query content versions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	versions := []*models.ContentVersion{}
	for rows.Next() {
		v, scanErr := scanContentVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		versions = append(versions, v)
	}

	if err := rows.Err(); err != nil {
		logrus.WithError(err).Error("Failed to iterate content version rows")
		return nil, fmt.Errorf("failed to iterate content versions: %w", err)
	}

	return versions, nil
}

// GetByVersionNumber returns a single version for the resource, mapping a missing
// row to ErrContentVersionNotFound.
func (r *contentVersionRepository) GetByVersionNumber(
	ctx context.Context, teamID, resourceType, resourceID string, versionNumber int,
) (*models.ContentVersion, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, team_id, resource_type, resource_id, version_number, content,
			change_summary, actor_type, created_by, created_at
		FROM content_versions
		WHERE team_id = $1 AND resource_type = $2 AND resource_id = $3 AND version_number = $4`

	v, err := scanContentVersion(r.db.QueryRowContext(ctx, query, teamID, resourceType, resourceID, versionNumber))
	if err != nil {
		return nil, mapNoRows(err, repositories.ErrContentVersionNotFound)
	}

	return v, nil
}

// PruneToCap deletes all but the newest `keep` versions for the resource. A non-positive
// keep deletes nothing (defensive: callers pass the adapter's retention cap).
func (r *contentVersionRepository) PruneToCap(
	ctx context.Context, resourceType, resourceID string, keep int,
) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if keep <= 0 {
		return nil
	}

	query := `
		DELETE FROM content_versions
		WHERE resource_type = $1 AND resource_id = $2
		AND version_number <= (
			SELECT COALESCE(MAX(version_number), 0) FROM content_versions
			WHERE resource_type = $1 AND resource_id = $2
		) - $3`

	if _, err := r.db.ExecContext(ctx, query, resourceType, resourceID, keep); err != nil {
		logrus.WithError(err).Error("Failed to prune content versions")
		return fmt.Errorf("failed to prune content versions: %w", err)
	}

	return nil
}

// scanContentVersion scans one row into a ContentVersion, decoding the nullable
// created_by into the model's *string.
func scanContentVersion(s rowScanner) (*models.ContentVersion, error) {
	var v models.ContentVersion
	var createdBy, changeSummary sql.NullString
	if err := s.Scan(
		&v.ID,
		&v.TeamID,
		&v.ResourceType,
		&v.ResourceID,
		&v.VersionNumber,
		&v.Content,
		&changeSummary,
		&v.ActorType,
		&createdBy,
		&v.CreatedAt,
	); err != nil {
		return nil, err
	}
	if changeSummary.Valid {
		v.ChangeSummary = &changeSummary.String
	}
	if createdBy.Valid {
		v.CreatedBy = &createdBy.String
	}
	return &v, nil
}
