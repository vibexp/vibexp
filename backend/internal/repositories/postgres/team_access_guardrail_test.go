package postgres

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file guards the two repository-wide invariants that are enforced by
// convention rather than by the type system:
//
//  1. The team access-control predicate — the row-level tenant-isolation
//     boundary — is hand-copied as raw static SQL across many repositories.
//     TestTeamAccessPredicatesAreCanonical extracts every
//     `EXISTS (SELECT 1 FROM teams ...)` / `EXISTS (SELECT 1 FROM team_members ...)`
//     subexpression from the package sources and asserts each one matches the
//     canonical owner/member forms (see the helpers in sql.go).
//  2. Postgres error mapping goes through the sql.go helpers:
//     no inline "23505"/"23503" SQLSTATE literals and no direct
//     `== sql.ErrNoRows` comparisons outside sql.go.
//
// NON-GOALS — this guardrail is a lexical shape gate, not a semantic one:
//   - It cannot verify the correlation target is the RIGHT outer column; any
//     qualified column reference normalizes to `:team` and passes.
//   - `%s` template slots normalize like bound placeholders, so it cannot
//     detect string-interpolated values inside a predicate.
//   - Predicates assembled by concatenating non-constant parts are invisible
//     to extraction; the ≥100 predicate-count floor is the only backstop.
//   - Raw write-context queries (UPDATE/DELETE) that use the read form are
//     not distinguished from genuine read-access checks.

// Canonical normalized forms of the tenant-isolation predicate. `:p` stands
// for any bound parameter and `:team` for the team-ID comparand (a bound
// parameter or a row-correlated column reference of the outer query).
const (
	canonicalOwnerExists       = "exists (select 1 from teams where id = :team and owner_id = :p)"
	canonicalMemberExists      = "exists (select 1 from team_members where team_id = :team and user_id = :p)"
	canonicalMemberWriteExists = "exists (select 1 from team_members " +
		"where team_id = :team and user_id = :p and role in ('owner', 'admin'))"
)

// teamAccessAllowlist lists normalized predicates, keyed by file name, that
// are intentionally allowed to diverge from the canonical forms. Every entry
// must carry a justification. Keep this list empty unless a predicate is
// genuinely not a tenant-isolation check (or is a documented finding awaiting
// its own fix).
var teamAccessAllowlist = map[string][]string{}

// extractedPredicate is one EXISTS-on-teams/team_members subexpression found
// in a SQL string literal of a non-test package source file.
type extractedPredicate struct {
	file  string
	line  int // approximate: line of the enclosing string literal
	table string
	raw   string
}

func TestTeamAccessPredicatesAreCanonical(t *testing.T) {
	preds := extractTeamAccessPredicates(t)

	// The predicate is copied well over a hundred times; a collapse of this
	// count means the extraction broke, not that the copies disappeared.
	require.GreaterOrEqual(t, len(preds), 100,
		"team-access predicate extraction found suspiciously few predicates; extraction is likely broken")

	allowed := map[string]map[string]bool{}
	for file, entries := range teamAccessAllowlist {
		allowed[file] = map[string]bool{}
		for _, e := range entries {
			allowed[file][e] = true
		}
	}

	checked := 0
	for _, p := range preds {
		normalized := normalizeTeamAccessPredicate(p.raw)

		var ok bool
		switch p.table {
		case "teams":
			ok = normalized == canonicalOwnerExists
		case "team_members":
			ok = normalized == canonicalMemberExists || normalized == canonicalMemberWriteExists
		}
		if !ok && allowed[p.file][normalized] {
			ok = true
		}
		if !ok {
			t.Errorf(
				"%s:%d: non-canonical team-access predicate on %s:\n  normalized: %s\n"+
					"  canonical owner:        %s\n  canonical member:       %s\n  canonical member write: %s",
				p.file, p.line, p.table, normalized,
				canonicalOwnerExists, canonicalMemberExists, canonicalMemberWriteExists,
			)
			continue
		}
		checked++
	}

	t.Logf("validated %d team-access EXISTS predicates across the package", checked)
}

// extractTeamAccessPredicates parses every non-test .go file in the package
// directory and returns each EXISTS subexpression whose subquery selects from
// teams or team_members.
func extractTeamAccessPredicates(t *testing.T) []extractedPredicate {
	t.Helper()

	var preds []extractedPredicate
	forEachPackageFile(t, func(name string, fset *token.FileSet, file *ast.File) {
		for _, lit := range sqlStringLiterals(fset, file) {
			for _, raw := range existsSubexpressions(lit.value) {
				table := existsTableRe.FindStringSubmatch(normalizeSQLText(raw))
				if table == nil {
					continue
				}
				preds = append(preds, extractedPredicate{
					file:  name,
					line:  lit.line,
					table: table[1],
					raw:   raw,
				})
			}
		}
	})
	return preds
}

