package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func runFrontMatterTests(t *testing.T, tests []struct {
	name         string
	input        string
	wantMetadata map[string]any
	wantBody     string
	wantHasFM    bool
}) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseFrontMatter(tt.input)
			assert.Equal(t, tt.wantHasFM, result.HasFrontMatter, "HasFrontMatter mismatch")
			assert.Equal(t, tt.wantBody, result.Body, "Body mismatch")
			assert.Equal(t, tt.wantMetadata, result.Metadata, "Metadata mismatch")
		})
	}
}

func TestParseFrontMatter_KeyExtraction(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMetadata map[string]any
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:  "standard frontmatter with name, description, and extra key",
			input: "---\nname: My Agent\ndescription: Does X\nmodel: sonnet\n---\nBody here",
			wantMetadata: map[string]any{
				"name": "My Agent", "description": "Does X", "model": "sonnet",
			},
			wantBody:  "Body here",
			wantHasFM: true,
		},
		{
			name:  "frontmatter with only title key (not name)",
			input: "---\ntitle: My Title\ndescription: Desc\n---\nBody content",
			wantMetadata: map[string]any{
				"title": "My Title", "description": "Desc",
			},
			wantBody:  "Body content",
			wantHasFM: true,
		},
		{
			name:  "name takes priority over title when both present",
			input: "---\nname: Name Value\ntitle: Title Value\n---\nBody",
			wantMetadata: map[string]any{
				"name": "Name Value", "title": "Title Value",
			},
			wantBody:  "Body",
			wantHasFM: true,
		},
	}
	runFrontMatterTests(t, tests)
}

func TestParseFrontMatter_ContentStripping(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMetadata map[string]any
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:         "empty frontmatter block",
			input:        "---\n---\nBody only",
			wantMetadata: map[string]any{},
			wantBody:     "Body only",
			wantHasFM:    true,
		},
		{
			name:  "frontmatter with multiline body",
			input: "---\nname: Agent\n---\n# Heading\n\nParagraph one.\n\nParagraph two.",
			wantMetadata: map[string]any{
				"name": "Agent",
			},
			wantBody:  "# Heading\n\nParagraph one.\n\nParagraph two.",
			wantHasFM: true,
		},
		{
			name:  "frontmatter body with extra blank lines",
			input: "---\nname: Agent\n---\n\n\nContent after blanks",
			wantMetadata: map[string]any{
				"name": "Agent",
			},
			wantBody:  "Content after blanks",
			wantHasFM: true,
		},
		{
			name:         "frontmatter with empty body after stripping",
			input:        "---\nname: Agent\n---\n",
			wantMetadata: map[string]any{"name": "Agent"},
			wantBody:     "",
			wantHasFM:    true,
		},
	}
	runFrontMatterTests(t, tests)
}

// TestParseFrontMatter_TypedScalars verifies typed scalars are now PRESERVED
// (not stringified): numbers stay numeric, bools stay bool, and an empty value
// stays nil. Flat string-only frontmatter is unchanged.
func TestParseFrontMatter_TypedScalars(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMetadata map[string]any
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:  "frontmatter with Windows line endings (CRLF)",
			input: "---\r\nname: Windows Agent\r\ndescription: Windows desc\r\n---\r\nBody content",
			wantMetadata: map[string]any{
				"name": "Windows Agent", "description": "Windows desc",
			},
			wantBody:  "Body content",
			wantHasFM: true,
		},
		{
			name:         "empty frontmatter block with Windows line endings (CRLF)",
			input:        "---\r\n---\r\nBody",
			wantMetadata: map[string]any{},
			wantBody:     "Body",
			wantHasFM:    true,
		},
		{
			name:         "numeric value preserved as int",
			input:        "---\nname: Agent\nversion: 3\n---\nContent",
			wantMetadata: map[string]any{"name": "Agent", "version": 3},
			wantBody:     "Content",
			wantHasFM:    true,
		},
		{
			name:         "float value preserved as float64",
			input:        "---\nname: Agent\ntemperature: 0.7\n---\nContent",
			wantMetadata: map[string]any{"name": "Agent", "temperature": 0.7},
			wantBody:     "Content",
			wantHasFM:    true,
		},
		{
			name:         "boolean value preserved as bool",
			input:        "---\nname: Agent\nenabled: true\n---\nContent",
			wantMetadata: map[string]any{"name": "Agent", "enabled": true},
			wantBody:     "Content",
			wantHasFM:    true,
		},
		{
			name:         "nil value preserved as nil",
			input:        "---\nname: Agent\nnullkey:\n---\nContent",
			wantMetadata: map[string]any{"name": "Agent", "nullkey": nil},
			wantBody:     "Content",
			wantHasFM:    true,
		},
	}
	runFrontMatterTests(t, tests)
}

