package postgres

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// consumeAccountLink spells out the columns scanAccountToken reads instead of
// using accountLinkColumns. This keeps the two lists the same.
func TestAccountLinkQueriesReturnTheSameColumns(t *testing.T) {
	_, returning, found := strings.Cut(consumeAccountLink, "RETURNING")
	require.True(t, found)

	assert.Equal(t, strings.Fields(accountLinkColumns), strings.Fields(returning))
}
