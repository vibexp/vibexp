package postgres

import "strings"

// memoryNameExcerptLen caps the body-derived display name of an untitled memory.
const memoryNameExcerptLen = "120"

// memoryDisplayNameExpr is the ONE definition of the name a memory goes by in
// any query that labels a resource (access stats, relations, similar
// resources, recent comments, search results). A memory's title is optional,
// so the rule is: the trimmed title when it is non-blank, otherwise an excerpt
// of the body. It mirrors the frontend's memoryDisplayName helper closely
// enough for a label — a leading markdown heading marker is dropped and
// whitespace is collapsed so the name never starts with "#" or spans lines;
// anything more (a full markdown strip) is deliberately not attempted in SQL.
//
// alias is the query's alias for the memories table. The expression is NULL
// when that alias is a LEFT JOIN miss, so it composes inside the per-type
// COALESCE the callers build; for a joined row it is never NULL (a blank
// memory yields ”). The body is cut before the regexes run so their cost
// does not grow with an unbounded text.
func memoryDisplayNameExpr(alias string) string {
	const tmpl = `CASE WHEN m.id IS NULL THEN NULL ELSE COALESCE(` +
		`NULLIF(BTRIM(m.title, E' \t\n\r'), ''), ` +
		`RTRIM(LEFT(BTRIM(regexp_replace(regexp_replace(LEFT(m.text, 400), '^\s*#{1,6}\s+', ''), '\s+', ' ', 'g')), ` +
		memoryNameExcerptLen + `))) END`
	return strings.ReplaceAll(tmpl, "m.", alias+".")
}
