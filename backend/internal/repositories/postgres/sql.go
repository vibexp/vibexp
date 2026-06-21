package postgres

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
)

// Query construction rule for this package:
//
//	dynamic SQL  → squirrel (build through psql)
//	static SQL   → raw query strings (later: sqlc, see #1588)
//
// "Dynamic" means the WHERE/IN/ORDER BY/LIMIT shape varies with the inputs.
// Hand-assembling such SQL with fmt.Sprintf and manual $n counters is the
// exact bug class squirrel eliminates, so it is not allowed here. Static
// queries whose text never changes stay as plain strings.
//
// psql is the shared squirrel statement builder configured for PostgreSQL
// ($1, $2, ...) placeholders. lib/pq rejects the default `?` placeholders,
// so every dynamic query in this package must build through psql.
var psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

// Postgres error codes detected by the repositories in this package. The
// SQLSTATE string literals live only here; call sites go through
// uniqueViolation / isFKViolation. Untyped so they compare against
// pq.ErrorCode without naming that deprecated type.
const (
	uniqueViolationCode = "23505"
	fkViolationCode     = "23503"
)

// uniqueViolation returns the underlying *pq.Error when err is a Postgres
// unique-constraint violation (SQLSTATE 23505), and nil otherwise, so call
// sites that need the violated constraint (Detail/Constraint) get it in the
// same check.
func uniqueViolation(err error) *pq.Error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == uniqueViolationCode {
		return pqErr
	}
	return nil
}

// isFKViolation reports whether err is a Postgres foreign-key-constraint
// violation (SQLSTATE 23503).
func isFKViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == fkViolationCode
}

// Team access-control predicate. This is the row-level tenant-isolation
// boundary: a user may read a team's resources when they own the team or are
// a member of it, and may write (update/delete) when they own the team or are
// a member with role owner/admin. The canonical forms are:
//
//	EXISTS (SELECT 1 FROM teams WHERE id = <team> AND owner_id = <user>)
//	OR EXISTS (SELECT 1 FROM team_members WHERE team_id = <team> AND user_id = <user>)
//
// with the write variant appending `AND role IN ('owner', 'admin')` to the
// team_members EXISTS. All dynamic (squirrel-built) queries must build the
// predicate through the helpers below. The many raw static-SQL copies of the
// same predicate across this package are not rewritten (see #1588 for the
// static-SQL story); they are guarded by team_access_guardrail_test.go, which
// extracts every EXISTS-on-teams/team_members subexpression from the package
// sources and asserts it matches the canonical forms.

// teamReadAccess returns the canonical read-access predicate for a bound
// team ID: the user owns the team or is a member of it.
func teamReadAccess(teamID, userID string) squirrel.Sqlizer {
	return squirrel.Expr(
		"(EXISTS (SELECT 1 FROM teams WHERE id = ? AND owner_id = ?) "+
			"OR EXISTS (SELECT 1 FROM team_members WHERE team_id = ? AND user_id = ?))",
		teamID, userID, teamID, userID,
	)
}

// teamRowReadAccess is teamReadAccess for queries that check access per row:
// teamIDColumn is a column reference of the surrounding query (for example
// "a.team_id"), correlated into the EXISTS subqueries instead of a bound
// team-ID parameter. teamIDColumn must be a compile-time constant column
// reference, never user input — it is interpolated into the SQL text.
func teamRowReadAccess(teamIDColumn, userID string) squirrel.Sqlizer {
	return squirrel.Expr(
		fmt.Sprintf(
			"(EXISTS (SELECT 1 FROM teams t WHERE t.id = %s AND t.owner_id = ?) "+
				"OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = %s AND tm.user_id = ?))",
			teamIDColumn, teamIDColumn,
		),
		userID, userID,
	)
}

// teamWriteAccess returns the canonical write-access predicate for a bound
// team ID: the user owns the team or is a member with role owner or admin.
// It currently has no dynamic (squirrel) production call sites — all
// write-form predicates today are static SQL guarded by the guardrail test —
// and exists to pin the canonical write form for future dynamic sites and
// the #1588 migration.
func teamWriteAccess(teamID, userID string) squirrel.Sqlizer {
	return squirrel.Expr(
		"(EXISTS (SELECT 1 FROM teams WHERE id = ? AND owner_id = ?) "+
			"OR EXISTS (SELECT 1 FROM team_members "+
			"WHERE team_id = ? AND user_id = ? AND role IN ('owner', 'admin')))",
		teamID, userID, teamID, userID,
	)
}

// mapNoRows returns noRows when err is (or wraps) sql.ErrNoRows, and err
// unchanged otherwise. It exists so every repository maps "no rows" through
// one place instead of hand-writing the comparison (historically a mix of
// `err == sql.ErrNoRows` and `errors.Is`).
func mapNoRows(err, noRows error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return noRows
	}
	return err
}