// forEachPackageFile parses every non-test .go file in the current directory
// (the package directory at test runtime) and invokes fn with its AST.
func forEachPackageFile(t *testing.T, fn func(name string, fset *token.FileSet, file *ast.File)) {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err, "parsing %s", name)
		fn(name, fset, file)
	}
}

type stringLiteral struct {
	value string
	line  int
}

// sqlStringLiterals returns every string constant in the file, joining `+`
// concatenations of literals (squirrel templates are split across lines) into
// a single value so EXISTS expressions are never seen half.
func sqlStringLiterals(fset *token.FileSet, file *ast.File) []stringLiteral {
	var lits []stringLiteral
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if value, ok := stringConstValue(expr); ok {
			lits = append(lits, stringLiteral{value: value, line: fset.Position(expr.Pos()).Line})
			return false // don't descend into the parts of a concatenation
		}
		return true
	})
	return lits
}

// stringConstValue resolves an expression that is entirely composed of string
// literals (possibly concatenated with +) to its constant value.
func stringConstValue(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(e.Value)
		if err != nil {
			return "", false
		}
		return value, true
	case *ast.ParenExpr:
		return stringConstValue(e.X)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, lok := stringConstValue(e.X)
		right, rok := stringConstValue(e.Y)
		if !lok || !rok {
			return "", false
		}
		return left + right, true
	}
	return "", false
}

var existsKeywordRe = regexp.MustCompile(`(?i)\bEXISTS\s*\(`)

// existsTableRe matches only the subqueries this guardrail cares about; other
// EXISTS subqueries (feed_items, prompt_shares, ...) are out of scope.
var existsTableRe = regexp.MustCompile(`^exists \(select 1 from (teams|team_members)\b`)

// existsSubexpressions returns every `EXISTS (...)` subexpression in s,
// including nested ones, with balanced parentheses.
func existsSubexpressions(s string) []string {
	var out []string
	for _, loc := range existsKeywordRe.FindAllStringIndex(s, -1) {
		open := loc[1] - 1 // index of the '(' matched by the regex
		depth := 0
		for i := open; i < len(s); i++ {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 {
				out = append(out, s[loc[0]:i+1])
				break
			}
		}
	}
	return out
}

var (
	whitespaceRe     = regexp.MustCompile(`\s+`)
	placeholderRe    = regexp.MustCompile(`\$\d+|\?|%s`)
	commaSpacingRe   = regexp.MustCompile(`\s*,\s*`)
	openParenSpaceRe = regexp.MustCompile(`\(\s+`)
	closeParenRe     = regexp.MustCompile(`\s+\)`)
	subqueryAliasRe  = regexp.MustCompile(`from (teams|team_members) ([a-z][a-z0-9_]*) where`)

	// Team-ID comparands: a bound parameter, a column reference correlated to
	// the outer query (qualified, e.g. p.team_id or t.id), or — in the teams
	// subquery only — an unqualified team_id (teams has no team_id column, so
	// it can only resolve to the outer row). Self-comparisons (the always-true
	// bug class, issue #1718) are rejected in both spellings: an unqualified
	// team_id inside the team_members subquery (it resolves to
	// team_members.team_id itself) is not matched by memberComparandRe, and a
	// comparand explicitly qualified with the subquery's OWN table — e.g.
	// `team_id = team_members.team_id` inside the team_members EXISTS, or
	// `id = teams.<col>` inside the teams EXISTS — is filtered out by
	// replaceComparand in normalizeTeamAccessPredicate.
	ownerComparandRe  = regexp.MustCompile(`\bid = (:p|[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*|team_id)\b`)
	memberComparandRe = regexp.MustCompile(`\bteam_id = (:p|[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*)\b`)
)

// normalizeSQLText canonicalizes only the lexical shape of a SQL fragment:
// lowercase, single spaces, and no spaces inside parentheses or before commas.
func normalizeSQLText(raw string) string {
	s := strings.ToLower(raw)
	s = whitespaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "exists(", "exists (")
	s = openParenSpaceRe.ReplaceAllString(s, "(")
	s = closeParenRe.ReplaceAllString(s, ")")
	s = commaSpacingRe.ReplaceAllString(s, ", ")
	return s
}

