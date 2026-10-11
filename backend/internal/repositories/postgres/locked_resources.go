package postgres

import (
	"context"
	"database/sql"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The guards for the paths that delete, move or rewrite prompts, blueprints
// and artifacts without going through the resource services (issue #1290):
// project deletion and migration, custom type deletion, custom status
// deletion. Each one lists the locked resources its write would touch and, if
// there are any, refuses with a repositories.LockedResourcesError that names
// them.
//
// A lock predicate is tenancy data, not a role predicate (decision D3): every
// statement is scoped by team_id plus the path's own key, and reads locked_by,
// the one column that decides whether a row is locked (see resourceLockScan).
// The partial indexes idx_{prompts,blueprints,artifacts}_locked (team_id)
// WHERE locked_by IS NOT NULL keep them cheap in a team with few locks.
//
// The statements are whole literals. Every one returns resource_type, id,
// slug, title -- scanLockedResource's order -- and takes its row limit as its
// last parameter.
const (
	// projectLockedResourcesSQL lists the locked resources of project $2 of
	// team $1.
	projectLockedResourcesSQL = `
		SELECT resource_type, id, slug, title
		  FROM (
			SELECT 'prompt' AS resource_type, id::text AS id, slug, name AS title
			  FROM prompts
			 WHERE team_id = $1 AND project_id = $2 AND locked_by IS NOT NULL
			UNION ALL
			SELECT 'blueprint', id::text, slug, title
			  FROM blueprints
			 WHERE team_id = $1 AND project_id = $2 AND locked_by IS NOT NULL
			UNION ALL
			SELECT 'artifact', id::text, slug, title
			  FROM artifacts
			 WHERE team_id = $1 AND project_id = $2 AND locked_by IS NOT NULL
		  ) locked
		 ORDER BY resource_type, title, id
		 LIMIT $3`

	// typeLockedArtifactsSQL lists the locked artifacts of team $1 carrying
	// type $2. Only artifacts consume types today.
	typeLockedArtifactsSQL = `
		SELECT 'artifact', id::text, slug, title
		  FROM artifacts
		 WHERE team_id = $1 AND type = $2 AND locked_by IS NOT NULL
		 ORDER BY title, id
		 LIMIT $3`
)

// scanLockedResource scans one row of a locked-resource statement.
func scanLockedResource(rows *sql.Rows) (models.LockedResourceRef, error) {
	var ref models.LockedResourceRef
	err := rows.Scan(&ref.ResourceType, &ref.ID, &ref.Slug, &ref.Title)
	return ref, err
}

// listLockedResources runs one locked-resource statement, appending the row
// limit as its last argument. The result is never nil.
func listLockedResources(
	ctx context.Context, q rowsQuerier, query string, args ...any,
) ([]models.LockedResourceRef, error) {
	return queryRows(ctx, q, "locked resources", scanLockedResource,
		query, append(args, models.LockedResourceListLimit)...)
}

// refuseIfLocked runs one locked-resource statement and returns a
// repositories.LockedResourcesError carrying reason when it finds a row, nil
// when it finds none.
func refuseIfLocked(
	ctx context.Context, q rowsQuerier, reason error, query string, args ...any,
) error {
	locked, err := listLockedResources(ctx, q, query, args...)
	if err != nil {
		return err
	}
	if len(locked) > 0 {
		return &repositories.LockedResourcesError{Reason: reason, Resources: locked}
	}
	return nil
}
