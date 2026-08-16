package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// trailingSlashTestServer builds a server with the real route tree. The
// container is nil, so every authenticated route answers 401 — which is
// exactly what these assertions need: they are about which ROUTE a path
// resolves to, and a 401 proves the request reached an authenticated route
// rather than falling through to the NotFound handler.
func trailingSlashTestServer(t *testing.T) *Server {
	t.Helper()
	return New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
}

func statusFor(t *testing.T, srv *Server, path string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr.Code
}

// The regression #800 fixes. Before the strict-server conversions each domain
// sat under an `r.Route("/api/v1/{team_id}/<d>")` prefix subrouter whose
// `Get("/")` matched the bare path AND the trailing-slash form. The generated
// handlers register absolute paths, so those prefix groups were dissolved and
// the trailing-slash form stopped matching any route at all.
//
// Asserted as PARITY with the bare path rather than against a literal status:
// what matters is that both spellings reach the same route, and pinning 401
// here would make the test about the auth fixture instead.
func TestTrailingSlash_CollectionResolvesLikeTheBarePath(t *testing.T) {
	srv := trailingSlashTestServer(t)
	const team = "/api/v1/550e8400-e29b-41d4-a716-446655440000/"

	for _, domain := range []string{"memories", "artifacts", "blueprints", "prompts"} {
		t.Run(domain, func(t *testing.T) {
			bare := statusFor(t, srv, team+domain)
			slashed := statusFor(t, srv, team+domain+"/")

			assert.Equal(t, bare, slashed,
				"the trailing-slash form must resolve to the same route as the bare path")
			assert.NotEqual(t, http.StatusNotFound, slashed,
				"a 404 here is the regression: the collection stopped being reachable with a trailing slash")
		})
	}
}

// The middleware is scoped to the REST API prefix, and these are the namespaces
// that must not be touched by it. The SPA is the chi NotFound catch-all and the
// OAuth AS routes are HTTPS-gated, so normalising paths for them would change
// behaviour well outside this issue.
//
// Each assertion is that the path still RESOLVES as it did — not a specific
// status, which would pin unrelated fixtures.
func TestTrailingSlash_LeavesNonAPINamespacesAlone(t *testing.T) {
	srv := trailingSlashTestServer(t)

	t.Run("SPA catch-all still answers", func(t *testing.T) {
		// Not a backend prefix, so handleSPA owns it. With no embedded bundle
		// this build serves nothing, but it must not be a routing error.
		code := statusFor(t, srv, "/some/spa/route")
		assert.NotEqual(t, http.StatusMethodNotAllowed, code)
	})

	t.Run("health and ping are unaffected", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, statusFor(t, srv, "/ping"))
	})

	t.Run("openapi spec is unaffected", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, statusFor(t, srv, "/openapi.json"))
	})

	t.Run("a non-API trailing slash is NOT normalised", func(t *testing.T) {
		// /ping/ must stay whatever it was: outside the prefix the middleware
		// passes the request through untouched, so this does not become /ping.
		assert.NotEqual(t, http.StatusOK, statusFor(t, srv, "/ping/"),
			"only /api/v1/ paths are normalised; widening that is a separate decision")
	})

	t.Run("the root path is untouched", func(t *testing.T) {
		assert.NotEqual(t, http.StatusMethodNotAllowed, statusFor(t, srv, "/"))
	})

	// The two namespaces the acceptance criterion names explicitly, because
	// they are the ones where a normalisation would be more than cosmetic: the
	// MCP endpoint is mounted as a prefix, and the OAuth AS metadata path is
	// part of a security-relevant surface (its siblings are HTTPS-gated by
	// requireHTTPSMiddleware). Both sit outside /api/v1/, so the middleware
	// never touches them -- asserted rather than assumed.
	t.Run("MCP endpoint still resolves", func(t *testing.T) {
		code := statusFor(t, srv, "/mcp/v1/common")
		assert.NotEqual(t, http.StatusNotFound, code, "the MCP mount must still be reachable")
	})

	t.Run("OAuth AS metadata still resolves", func(t *testing.T) {
		code := statusFor(t, srv, mcpAuthorizationServerMetadataPath)
		assert.NotEqual(t, http.StatusMethodNotAllowed, code)
	})
}

// The middleware must not let a trailing slash skip authentication: it rewrites
// the path BEFORE routing, so the matched route carries its own middleware
// chain exactly as the bare path does.
func TestTrailingSlash_DoesNotBypassAuth(t *testing.T) {
	srv := trailingSlashTestServer(t)

	code := statusFor(t, srv, "/api/v1/550e8400-e29b-41d4-a716-446655440000/memories/")
	assert.Equal(t, http.StatusUnauthorized, code,
		"the normalised path must still run the authenticated group's middleware")
}
