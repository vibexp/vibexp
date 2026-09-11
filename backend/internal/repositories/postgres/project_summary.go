package postgres

import (
	"database/sql"

	"github.com/vibexp/vibexp/internal/models"
)

// The detail reads of the four resource domains resolve their owning project in
// the same query rather than making the client fetch the project list and match
// the id client-side — a lookup that silently returned nothing past the list's
// 100-project cap (issue #929). Mirrors search.go's projectsLeftJoin.
const (
	// projectSummaryProjection is the column list the join contributes. `id` is
	// cast to text because the column is a uuid and the scan target is a string.
	projectSummaryProjection = "proj.id::text, proj.name, proj.slug"
	// projectSummaryJoinOn is the join predicate, parameterised by the source
	// table's alias. LEFT so a resource with no project — or one whose project
	// row is gone — still returns its row, with a nil summary. `projects.id` is
	// the primary key, so the join matches at most one row and cannot duplicate
	// the source row.
	projectSummaryJoinOn = " LEFT JOIN projects proj ON proj.id = "
)

// projectSummaryJoin renders the LEFT JOIN for a source table aliased as alias.
func projectSummaryJoin(alias string) string {
	return projectSummaryJoinOn + alias + ".project_id"
}

// projectSummaryScan holds the join's three nullable columns. Every column is
// NULL together when the outer join misses, so `id` alone decides whether there
// is a summary to report.
type projectSummaryScan struct {
	id   sql.NullString
	name sql.NullString
	slug sql.NullString
}

// value converts the scanned columns to a summary, or nil when the join missed.
func (p *projectSummaryScan) value() *models.ProjectSummary {
	if !p.id.Valid {
		return nil
	}
	return &models.ProjectSummary{ID: p.id.String, Name: p.name.String, Slug: p.slug.String}
}
