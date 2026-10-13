package postgres

import (
	"database/sql"

	"github.com/vibexp/vibexp/internal/models"
)

// The channel that last wrote a prompt, artifact, blueprint or memory is not a
// column on the resource row: it is the source of the resource's newest
// resource_write_events row (#1418). Every read of those four tables resolves
// it the same way, with one LEFT JOIN LATERAL aliased lw, and scans it through
// resourceLastWriteScan, the one place a LastWrite is built.
//
// The lateral subquery is LIMIT 1, so it yields at most one row per resource
// and can neither duplicate a resource row nor change a count; LEFT keeps a
// resource that has no write event, whose two columns then read NULL. It runs
// once per row of the page it is joined to, each time as a single descent of
// idx_resource_write_events_resource_latest (migration 026), so a list costs
// one statement however many rows it returns.
//
// There is no team predicate inside the subquery: an event's resource_id is the
// primary key of a row the outer statement has already scoped to its team.

// resourceLastWriteProjection is the last-write projection, in the order
// readers scan it into a resourceLastWriteScan.
const resourceLastWriteProjection = "lw.source, lw.assistant_name"

// resourceLastWriteColumns is resourceLastWriteProjection for the squirrel-built
// list reads.
var resourceLastWriteColumns = []string{"lw.source", "lw.assistant_name"}

// resourceLastWriteLateral returns the LEFT JOIN LATERAL operand that resolves
// the newest write event of a resource table aliased as alias. resourceType is
// the event's singular resource_type (models.RelationResourceType*), never
// caller input: it is spliced into the statement as a literal.
func resourceLastWriteLateral(resourceType, alias string) string {
	return "LATERAL (SELECT e.source, e.assistant_name FROM resource_write_events e " +
		"WHERE e.resource_type = '" + resourceType + "' AND e.resource_id = " + alias + ".id " +
		"ORDER BY e.created_at DESC LIMIT 1) lw ON true"
}

// resourceLastWriteScan holds the two nullable last-write columns while a row
// is scanned. Readers pass its fields to Scan in resourceLastWriteProjection
// order: source, assistantName.
type resourceLastWriteScan struct {
	source        sql.NullString
	assistantName sql.NullString
}

// value converts the scanned columns to a LastWrite. A resource with no write
// event reports a nil source: unknown, never a guessed channel.
func (s *resourceLastWriteScan) value() models.LastWrite {
	var lastWrite models.LastWrite
	if !s.source.Valid {
		return lastWrite
	}
	lastWrite.LastWriteSource = &s.source.String
	if s.assistantName.Valid {
		lastWrite.LastWriteAssistantName = &s.assistantName.String
	}
	return lastWrite
}
