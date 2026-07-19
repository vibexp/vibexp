package utils

import (
	"sort"
	"strings"

	"github.com/adrg/frontmatter"
	"gopkg.in/yaml.v3"
)

// FrontMatterResult holds the result of parsing YAML frontmatter from markdown content.
type FrontMatterResult struct {
	// Metadata contains all key-value pairs parsed from the frontmatter block,
	// with nested YAML structure preserved: maps become map[string]any, lists
	// become []any, and scalars keep their YAML type (string, int, float64,
	// bool, or nil). This mirrors what the metadata JSONB column can hold and is
	// required so raw frontmatter can be regenerated faithfully (epic #334).
	Metadata map[string]any

	// Body is the markdown content after the frontmatter block has been stripped.
	// If no frontmatter is present, Body equals the original input content.
	Body string

	// HasFrontMatter is true when a valid frontmatter block was found and parsed.
	HasFrontMatter bool
}

// yamlFrontMatterFormat restricts adrg/frontmatter to "---"-delimited YAML blocks.
// The library's default formats must not be used: they auto-detect TOML/JSON
// frontmatter and unmarshal YAML via yaml.v2, whose map keys decode as
// map[interface{}]interface{} rather than the map[string]interface{} that both
// the metadata JSONB column and SerializeFrontMatter rely on.
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
//   - Nested YAML values (maps, lists) and typed scalars (numbers, bools, nil)
//     are preserved as-is in Metadata (map[string]any); structure is not
//     flattened.
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

	rawMap := make(map[string]any)
	body, err := frontmatter.MustParse(strings.NewReader(content), &rawMap, yamlFrontMatterFormat)
	if err != nil {
		// Covers malformed YAML and, defensively, frontmatter.ErrNotFound.
		return noFrontMatter
	}

	return FrontMatterResult{
		Metadata:       rawMap,
		Body:           strings.TrimLeft(string(body), "\r\n"),
		HasFrontMatter: true,
	}
}

// SerializeFrontMatter regenerates a frontmatter document ("---\n<yaml>\n---\n"
// followed by body) from parsed parts. It is deterministic — map keys are
// emitted in sorted order at every nesting level and scalar formatting is stable
// — so it is idempotent over ParseFrontMatter:
//
//	SerializeFrontMatter(parse(SerializeFrontMatter(parse(x)))) == SerializeFrontMatter(parse(x))
//
// An empty (or nil) metadata map emits an empty frontmatter block ("---\n---\n").
// Callers should restrict metadata to JSON-compatible values (what the metadata
// JSONB column holds) — the same shape ParseFrontMatter produces.
func SerializeFrontMatter(metadata map[string]any, body string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	if len(metadata) > 0 {
		node := mappingNode(metadata)
		if out, err := yaml.Marshal(node); err == nil {
			sb.Write(out) // yaml.Marshal terminates with a newline
		}
	}
	sb.WriteString("---\n")
	sb.WriteString(body)
	return sb.String()
}

// mappingNode builds a yaml.Node for a map with keys sorted, recursing into
// nested maps and slices. Building the node tree explicitly (rather than
// marshaling the Go map directly) pins key order and node styles so the output
// is stable regardless of yaml library internals.
func mappingNode(m map[string]any) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
		node.Content = append(node.Content, keyNode, valueNode(m[k]))
	}
	return node
}

// valueNode builds a yaml.Node for an arbitrary parsed value, sorting nested
// map keys and recursing through slices. Scalars are encoded via yaml's own
// encoder so their type-preserving representation (quoted strings, bare
// numbers/bools, null) matches what ParseFrontMatter reads back.
func valueNode(v any) *yaml.Node {
	switch t := v.(type) {
	case map[string]any:
		return mappingNode(t)
	case []any:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range t {
			seq.Content = append(seq.Content, valueNode(item))
		}
		return seq
	default:
		scalar := &yaml.Node{}
		// Encode returns an error only for unsupported types; fall back to a
		// null node so serialization never panics on unexpected input.
		if err := scalar.Encode(v); err != nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		}
		return scalar
	}
}
