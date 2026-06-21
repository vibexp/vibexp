package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func runFrontMatterTests(t *testing.T, tests []struct {
	name         string
	input        string
	wantMetadata map[string]string
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
		wantMetadata map[string]string
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:  "standard frontmatter with name, description, and extra key",
			input: "---\nname: My Agent\ndescription: Does X\nmodel: sonnet\n---\nBody here",
			wantMetadata: map[string]string{
				"name": "My Agent", "description": "Does X", "model": "sonnet",
			},
			wantBody:  "Body here",
			wantHasFM: true,
		},
		{
			name:  "frontmatter with only title key (not name)",
			input: "---\ntitle: My Title\ndescription: Desc\n---\nBody content",
			wantMetadata: map[string]string{
				"title": "My Title", "description": "Desc",
			},
			wantBody:  "Body content",
			wantHasFM: true,
		},
		{
			name:  "name takes priority over title when both present",
			input: "---\nname: Name Value\ntitle: Title Value\n---\nBody",
			wantMetadata: map[string]string{
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
		wantMetadata map[string]string
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:         "empty frontmatter block",
			input:        "---\n---\nBody only",
			wantMetadata: map[string]string{},
			wantBody:     "Body only",
			wantHasFM:    true,
		},
		{
			name:  "frontmatter with multiline body",
			input: "---\nname: Agent\n---\n# Heading\n\nParagraph one.\n\nParagraph two.",
			wantMetadata: map[string]string{
				"name": "Agent",
			},
			wantBody:  "# Heading\n\nParagraph one.\n\nParagraph two.",
			wantHasFM: true,
		},
		{
			name:  "frontmatter body with extra blank lines",
			input: "---\nname: Agent\n---\n\n\nContent after blanks",
			wantMetadata: map[string]string{
				"name": "Agent",
			},
			wantBody:  "Content after blanks",
			wantHasFM: true,
		},
		{
			name:         "frontmatter with empty body after stripping",
			input:        "---\nname: Agent\n---\n",
			wantMetadata: map[string]string{"name": "Agent"},
			wantBody:     "",
			wantHasFM:    true,
		},
	}
	runFrontMatterTests(t, tests)
}

func TestParseFrontMatter_TypeConversion(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMetadata map[string]string
		wantBody     string
		wantHasFM    bool
	}{
		{
			name:  "frontmatter with Windows line endings (CRLF)",
			input: "---\r\nname: Windows Agent\r\ndescription: Windows desc\r\n---\r\nBody content",
			wantMetadata: map[string]string{
				"name": "Windows Agent", "description": "Windows desc",
			},
			wantBody:  "Body content",
			wantHasFM: true,
		},
		{
			name:         "empty frontmatter block with Windows line endings (CRLF)",
			input:        "---\r\n---\r\nBody",
			wantMetadata: map[string]string{},
			wantBody:     "Body",
			wantHasFM:    true,
		},
		{
			name:         "frontmatter with numeric value converted to string",
			input:        "---\nname: Agent\nversion: 3\n---\nContent",
			wantMetadata: map[string]string{"name": "Agent", "version": "3"},
			wantBody:     "Content",
			wantHasFM:    true,
		},
		{
			name:         "frontmatter with boolean value converted to string",
			input:        "---\nname: Agent\nenabled: true\n---\nContent",
			wantMetadata: map[string]string{"name": "Agent", "enabled": "true"},
			wantBody:     "Content",
			wantHasFM:    true,
		},
		{
			name:  "frontmatter with nil value",
			input: "---\nname: Agent\nnullkey:\n---\nContent",
			wantMetadata: map[string]string{
				"name": "Agent", "nullkey": "",
			},
			wantBody:  "Content",
			wantHasFM: true,
		},
	}
	runFrontMatterTests(t, tests)
}

func TestParseFrontMatter_FallbackCases(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMetadata map[string]string
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
		wantMetadata map[string]string
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
			wantMetadata: map[string]string{"name": "Agent"},
			wantBody:     "extra\n---\nBody",
			wantHasFM:    true,
		},
	}
	runFrontMatterTests(t, tests)
}
