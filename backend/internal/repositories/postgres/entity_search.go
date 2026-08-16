package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// The keyword ranking ladder shared by team and project search (#813).
//
// It mirrors SearchKeyword's pass structure (search.go) rather than inventing a
// second one, but reads the `teams` / `projects` tables directly instead of the
// `embeddings` table: neither entity is embedded, and both are short-name rows
// where FTS plus trigram beats an embedding pipeline.
//
// Passes run in order and STOP at the first one that returns rows, so a precise
// query keeps its precise ranking and never has looser matches appended to it:
//
//  1. exact    — slug, id, or (projects) git_url. Score 1.0. The dominant call
//     pattern is "I already know the slug", and it should cost one indexed
//     lookup rather than a full-text match.
//  2. strict   — websearch_to_tsquery, ANDing plain words. Ranked by ts_rank.
//  3. relaxed  — the same lexemes OR-joined, for multi-word natural-language
//     queries that AND to nothing.
//  4. trigram  — word_similarity against the name, for typos AND for
//     slash/hyphen-heavy names. This pass is not a mere fallback: the english
//     parser tokenizes `shaharia-lab/games-for-agents` as a file path, so a
//     query of "games for agents" matches NEITHER FTS pass and only reaches the
//     row here (measured word_similarity 1.0).
//
// The tsquery constants and the trigram threshold are shared with search.go —
// duplicating them would let the two ladders drift apart silently.

// entitySearchSpec describes one searchable table for the ladder. Every field
// that reaches SQL is a compile-time constant assembled in this package: user
// input travels only through bound parameters.
type entitySearchSpec struct {
	// table is the FROM clause including the alias, e.g. "teams t".
	table string
	// columns is the projection of entity columns, in scan order.
	columns string
	// nameExpr / bodyExpr are the qualified columns the FTS vector is built from.
	// They must render an expression byte-identical (modulo qualification) to the
	// migration-016 index expression, or the GIN index is silently unused.
	nameExpr string
	bodyExpr string
	// slugExpr and idExpr are the exact-match identity columns.
	slugExpr string
	idExpr   string
	// extraExactExprs are additional columns an exact match may hit — git_url for
	// projects, nothing for teams.
	extraExactExprs []string
	// accessWhere is the tenancy predicate, written against the alias and reading
	// the user id from $2. Tenancy-only: no role predicates ever appear here
	// (decision D3).
	accessWhere string
	// extraWhere is an optional additional filter — the projects team narrowing.
	// It writes its placeholder as the literal `$T`, which the pass builder
	// replaces with the real position, since that position differs per pass.
	extraWhere string
	// orderTiebreak is appended after the score so results are deterministic —
	// ts_rank and word_similarity tie constantly across short names.
	orderTiebreak string
}

// ftsMatchExpr renders the tsvector both the WHERE match and the ts_rank score
// use. The two-argument to_tsvector (explicit regconfig) keeps it IMMUTABLE so
// the migration-016 GIN index applies — same requirement as ftsExpr in search.go.
func (s entitySearchSpec) ftsMatchExpr() string {
	return fmt.Sprintf(
		"to_tsvector('english', coalesce(%s, '') || ' ' || coalesce(%s, ''))",
		s.nameExpr, s.bodyExpr,
	)
}

// trgmNameExpr renders the name expression the pg_trgm pass matches against,
// kept byte-for-byte in sync with the migration-016 gin_trgm_ops index so the
// `%>` operator stays index-accelerated.
func (s entitySearchSpec) trgmNameExpr() string {
	return fmt.Sprintf("coalesce(%s, '')", s.nameExpr)
}

// exactPredicate matches the identity columns. The id arm binds a SEPARATE
// uuid-typed parameter that is NULL for non-UUID input rather than casting the
// column to text: `id::text = $1` matches no index and turns the cheapest pass
// in the ladder into a full table scan (#812).
func (s entitySearchSpec) exactPredicate() string {
	terms := make([]string, 0, 2+len(s.extraExactExprs))
	terms = append(terms,
		fmt.Sprintf("%s = $1", s.slugExpr),
		fmt.Sprintf("($3::uuid IS NOT NULL AND %s = $3::uuid)", s.idExpr),
	)
	for _, expr := range s.extraExactExprs {
		terms = append(terms, fmt.Sprintf("%s = $1", expr))
	}
	return "(" + strings.Join(terms, " OR ") + ")"
}

const (
	// defaultSearchLimit is what an unset limit becomes. `LIMIT 0` is valid SQL
	// that returns nothing, so passing the zero value of a filters struct through
	// unchanged would turn "I forgot to set a limit" into "nothing matched" —
	// silently, and identically to a genuine miss.
	defaultSearchLimit = 20
	// maxSearchLimit caps what a caller can ask for. These are discovery
	// lookups feeding an agent's context window, not a bulk export.
	maxSearchLimit = 100
)

// clampSearchLimit turns any caller-supplied limit into a usable one: a
// non-positive limit (including a negative, which Postgres rejects outright with
// "LIMIT must not be negative") becomes the default, and anything oversized is
// capped.
func clampSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}

// searchInputs are the values the ladder binds, in whatever positions each pass
// needs them.
type searchInputs struct {
	query  string
	userID string
	// uuidArg is the query parsed as a uuid, or nil — see uuidOrNil. Only the
	// exact pass consumes it.
	uuidArg interface{}
	// teamArg narrows to one team, or nil for every team the caller can read.
	// Only specs with extraWhere consume it.
	teamArg interface{}
	limit   int
}

