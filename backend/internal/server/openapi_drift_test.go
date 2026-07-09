package server

// OpenAPI drift gate (#1695, epic #1693).
//
// This test mechanically links the chi router (internal/server/server.go) to
// the hand-written spec (backend-api/openapi.yaml): every live REST route must
// be documented or explicitly allowlisted, and every documented operation must
// exist as a live route. Param NAMES are part of the comparison, so renaming
// {project_id} in the router without updating the spec fails here.
//
// The allowlist is shrink-only by construction: an entry whose route no longer
// exists, or that has since been documented, fails the test and must be
// removed. The #1696 backfill burned the original 88-entry TODO list down to
// zero — new endpoints must be documented in openapi.yaml, never allowlisted
// (only scope-policy exclusions belong here).

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/vibexp/vibexp/internal/config"
)

// specPath is relative to this package directory (internal/server).
const specPath = "../../openapi.yaml"

// undocumentedPrefixAllowlist exempts whole route subtrees (mounts and
// derived discovery paths) from the documentation requirement, keyed by path
// prefix with a written justification. These mirror the documentation scope
// policy in openapi.yaml's info.description.
var undocumentedPrefixAllowlist = map[string]string{
	"/internal/jobs/": "scheduler-invoked internal jobs (Pub/Sub OIDC), not client-facing",
	"/mcp/v1/common":  "MCP mount — documented by the MCP protocol, not REST",
	"/.well-known/":   "OAuth 2.1 resource-server discovery metadata (RFC 9728), derived at startup",
	"/favicon.ico":    "browser plumbing for connect.vibexp.io (PR #1514)",
}

// notFoundStubRoutes are paths registered via HandleFunc in setupTestRoutes
// purely to serve explicit 404s for legacy probes; chi.Walk reports them for
// every REST verb.
var notFoundStubRoutes = map[string]string{
	"/api/v1/prompts-invalid": "registered 404 stub (setupTestRoutes)",
}

// undocumentedRouteAllowlist exempts individual operations, keyed
// "METHOD /path" (path with normalized {param} names), each with a written
// justification. Only documentation-scope-policy exclusions belong here —
// the #1696 backfill eliminated all "TODO" entries; never add new ones.
var undocumentedRouteAllowlist = map[string]string{
	// Intentionally excluded by the documentation scope policy.
	"POST /api/v1/integrations/github/webhook": "308 permanent-redirect shim to /api/v1/webhooks/github",
	// The bundled spec served for external consumers (#139): serving the spec at
	// its own routes must not require documenting those routes inside the spec —
	// like /favicon.ico, they are static plumbing, not part of the API contract.
	"GET /openapi.yaml": "serves the bundled OpenAPI spec itself (#139); not part of the documented API",
	"GET /openapi.json": "serves the bundled OpenAPI spec itself (#139); not part of the documented API",
}

// driftParamRegexp strips chi regex constraints: {id:[0-9]+} → {id}.
var driftParamRegexp = regexp.MustCompile(`\{([^}:]+):[^}]*\}`)

// normalizeRoutePath canonicalizes a chi.Walk route pattern for comparison
// with spec path keys: regex constraints removed, trailing slash trimmed
// (chi reports `r.Post("/")` inside a Route as "/prefix/").
func normalizeRoutePath(p string) string {
	p = driftParamRegexp.ReplaceAllString(p, "{$1}")
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// newDriftTestServer constructs the server DB-free, exactly like
// newCORSTestServer, but pins MCPResourceURI so the derived
// /.well-known metadata route is deterministic regardless of env defaults.
func newDriftTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{
		MCP: config.MCPConfig{ResourceURI: "https://connect.vibexp.io/mcp/v1/common"},
	}
	logger := slog.New(slog.DiscardHandler)
	return New("8080", nil, "test-api-key", cfg, logger)
}

// collectRouterOps walks the live router and returns the set of
// "METHOD /normalized/path" operations served by the API.
func collectRouterOps(t *testing.T) map[string]struct{} {
	t.Helper()
	srv := newDriftTestServer(t)
	ops := make(map[string]struct{})
	for _, r := range collectRESTRoutes(t, srv) {
		ops[r.method+" "+normalizeRoutePath(r.path)] = struct{}{}
	}
	return ops
}

// resolvePathItemRef follows a single-level path-item $ref of the form
// "./paths/<domain>.yaml#/<key>" (the #1697 per-domain split layout) and
// returns the referenced path item. Deeper or remote refs are not supported
// by design — the layout convention is exactly one level.
func resolvePathItemRef(t *testing.T, ref string) map[string]any {
	t.Helper()
	file, key, ok := strings.Cut(ref, "#/")
	if !ok || strings.Contains(key, "/") ||
		!strings.HasPrefix(file, "./paths/") || !strings.HasSuffix(file, ".yaml") ||
		strings.Contains(file, "..") {
		t.Fatalf("unsupported path-item $ref %q (expected ./paths/<domain>.yaml#/<key>)", ref)
	}
	// #nosec G304 -- ref validated above: ./paths/*.yaml inside the spec tree, no traversal
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(specPath), file))
	if err != nil {
		t.Fatalf("read path-item ref target %s: %v", ref, err)
	}
	var doc map[string]map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse path-item ref target %s: %v", ref, err)
	}
	item, ok := doc[key]
	if !ok {
		t.Fatalf("path-item ref %q: key %q not found in %s", ref, key, file)
	}
	return item
}

