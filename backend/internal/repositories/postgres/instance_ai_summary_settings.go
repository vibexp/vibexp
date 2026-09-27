package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceAISummarySettingsRepository implements
// repositories.InstanceAISummarySettingsRepository for PostgreSQL.
//
// The table's primary key is `id boolean CHECK (id)`, so it holds at most one
// row and statements carry no WHERE clause and no tenancy or role predicate:
// the scope is the instance itself.
type InstanceAISummarySettingsRepository struct {
	db *database.DB
}

// NewInstanceAISummarySettingsRepository creates a new InstanceAISummarySettingsRepository.
func NewInstanceAISummarySettingsRepository(db *database.DB) repositories.InstanceAISummarySettingsRepository {
	return &InstanceAISummarySettingsRepository{db: db}
}

// instanceAISummarySettingsInsert is the column list and values shared by
// Upsert and InsertIfAbsent, in instanceAISummarySettingsArgs order. id is
// omitted: its default (true) is the only value the singleton CHECK admits.
const instanceAISummarySettingsInsert = `
	INSERT INTO instance_ai_summary_settings
	(enabled, top_n, style, max_output_tokens, per_document_chars,
	total_context_chars, request_timeout_ms, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

// instanceAISummarySettingsArgs returns the values in
// instanceAISummarySettingsInsert order. RequestTimeout is stored in whole
// milliseconds.
func instanceAISummarySettingsArgs(s *models.InstanceAISummarySettings) []any {
	return []any{
		s.Enabled, s.TopN, s.Style, s.MaxOutputTokens, s.PerDocumentChars,
		s.TotalContextChars, s.RequestTimeout.Milliseconds(), s.UpdatedBy,
	}
}

// instanceAISummarySettingsSelect reads the singleton row;
// scanInstanceAISummarySettings reads its columns in this order.
const instanceAISummarySettingsSelect = `SELECT enabled, top_n, style, max_output_tokens, per_document_chars,
		total_context_chars, request_timeout_ms, created_at, updated_at, updated_by, version
		FROM instance_ai_summary_settings`

// instanceAISummarySettingsUpsert creates or replaces the row in one INSERT ...
// ON CONFLICT (id), so two concurrent writers cannot both decide it is absent.
// It repeats instanceAISummarySettingsInsert's columns as one literal (same
// order as instanceAISummarySettingsArgs) rather than concatenating, so it is a
// single static statement.
const instanceAISummarySettingsUpsert = `
	INSERT INTO instance_ai_summary_settings
	(enabled, top_n, style, max_output_tokens, per_document_chars,
	total_context_chars, request_timeout_ms, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id)
		DO UPDATE SET
			enabled = EXCLUDED.enabled,
			top_n = EXCLUDED.top_n,
			style = EXCLUDED.style,
			max_output_tokens = EXCLUDED.max_output_tokens,
			per_document_chars = EXCLUDED.per_document_chars,
			total_context_chars = EXCLUDED.total_context_chars,
			request_timeout_ms = EXCLUDED.request_timeout_ms,
			updated_by = EXCLUDED.updated_by,
			updated_at = CURRENT_TIMESTAMP,
			version = instance_ai_summary_settings.version + 1
		RETURNING created_at, updated_at, version`

// instanceAISummarySettingsSubject names the table's settings in errors and
// logs.
const instanceAISummarySettingsSubject = "instance AI summary settings"

// instanceAISummarySettingsWriteLock serializes audited writes; see
// runAuditedSingletonTx for why it is a table lock.
const instanceAISummarySettingsWriteLock = `LOCK TABLE instance_ai_summary_settings IN SHARE ROW EXCLUSIVE MODE`

// scanInstanceAISummarySettings reads one row in instanceAISummarySettingsSelect
// order, converting request_timeout_ms back to a Duration.
func scanInstanceAISummarySettings(row *sql.Row) (*models.InstanceAISummarySettings, error) {
	var s models.InstanceAISummarySettings
	var timeoutMS int64
	if err := row.Scan(
		&s.Enabled, &s.TopN, &s.Style, &s.MaxOutputTokens, &s.PerDocumentChars,
		&s.TotalContextChars, &timeoutMS, &s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.Version,
	); err != nil {
		return nil, err
	}
	s.RequestTimeout = time.Duration(timeoutMS) * time.Millisecond
	return &s, nil
}

// Get retrieves the stored settings, or ErrInstanceAISummarySettingsNotFound
// when none are stored.
func (r *InstanceAISummarySettingsRepository) Get(ctx context.Context) (*models.InstanceAISummarySettings, error) {
	s, err := scanInstanceAISummarySettings(r.db.QueryRowContext(ctx, instanceAISummarySettingsSelect))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get instance AI summary settings: %w", err),
			repositories.ErrInstanceAISummarySettingsNotFound,
		)
	}

	return s, nil
}

// Upsert creates or replaces the stored settings.
func (r *InstanceAISummarySettingsRepository) Upsert(ctx context.Context, s *models.InstanceAISummarySettings) error {
	err := r.db.QueryRowContext(ctx, instanceAISummarySettingsUpsert, instanceAISummarySettingsArgs(s)...).
		Scan(&s.CreatedAt, &s.UpdatedAt, &s.Version)
	if err != nil {
		return fmt.Errorf("failed to upsert instance AI summary settings: %w", err)
	}

	return nil
}

// InsertIfAbsent stores the settings only when no row exists and reports
// whether it did. See insertSingletonIfAbsent.
func (r *InstanceAISummarySettingsRepository) InsertIfAbsent(
	ctx context.Context, s *models.InstanceAISummarySettings,
) (bool, error) {
	query := instanceAISummarySettingsInsert + `
		ON CONFLICT (id) DO NOTHING
		RETURNING created_at, updated_at, version`

	inserted, err := insertSingletonIfAbsent(ctx, r.db, query, instanceAISummarySettingsArgs(s),
		&s.CreatedAt, &s.UpdatedAt, &s.Version)
	if err != nil {
		return false, fmt.Errorf("failed to insert instance AI summary settings: %w", err)
	}

	return inserted, nil
}

// Delete removes the stored settings. Deleting when no row exists is a no-op.
func (r *InstanceAISummarySettingsRepository) Delete(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM instance_ai_summary_settings`); err != nil {
		return fmt.Errorf("failed to delete instance AI summary settings: %w", err)
	}

	return nil
}

