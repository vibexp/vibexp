package postgres

import (
	"context"
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

// Get retrieves the stored defaults, or ErrInstanceSearchSettingsNotFound when
// none are stored.
func (r *InstanceSearchSettingsRepository) Get(ctx context.Context) (*models.InstanceSearchSettings, error) {
	query := `SELECT recency_ranking_enabled, rank_weight_relevance, rank_weight_created,
		rank_weight_updated, rank_half_life_days, rank_candidate_cap,
		created_at, updated_at, updated_by, version
		FROM instance_search_settings`

	var s models.InstanceSearchSettings
	err := r.db.QueryRowContext(ctx, query).Scan(
		&s.RecencyRankingEnabled, &s.RankWeightRelevance, &s.RankWeightCreated,
		&s.RankWeightUpdated, &s.RankHalfLifeDays, &s.RankCandidateCap,
		&s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.Version,
	)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get instance search settings: %w", err),
			repositories.ErrInstanceSearchSettingsNotFound,
		)
	}

	return &s, nil
}

// Upsert creates or replaces the stored defaults in one INSERT ... ON CONFLICT
// (id), so two concurrent writers cannot both decide the row is absent.
func (r *InstanceSearchSettingsRepository) Upsert(ctx context.Context, s *models.InstanceSearchSettings) error {
	query := instanceSearchSettingsInsert + `
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

	err := r.db.QueryRowContext(ctx, query, instanceSearchSettingsArgs(s)...).
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
