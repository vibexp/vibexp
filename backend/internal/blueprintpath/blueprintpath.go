// Package blueprintpath is the single, bidirectional source of truth for the
// mapping between a repository-relative file path and a blueprint's
// (type, subtype).
//
// It answers both directions from one data table:
//   - FromPath(path) -> (type, subtype, ok): classify an imported file.
//   - DefaultPath(type, subtype, slug) -> (path, error): the canonical
//     repo-relative path a VibeXP-authored blueprint of that kind lives at.
//
// Import and the future export/materializer both consume this table so the two
// directions can never drift. The forward rules preserve exactly the behavior
// of the importer's former determineTypeFromPath: an exact root-file map is
// consulted first, then an ordered prefix list is scanned first-match-wins
// (so, e.g., ".cursor/rules/" shadows ".cursor/"), with a (general, "")
// fallback when nothing matches.
package blueprintpath

import (
	"fmt"
	"strings"
)

// Blueprint type constants shared by the forward and reverse tables.
const (
	TypeClaude     = "claude"
	TypeCursor     = "cursor"
	TypeCodex      = "codex"
	TypeClaudeCode = "claude-code"
	// TypeGeneral is returned by FromPath when no rule matches.
	TypeGeneral = "general"
)

// revKind selects how a rule's canonical default path is built from its match
// string and a slug. revNone marks a forward rule that is not the canonical
// reverse for its (type, subtype) — several forward rules may share a pair (e.g.
// both ".codex/skills/" and ".agents/skills/" yield (codex, skills)), and
// exactly one carries a reverse kind so the reverse mapping stays unambiguous.
type revKind int

const (
	revNone  revKind = iota // not the canonical reverse for its (type, subtype)
	revExact                // the match string itself (root files)
	revFlat                 // match + slug + ".md"
	revSkill                // match + slug + "/SKILL.md" (Agent Skill directory)
	revMDC                  // match + slug + ".mdc" (Cursor rules)
)

// build renders the default path for this reverse kind given the rule's match
// string and a slug.
func (k revKind) build(match, slug string) string {
	switch k {
	case revExact:
		return match
	case revFlat:
		return match + slug + ".md"
	case revSkill:
		return match + slug + "/SKILL.md"
	case revMDC:
		return match + slug + ".mdc"
	default:
		return ""
	}
}

// rule is one entry of the mapping table. A rule matches a path either exactly
// (isPrefix=false, path == match) or by prefix (isPrefix=true,
// strings.HasPrefix(path, match)). rev, when not revNone, marks this rule as the
// canonical reverse default for its (typ, subtype).
type rule struct {
	match    string
	isPrefix bool
	typ      string
	subtype  string
	rev      revKind
}

// rules is the one ordered mapping table backing both directions.
//
// Ordering is significant for FromPath: exact rules are listed first and match
// by full-string equality, then prefix rules are scanned in order with
// first-match-wins. This reproduces the importer's historical "exact map, then
// ordered prefix list" precedence — more-specific prefixes (".claude/agents/")
// precede their parent (".claude/").
var rules = []rule{
	// Exact root-level files.
	{match: "CLAUDE.md", typ: TypeClaude, subtype: "claude-md", rev: revExact},
	{match: "CURSOR.md", typ: TypeCursor, subtype: "cursor-md", rev: revExact},
	{match: "AGENTS.md", typ: TypeCodex, subtype: "agents-md", rev: revExact},

	// Prefix rules, most specific first.
	{match: ".claude/agents/", isPrefix: true, typ: TypeClaudeCode, subtype: "sub-agents", rev: revFlat},
	{match: ".claude/skills/", isPrefix: true, typ: TypeClaudeCode, subtype: "skills", rev: revSkill},
	{match: ".claude/commands/", isPrefix: true, typ: TypeClaudeCode, subtype: "slash-commands", rev: revFlat},
	{match: ".claude/", isPrefix: true, typ: TypeClaudeCode, subtype: "others", rev: revFlat},

	{match: ".cursor/skills/", isPrefix: true, typ: TypeCursor, subtype: "skills", rev: revSkill},
	{match: ".cursor/agents/", isPrefix: true, typ: TypeCursor, subtype: "agents", rev: revFlat},
	{match: ".cursor/commands/", isPrefix: true, typ: TypeCursor, subtype: "commands", rev: revFlat},
	{match: ".cursor/rules/", isPrefix: true, typ: TypeCursor, subtype: "rules", rev: revMDC},
	// (cursor, cursor-md) is also reachable via the ".cursor/" prefix, but its
	// canonical reverse default is the root CURSOR.md exact rule above.
	{match: ".cursor/", isPrefix: true, typ: TypeCursor, subtype: "cursor-md"},

	{match: ".codex/rules/", isPrefix: true, typ: TypeCodex, subtype: "rules", rev: revFlat},
	{match: ".codex/skills/", isPrefix: true, typ: TypeCodex, subtype: "skills", rev: revSkill},
	{match: ".codex/", isPrefix: true, typ: TypeCodex, subtype: "others", rev: revFlat},
	// (codex, skills) and (codex, others) are also reachable via ".agents/…";
	// their canonical reverse defaults are the ".codex/…" rules above.
	{match: ".agents/skills/", isPrefix: true, typ: TypeCodex, subtype: "skills"},
	{match: ".agents/", isPrefix: true, typ: TypeCodex, subtype: "others"},
}

