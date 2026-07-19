package blueprintpath

import (
	"testing"
)

// TestFromPath_ForwardRules enumerates every forward rule (exact + prefix) with
// a representative path and asserts the resulting (type, subtype). This pins the
// full mapping preserved from the importer's former determineTypeFromPath.
func TestFromPath_ForwardRules(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		typ     string
		subtype string
	}{
		// Exact root files.
		{"claude-md", "CLAUDE.md", "claude", "claude-md"},
		{"cursor-md", "CURSOR.md", "cursor", "cursor-md"},
		{"agents-md", "AGENTS.md", "codex", "agents-md"},

		// .claude prefixes.
		{"claude-sub-agents", ".claude/agents/reviewer.md", "claude-code", "sub-agents"},
		{"claude-skills", ".claude/skills/deploy/SKILL.md", "claude-code", "skills"},
		{"claude-commands", ".claude/commands/ship.md", "claude-code", "slash-commands"},
		{"claude-others", ".claude/settings.md", "claude-code", "others"},

		// .cursor prefixes.
		{"cursor-skills", ".cursor/skills/x/SKILL.md", "cursor", "skills"},
		{"cursor-agents", ".cursor/agents/x.md", "cursor", "agents"},
		{"cursor-commands", ".cursor/commands/x.md", "cursor", "commands"},
		{"cursor-rules", ".cursor/rules/x.mdc", "cursor", "rules"},
		{"cursor-others", ".cursor/anything.md", "cursor", "cursor-md"},

		// .codex + .agents prefixes.
		{"codex-rules", ".codex/rules/x.md", "codex", "rules"},
		{"codex-skills", ".codex/skills/x/SKILL.md", "codex", "skills"},
		{"codex-others", ".codex/x.md", "codex", "others"},
		{"agents-skills", ".agents/skills/x/SKILL.md", "codex", "skills"},
		{"agents-others", ".agents/x.md", "codex", "others"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			typ, subtype, ok := FromPath(c.path)
			if !ok {
				t.Fatalf("FromPath(%q) ok=false, want a specific match", c.path)
			}
			if typ != c.typ || subtype != c.subtype {
				t.Fatalf("FromPath(%q) = (%q, %q), want (%q, %q)", c.path, typ, subtype, c.typ, c.subtype)
			}
		})
	}
}

// TestFromPath_PrecedenceOrder proves that more-specific prefixes shadow their
// parents (first-match-wins over the ordered prefix list) and that the exact map
// is consulted before any prefix.
func TestFromPath_PrecedenceOrder(t *testing.T) {
	cases := []struct {
		path    string
		typ     string
		subtype string
	}{
		{".claude/agents/a.md", "claude-code", "sub-agents"}, // not "others"
		{".claude/skills/s/SKILL.md", "claude-code", "skills"},
		{".claude/commands/c.md", "claude-code", "slash-commands"},
		{".cursor/rules/r.mdc", "cursor", "rules"}, // not "cursor-md"
		{".cursor/skills/s/SKILL.md", "cursor", "skills"},
		{".codex/rules/r.md", "codex", "rules"}, // not "others"
		{".codex/skills/s/SKILL.md", "codex", "skills"},
		{".agents/skills/s/SKILL.md", "codex", "skills"}, // not "others"
	}
	for _, c := range cases {
		typ, subtype, ok := FromPath(c.path)
		if !ok || typ != c.typ || subtype != c.subtype {
			t.Errorf("FromPath(%q) = (%q, %q, %v), want (%q, %q, true)", c.path, typ, subtype, ok, c.typ, c.subtype)
		}
	}
}

// TestFromPath_Fallback covers unmapped paths returning the (general, "", false)
// fallback.
func TestFromPath_Fallback(t *testing.T) {
	for _, p := range []string{"README.md", "docs/guide.md", "", ".github/workflows/ci.yml", "CLAUDE.mdx"} {
		typ, subtype, ok := FromPath(p)
		if ok || typ != TypeGeneral || subtype != "" {
			t.Errorf("FromPath(%q) = (%q, %q, %v), want (general, \"\", false)", p, typ, subtype, ok)
		}
	}
}