// TestParseFrontMatter_NestedStructure verifies nested maps and lists survive
// parsing with structure intact (the core of #336).
func TestParseFrontMatter_NestedStructure(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMetadata map[string]any
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:  "nested map preserved",
			input: "---\nname: Skill\nmetadata:\n  type: user\n  level: 2\n---\nBody",
			wantMetadata: map[string]any{
				"name": "Skill",
				"metadata": map[string]any{
					"type": "user", "level": 2,
				},
			},
			wantBody:  "Body",
			wantHasFM: true,
		},
		{
			name:  "list of strings preserved",
			input: "---\nname: Skill\ntags:\n  - alpha\n  - beta\n---\nBody",
			wantMetadata: map[string]any{
				"name": "Skill",
				"tags": []any{"alpha", "beta"},
			},
			wantBody:  "Body",
			wantHasFM: true,
		},
		{
			name:  "deeply nested map and mixed-type list",
			input: "---\nallowed-tools:\n  - Bash\n  - Read\nconfig:\n  retries: 3\n  nested:\n    on: true\n---\nBody",
			wantMetadata: map[string]any{
				"allowed-tools": []any{"Bash", "Read"},
				"config": map[string]any{
					"retries": 3,
					"nested": map[string]any{
						// yaml.v3 treats bare "on" as a string, not a bool.
						"on": true,
					},
				},
			},
			wantBody:  "Body",
			wantHasFM: true,
		},
	}
	runFrontMatterTests(t, tests)
}

func TestParseFrontMatter_FallbackCases(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMetadata map[string]any
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:         "no frontmatter - plain content",
			input:        "# Just a plain markdown file\n\nWith some content",
			wantMetadata: nil,
			wantBody:     "# Just a plain markdown file\n\nWith some content",
			wantHasFM:    false,
		},
		{
			name:         "malformed frontmatter - no closing delimiter",
			input:        "---\nname: My Agent\ndescription: Does X\nBody without closing delimiter",
			wantMetadata: nil,
			wantBody:     "---\nname: My Agent\ndescription: Does X\nBody without closing delimiter",
			wantHasFM:    false,
		},
		{
			name:         "empty input",
			input:        "",
			wantMetadata: nil,
			wantBody:     "",
			wantHasFM:    false,
		},
		{
			name:         "content starting with --- but no newline after",
			input:        "---name: Test",
			wantMetadata: nil,
			wantBody:     "---name: Test",
			wantHasFM:    false,
		},
		{
			name:         "malformed YAML inside frontmatter",
			input:        "---\nname: [unclosed bracket\n---\nBody",
			wantMetadata: nil,
			wantBody:     "---\nname: [unclosed bracket\n---\nBody",
			wantHasFM:    false,
		},
		{
			name:         "frontmatter with only closing delimiter and no body",
			input:        "---\nname: Agent\n---",
			wantMetadata: nil,
			wantBody:     "---\nname: Agent\n---",
			wantHasFM:    false,
		},
	}
	runFrontMatterTests(t, tests)
}

