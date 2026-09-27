package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceSearchSettingsRepository implements
// repositories.InstanceSearchSettingsRepository for PostgreSQL.
//
// The table's primary key is `id boolean CHECK (id)`, so it holds at most one
// row and statements carry no WHERE clause and no tenancy or role predicate:
// the scope is the instance itself.
type InstanceSearchSettingsRepository struct {
	db *database.DB
}

// NewInstanceSearchSettingsRepository creates a new InstanceSearchSettingsRepository.
func NewInstanceSearchSettingsRepository(db *database.DB) repositories.InstanceSearchSettingsRepository {
	return &InstanceSearchSettingsRepository{db: db}
}

// instanceSearchSettingsInsert is the column list and values shared by Upsert
// and InsertIfAbsent, in instanceSearchSettingsArgs order. id is omitted: its
// default (true) is the only value the singleton CHECK admits.
const instanceSearchSettingsInsert = `
	INSERT INTO instance_search_settings
	(recency_ranking_enabled, rank_weight_relevance, rank_weight_created,
	rank_weight_updated, rank_half_life_days, rank_candidate_cap, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7)`

func instanceSearchSettingsArgs(s *models.InstanceSearchSettings) []any {
	return []any{
		s.RecencyRankingEnabled, s.RankWeightRelevance, s.RankWeightCreated,
		s.RankWeightUpdated, s.RankHalfLifeDays, s.RankCandidateCap, s.UpdatedBy,
	}
}

// instanceSearchSettingsSelect reads the singleton row; scanInstanceSearchSettings
// reads its columns in this order.
const instanceSearchSettingsSelect = `SELECT recency_ranking_enabled, rank_weight_relevance, rank_weight_created,
		rank_weight_updated, rank_half_life_days, rank_candidate_cap,
		created_at, updated_at, updated_by, version
		FROM instance_search_settings`

// instanceSearchSettingsUpsert creates or replaces the row in one INSERT ... ON
// CONFLICT (id), so two concurrent writers cannot both decide it is absent. It
// repeats instanceSearchSettingsInsert's columns as one literal (same order as
// instanceSearchSettingsArgs) rather than concatenating, so it is a single
// static statement.
const instanceSearchSettingsUpsert = `
	INSERT INTO instance_search_settings
	(recency_ranking_enabled, rank_weight_relevance, rank_weight_created,
	rank_weight_updated, rank_half_life_days, rank_candidate_cap, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id)
		DO UPDATE SET
			recency_ranking_enabled = EXCLUDED.recency_ranking_enabled,
			rank_weight_relevance = EXCLUDED.rank_weight_relevance,
			rank_weight_created = EXCLUDED.rank_weight_created,
			rank_weight_updated = EXCLUDED.rank_weight_updated,
			rank_half_life_days = EXCLUDED.rank_half_life_days,
			rank_candidate_cap = EXCLUDED.rank_candidate_cap,
			updated_by = EXCLUDED.updated_by,
			updated_at = CURRENT_TIMESTAMP,
			version = instance_search_settings.version + 1
		RETURNING created_at, updated_at, version`

func scanInstanceSearchSettings(row *sql.Row) (*models.InstanceSearchSettings, error) {
	var s models.InstanceSearchSettings
	if err := row.Scan(
		&s.RecencyRankingEnabled, &s.RankWeightRelevance, &s.RankWeightCreated,
		&s.RankWeightUpdated, &s.RankHalfLifeDays, &s.RankCandidateCap,
		&s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.Version,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

// Get retrieves the stored defaults, or ErrInstanceSearchSettingsNotFound when
// none are stored.
func (r *InstanceSearchSettingsRepository) Get(ctx context.Context) (*models.InstanceSearchSettings, error) {
	s, err := scanInstanceSearchSettings(r.db.QueryRowContext(ctx, instanceSearchSettingsSelect))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get instance search settings: %w", err),
			repositories.ErrInstanceSearchSettingsNotFound,
		)
	}

	return s, nil
}

