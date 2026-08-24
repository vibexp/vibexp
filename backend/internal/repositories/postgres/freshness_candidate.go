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
// The staleness test is a single comparison rather than an aggregate over
// resource_access_events: `GREATEST(updated_at, <selected last-accessed
// columns>) < now() - threshold`. Two properties of GREATEST make that the
// whole rule:
//
//   - it IGNORES NULLs, so a resource never accessed through the selected
//     mediums falls back to updated_at instead of vanishing from the result;
//   - including updated_at is what makes an edit keep a resource fresh, which
//     is the behaviour the epic specifies for a resource that is being worked
//     on but not re-read.
//
// GREATEST is NULL only when every argument is, and `NULL < x` is NULL, which
// filters the row out -- so a row with a NULL updated_at and no accesses would
// be permanently EXEMPT from staleness, the exact opposite of the rule. That
// is not hypothetical: `updated_at` on all four resource tables carries a
// DEFAULT but no NOT NULL (001_baseline), so an explicit NULL insert is
// accepted. COALESCE to the epoch makes such a row read as maximally stale,
// which is the correct reading of "never touched".
//
// The comparison is strict (`<`), so a resource whose last touch is EXACTLY
// the threshold ago is not yet stale -- "more than N days" as written.
//
// # Scan bound: measured, and deliberately not indexed (#766)
//
// No index backs the staleness predicate, and that is a recorded decision
// rather than an oversight. Measured on PG17, 245k prompts across 10 teams
// (200k in the largest), ~1% of a team's rows stale, all buffers cached.
// Sampled on `prompts`, but the finding is table-independent: the dominant
// cost is the random-uuid `ORDER BY id` every one of the four shares, and the
// GREATEST expression is unsargable in all of them (in `memories` doubly so --
// its updated_at is naive, so the expression wraps it in AT TIME ZONE).
//
//	full drain of the largest team   ~5.9k buffers / ~97ms   (one pass)
//	ONE LIMIT-500 batch              ~55k buffers / ~45ms
//
// Read those as I/O, not wall clock: the batch is FASTER in elapsed time
// because it stops as soon as 500 rows match, while doing ~9x the buffer
// accesses of scanning the entire table once. Draining a team walks the whole
// id space across its batches, so ~246k accesses, ~42x the full scan. Every
// buffer above was a cache hit, which is why 45ms hides it; on a cold cache
// the access count is what would be felt.
//
// The cause is NOT the missing index: `id` is a random uuid, so `ORDER BY id`
// walks the heap in random physical order (~1.0 buffer/row) where a
// team-ordered scan is near-sequential (~0.05). The planner picks that walk
// because it estimates ~66k matching rows when 2.3k match -- a 29x
// overestimate it cannot fix, since the GREATEST expression varies with the
// rule's medium set and so carries no statistics.
//
// The three candidate fixes were measured and rejected:
//
//   - composite (team_id, id): -18% on the largest team, and not chosen at all
//     for small teams (the existing single-column team_id index already serves
//     them: 128 buffers / 2.4ms). Four indexes' write cost for that is a bad
//     trade.
//   - denormalized last_touched_at + (team_id, last_touched_at): -96% and the
//     only sargable option, but WRONG. It can only materialize the any-medium
//     GREATEST, and a medium-scoped rule needs a smaller one: measured, a
//     web-only rule's true answer was 16220 rows where the any-medium column
//     reported 2316. Correctness would need one column per medium subset (15),
//     plus a write-path change that belongs to #730.
//   - `COALESCE(updated_at,'epoch') < threshold` as a sargable pre-filter (it
//     is implied by the GREATEST test, so it never drops a stale row): useless
//     in practice and structurally so -- 96.9% of rows passed it, because a
//     knowledge base is read far more than written, so an old updated_at is
//     the common case rather than the selective one.
//
// Accepted because the cost is small in absolute terms and well placed: the
// pass runs at most once a day per team, serialized under the scheduler's
// advisory lock, off the request path, and batched per resource type.
//
// Revisit when a SINGLE team exceeds ~1M resources of one type, or if this
// query ever moves onto a request path. The fix at that point is the batching,
// not an index -- see #862.
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
//
// The COALESCE is the NULL guard described on ListStaleCandidates: none of the
// four tables declares updated_at NOT NULL.
func freshnessTouchExpression(table string, mediums []string) (string, error) {
	updatedAt := "COALESCE(updated_at, 'epoch'::timestamptz)"
	if table == "memories" {
		updatedAt = "COALESCE(updated_at AT TIME ZONE 'UTC', 'epoch'::timestamptz)"
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
