package models

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAssistantName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want *string
	}{
		{name: "empty is not reported", in: "", want: nil},
		{name: "whitespace only is not reported", in: " \t\n ", want: nil},
		{name: "plain name is kept", in: "claude-code", want: strPtrForTest("claude-code")},
		{name: "surrounding whitespace is trimmed", in: "  Cursor \n", want: strPtrForTest("Cursor")},
		{
			name: "exactly the cap is kept whole",
			in:   strings.Repeat("a", MaxAssistantNameLength),
			want: strPtrForTest(strings.Repeat("a", MaxAssistantNameLength)),
		},
		{
			name: "longer than the cap is cut to it",
			in:   strings.Repeat("a", MaxAssistantNameLength+12),
			want: strPtrForTest(strings.Repeat("a", MaxAssistantNameLength)),
		},
		{
			name: "a cut that lands after a space is trimmed again",
			in:   strings.Repeat("a", MaxAssistantNameLength-1) + " tail",
			want: strPtrForTest(strings.Repeat("a", MaxAssistantNameLength-1)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NormalizeAssistantName(tt.in)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, *tt.want, *got)
		})
	}
}

// The cap counts runes, not bytes: the column is varchar(30), which Postgres
// measures in characters, and a byte cut could split a multi-byte rune and
// store invalid UTF-8.
func TestNormalizeAssistantName_CapsByRune(t *testing.T) {
	t.Parallel()

	got := NormalizeAssistantName(strings.Repeat("é", MaxAssistantNameLength+5))

	require.NotNil(t, got)
	assert.Equal(t, MaxAssistantNameLength, utf8.RuneCountInString(*got))
	assert.True(t, utf8.ValidString(*got))
}

func strPtrForTest(s string) *string { return &s }
