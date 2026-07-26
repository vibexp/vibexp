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

// TestResourceUsageRoute_Removed guards epic #646 / issue #649: the
// GET /api/v1/resource-usage reporting endpoint is gone, so the router must not
// register it. Same shape and intent as TestAIToolsRoutes_Removed (#613).
//
// It asserts on the ROUTE TABLE rather than on a response status, deliberately.
// The path sat under the authenticated /api/v1 surface, where chi runs the auth
// middleware before it finishes matching, so an unauthenticated request returns
// 401 whether or not the route exists — a status assertion could not tell the two
// apart. Walking the registered patterns can.
func TestResourceUsageRoute_Removed(t *testing.T) {
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))

	var patterns []string
	require.NoError(t, chi.Walk(srv.router,
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			patterns = append(patterns, method+" "+route)
			return nil
		}))
	require.NotEmpty(t, patterns, "route walk found nothing — the guard would pass vacuously")

	for _, p := range patterns {
		assert.NotContains(t, p, "/resource-usage", "resource-usage route still registered: %s", p)
	}
}

// TestResourceUsageRoute_RemovedGuardIsMeaningful proves the guard above can
// actually fail: the same assertion applied to a router that DOES register the
// route must flag it. Without this, a bug in the walk would leave the guard
// silently vacuous.
func TestResourceUsageRoute_RemovedGuardIsMeaningful(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/v1/resource-usage", func(http.ResponseWriter, *http.Request) {})

	var found bool
	require.NoError(t, chi.Walk(r,
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if strings.Contains(method+" "+route, "/resource-usage") {
				found = true
			}
			return nil
		}))
	assert.True(t, found, "the walk must detect a registered /resource-usage route")
}