func TestParseFrontMatter_OnlyYAMLDelimitersAccepted(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMetadata map[string]any
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:         "TOML delimiters are not frontmatter",
			input:        "+++\ntitle = \"TOML\"\n+++\nBody",
			wantMetadata: nil,
			wantBody:     "+++\ntitle = \"TOML\"\n+++\nBody",
			wantHasFM:    false,
		},
		{
			name:         "JSON delimiters are not frontmatter",
			input:        ";;;\n{\"title\": \"JSON\"}\n;;;\nBody",
			wantMetadata: nil,
			wantBody:     ";;;\n{\"title\": \"JSON\"}\n;;;\nBody",
			wantHasFM:    false,
		},
		{
			name:         "explicit ---yaml opening delimiter is not frontmatter",
			input:        "---yaml\nname: Agent\n---\nBody",
			wantMetadata: nil,
			wantBody:     "---yaml\nname: Agent\n---\nBody",
			wantHasFM:    false,
		},
		{
			name:         "leading blank line before opening delimiter is not frontmatter",
			input:        "\n---\nname: Agent\n---\nBody",
			wantMetadata: nil,
			wantBody:     "\n---\nname: Agent\n---\nBody",
			wantHasFM:    false,
		},
		{
			// Divergence from the pre-library parser, pinned intentionally:
			// the library trims whitespace when matching delimiter lines, so a
			// padded "---" inside the block closes the frontmatter early when
			// an exact closing delimiter also exists later.
			name:         "whitespace-padded delimiter line closes the block early",
			input:        "---\nname: Agent\n --- \nextra\n---\nBody",
			wantMetadata: map[string]any{"name": "Agent"},
			wantBody:     "extra\n---\nBody",
			wantHasFM:    true,
		},
	}
	runFrontMatterTests(t, tests)
}

// TestSerializeFrontMatter_Deterministic pins the exact output for representative
// inputs: sorted keys, empty block for empty metadata, verbatim body.
func TestSerializeFrontMatter_Deterministic(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		body     string
		want     string
	}{
		{
			name:     "sorted keys, scalar body",
			metadata: map[string]any{"name": "Agent", "model": "sonnet"},
			body:     "Body",
			want:     "---\nmodel: sonnet\nname: Agent\n---\nBody",
		},
		{
			name:     "nil metadata emits empty block",
			metadata: nil,
			body:     "Body",
			want:     "---\n---\nBody",
		},
		{
			name:     "empty metadata emits empty block",
			metadata: map[string]any{},
			body:     "Body",
			want:     "---\n---\nBody",
		},
		{
			name:     "typed scalars round-trip unquoted",
			metadata: map[string]any{"version": 3, "enabled": true},
			body:     "",
			want:     "---\nenabled: true\nversion: 3\n---\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SerializeFrontMatter(c.metadata, c.body)
			assert.Equal(t, c.want, got)
		})
	}
}

// TestSerializeFrontMatter_Idempotent is the property test from the acceptance
// criteria: serialize(parse(serialize(parse(x)))) == serialize(parse(x)) across
// varied inputs, including nested structure and tricky scalars.
func TestSerializeFrontMatter_Idempotent(t *testing.T) {
	inputs := []string{
		"---\nname: Agent\nmodel: sonnet\n---\nBody here",
		"---\nversion: 3\nenabled: true\ntemperature: 0.7\nnullkey:\n---\nContent",
		"---\nname: Skill\nmetadata:\n  type: user\n  tags:\n    - a\n    - b\n---\n# Heading\n\ntext",
		"---\nquoted: \"123\"\ncolon: \"a: b\"\nspecial: \"line1\\nline2\"\n---\nBody",
		"---\n---\nBody only",
		"# no frontmatter here\n\njust content",
		"---\nallowed-tools:\n  - Bash(git:*)\n  - Read\ndescription: A skill\n---\nText",
	}
	for _, x := range inputs {
		fm := ParseFrontMatter(x)
		y := SerializeFrontMatter(fm.Metadata, fm.Body)
		fm2 := ParseFrontMatter(y)
		y2 := SerializeFrontMatter(fm2.Metadata, fm2.Body)
		assert.Equal(t, y, y2, "not idempotent for input:\n%q\nfirst serialize:\n%q", x, y)
	}
}

// TestSerializeFrontMatter_RoundTripsMetadata verifies that parsing a serialized
// document reproduces the original parsed metadata (structure + types intact).
func TestSerializeFrontMatter_RoundTripsMetadata(t *testing.T) {
	original := "---\nname: Skill\nconfig:\n  retries: 3\n  nested:\n    flag: false\ntags:\n  - x\n  - y\n---\nBody"
	fm := ParseFrontMatter(original)
	serialized := SerializeFrontMatter(fm.Metadata, fm.Body)
	reparsed := ParseFrontMatter(serialized)
	assert.Equal(t, fm.Metadata, reparsed.Metadata, "metadata not preserved through serialize/parse")
	assert.Equal(t, fm.Body, reparsed.Body, "body not preserved through serialize/parse")
}