// TestDefaultPath_EveryForwardPairHasReverse asserts every distinct (type,
// subtype) produced by a forward rule has a reverse default — acceptance
// criterion: "reverse defaults defined for every (type, subtype) pair that has
// a forward rule".
func TestDefaultPath_EveryForwardPairHasReverse(t *testing.T) {
	seen := map[typeSubtype]bool{}
	for _, r := range rules {
		key := typeSubtype{r.typ, r.subtype}
		if seen[key] {
			continue
		}
		seen[key] = true
		got, err := DefaultPath(r.typ, r.subtype, "sample")
		if err != nil {
			t.Errorf("DefaultPath(%q, %q, ...) has no reverse default: %v", r.typ, r.subtype, err)
			continue
		}
		if got == "" {
			t.Errorf("DefaultPath(%q, %q, ...) returned empty path", r.typ, r.subtype)
		}
	}
}

// TestRoundTrip is the property test: FromPath(DefaultPath(t, s, slug)) == (t, s)
// for every reverse-mapping entry, including the (general, "") fallback.
func TestRoundTrip(t *testing.T) {
	for key := range reverseDefaults {
		path, err := DefaultPath(key.typ, key.subtype, "sample")
		if err != nil {
			t.Fatalf("DefaultPath(%q, %q, ...) error: %v", key.typ, key.subtype, err)
		}
		typ, subtype, _ := FromPath(path)
		if typ != key.typ || subtype != key.subtype {
			t.Errorf("round trip for (%q, %q): DefaultPath=%q FromPath=(%q, %q)", key.typ, key.subtype, path, typ, subtype)
		}
	}
}

// TestDefaultPath_Values pins the exact reverse-default strings per the PRD.
func TestDefaultPath_Values(t *testing.T) {
	cases := []struct {
		typ, subtype, slug, want string
	}{
		{"claude", "claude-md", "x", "CLAUDE.md"},
		{"cursor", "cursor-md", "x", "CURSOR.md"},
		{"codex", "agents-md", "x", "AGENTS.md"},
		{"claude-code", "sub-agents", "reviewer", ".claude/agents/reviewer.md"},
		{"claude-code", "skills", "deploy", ".claude/skills/deploy/SKILL.md"},
		{"claude-code", "slash-commands", "ship", ".claude/commands/ship.md"},
		{"claude-code", "others", "settings", ".claude/settings.md"},
		{"cursor", "skills", "s", ".cursor/skills/s/SKILL.md"},
		{"cursor", "agents", "a", ".cursor/agents/a.md"},
		{"cursor", "commands", "c", ".cursor/commands/c.md"},
		{"cursor", "rules", "r", ".cursor/rules/r.mdc"},
		{"codex", "rules", "r", ".codex/rules/r.md"},
		{"codex", "skills", "s", ".codex/skills/s/SKILL.md"},
		{"codex", "others", "o", ".codex/o.md"},
		{"general", "", "note", "note.md"},
	}
	for _, c := range cases {
		got, err := DefaultPath(c.typ, c.subtype, c.slug)
		if err != nil {
			t.Errorf("DefaultPath(%q, %q, %q) error: %v", c.typ, c.subtype, c.slug, err)
			continue
		}
		if got != c.want {
			t.Errorf("DefaultPath(%q, %q, %q) = %q, want %q", c.typ, c.subtype, c.slug, got, c.want)
		}
	}
}

// TestDefaultPath_Errors covers unknown pairs and the empty-slug guard.
func TestDefaultPath_Errors(t *testing.T) {
	if _, err := DefaultPath("nope", "nope", "x"); err == nil {
		t.Error("DefaultPath with unknown (type, subtype) should error")
	}
	if _, err := DefaultPath("claude", "claude-md", ""); err == nil {
		t.Error("DefaultPath with empty slug should error")
	}
}
