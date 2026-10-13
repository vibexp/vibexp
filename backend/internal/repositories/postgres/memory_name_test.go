package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMemoryDisplayNameExpr(t *testing.T) {
	expr := memoryDisplayNameExpr("src")

	// The title wins when it is non-blank; the body is only the fallback.
	assert.Less(t, strings.Index(expr, "src.title"), strings.Index(expr, "src.text"))
	assert.Contains(t, expr, "NULLIF(BTRIM(src.title")
	// A LEFT JOIN miss stays NULL so the callers' per-type COALESCE moves on.
	assert.True(t, strings.HasPrefix(expr, "CASE WHEN src.id IS NULL THEN NULL"))
	// Every column reference follows the caller's alias.
	assert.NotContains(t, expr, "m.")
	assert.NotContains(t, memoryDisplayNameExpr("m"), "src.")
}

// Every query that labels a memory uses the one shared rule; none goes back to
// labelling it by a bare slice of the body.
func TestMemoryLabelQueriesUseTheSharedRule(t *testing.T) {
	accessQuery, _ := buildTopAccessedQuery("team-1", time.Time{}, "", 5)
	queries := map[string]string{
		"related resources": relatedResourceQuery,
		"similar resources": similarInTeamQuery,
		"recent comments":   recentCommentsQuery,
		"top accessed":      accessQuery,
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, query, memoryDisplayNameExpr("m"))
			assert.NotContains(t, query, "LEFT(m.text, 1")
		})
	}
}

func TestMemoryTimestamp_IsAlwaysUTC(t *testing.T) {
	local := time.Date(2026, 10, 12, 23, 43, 22, 0, time.FixedZone("UTC+2", 2*60*60))

	got := memoryTimestamp(local)

	// Same instant, but the wall clock a timezone-less column will keep is UTC's.
	assert.True(t, got.Equal(local))
	assert.Equal(t, time.UTC, got.Location())
	assert.Equal(t, 21, got.Hour())
}
