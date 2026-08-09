package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// lastAccessedTables maps a resource type to the table carrying its
// last-accessed columns.
//
// The four freshness-eligible types only: `project` and `agent` accesses are
// recorded in resource_access_events too, but have no denormalized columns and
// are out of scope for freshness. The literal keys mirror the resource-type
// constants in internal/server (and resourceaccess.Source* for the columns
// below) rather than importing them -- a repository must not depend on the
// service or transport layer.
var lastAccessedTables = map[string]string{
	"prompt":    "prompts",
	"artifact":  "artifacts",
	"blueprint": "blueprints",
	"memory":    "memories",
}

// lastAccessedColumns maps an access source (resourceaccess.SourceWeb, ...) to
// its per-medium column. Identical on all four tables.
var lastAccessedColumns = map[string]string{
	"web": "last_accessed_web_at",
	"cli": "last_accessed_cli_at",
	"mcp": "last_accessed_mcp_at",
	"api": "last_accessed_api_at",
}

// ResourceLastAccessedRepository implements
// repositories.ResourceLastAccessedRepository for PostgreSQL.
type ResourceLastAccessedRepository struct {
	db *database.DB
}

// NewResourceLastAccessedRepository creates a new
// ResourceLastAccessedRepository.
func NewResourceLastAccessedRepository(db *database.DB) repositories.ResourceLastAccessedRepository {
	return &ResourceLastAccessedRepository{db: db}
}

// UpdateLastAccessed advances one resource's per-medium last-accessed column.
//
// The write is monotonic by construction: `GREATEST(col, $1)` cannot move the
// value backwards, so the out-of-order delivery the async access path allows
// (several goroutines persisting events concurrently, each with its own
// database round-trip) cannot make a resource look less recently accessed than
// it is. Postgres's GREATEST ignores NULLs, so the first write on a
// never-accessed resource simply stores the timestamp.
//
// `updated_at` is deliberately NOT touched: it is the edit signal, and a read
// must not look like an edit. On `memories` that also depends on migration 014,
// which narrowed its BEFORE UPDATE trigger so a last-accessed-only write does
// not fire it.
//
// An unknown resource type or source returns
// repositories.ErrUnsupportedLastAccessedResource without touching the
// database, so callers can treat it as the expected no-op it is rather than a
// failure.
func (r *ResourceLastAccessedRepository) UpdateLastAccessed(
	ctx context.Context, resourceType, resourceID, source string, at time.Time,
) error {
	table, ok := lastAccessedTables[resourceType]
	if !ok {
		return fmt.Errorf("%w: resource type %q", repositories.ErrUnsupportedLastAccessedResource, resourceType)
	}
	column, ok := lastAccessedColumns[source]
	if !ok {
		return fmt.Errorf("%w: source %q", repositories.ErrUnsupportedLastAccessedResource, source)
	}

	// Both identifiers come from the closed allowlists above and can never be
	// caller-controlled, so this interpolation cannot carry injection; the
	// timestamp and id remain bound parameters.
	query := fmt.Sprintf(
		`UPDATE %s SET %s = GREATEST(%s, $1) WHERE id = $2`,
		table, column, column,
	)

	if _, err := r.db.ExecContext(ctx, query, at, resourceID); err != nil {
		return fmt.Errorf("failed to update last accessed for %s: %w", resourceType, err)
	}
	return nil
}

// LastAccessedTargets returns the (resourceType -> table) and
// (source -> column) allowlists this repository dispatches on, so tests can
// assert the mapping covers every type and medium the access path produces
// without duplicating the maps.
func LastAccessedTargets() (tables, columns map[string]string) {
	tables = make(map[string]string, len(lastAccessedTables))
	for k, v := range lastAccessedTables {
		tables[k] = v
	}
	columns = make(map[string]string, len(lastAccessedColumns))
	for k, v := range lastAccessedColumns {
		columns[k] = v
	}
	return tables, columns
}
