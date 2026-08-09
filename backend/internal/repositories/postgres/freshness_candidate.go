package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// freshnessCandidateTables maps a freshness resource type to its table. It
// mirrors lastAccessedTables (same four types, same tables); the two are kept
// as separate literals rather than shared because they answer different
// questions and either could gain a type the other must not.
var freshnessCandidateTables = map[string]string{
	"prompt":    "prompts",
	"artifact":  "artifacts",
	"blueprint": "blueprints",
	"memory":    "memories",
}

// freshnessCandidateMediums maps a rule medium to its per-medium
// last-accessed column. "api" has no rule-level spelling (the service layer
// rejects it as rule input) but is included in the ANY-medium set below: an
// access through a generic API client is still an access.
var freshnessCandidateMediums = map[string]string{
	"web": "last_accessed_web_at",
	"cli": "last_accessed_cli_at",
	"mcp": "last_accessed_mcp_at",
	"api": "last_accessed_api_at",
}

// freshnessAnyMediumColumns is the column set an empty Mediums resolves to --
// every medium, so "any" genuinely means any. Listed explicitly and in a fixed
// order so the generated SQL is deterministic (map iteration is not).
var freshnessAnyMediumColumns = []string{
	"last_accessed_web_at",
	"last_accessed_cli_at",
	"last_accessed_mcp_at",
	"last_accessed_api_at",
}

// defaultFreshnessCandidateLimit bounds a query that did not ask for a batch
// size, so a caller bug cannot pull an entire team's resources into memory.
const defaultFreshnessCandidateLimit = 500

// FreshnessCandidateRepository implements
// repositories.FreshnessCandidateRepository for PostgreSQL.
type FreshnessCandidateRepository struct {
	db *database.DB
}

// NewFreshnessCandidateRepository creates a new FreshnessCandidateRepository.
func NewFreshnessCandidateRepository(db *database.DB) repositories.FreshnessCandidateRepository {
	return &FreshnessCandidateRepository{db: db}
}

// ListStaleCandidates returns one batch of resources a rule currently
// considers stale.
//
// The staleness test is a single indexed comparison rather than an aggregate
// over resource_access_events: `GREATEST(updated_at, <selected last-accessed
// columns>) < now() - threshold`. Three properties of GREATEST make that the
// whole rule:
//
//   - it IGNORES NULLs, so a resource never accessed through the selected
//     mediums falls back to updated_at instead of vanishing from the result;
//   - including updated_at is what makes an edit keep a resource fresh, which
//     is the behaviour the epic specifies for a resource that is being worked
//     on but not re-read;
//   - updated_at is NOT NULL on all four tables, so the expression is never
//     NULL and the comparison never silently drops a row.
//
// The comparison is strict (`<`), so a resource whose last touch is EXACTLY
// the threshold ago is not yet stale -- "more than N days" as written.
func (r *FreshnessCandidateRepository) ListStaleCandidates(
	ctx context.Context, query models.FreshnessCandidateQuery,
) ([]models.FreshnessCandidate, error) {
	table, ok := freshnessCandidateTables[query.ResourceType]
	if !ok {
		return nil, fmt.Errorf(
			"%w: resource type %q", repositories.ErrUnsupportedFreshnessResource, query.ResourceType)
	}

	touchExpr, err := freshnessTouchExpression(table, query.Mediums)
	if err != nil {
		return nil, err
	}

	limit := query.Limit
	if limit <= 0 {
		limit = defaultFreshnessCandidateLimit
	}

	// `table` and `touchExpr` are built exclusively from the closed allowlists
	// above -- never from caller input -- so this interpolation cannot carry
	// injection. Everything the caller supplies stays a bound parameter.
	//
	// The optional filters are expressed as `$n IS NULL OR ...` rather than by
	// assembling the predicate conditionally: one query text means one sqlmock
	// shape and one plan to reason about. Each placeholder is cast once, at
	// every use, because lib/pq must infer a single type per placeholder.
	sqlText := fmt.Sprintf(`
		SELECT id, project_id
		FROM %s
		WHERE team_id = $1::uuid
		  AND ($2::uuid IS NULL OR project_id = $2::uuid)
		  AND ($3::uuid IS NULL OR id > $3::uuid)
		  AND %s < now() - make_interval(days => $4::integer)
		ORDER BY id
		LIMIT $5
	`, table, touchExpr)

	var projectID interface{}
	if query.ProjectID != nil {
		projectID = *query.ProjectID
	}
	var afterID interface{}
	if query.AfterID != "" {
		afterID = query.AfterID
	}

	rows, err := r.db.QueryContext(ctx, sqlText, query.TeamID, projectID, afterID, query.ThresholdDays, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list stale %s candidates: %w", query.ResourceType, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close stale candidate rows", "error", closeErr)
		}
	}()

	candidates := make([]models.FreshnessCandidate, 0)
	for rows.Next() {
		candidate := models.FreshnessCandidate{ResourceType: query.ResourceType}
		if err := rows.Scan(&candidate.ResourceID, &candidate.ProjectID); err != nil {
			return nil, fmt.Errorf("failed to scan stale %s candidate: %w", query.ResourceType, err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate stale %s candidates: %w", query.ResourceType, err)
	}
	return candidates, nil
}

// freshnessTouchExpression builds the GREATEST(...) expression that dates a
// resource's most recent touch for the given mediums. An empty mediums slice
// means any medium.
//
// `memories.updated_at` is a NAIVE timestamp while every other column in the
// expression is timestamptz -- it is the only one of the four resource tables
// with that legacy typing. GREATEST over mixed types would resolve to the
// naive type and reinterpret every aware value in the server's timezone, so
// the naive column is converted first. UTC is the right zone because that is
// what the column has always held.
func freshnessTouchExpression(table string, mediums []string) (string, error) {
	updatedAt := "updated_at"
	if table == "memories" {
		updatedAt = "(updated_at AT TIME ZONE 'UTC')"
	}

	columns := freshnessAnyMediumColumns
	if len(mediums) > 0 {
		columns = make([]string, 0, len(mediums))
		for _, medium := range mediums {
			column, ok := freshnessCandidateMediums[medium]
			if !ok {
				return "", fmt.Errorf("%w: medium %q", repositories.ErrUnsupportedFreshnessResource, medium)
			}
			columns = append(columns, column)
		}
	}

	return "GREATEST(" + updatedAt + ", " + strings.Join(columns, ", ") + ")", nil
}

// freshnessCandidateTargets exposes the resource-type and medium allowlists
// this repository dispatches on, so tests can assert the mappings stay in step
// with the values the service layer accepts without duplicating the maps.
func freshnessCandidateTargets() (tables, mediums map[string]string) {
	tables = make(map[string]string, len(freshnessCandidateTables))
	for k, v := range freshnessCandidateTables {
		tables[k] = v
	}
	mediums = make(map[string]string, len(freshnessCandidateMediums))
	for k, v := range freshnessCandidateMediums {
		mediums[k] = v
	}
	return tables, mediums
}