// searchPass is one rung of the ladder: the full SQL and exactly the arguments
// that SQL references.
//
// Each pass carries its OWN argument list rather than sharing one across the
// ladder, because only the exact pass mentions the uuid parameter. Binding an
// argument the statement never references is not merely wasteful — Postgres
// cannot infer its type and rejects the statement with 42P18 ("could not
// determine data type of parameter"). That makes the positional numbering
// pass-dependent, which is why the numbers are computed rather than fixed.
type searchPass struct {
	name  string
	query string
	args  []interface{}
}

// buildPasses renders the four passes, each with its own SQL and arguments.
func (s entitySearchSpec) buildPasses(in searchInputs) []searchPass {
	fts := s.ftsMatchExpr()
	name := s.trgmNameExpr()

	return []searchPass{
		s.newPass(in, true).build("exact", "1.0", s.exactPredicate()),
		s.newPass(in, false).build("strict",
			fmt.Sprintf("ts_rank(%s, %s)", fts, keywordStrictTSQuery),
			fmt.Sprintf("%s @@ %s", fts, keywordStrictTSQuery)),
		s.newPass(in, false).build("relaxed",
			fmt.Sprintf("ts_rank(%s, %s)", fts, keywordRelaxedTSQuery),
			fmt.Sprintf("%s @@ %s", fts, keywordRelaxedTSQuery)),
		s.newPass(in, false).build("trigram",
			fmt.Sprintf("word_similarity($1, %s)", name),
			fmt.Sprintf("%s %%> $1", name)),
	}
}

// passBuilder assembles one pass's SQL and argument list together, so a
// placeholder number and the value bound to it can never drift apart.
type passBuilder struct {
	spec     entitySearchSpec
	args     []interface{}
	teamArg  int
	limitArg int
}

// newPass binds the arguments every pass shares ($1 query, $2 user), the uuid at
// $3 when the pass is the exact one, then the optional team narrowing and the
// limit — recording the positions those last two landed on.
func (s entitySearchSpec) newPass(in searchInputs, withUUID bool) *passBuilder {
	b := &passBuilder{spec: s, args: []interface{}{in.query, in.userID}}
	if withUUID {
		b.args = append(b.args, in.uuidArg) // $3, referenced only by exactPredicate
	}
	if s.extraWhere != "" {
		b.args = append(b.args, in.teamArg)
		b.teamArg = len(b.args)
	}
	b.args = append(b.args, clampSearchLimit(in.limit))
	b.limitArg = len(b.args)
	return b
}

// build renders the SELECT for one pass around its score and predicate.
func (b *passBuilder) build(name, score, where string) searchPass {
	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT %s, %s AS score FROM %s WHERE %s",
		b.spec.columns, score, b.spec.table, b.spec.accessWhere)
	if b.spec.extraWhere != "" {
		fmt.Fprintf(&sb, " AND %s",
			strings.ReplaceAll(b.spec.extraWhere, "$T", fmt.Sprintf("$%d", b.teamArg)))
	}
	fmt.Fprintf(&sb, " AND %s ORDER BY score DESC, %s LIMIT $%d",
		where, b.spec.orderTiebreak, b.limitArg)
	return searchPass{name: name, query: sb.String(), args: b.args}
}

// scanRowsFunc consumes the rows of one pass, returning how many entities it
// scanned so the ladder can tell whether the pass produced anything.
type scanRowsFunc func(*sql.Rows) (int, error)

// txBeginner is the slice of *database.DB the ladder needs, so the runner can be
// exercised against a sqlmock-backed handle.
type txBeginner interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// runSearchLadder executes the passes in order and stops at the first one that
// produces rows.
//
// The trigram pass needs `pg_trgm.word_similarity_threshold` lowered to 0.3 (the
// default 0.6 rejects a single transposition), and set_config is session-scoped
// — so the whole ladder runs inside one read-only transaction with the
// transaction-local form. Running it against the pool would set the threshold on
// whichever connection served the SET and leave the query on another: wrong, and
// intermittently right, which is worse.
func runSearchLadder(
	ctx context.Context,
	db txBeginner,
	spec entitySearchSpec,
	in searchInputs,
	scan scanRowsFunc,
) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("failed to begin entity search transaction: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back entity search transaction", "error", rbErr)
		}
	}()

	if _, err = tx.ExecContext(ctx,
		"SELECT set_config('pg_trgm.word_similarity_threshold', $1, true)", keywordTrgmThreshold); err != nil {
		return fmt.Errorf("failed to set trgm word_similarity threshold: %w", err)
	}

	for _, pass := range spec.buildPasses(in) {
		found, passErr := runSearchPass(ctx, tx, pass, scan)
		if passErr != nil {
			return fmt.Errorf("%s search pass failed: %w", pass.name, passErr)
		}
		if found > 0 {
			return nil
		}
	}

	return nil
}

// runSearchPass executes one pass and hands its rows to scan.
func runSearchPass(ctx context.Context, tx *sql.Tx, pass searchPass, scan scanRowsFunc) (int, error) {
	rows, err := tx.QueryContext(ctx, pass.query, pass.args...)
	if err != nil {
		return 0, fmt.Errorf("failed to execute search query: %w", err)
	}
	defer rows.Close() //nolint:errcheck // matches every other rows iteration in this package

	found, err := scan(rows)
	if err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("error iterating search results: %w", err)
	}

	return found, nil
}
