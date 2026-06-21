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

// AttachmentRepository is the Postgres implementation of
// repositories.AttachmentRepository. Every query is keyed by the polymorphic
// (owner_type, owner_id) pair; owner_id has no foreign key (cf. embeddings).
type AttachmentRepository struct {
	db *database.DB
}

// NewAttachmentRepository creates a new AttachmentRepository.
func NewAttachmentRepository(db *database.DB) repositories.AttachmentRepository {
	return &AttachmentRepository{db: db}
}

// attachmentColumns is the shared SELECT column list, ordered to match scanAttachment.
const attachmentColumns = "id, team_id, user_id, owner_type, owner_id, " +
	"file_name, content_type, size_bytes, gcs_object_key, created_at"

func (r *AttachmentRepository) Create(ctx context.Context, attachment *models.Attachment) error {
	// user_id is nullable (ON DELETE SET NULL); store NULL for an empty creator.
	var userID interface{}
	if attachment.UserID != "" {
		userID = attachment.UserID
	}

	query := `
		INSERT INTO attachments
		(team_id, user_id, owner_type, owner_id, file_name, content_type, size_bytes, gcs_object_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`
	err := r.db.QueryRowContext(ctx, query,
		attachment.TeamID, userID, attachment.OwnerType, attachment.OwnerID,
		attachment.FileName, attachment.ContentType, attachment.SizeBytes, attachment.GCSObjectKey,
	).Scan(&attachment.ID, &attachment.CreatedAt)
	if err != nil {
		if isFKViolation(err) {
			return fmt.Errorf("team not found for attachment: %w", err)
		}
		return fmt.Errorf("failed to create attachment: %w", err)
	}
	return nil
}

func (r *AttachmentRepository) GetByID(
	ctx context.Context, ownerType, ownerID, id string,
) (*models.Attachment, error) {
	query := "SELECT " + attachmentColumns + " FROM attachments " +
		"WHERE id = $1 AND owner_type = $2 AND owner_id = $3"

	var (
		att    models.Attachment
		userID sql.NullString
	)
	err := r.db.QueryRowContext(ctx, query, id, ownerType, ownerID).Scan(
		&att.ID, &att.TeamID, &userID, &att.OwnerType, &att.OwnerID,
		&att.FileName, &att.ContentType, &att.SizeBytes, &att.GCSObjectKey, &att.CreatedAt,
	)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get attachment by ID: %w", err),
			repositories.ErrAttachmentNotFound,
		)
	}
	att.UserID = userID.String
	return &att, nil
}

func (r *AttachmentRepository) GetByIDInTeam(
	ctx context.Context, teamID, id string,
) (*models.Attachment, error) {
	query := "SELECT " + attachmentColumns + " FROM attachments " +
		"WHERE id = $1 AND team_id = $2"

	var (
		att    models.Attachment
		userID sql.NullString
	)
	err := r.db.QueryRowContext(ctx, query, id, teamID).Scan(
		&att.ID, &att.TeamID, &userID, &att.OwnerType, &att.OwnerID,
		&att.FileName, &att.ContentType, &att.SizeBytes, &att.GCSObjectKey, &att.CreatedAt,
	)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get attachment by ID in team: %w", err),
			repositories.ErrAttachmentNotFound,
		)
	}
	att.UserID = userID.String
	return &att, nil
}

func (r *AttachmentRepository) ListByOwner(
	ctx context.Context, ownerType, ownerID string,
) ([]models.Attachment, error) {
	query := "SELECT " + attachmentColumns + " FROM attachments " +
		"WHERE owner_type = $1 AND owner_id = $2 ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list attachments: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	attachments := make([]models.Attachment, 0)
	for rows.Next() {
		var (
			att    models.Attachment
			userID sql.NullString
		)
		if scanErr := rows.Scan(
			&att.ID, &att.TeamID, &userID, &att.OwnerType, &att.OwnerID,
			&att.FileName, &att.ContentType, &att.SizeBytes, &att.GCSObjectKey, &att.CreatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan attachment: %w", scanErr)
		}
		att.UserID = userID.String
		attachments = append(attachments, att)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate attachments: %w", err)
	}
	return attachments, nil
}

func (r *AttachmentRepository) SumSizeByOwner(
	ctx context.Context, ownerType, ownerID string,
) (int64, error) {
	query := "SELECT COALESCE(SUM(size_bytes), 0) FROM attachments " +
		"WHERE owner_type = $1 AND owner_id = $2"

	var total int64
	if err := r.db.QueryRowContext(ctx, query, ownerType, ownerID).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to sum attachment sizes: %w", err)
	}
	return total, nil
}

func (r *AttachmentRepository) Delete(ctx context.Context, ownerType, ownerID, id string) error {
	query := "DELETE FROM attachments WHERE id = $1 AND owner_type = $2 AND owner_id = $3"

	result, err := r.db.ExecContext(ctx, query, id, ownerType, ownerID)
	if err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read delete result: %w", err)
	}
	if affected == 0 {
		return repositories.ErrAttachmentNotFound
	}
	return nil
}

func (r *AttachmentRepository) DeleteByOwner(
	ctx context.Context, ownerType, ownerID string,
) ([]models.Attachment, error) {
	query := "DELETE FROM attachments WHERE owner_type = $1 AND owner_id = $2 RETURNING " + attachmentColumns

	rows, err := r.db.QueryContext(ctx, query, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete attachments for owner: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	deleted := make([]models.Attachment, 0)
	for rows.Next() {
		var (
			att    models.Attachment
			userID sql.NullString
		)
		if scanErr := rows.Scan(
			&att.ID, &att.TeamID, &userID, &att.OwnerType, &att.OwnerID,
			&att.FileName, &att.ContentType, &att.SizeBytes, &att.GCSObjectKey, &att.CreatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan deleted attachment: %w", scanErr)
		}
		att.UserID = userID.String
		deleted = append(deleted, att)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate deleted attachments: %w", err)
	}
	return deleted, nil
}
