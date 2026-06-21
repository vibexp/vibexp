package utils

import (
	"fmt"
	"strings"

	"github.com/adrg/frontmatter"
	"gopkg.in/yaml.v3"
)

// FrontMatterResult holds the result of parsing YAML frontmatter from markdown content.
type FrontMatterResult struct {
	// Metadata contains all key-value pairs parsed from the frontmatter block.
	// Only flat string values are supported; non-string values are converted via fmt.Sprintf.
	Metadata map[string]string

	// Body is the markdown content after the frontmatter block has been stripped.
	// If no frontmatter is present, Body equals the original input content.
	Body string

	// HasFrontMatter is true when a valid frontmatter block was found and parsed.
	HasFrontMatter bool
}

// yamlFrontMatterFormat restricts adrg/frontmatter to "---"-delimited YAML blocks.
// The library's default formats must not be used: they auto-detect TOML/JSON
// frontmatter and unmarshal YAML via yaml.v2, whose nested-value representation
// differs from the yaml.v3 output that convertToStringMap flattening relies on.
var yamlFrontMatterFormat = frontmatter.NewFormat("---", "---", yaml.Unmarshal)

// hasFrontMatterDelimiters reports whether content is structurally eligible for
// frontmatter parsing. It pre-validates the cases where adrg/frontmatter is more
// lenient than this package's contract (pinned by frontmatter_test.go):
//   - content must start with "---\n" or "---\r\n" (the library skips leading
//     blank lines before the opening delimiter)
//   - the closing "---" must be followed by a newline (the library accepts a
//     bare trailing "---" at EOF, which this package treats as no frontmatter)
func hasFrontMatterDelimiters(content string) bool {
	var rest string
	switch {
	case strings.HasPrefix(content, "---\r\n"):
		rest = content[5:]
	case strings.HasPrefix(content, "---\n"):
		rest = content[4:]
	default:
		return false
	}
	// Empty frontmatter: closing delimiter immediately follows the opening one.
	return strings.HasPrefix(rest, "---\n") || strings.HasPrefix(rest, "---\r\n") ||
		strings.Contains(rest, "\n---\n") || strings.Contains(rest, "\n---\r\n")
}

// convertToStringMap converts a map[string]interface{} to map[string]string.
// Non-string scalar values are converted via fmt.Sprintf; nil values become "".
func convertToStringMap(rawMap map[string]interface{}) map[string]string {
	metadata := make(map[string]string, len(rawMap))
	for k, v := range rawMap {
		if v == nil {
			metadata[k] = ""
			continue
		}
		if s, ok := v.(string); ok {
			metadata[k] = s
		} else {
			metadata[k] = fmt.Sprintf("%v", v)
		}
	}
	return metadata
}

// ParseFrontMatter parses YAML frontmatter from markdown content.
//
// Frontmatter is a block delimited by "---" on its own line at the start of the
// content and a closing "---" on its own line. For example:
//
//	---
//	name: My Agent
//	description: Does things
//	model: sonnet
//	---
//	Body content here
//
// Rules:
//   - Content must start with "---\n" or "---\r\n" to be considered.
//   - A closing "---" delimiter followed by a newline must be present; otherwise
//     no frontmatter is parsed.
//   - Only YAML frontmatter is supported; TOML/JSON delimiters are not recognized.
//   - Invalid YAML inside the block causes a graceful fallback (no frontmatter).
//   - Non-string scalar YAML values are converted with fmt.Sprintf("%v", v).
//   - Nested/complex YAML values (maps, slices) are converted to their string representation.
//   - An empty frontmatter block ("---\n---\n") produces an empty Metadata map.
func ParseFrontMatter(content string) FrontMatterResult {
	noFrontMatter := FrontMatterResult{
		Metadata:       nil,
		Body:           content,
		HasFrontMatter: false,
	}

	if !hasFrontMatterDelimiters(content) {
		return noFrontMatter
	}

	rawMap := make(map[string]interface{})
	body, err := frontmatter.MustParse(strings.NewReader(content), &rawMap, yamlFrontMatterFormat)
	if err != nil {
		// Covers malformed YAML and, defensively, frontmatter.ErrNotFound.
		return noFrontMatter
	}

	return FrontMatterResult{
		Metadata:       convertToStringMap(rawMap),
		Body:           strings.TrimLeft(string(body), "\r\n"),
		HasFrontMatter: true,
	}
}