// normalizeTeamAccessPredicate maps one extracted EXISTS subexpression to its
// normalized form: lexical shape via normalizeSQLText, `:p` for every bound
// parameter (or %s template slot), subquery table alias stripped, and the
// team-ID comparand replaced by `:team`.
func normalizeTeamAccessPredicate(raw string) string {
	s := normalizeSQLText(raw)
	s = placeholderRe.ReplaceAllString(s, ":p")

	if m := subqueryAliasRe.FindStringSubmatch(s); m != nil {
		s = strings.Replace(s, m[0], "from "+m[1]+" where", 1)
		s = strings.ReplaceAll(s, m[2]+".", "")
	}

	if strings.Contains(s, "from team_members") {
		s = replaceComparand(s, memberComparandRe, "team_members.", "team_id = :team")
	} else {
		s = replaceComparand(s, ownerComparandRe, "teams.", "id = :team")
	}
	return s
}

// replaceComparand rewrites every comparand match to canon, except when the
// comparand column is qualified with the subquery's own table (ownTable, with
// trailing dot): that is a self-comparison — always true — and must stay
// verbatim so it fails canonicalization.
func replaceComparand(s string, re *regexp.Regexp, ownTable, canon string) string {
	return re.ReplaceAllStringFunc(s, func(match string) string {
		comparand := re.FindStringSubmatch(match)[1]
		if strings.HasPrefix(comparand, ownTable) {
			return match
		}
		return canon
	})
}

// TestNormalizeTeamAccessPredicateSelfComparisons pins the normalizer
// hardening for the always-true self-comparison bug class (issue #1718): a
// comparand qualified with the subquery's own table must NOT normalize to the
// canonical forms, while genuinely correlated comparands still must.
func TestNormalizeTeamAccessPredicateSelfComparisons(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		canonical string
		wantMatch bool
	}{
		{
			name:      "member self-comparison via own-table qualifier is rejected",
			raw:       "EXISTS (SELECT 1 FROM team_members WHERE team_id = team_members.team_id AND user_id = $1)",
			canonical: canonicalMemberExists,
			wantMatch: false,
		},
		{
			name:      "owner self-comparison via own-table qualifier is rejected",
			raw:       "EXISTS (SELECT 1 FROM teams WHERE id = teams.id AND owner_id = $1)",
			canonical: canonicalOwnerExists,
			wantMatch: false,
		},
		{
			name:      "member comparand correlated to the outer query is accepted",
			raw:       "EXISTS (SELECT 1 FROM team_members WHERE team_id = p.team_id AND user_id = $1)",
			canonical: canonicalMemberExists,
			wantMatch: true,
		},
		{
			name:      "member comparand bound to a parameter is accepted",
			raw:       "EXISTS (SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2)",
			canonical: canonicalMemberExists,
			wantMatch: true,
		},
		{
			name:      "owner comparand correlated to the outer query is accepted",
			raw:       "EXISTS (SELECT 1 FROM teams WHERE id = p.team_id AND owner_id = $1)",
			canonical: canonicalOwnerExists,
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := normalizeTeamAccessPredicate(tt.raw)
			if tt.wantMatch {
				require.Equal(t, tt.canonical, normalized)
			} else {
				require.NotEqual(t, tt.canonical, normalized)
			}
		})
	}
}

// TestPostgresErrorHandlingGoesThroughHelpers asserts that the package never
// reintroduces inline Postgres error handling outside sql.go: no
// "23505"/"23503" SQLSTATE string literals (use uniqueViolation /
// isFKViolation) and no direct == / != comparisons against sql.ErrNoRows
// (use mapNoRows). Comments are ignored — only code counts.
func TestPostgresErrorHandlingGoesThroughHelpers(t *testing.T) {
	forEachPackageFile(t, func(name string, fset *token.FileSet, file *ast.File) {
		if name == "sql.go" {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			reportInlineErrorHandling(t, name, fset, n)
			return true
		})
	})
}

// reportInlineErrorHandling fails the test when n is an inline SQLSTATE
// string literal or a direct comparison against sql.ErrNoRows.
func reportInlineErrorHandling(t *testing.T, name string, fset *token.FileSet, n ast.Node) {
	t.Helper()
	switch e := n.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return
		}
		if value, err := strconv.Unquote(e.Value); err == nil &&
			(strings.Contains(value, "23505") || strings.Contains(value, "23503")) {
			t.Errorf("%s:%d: inline SQLSTATE literal %q; use the sql.go helpers",
				name, fset.Position(e.Pos()).Line, value)
		}
	case *ast.BinaryExpr:
		if (e.Op == token.EQL || e.Op == token.NEQ) &&
			(isSQLErrNoRows(e.X) || isSQLErrNoRows(e.Y)) {
			t.Errorf("%s:%d: direct comparison against sql.ErrNoRows; use mapNoRows",
				name, fset.Position(e.Pos()).Line)
		}
	}
}

func isSQLErrNoRows(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "sql" && sel.Sel.Name == "ErrNoRows"
}
