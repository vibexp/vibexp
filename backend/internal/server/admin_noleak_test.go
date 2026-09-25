package server

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No-leak helpers for the instance-admin team configuration surface (#1140,
// reused by #1141).
//
// Each admin config section is an allowlisted DTO. A schema with
// additionalProperties: false catches an unexpected key only when a test
// happens to produce it; these helpers pin the COMPLETE key set of a response
// instead, so a field that starts flowing through — a new column mapped by
// mistake, a team-scoped struct embedded instead of converted — fails a test
// even when the spec was widened to match.

// assertJSONKeyPaths marshals v (a value, or raw JSON bytes) and requires its
// set of key paths to equal want exactly.
//
// A key path is the dotted chain of object keys from the root, with array
// elements written as `[]`: `values.top_n`, `rules[].project_id`. Every
// object key contributes a path, including those whose value is itself an
// object or array (`values`, `rules`), so the caller lists parents as well as
// leaves. Elements of one array are merged, so fixtures should populate every
// optional-but-present key on at least one element.
func assertJSONKeyPaths(t *testing.T, v any, want ...string) {
	t.Helper()

	raw, ok := v.([]byte)
	if !ok {
		var err error
		raw, err = json.Marshal(v)
		require.NoError(t, err)
	}

	var decoded any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	seen := map[string]struct{}{}
	collectJSONKeyPaths(decoded, "", seen)
	got := make([]string, 0, len(seen))
	for path := range seen {
		got = append(got, path)
	}
	sort.Strings(got)

	wantSorted := slices.Clone(want)
	sort.Strings(wantSorted)
	assert.Equal(t, wantSorted, got, "response key paths differ from the allowlist")
}

// collectJSONKeyPaths walks a decoded JSON value and records every key path.
func collectJSONKeyPaths(node any, prefix string, seen map[string]struct{}) {
	switch typed := node.(type) {
	case map[string]any:
		for key, child := range typed {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			seen[path] = struct{}{}
			collectJSONKeyPaths(child, path, seen)
		}
	case []any:
		for _, child := range typed {
			collectJSONKeyPaths(child, prefix+"[]", seen)
		}
	}
}

// assertBodyExcludes requires that none of the sentinel values planted in a
// fixture appear anywhere in a raw response body — the value-level companion to
// assertJSONKeyPaths, for secrets that could ride along under an allowed key.
func assertBodyExcludes(t *testing.T, body []byte, sentinels ...string) {
	t.Helper()
	for _, sentinel := range sentinels {
		assert.False(t, strings.Contains(string(body), sentinel),
			"response body leaks fixture sentinel %q", sentinel)
	}
}

// TestAssertJSONKeyPaths pins the helper's own path grammar, since #1141 relies
// on it as-is.
func TestAssertJSONKeyPaths(t *testing.T) {
	body := []byte(`{"a":1,"b":{"c":null,"d":[{"e":1},{"f":2}]},"g":[]}`)
	assertJSONKeyPaths(t, body, "a", "b", "b.c", "b.d", "b.d[].e", "b.d[].f", "g")

	seen := map[string]struct{}{}
	collectJSONKeyPaths(map[string]any{"x": []any{[]any{map[string]any{"y": 1}}}}, "", seen)
	assert.Contains(t, seen, "x[][].y")
}