// UpsertAudited creates or replaces the stored settings and appends the audit
// entry audit builds, in one transaction. A non-nil expectedVersion must match
// the row read under the lock (see checkSingletonVersion).
func (r *InstanceAISummarySettingsRepository) UpsertAudited(
	ctx context.Context, s *models.InstanceAISummarySettings, expectedVersion *int64,
	audit repositories.InstanceAISummarySettingsAuditFunc,
) error {
	return r.inAuditedTx(ctx, "upsert", func(tx *sql.Tx, before *models.InstanceAISummarySettings) (bool, error) {
		var storedVersion *int64
		if before != nil {
			storedVersion = &before.Version
		}
		if err := checkSingletonVersion(expectedVersion, storedVersion); err != nil {
			return false, err
		}
		err := tx.QueryRowContext(ctx, instanceAISummarySettingsUpsert, instanceAISummarySettingsArgs(s)...).
			Scan(&s.CreatedAt, &s.UpdatedAt, &s.Version)
		if err != nil {
			return false, fmt.Errorf("failed to upsert instance AI summary settings: %w", err)
		}
		return true, appendBuiltSingletonAudit[models.InstanceAISummarySettings](
			ctx, tx, instanceAISummarySettingsSubject, audit, before, s)
	})
}

// DeleteAudited removes the stored settings and appends the audit entry audit
// builds, in one transaction. With no row stored it writes nothing.
func (r *InstanceAISummarySettingsRepository) DeleteAudited(
	ctx context.Context, audit repositories.InstanceAISummarySettingsAuditFunc,
) (bool, error) {
	var deleted bool
	err := r.inAuditedTx(ctx, "delete", func(tx *sql.Tx, before *models.InstanceAISummarySettings) (bool, error) {
		if before == nil {
			return false, nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM instance_ai_summary_settings`); err != nil {
			return false, fmt.Errorf("failed to delete instance AI summary settings: %w", err)
		}
		deleted = true
		return true, appendBuiltSingletonAudit[models.InstanceAISummarySettings](
			ctx, tx, instanceAISummarySettingsSubject, audit, before, nil)
	})
	return deleted, err
}

// inAuditedTx runs change under the table's write lock with the current row;
// see runAuditedSingletonTx.
func (r *InstanceAISummarySettingsRepository) inAuditedTx(
	ctx context.Context, op string,
	change func(tx *sql.Tx, before *models.InstanceAISummarySettings) (bool, error),
) error {
	return runAuditedSingletonTx(ctx, r.db, instanceAISummarySettingsSubject, op, instanceAISummarySettingsWriteLock,
		func(tx *sql.Tx) (*models.InstanceAISummarySettings, error) {
			return scanInstanceAISummarySettings(tx.QueryRowContext(ctx, instanceAISummarySettingsSelect))
		}, change)
}
