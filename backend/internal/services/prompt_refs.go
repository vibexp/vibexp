package services

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// explicitRefPrefix is the canonical prompt reference prefix: @prompt:<slug>.
const explicitRefPrefix = "@prompt:"

var (
	// explicitRefRegex matches an explicit @prompt:slug reference. A match
	// preceded by a word character (mail@prompt:x) is literal text; that check
	// lives in parsePromptReferences because Go's regexp has no lookbehind.
	explicitRefRegex = regexp.MustCompile(`@prompt:([a-zA-Z0-9_-]+)`)
	// legacyRefRegex matches a bare @slug, the pre-#1097 reference syntax. It
	// also matches emails, handles and git@host remotes, which is why a legacy
	// reference only ever expands when it resolves (and never in strict mode).
	legacyRefRegex = regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)
)

// RenderOptions tunes how a prompt is rendered.
type RenderOptions struct {
	// Strict resolves only explicit @prompt:slug references, leaving a bare
	// @word as literal text, and fails the render with ErrUnresolvedReferences
	// when an explicit reference does not resolve (#1097).
	Strict bool
}

// ErrUnresolvedReferences is returned by a strict render when one or more
// explicit @prompt:slug references do not resolve to a prompt in the team.
type ErrUnresolvedReferences struct {
	// Slugs are the unresolved slugs, deduplicated in first-occurrence order.
	Slugs []string
}

func (e *ErrUnresolvedReferences) Error() string {
	return fmt.Sprintf("unresolved prompt references: %s", strings.Join(e.Slugs, ", "))
}

// promptRef is one reference found in a prompt body.
type promptRef struct {
	// Start and End delimit the whole reference (@prompt:slug or @slug) in the
	// parsed body, as byte offsets suitable for slicing.
	Start, End int
	Slug       string
	// Explicit is true for @prompt:slug and false for a legacy bare @slug.
	Explicit bool
}

// parsePromptReferences returns the references in body, ordered by position.
// The caller replaces escaped @@ sequences first, so an escape never parses as
// a reference. A legacy match never overlaps an explicit one: the bare
// "@prompt" prefix of an explicit reference is not itself a reference.
func parsePromptReferences(body string) []promptRef {
	var refs []promptRef
	claimed := make(map[int]bool)

	for _, m := range explicitRefRegex.FindAllStringSubmatchIndex(body, -1) {
		if m[0] > 0 && isRefWordByte(body[m[0]-1]) {
			continue // mail@prompt:x is literal text, not a reference
		}
		refs = append(refs, promptRef{Start: m[0], End: m[1], Slug: body[m[2]:m[3]], Explicit: true})
		claimed[m[0]] = true
	}

	for _, m := range legacyRefRegex.FindAllStringSubmatchIndex(body, -1) {
		if claimed[m[0]] {
			continue
		}
		refs = append(refs, promptRef{Start: m[0], End: m[1], Slug: body[m[2]:m[3]]})
	}

	sort.Slice(refs, func(i, j int) bool { return refs[i].Start < refs[j].Start })
	return refs
}

// isRefWordByte reports whether b is a word character ([A-Za-z0-9_]).
func isRefWordByte(b byte) bool {
	return b == '_' || ('0' <= b && b <= '9') || ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z')
}