// typeSubtype keys the reverse mapping.
type typeSubtype struct {
	typ     string
	subtype string
}

// reverseDefaults maps (type, subtype) -> canonical default-path builder. It is
// derived once from the rules table plus the (general, "") fallback, and panics
// at init if two rules claim the same (type, subtype) as their reverse — a
// programming error that would make DefaultPath ambiguous.
var reverseDefaults = buildReverseDefaults()

func buildReverseDefaults() map[typeSubtype]func(string) string {
	m := make(map[typeSubtype]func(string) string)
	for _, r := range rules {
		if r.rev == revNone {
			continue
		}
		key := typeSubtype{r.typ, r.subtype}
		if _, dup := m[key]; dup {
			panic(fmt.Sprintf("blueprintpath: duplicate reverse default for (%s, %s)", r.typ, r.subtype))
		}
		match, kind := r.match, r.rev
		m[key] = func(slug string) string { return kind.build(match, slug) }
	}
	// The (general, "") fallback materializes as a flat root-level "<slug>.md",
	// which FromPath classifies back to (general, "") — keeping the round trip
	// total over every reverse entry.
	m[typeSubtype{TypeGeneral, ""}] = func(slug string) string { return slug + ".md" }
	return m
}

// FromPath classifies a repository-relative path into a blueprint (type,
// subtype). ok is true when a specific rule matched; when nothing matches it
// returns (TypeGeneral, "", false). The exact-then-prefix, first-match-wins
// precedence matches the importer's historical behavior exactly.
func FromPath(path string) (typ, subtype string, ok bool) {
	for _, r := range rules {
		if r.isPrefix {
			if strings.HasPrefix(path, r.match) {
				return r.typ, r.subtype, true
			}
		} else if path == r.match {
			return r.typ, r.subtype, true
		}
	}
	return TypeGeneral, "", false
}

// ValidateRelativePath rejects a repo-relative path that is unsafe to
// materialize into a working tree: empty, absolute (leading "/"), containing a
// backslash, starting with "./", or containing a ".." path segment. It is the
// single validation used for both blueprint paths (#339) and attachment
// relative paths (#338). A valid path uses forward slashes and stays within the
// tree.
func ValidateRelativePath(p string) error {
	if p == "" {
		return fmt.Errorf("path must not be empty")
	}
	if strings.Contains(p, "\\") {
		return fmt.Errorf("path must not contain backslashes")
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("path must be relative (no leading %q)", "/")
	}
	if strings.HasPrefix(p, "./") {
		return fmt.Errorf("path must not start with %q", "./")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("path must not contain %q segments", "..")
		}
	}
	return nil
}

// DefaultPath returns the canonical repo-relative path for a VibeXP-authored
// blueprint of the given (type, subtype) and slug. It errors when the pair has
// no reverse default. slug must be non-empty. By construction
// FromPath(DefaultPath(t, s, slug)) == (t, s) for every mapping entry.
func DefaultPath(typ, subtype, slug string) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("blueprintpath: slug must not be empty")
	}
	build, ok := reverseDefaults[typeSubtype{typ, subtype}]
	if !ok {
		return "", fmt.Errorf("blueprintpath: no default path for type %q subtype %q", typ, subtype)
	}
	return build(slug), nil
}