// Upsert creates or replaces the stored defaults.
func (r *InstanceSearchSettingsRepository) Upsert(ctx context.Context, s *models.InstanceSearchSettings) error {
	err := r.db.QueryRowContext(ctx, instanceSearchSettingsUpsert, instanceSearchSettingsArgs(s)...).
		Scan(&s.CreatedAt, &s.UpdatedAt, &s.Version)
	if err != nil {
		return fmt.Errorf("failed to upsert instance search settings: %w", err)
	}

	return nil
}

// InsertIfAbsent stores the defaults only when no row exists and reports
// whether it did. See insertSingletonIfAbsent.
func (r *InstanceSearchSettingsRepository) InsertIfAbsent(
	ctx context.Context, s *models.InstanceSearchSettings,
) (bool, error) {
	query := instanceSearchSettingsInsert + `
		ON CONFLICT (id) DO NOTHING
		RETURNING created_at, updated_at, version`

	inserted, err := insertSingletonIfAbsent(ctx, r.db, query, instanceSearchSettingsArgs(s),
		&s.CreatedAt, &s.UpdatedAt, &s.Version)
	if err != nil {
		return false, fmt.Errorf("failed to insert instance search settings: %w", err)
	}

	return inserted, nil
}

// Delete removes the stored defaults. Deleting when no row exists is a no-op.
func (r *InstanceSearchSettingsRepository) Delete(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM instance_search_settings`); err != nil {
		return fmt.Errorf("failed to delete instance search settings: %w", err)
	}

	return nil
}

// instanceSearchSettingsSubject names the table's settings in errors and logs.
const instanceSearchSettingsSubject = "instance search settings"

// UpsertAudited creates or replaces the stored defaults and appends the audit
// entry audit builds, in one transaction.
func (r *InstanceSearchSettingsRepository) UpsertAudited(
	ctx context.Context, s *models.InstanceSearchSettings, audit repositories.InstanceSearchSettingsAuditFunc,
) error {
	return r.inAuditedTx(ctx, "upsert", func(tx *sql.Tx, before *models.InstanceSearchSettings) (bool, error) {
		err := tx.QueryRowContext(ctx, instanceSearchSettingsUpsert, instanceSearchSettingsArgs(s)...).
			Scan(&s.CreatedAt, &s.UpdatedAt, &s.Version)
		if err != nil {
			return false, fmt.Errorf("failed to upsert instance search settings: %w", err)
		}
		return true, appendBuiltSingletonAudit[models.InstanceSearchSettings](
			ctx, tx, instanceSearchSettingsSubject, audit, before, s)
	})
}

// DeleteAudited removes the stored defaults and appends the audit entry audit
// builds, in one transaction. With no row stored it writes nothing.
func (r *InstanceSearchSettingsRepository) DeleteAudited(
	ctx context.Context, audit repositories.InstanceSearchSettingsAuditFunc,
) (bool, error) {
	var deleted bool
	err := r.inAuditedTx(ctx, "delete", func(tx *sql.Tx, before *models.InstanceSearchSettings) (bool, error) {
		if before == nil {
			return false, nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM instance_search_settings`); err != nil {
			return false, fmt.Errorf("failed to delete instance search settings: %w", err)
		}
		deleted = true
		return true, appendBuiltSingletonAudit[models.InstanceSearchSettings](
			ctx, tx, instanceSearchSettingsSubject, audit, before, nil)
	})
	return deleted, err
}

// instanceSearchSettingsWriteLock serializes audited writes; see
// runAuditedSingletonTx for why it is a table lock.
const instanceSearchSettingsWriteLock = `LOCK TABLE instance_search_settings IN SHARE ROW EXCLUSIVE MODE`

// inAuditedTx runs change under the table's write lock with the current row;
// see runAuditedSingletonTx.
func (r *InstanceSearchSettingsRepository) inAuditedTx(
	ctx context.Context, op string,
	change func(tx *sql.Tx, before *models.InstanceSearchSettings) (bool, error),
) error {
	return runAuditedSingletonTx(ctx, r.db, instanceSearchSettingsSubject, op, instanceSearchSettingsWriteLock,
		func(tx *sql.Tx) (*models.InstanceSearchSettings, error) {
			return scanInstanceSearchSettings(tx.QueryRowContext(ctx, instanceSearchSettingsSelect))
		}, change)
}