// collectSpecOps parses openapi.yaml (root index + per-domain path files)
// and returns the set of "METHOD /path" operations it documents.
func collectSpecOps(t *testing.T) map[string]struct{} {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse %s: %v", specPath, err)
	}
	if len(spec.Paths) == 0 {
		t.Fatalf("%s: no paths parsed; file structure changed?", specPath)
	}
	ops := make(map[string]struct{})
	for path, item := range spec.Paths {
		if ref, ok := item["$ref"].(string); ok {
			item = resolvePathItemRef(t, ref)
		}
		for method := range item {
			upper := strings.ToUpper(method)
			if _, ok := restVerbs[upper]; !ok {
				continue // path-level keys like parameters, summary
			}
			ops[upper+" "+path] = struct{}{}
		}
	}
	return ops
}

// allowlisted reports whether the given "METHOD /path" operation is exempt
// from the documentation requirement, and why.
func allowlisted(op string) (string, bool) {
	if reason, ok := undocumentedRouteAllowlist[op]; ok {
		return reason, true
	}
	path := op[strings.Index(op, " ")+1:]
	if reason, ok := notFoundStubRoutes[path]; ok {
		return reason, true
	}
	for prefix, reason := range undocumentedPrefixAllowlist {
		if strings.HasPrefix(path, prefix) {
			return reason, true
		}
	}
	return "", false
}

// reportUndocumented fails for every live route that is neither documented
// nor allowlisted, and returns the offending ops for the summary log.
func reportUndocumented(t *testing.T, routerOps, specOps map[string]struct{}) []string {
	t.Helper()
	undocumented := make([]string, 0, len(routerOps))
	for op := range routerOps {
		if _, ok := specOps[op]; ok {
			continue
		}
		if _, ok := allowlisted(op); ok {
			continue
		}
		undocumented = append(undocumented, op)
	}
	sort.Strings(undocumented)
	for _, op := range undocumented {
		t.Errorf("live route not documented in openapi.yaml: %s\n"+
			"  → add it to openapi.yaml (preferred) or, if it is intentionally internal,\n"+
			"    to undocumentedRouteAllowlist with a justification", op)
	}
	return undocumented
}

// reportStaleSpec fails for every documented operation the router no longer
// serves (removed/renamed route, or a param-name mismatch).
func reportStaleSpec(t *testing.T, routerOps, specOps map[string]struct{}) {
	t.Helper()
	stale := make([]string, 0, len(specOps))
	for op := range specOps {
		if _, ok := routerOps[op]; !ok {
			stale = append(stale, op)
		}
	}
	sort.Strings(stale)
	for _, op := range stale {
		t.Errorf("openapi.yaml documents an operation the router does not serve: %s\n"+
			"  → the route was removed/renamed (param names count); update openapi.yaml", op)
	}
}

// reportStaleAllowlist enforces shrink-only: an allowlist entry whose route
// no longer exists, or that has since been documented, must be removed.
func reportStaleAllowlist(t *testing.T, routerOps, specOps map[string]struct{}) {
	t.Helper()
	for op, reason := range undocumentedRouteAllowlist {
		if _, ok := routerOps[op]; !ok {
			t.Errorf("stale allowlist entry (route no longer exists): %q (%s) — remove it", op, reason)
		}
		if _, ok := specOps[op]; ok {
			t.Errorf("allowlist entry is now documented in openapi.yaml: %q — remove it", op)
		}
	}
}

// TestOpenAPISpecMatchesRouter is the drift gate. It fails when the chi
// router and openapi.yaml diverge in either direction, or when an allowlist
// entry has gone stale (route removed, or operation now documented).
func TestOpenAPISpecMatchesRouter(t *testing.T) {
	routerOps := collectRouterOps(t)
	specOps := collectSpecOps(t)

	undocumented := reportUndocumented(t, routerOps, specOps)
	reportStaleSpec(t, routerOps, specOps)
	reportStaleAllowlist(t, routerOps, specOps)

	if t.Failed() {
		t.Logf("router serves %d REST operations; openapi.yaml documents %d", len(routerOps), len(specOps))
		t.Logf("copy-pasteable allowlist seeds for genuinely-internal routes:")
		for _, op := range undocumented {
			t.Logf("\t%q: \"TODO(#1696): undocumented\",", op)
		}
	}
}

// TestDriftGateFailureModes guards the gate itself: a fabricated router-only
// op and a fabricated spec-only op must both be reported, so the gate cannot
// silently rot into a no-op.
func TestDriftGateFailureModes(t *testing.T) {
	routerOps := collectRouterOps(t)
	specOps := collectSpecOps(t)

	const fake = "GET /api/v1/this-route-does-not-exist"
	if _, ok := routerOps[fake]; ok {
		t.Fatalf("fabricated op unexpectedly served by router: %s", fake)
	}
	if _, ok := specOps[fake]; ok {
		t.Fatalf("fabricated op unexpectedly documented in spec: %s", fake)
	}

	// Param-name sensitivity: the blueprint routes must be keyed by
	// {project_id} (the router's name), not {project_name} (#1694).
	const renamed = "GET /api/v1/{team_id}/blueprints/{project_name}/{slug}"
	const correct = "GET /api/v1/{team_id}/blueprints/{project_id}/{slug}"
	if _, ok := routerOps[renamed]; ok {
		t.Errorf("router op uses param name the spec abandoned in #1694: %s", renamed)
	}
	if _, ok := routerOps[correct]; !ok {
		t.Errorf("expected router op missing (param normalization broken?): %s", correct)
	}
	if _, ok := specOps[correct]; !ok {
		t.Errorf("expected spec op missing (param names must match the router): %s", correct)
	}

	// Normalization: regex constraints and trailing slashes must be stripped.
	if got := normalizeRoutePath("/api/v1/things/{id:[0-9]+}/"); got != "/api/v1/things/{id}" {
		t.Errorf("normalizeRoutePath: got %q", got)
	}
}
