package server

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// TestAIToolsRoutes_Removed guards epic #610 / issue #613: the AI-tool hook ingest
// endpoints and the whole /api/v1/ai-tools/** read API are gone, so the router must
// not register any of those 14 operations. Same intent as
// TestEmbeddingsBackfillRoute_Removed_Returns404 (#146) — a regression guard against
// them being re-added.
//
// It asserts on the ROUTE TABLE rather than on a response status, deliberately.
// Every one of these paths sits under the authenticated /api/v1 surface, where chi
// runs the auth middleware before it finishes matching, so an unauthenticated
// request returns 401 whether or not the route exists — a status assertion could
// not tell the two apart. Walking the registered patterns can.
func TestAIToolsRoutes_Removed(t *testing.T) {
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))

	var patterns []string
	require.NoError(t, chi.Walk(srv.router,
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			patterns = append(patterns, method+" "+route)
			return nil
		}))
	require.NotEmpty(t, patterns, "route walk found nothing — the guard would pass vacuously")

	for _, p := range patterns {
		assert.NotContains(t, p, "/ai-tools", "AI-tools read API route still registered: %s", p)
		assert.NotContains(t, p, "/claude-code/hooks", "hook ingest route still registered: %s", p)
		assert.NotContains(t, p, "/cursor-ide/hooks", "hook ingest route still registered: %s", p)
	}
}

// TestAIToolsRoutes_RemovedGuardIsMeaningful proves the guard above can actually
// fail: the same assertions applied to a router that DOES register an ai-tools
// route must flag it. Without this, a bug in the walk would leave the guard
// silently vacuous.
func TestAIToolsRoutes_RemovedGuardIsMeaningful(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/v1/ai-tools/claude-code/hooks", func(http.ResponseWriter, *http.Request) {})

	var found bool
	require.NoError(t, chi.Walk(r,
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if strings.Contains(method+" "+route, "/ai-tools") {
				found = true
			}
			return nil
		}))
	assert.True(t, found, "the walk must detect a registered /ai-tools route")
}
