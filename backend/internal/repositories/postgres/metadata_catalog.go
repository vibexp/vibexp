package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// maxMetadataCatalogLimit bounds a single catalog page. It matches the maximum
// the spec declares for the `limit` parameter.
const maxMetadataCatalogLimit = 500

// metadataCatalogTables is the closed map from resource type to table name. The
// table name is the only fragment interpolated into the catalog SQL, so it may
// only ever come from this map — never from a request value. A resource type
// absent from it is rejected before any SQL is built.
var metadataCatalogTables = map[repositories.MetadataResourceType]string{
	repositories.MetadataResourceArtifacts:  "artifacts",
	repositories.MetadataResourceBlueprints: "blueprints",
	repositories.MetadataResourceMemories:   "memories",
}

// metadataCatalogSource resolves the table to read and the page size to use,
// the prologue both catalog lookups share.
func metadataCatalogSource(query repositories.MetadataCatalogQuery) (table string, limit int, err error) {
	table, ok := metadataCatalogTables[query.ResourceType]
	if !ok {
		return "", 0, fmt.Errorf("unknown metadata resource type: %s", query.ResourceType)
	}
	return table, clampMetadataCatalogLimit(query.Limit), nil
}

// MetadataCatalogRepository enumerates the metadata keys and values in use
// across a team's artifacts, blueprints or memories.
type MetadataCatalogRepository struct {
	db *database.DB
}

// NewMetadataCatalogRepository creates a new MetadataCatalogRepository.
func NewMetadataCatalogRepository(db *database.DB) repositories.MetadataCatalogRepository {
	return &MetadataCatalogRepository{db: db}
}

var _ repositories.MetadataCatalogRepository = (*MetadataCatalogRepository)(nil)

// Keys returns the distinct metadata keys present on the rows the caller can
// read, in ascending order.
func (r *MetadataCatalogRepository) Keys(
	ctx context.Context, query repositories.MetadataCatalogQuery,
) (repositories.MetadataCatalogResult, error) {
	table, limit, err := metadataCatalogSource(query)
	if err != nil {
		return repositories.MetadataCatalogResult{}, err
	}

	builder := psql.Select("DISTINCT k AS entry").
		From(table + " t").
		JoinClause("CROSS JOIN LATERAL jsonb_object_keys(t.metadata) AS k").
		Where(metadataCatalogTenancy(query)).
		OrderBy("entry").
		// One extra row tells us whether more exist without a second count query.
		Limit(metadataCatalogRowLimit(limit))

	return r.collect(ctx, builder, limit, "metadata keys")
}

// Values returns the distinct values stored under query.Key on the rows the
// caller can read, in ascending order.
func (r *MetadataCatalogRepository) Values(
	ctx context.Context, query repositories.MetadataCatalogQuery,
) (repositories.MetadataCatalogResult, error) {
	table, limit, err := metadataCatalogSource(query)
	if err != nil {
		return repositories.MetadataCatalogResult{}, err
	}

	// A value may be stored as a scalar or inside an array, so coerce to an
	// array before expanding it and both shapes flatten to the same value list.
	lateral := "CROSS JOIN LATERAL jsonb_array_elements_text(" +
		"CASE WHEN jsonb_typeof(t.metadata -> ?) = 'array' THEN t.metadata -> ? " +
		"ELSE jsonb_build_array(t.metadata -> ?) END) AS v"

	where := metadataCatalogTenancy(query)
	// jsonb_exists keeps rows lacking the key out of the expansion entirely —
	// without it, `-> key` is NULL and the coercion yields a spurious [null].
	where = append(where, squirrel.Expr("jsonb_exists(t.metadata, ?)", query.Key))
	// An object-valued key has no meaningful value list; its text form would be
	// a JSON blob nobody can filter on.
	where = append(where, squirrel.Expr("jsonb_typeof(t.metadata -> ?) <> 'object'", query.Key))
	where = append(where, squirrel.Expr("v IS NOT NULL"))
	if query.Search != nil && *query.Search != "" {
		where = append(where, squirrel.Expr("v ILIKE ?", "%"+*query.Search+"%"))
	}

	builder := psql.Select("DISTINCT v AS entry").
		From(table+" t").
		JoinClause(lateral, query.Key, query.Key, query.Key).
		Where(where).
		OrderBy("entry").
		Limit(metadataCatalogRowLimit(limit))

	return r.collect(ctx, builder, limit, "metadata values")
}

// metadataCatalogTenancy builds the predicate every catalog query carries: the
// same team scoping and read-access check the corresponding list query uses,
// plus the guards that keep the JSONB expansion well-defined.
func metadataCatalogTenancy(query repositories.MetadataCatalogQuery) squirrel.And {
	where := squirrel.And{
		squirrel.Eq{"t.team_id": query.TeamID},
		teamReadAccess(query.TeamID, query.UserID),
		// jsonb_object_keys and `->` both error on a non-object, and metadata is
		// only meaningful as an object.
		squirrel.Expr("jsonb_typeof(t.metadata) = 'object'"),
	}

	if query.ProjectID != nil && *query.ProjectID != "" {
		where = append(where, squirrel.Eq{"t.project_id": *query.ProjectID})
	}

	return where
}

// collect runs the built query and splits the limit+1 rows into the page and
// the truncation flag.
func (r *MetadataCatalogRepository) collect(
	ctx context.Context, builder squirrel.SelectBuilder, limit int, what string,
) (repositories.MetadataCatalogResult, error) {
	sqlText, args, err := builder.ToSql()
	if err != nil {
		return repositories.MetadataCatalogResult{}, fmt.Errorf("failed to build %s query: %w", what, err)
	}

	rows, err := r.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return repositories.MetadataCatalogResult{}, fmt.Errorf("failed to query %s: %w", what, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	entries := make([]string, 0, limit)
	truncated := false
	for rows.Next() {
		var entry string
		if scanErr := rows.Scan(&entry); scanErr != nil {
			return repositories.MetadataCatalogResult{}, fmt.Errorf("failed to scan %s: %w", what, scanErr)
		}
		if len(entries) == limit {
			truncated = true
			break
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return repositories.MetadataCatalogResult{}, fmt.Errorf("failed to iterate %s: %w", what, err)
	}

	return repositories.MetadataCatalogResult{Entries: entries, Truncated: truncated}, nil
}

// clampMetadataCatalogLimit bounds the caller's limit into [1, 500]; a
// non-positive or oversized value falls back to the maximum.
func clampMetadataCatalogLimit(limit int) int {
	if limit <= 0 || limit > maxMetadataCatalogLimit {
		return maxMetadataCatalogLimit
	}
	return limit
}

// metadataCatalogRowLimit is the SQL LIMIT: the clamped page size plus the one
// extra row that reveals truncation. Bounding inside the positive branch is
// what keeps gosec's G115 (int -> uint64) quiet.
func metadataCatalogRowLimit(limit int) uint64 {
	if limit > 0 && limit <= maxMetadataCatalogLimit {
		return uint64(limit) + 1
	}
	return uint64(maxMetadataCatalogLimit) + 1
}
