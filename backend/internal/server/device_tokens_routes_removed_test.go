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

// TestDeviceTokenRoutes_Removed guards issue #688: the Firebase/FCM web-push feature
// is gone, so the router must not register POST or DELETE /api/v1/device-tokens.
// Same intent and shape as TestAIToolsRoutes_Removed (#613) — a regression guard
// against the endpoints being re-added.
//
// It asserts on the ROUTE TABLE rather than on a response status, deliberately.
// Both paths sat under the authenticated /api/v1 surface, where chi runs the auth
// middleware before it finishes matching, so an unauthenticated request can return
// 401 whether or not the route exists — a status assertion could not tell the two
// apart. Walking the registered patterns can.
func TestDeviceTokenRoutes_Removed(t *testing.T) {
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))

	var patterns []string
	require.NoError(t, chi.Walk(srv.router,
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			patterns = append(patterns, method+" "+route)
			return nil
		}))
	require.NotEmpty(t, patterns, "route walk found nothing — the guard would pass vacuously")

	for _, p := range patterns {
		assert.NotContains(t, p, "/device-tokens", "device-tokens route still registered: %s", p)
	}
}

// TestDeviceTokenRoutes_RemovedGuardIsMeaningful proves the guard above can actually
// fail: the same assertion applied to a router that DOES register a device-tokens
// route must flag it. Without this, a bug in the walk would leave the guard
// silently vacuous.
func TestDeviceTokenRoutes_RemovedGuardIsMeaningful(t *testing.T) {
	r := chi.NewRouter()
	r.Post("/api/v1/device-tokens", func(http.ResponseWriter, *http.Request) {})

	var found bool
	require.NoError(t, chi.Walk(r,
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if strings.Contains(method+" "+route, "/device-tokens") {
				found = true
			}
			return nil
		}))
	assert.True(t, found, "the walk must detect a registered /device-tokens route")
}
