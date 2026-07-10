package postgres

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/models"
)

// coverageEntityTypes is the canonical, ordered set of embeddable entity types the
// coverage report covers. It mirrors backfillEntityTypes in the service layer so a
// team's coverage lists exactly the types the live pipeline embeds, in a stable order.
var coverageEntityTypes = []string{"prompt", "artifact", "memory", "blueprint", "feed_item"}

// CountCoverage returns, per entity type, how many of a team's embeddable entities
// exist (total) and how many have an embedding under modelID (embedded). It reuses
// the same source tables and "has an embedding for this model" predicate as the
// backfill (embeddings row with matching entity_type + entity_id + model_id), so the
// counts agree with what missing-only backfill would pick up. Every count is
// team-scoped, which keeps the queries on the (team_id, entity_type) index and off a
// global scan of the un-indexed model_id column. An empty modelID (no active
// provider) matches no embedding row, so embedded is 0 and everything is pending.
func (r *EmbeddingBackfillRepository) CountCoverage(
	ctx context.Context, modelID, teamID string,
) ([]models.EmbeddingCoverageCount, error) {
	counts := make([]models.EmbeddingCoverageCount, 0, len(coverageEntityTypes))
	for _, entityType := range coverageEntityTypes {
		q := backfillQueries[entityType]
		total, embedded, err := r.countCoverageForType(ctx, entityType, q, modelID, teamID)
		if err != nil {
			return nil, err
		}
		counts = append(counts, models.EmbeddingCoverageCount{
			EntityType: entityType,
			Total:      total,
			Embedded:   embedded,
		})
	}
	return counts, nil
}

// countCoverageForType counts one entity type's total rows and how many have an
// embedding for modelID, both scoped to teamID. entityType and the query fragments
// come from the fixed backfillQueries map (never user input), so embedding them as
// literals is injection-safe; teamID ($1) and modelID ($2) are bound parameters.
func (r *EmbeddingBackfillRepository) countCoverageForType(
	ctx context.Context, entityType string, q backfillQuery, modelID, teamID string,
) (total, embedded int64, err error) {
	sql := fmt.Sprintf(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (
				WHERE EXISTS (
					SELECT 1 FROM embeddings emb
					WHERE emb.entity_type = '%s' AND emb.entity_id = %s AND emb.model_id = $2
				)
			) AS embedded
		FROM %s
		WHERE %s = $1`, entityType, q.idExpr, q.from, q.teamCol)

	if scanErr := r.db.QueryRowContext(ctx, sql, teamID, modelID).Scan(&total, &embedded); scanErr != nil {
		return 0, 0, fmt.Errorf("failed to count %s embedding coverage: %w", entityType, scanErr)
	}
	return total, embedded, nil
}
