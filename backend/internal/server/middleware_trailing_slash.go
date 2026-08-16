package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

// apiTrailingSlashPrefix scopes the trailing-slash normalisation below. Only
// paths under the versioned REST API are normalised.
const apiTrailingSlashPrefix = "/api/v1/"

// stripAPITrailingSlash makes `/api/v1/.../memories/` resolve to the same route
// as `/api/v1/.../memories`, restoring behaviour the strict-server conversions
// removed (#800, epic #122).
//
// Before the conversions each domain was mounted under an
// `r.Route("/api/v1/{team_id}/<d>")` prefix subrouter whose `Get("/")` matched
// BOTH the bare path and the trailing-slash form. The generated handlers
// register absolute paths, so those prefix groups were dissolved and the
// trailing-slash form stopped matching any route — falling through to the chi
// NotFound handler, which 404s for backend prefixes (spa.go). That is a
// regression for any caller that was using it, and it lands on every domain
// #122 converts, so it is fixed once here rather than per domain.
//
// # Why this is scoped rather than a bare middleware.StripSlashes
//
// chi's StripSlashes rewrites the RouteContext path, so it has to run BEFORE
// routing — which means the root router, not a Group: an unmatched
// trailing-slash path never reaches a Group's middleware, because chi falls
// straight through to NotFound. Mounting it at the root unscoped would also
// normalise the SPA catch-all, the MCP endpoint and the OAuth AS routes, and
// the AS routes in particular are security-relevant (they are HTTPS-gated in
// setupOAuthASRoutes). Restricting it to the REST API prefix keeps the fix
// where the regression is and leaves every other namespace byte-for-byte as it
// was. Everything outside the prefix is passed straight through, untouched.
//
// RedirectSlashes was rejected: it answers 301, which is awkward on a non-GET
// and would turn a working POST into a redirect clients may not follow.
func stripAPITrailingSlash(next http.Handler) http.Handler {
	stripped := middleware.StripSlashes(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, apiTrailingSlashPrefix) {
			stripped.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
