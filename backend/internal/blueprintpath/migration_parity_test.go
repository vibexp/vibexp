package blueprintpath

import (
	"os"
	"strings"
	"testing"
)

// migration007Branches transcribes each (type, subtype) branch of the path
// backfill CASE in migrations/007_blueprint_sync.up.sql. For each branch the
// test asserts (a) the migration file actually contains the SQL fragment, and
// (b) DefaultPath produces the same path the SQL CASE would for a sample slug —
// pinning the Go mapping (#335) and the SQL backfill to each other so they can
// never drift. Keep this table in sync with the .sql CASE.
var migration007Branches = []struct {
	typ, subtype string
	sqlFragment  string // exact substring expected in the up migration
	wantPath     string // DefaultPath(typ, subtype, "sample")
}{
	{"claude", "claude-md", "THEN 'CLAUDE.md'", "CLAUDE.md"},
	{"cursor", "cursor-md", "THEN 'CURSOR.md'", "CURSOR.md"},
	{"codex", "agents-md", "THEN 'AGENTS.md'", "AGENTS.md"},
	{"claude-code", "sub-agents", "THEN '.claude/agents/' || slug || '.md'", ".claude/agents/sample.md"},
	{"claude-code", "skills", "THEN '.claude/skills/' || slug || '/SKILL.md'", ".claude/skills/sample/SKILL.md"},
	{"claude-code", "slash-commands", "THEN '.claude/commands/' || slug || '.md'", ".claude/commands/sample.md"},
	{"claude-code", "others", "THEN '.claude/' || slug || '.md'", ".claude/sample.md"},
	{"cursor", "skills", "THEN '.cursor/skills/' || slug || '/SKILL.md'", ".cursor/skills/sample/SKILL.md"},
	{"cursor", "agents", "THEN '.cursor/agents/' || slug || '.md'", ".cursor/agents/sample.md"},
	{"cursor", "commands", "THEN '.cursor/commands/' || slug || '.md'", ".cursor/commands/sample.md"},
	{"cursor", "rules", "THEN '.cursor/rules/' || slug || '.mdc'", ".cursor/rules/sample.mdc"},
	{"codex", "rules", "THEN '.codex/rules/' || slug || '.md'", ".codex/rules/sample.md"},
	{"codex", "skills", "THEN '.codex/skills/' || slug || '/SKILL.md'", ".codex/skills/sample/SKILL.md"},
	{"codex", "others", "THEN '.codex/' || slug || '.md'", ".codex/sample.md"},
}

func TestMigration007_SQLCaseMatchesDefaultPath(t *testing.T) {
	up, err := os.ReadFile("../../migrations/007_blueprint_sync.up.sql")
	if err != nil {
		t.Fatalf("reading migration: %v", err)
	}
	sql := string(up)

	for _, b := range migration007Branches {
		if !strings.Contains(sql, b.sqlFragment) {
			t.Errorf("migration missing CASE branch for (%s, %s): %q", b.typ, b.subtype, b.sqlFragment)
		}
		got, err := DefaultPath(b.typ, b.subtype, "sample")
		if err != nil {
			t.Errorf("DefaultPath(%s, %s) error: %v", b.typ, b.subtype, err)
			continue
		}
		if got != b.wantPath {
			t.Errorf("DefaultPath(%s, %s) = %q, want %q (SQL parity)", b.typ, b.subtype, got, b.wantPath)
		}
	}

	// The ELSE branch and the (general, "") default must both yield "<slug>.md".
	if !strings.Contains(sql, "ELSE slug || '.md'") {
		t.Error("migration missing the ELSE 'slug.md' fallback")
	}
	if got, gerr := DefaultPath("general", "", "sample"); gerr != nil || got != "sample.md" {
		t.Errorf("DefaultPath(general, \"\") = %q (err %v), want sample.md", got, gerr)
	}

	// Every reverse-default entry that maps to a real forward rule must be
	// covered by the parity table (guards against adding a mapping entry without
	// a matching SQL branch). The (general, "") fallback is the ELSE, checked above.
	covered := map[typeSubtype]bool{{TypeGeneral, ""}: true}
	for _, b := range migration007Branches {
		covered[typeSubtype{b.typ, b.subtype}] = true
	}
	for key := range reverseDefaults {
		if !covered[key] {
			t.Errorf("reverse default (%s, %s) has no migration parity branch", key.typ, key.subtype)
		}
	}
}
