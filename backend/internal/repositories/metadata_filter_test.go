package repositories_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

func TestParseMetadataFilter_Valid(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want repositories.MetadataFilter
	}{
		{
			name: "empty string is a no-op filter",
			raw:  "",
			want: nil,
		},
		{
			name: "empty object",
			raw:  `{}`,
			want: repositories.MetadataFilter{},
		},
		{
			name: "single key single value",
			raw:  `{"env":["prod"]}`,
			want: repositories.MetadataFilter{"env": {"prod"}},
		},
		{
			name: "multiple keys and values",
			raw:  `{"env":["prod","staging"],"team":["core"]}`,
			want: repositories.MetadataFilter{"env": {"prod", "staging"}, "team": {"core"}},
		},
		{
			name: "empty value array means key exists",
			raw:  `{"env":[]}`,
			want: repositories.MetadataFilter{"env": {}},
		},
		{
			name: "a key the old allowlist rejected is accepted",
			raw:  `{"spec.type":["openapi"]}`,
			want: repositories.MetadataFilter{"spec.type": {"openapi"}},
		},
		{
			name: "key at the length cap",
			raw:  `{"` + strings.Repeat("k", repositories.MaxMetadataFilterKeyLength) + `":["v"]}`,
			want: repositories.MetadataFilter{strings.Repeat("k", repositories.MaxMetadataFilterKeyLength): {"v"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repositories.ParseMetadataFilter(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseMetadataFilter_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantMsg string
	}{
		{name: "not JSON", raw: `{`, wantMsg: "not valid JSON"},
		{name: "JSON array", raw: `["env"]`, wantMsg: "must be a JSON object"},
		{name: "JSON string", raw: `"env"`, wantMsg: "must be a JSON object"},
		{name: "scalar value", raw: `{"env":"prod"}`, wantMsg: "must be an array of strings"},
		{name: "non-string element", raw: `{"env":[1]}`, wantMsg: "must be an array of strings"},
		{name: "nested object value", raw: `{"env":[{"a":"b"}]}`, wantMsg: "must be an array of strings"},
		{
			name:    "too many keys",
			raw:     manyKeys(repositories.MaxMetadataFilterKeys + 1),
			wantMsg: "at most 10 keys",
		},
		{
			name:    "too many values for one key",
			raw:     manyValues(repositories.MaxMetadataFilterValues + 1),
			wantMsg: "at most 25 are allowed",
		},
		{
			name:    "key too long",
			raw:     `{"` + strings.Repeat("k", repositories.MaxMetadataFilterKeyLength+1) + `":["v"]}`,
			wantMsg: "key length must be at most 255",
		},
		{
			name:    "value too long",
			raw:     `{"env":["` + strings.Repeat("v", repositories.MaxMetadataFilterValueLength+1) + `"]}`,
			wantMsg: "at most 512 are allowed",
		},
		{name: "empty key", raw: `{"":["v"]}`, wantMsg: "keys must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repositories.ParseMetadataFilter(tt.raw)

			require.Error(t, err)
			assert.Nil(t, got)
			// Every rejection must be recognizable as one class, so the transport
			// layers can map it to 400 without matching on message text.
			assert.ErrorIs(t, err, repositories.ErrInvalidMetadataFilter)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// TestValidateMetadataFilter_EnforcesSameLimitsAsParse guards the split the MCP
// tools depend on (#525): they receive an already-decoded map, so validation
// must be reachable without going through the string parser — and must not
// drift from it.
func TestValidateMetadataFilter_EnforcesSameLimitsAsParse(t *testing.T) {
	tests := []struct {
		name    string
		filter  repositories.MetadataFilter
		wantMsg string
	}{
		{name: "nil is valid", filter: nil},
		{name: "empty is valid", filter: repositories.MetadataFilter{}},
		{name: "ordinary filter is valid", filter: repositories.MetadataFilter{"env": {"prod"}}},
		{name: "key exists form is valid", filter: repositories.MetadataFilter{"env": {}}},
		{
			name:    "too many keys",
			filter:  filterWithKeys(repositories.MaxMetadataFilterKeys + 1),
			wantMsg: "at most 10 keys",
		},
		{
			name:    "too many values",
			filter:  repositories.MetadataFilter{"env": make([]string, repositories.MaxMetadataFilterValues+1)},
			wantMsg: "at most 25 are allowed",
		},
		{
			name:    "key too long",
			filter:  repositories.MetadataFilter{strings.Repeat("k", repositories.MaxMetadataFilterKeyLength+1): {"v"}},
			wantMsg: "key length must be at most 255",
		},
		{
			name:    "value too long",
			filter:  repositories.MetadataFilter{"env": {strings.Repeat("v", repositories.MaxMetadataFilterValueLength+1)}},
			wantMsg: "at most 512 are allowed",
		},
		{name: "empty key", filter: repositories.MetadataFilter{"": {"v"}}, wantMsg: "keys must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repositories.ValidateMetadataFilter(tt.filter)

			if tt.wantMsg == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, repositories.ErrInvalidMetadataFilter)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestMetadataFilter_SortedKeys(t *testing.T) {
	filter := repositories.MetadataFilter{"zeta": {"1"}, "alpha": {"2"}, "mid": {"3"}}

	assert.Equal(t, []string{"alpha", "mid", "zeta"}, filter.SortedKeys())
}

func TestParseMetadataResourceType(t *testing.T) {
	tests := []struct {
		raw     string
		want    repositories.MetadataResourceType
		wantOK  bool
		comment string
	}{
		{raw: "artifacts", want: repositories.MetadataResourceArtifacts, wantOK: true},
		{raw: "blueprints", want: repositories.MetadataResourceBlueprints, wantOK: true},
		{raw: "memories", want: repositories.MetadataResourceMemories, wantOK: true},
		{raw: "prompts", wantOK: false, comment: "prompts have no metadata column"},
		{raw: "", wantOK: false},
		{raw: "artifacts; DROP TABLE artifacts", wantOK: false, comment: "the injection the closed map prevents"},
		{raw: "Artifacts", wantOK: false, comment: "match is exact, not case-insensitive"},
	}

	for _, tt := range tests {
		t.Run(tt.raw+tt.comment, func(t *testing.T) {
			got, ok := repositories.ParseMetadataResourceType(tt.raw)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func manyKeys(n int) string {
	var b strings.Builder
	b.WriteString("{")
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"k`)
		b.WriteString(string(rune('a' + i)))
		b.WriteString(`":["v"]`)
	}
	b.WriteString("}")
	return b.String()
}

func manyValues(n int) string {
	var b strings.Builder
	b.WriteString(`{"env":[`)
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"v`)
		b.WriteString(string(rune('a' + i)))
		b.WriteString(`"`)
	}
	b.WriteString("]}")
	return b.String()
}

func filterWithKeys(n int) repositories.MetadataFilter {
	filter := make(repositories.MetadataFilter, n)
	for i := range n {
		filter["k"+string(rune('a'+i))] = []string{"v"}
	}
	return filter
}
