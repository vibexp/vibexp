package postgres

import (
	"context"
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

// Get retrieves the stored settings, or ErrInstanceAISummarySettingsNotFound
// when none are stored.
func (r *InstanceAISummarySettingsRepository) Get(ctx context.Context) (*models.InstanceAISummarySettings, error) {
	query := `SELECT enabled, top_n, style, max_output_tokens, per_document_chars,
		total_context_chars, request_timeout_ms, created_at, updated_at, updated_by, version
		FROM instance_ai_summary_settings`

	var s models.InstanceAISummarySettings
	var timeoutMS int64
	err := r.db.QueryRowContext(ctx, query).Scan(
		&s.Enabled, &s.TopN, &s.Style, &s.MaxOutputTokens, &s.PerDocumentChars,
		&s.TotalContextChars, &timeoutMS, &s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.Version,
	)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get instance AI summary settings: %w", err),
			repositories.ErrInstanceAISummarySettingsNotFound,
		)
	}
	s.RequestTimeout = time.Duration(timeoutMS) * time.Millisecond

	return &s, nil
}

// Upsert creates or replaces the stored settings in one INSERT ... ON CONFLICT
// (id), so two concurrent writers cannot both decide the row is absent.
func (r *InstanceAISummarySettingsRepository) Upsert(ctx context.Context, s *models.InstanceAISummarySettings) error {
	query := instanceAISummarySettingsInsert + `
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

	err := r.db.QueryRowContext(ctx, query, instanceAISummarySettingsArgs(s)...).
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
