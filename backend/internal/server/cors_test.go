package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/vibexp/vibexp/internal/config"
)

// corsTestOrigin matches the test config below; chosen because the bug (#1289)
// was reported for cross-origin requests from app.vibexp.io.
const corsTestOrigin = "https://app.vibexp.io"

// restVerbs is the set of HTTP verbs the API actually serves. chi.Walk
// over-reports HEAD/CONNECT/TRACE for routes registered via HandleFunc and
// catch-all mounts; restricting to REST verbs avoids false negatives.
var restVerbs = map[string]struct{}{
	http.MethodGet:    {},
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

func newCORSTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{CORSAllowedOrigins: []string{corsTestOrigin}}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	return New("8080", nil, "test-api-key", cfg, logger)
}

// preflight issues an OPTIONS request mimicking a browser CORS preflight for
// the given method and path, and returns the recorder.
func preflight(srv *Server, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, path, nil)
	req.Header.Set("Origin", corsTestOrigin)
	req.Header.Set("Access-Control-Request-Method", method)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

// concretePath replaces chi path params (e.g. {id}, {id:[0-9]+}) with a
// concrete value so the request actually matches the registered route.
// Uses "1" so that numeric regex constraints (e.g. {id:[0-9]+}) are satisfied;
// any chi-default param without a regex also matches a single digit.
func concretePath(p string) string {
	var b strings.Builder
	depth := 0
	for _, c := range p {
		switch {
		case c == '{':
			depth++
			if depth == 1 {
				b.WriteString("1")
			}
		case c == '}' && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// allowMethodsContains parses an Access-Control-Allow-Methods header value
// (a comma-separated, case-insensitive list of HTTP methods) and reports
// whether it contains the given method as a distinct entry. Avoids the
// substring-match brittleness of strings.Contains (e.g. "PATCHFOO" matching
// "PATCH").
func allowMethodsContains(headerValue, method string) bool {
	want := strings.ToUpper(method)
	for _, raw := range strings.Split(headerValue, ",") {
		if strings.ToUpper(strings.TrimSpace(raw)) == want {
			return true
		}
	}
	return false
}

type routeEntry struct{ method, path string }

func collectRESTRoutes(t *testing.T, srv *Server) []routeEntry {
	t.Helper()
	var routes []routeEntry
	walk := func(method string, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if _, ok := restVerbs[method]; !ok {
			return nil
		}
		routes = append(routes, routeEntry{method: method, path: path})
		return nil
	}
	if err := chi.Walk(srv.router, walk); err != nil {
		t.Fatalf("chi.Walk failed: %v", err)
	}
	if len(routes) == 0 {
		t.Fatal("no routes registered; walker produced empty set")
	}
	return routes
}

// TestCORSPreflightAllowsPATCH guards against regression of issue #1289 where
// PATCH was missing from the CORS allow-list, which broke
// PATCH /api/v1/notifications/{id}/read from app.vibexp.io.
func TestCORSPreflightAllowsPATCH(t *testing.T) {
	srv := newCORSTestServer(t)

	rr := preflight(srv, http.MethodPatch, "/api/v1/notifications/some-id/read")

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != corsTestOrigin {
		t.Errorf("Access-Control-Allow-Origin: got %q want %q", got, corsTestOrigin)
	}
	allowMethods := rr.Header().Get("Access-Control-Allow-Methods")
	if !allowMethodsContains(allowMethods, http.MethodPatch) {
		t.Errorf("Access-Control-Allow-Methods does not include PATCH: %q", allowMethods)
	}
}

// TestCORSAllowsAllRegisteredMethods walks the chi router and asserts that
// every standard REST verb the API actually serves is permitted by the CORS
// preflight. This catches future PATCH (or new GET/POST/PUT/DELETE) endpoints
// whose method might be missing from the AllowedMethods list (the original bug
// in #1289).
func TestCORSAllowsAllRegisteredMethods(t *testing.T) {
	srv := newCORSTestServer(t)
	routes := collectRESTRoutes(t, srv)

	seenVerbs := map[string]bool{}
	for _, r := range routes {
		path := concretePath(r.path)
		rr := preflight(srv, r.method, path)
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != corsTestOrigin {
			t.Errorf("preflight rejected: method=%s path=%s allow-origin=%q (route=%s)",
				r.method, path, got, r.path)
		}
		seenVerbs[r.method] = true
	}

	// PATCH is the verb that triggered #1289; if we ever stop registering any
	// PATCH route, the regression guard becomes vacuous and we should know.
	if !seenVerbs[http.MethodPatch] {
		t.Error("expected at least one PATCH route in the router (was the notifications PATCH endpoint removed?)")
	}
}
