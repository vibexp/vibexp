package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/vibexp/vibexp/internal/config"
)

// newSPATestServer builds a DB-free server and injects an in-memory frontend
// build so the SPA handler can be exercised without the `embedfrontend` build
// tag (the default test build embeds nothing).
func newSPATestServer(t *testing.T) *Server {
	t.Helper()
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	srv.spaFS = fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>app</title>")},
		"assets/app-abc123.js": {Data: []byte("console.log('app')")},
		"favicon.svg":          {Data: []byte("<svg></svg>")},
	}
	return srv
}

func TestHandleSPA_ServesIndexAtRoot(t *testing.T) {
	srv := newSPATestServer(t)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /: got %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET /: Content-Type = %q, want text/html", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("GET /: Cache-Control = %q, want no-cache", cc)
	}
	if !strings.Contains(rr.Body.String(), "<title>app</title>") {
		t.Errorf("GET /: body did not contain index.html: %q", rr.Body.String())
	}
}

func TestHandleSPA_ServesHashedAssetImmutable(t *testing.T) {
	srv := newSPATestServer(t)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET asset: got %d, want 200", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("asset Cache-Control = %q, want immutable", cc)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("asset Content-Type = %q, want javascript", ct)
	}
	if rr.Body.String() != "console.log('app')" {
		t.Errorf("asset body = %q", rr.Body.String())
	}
}

func TestHandleSPA_DeepLinkFallsBackToIndex(t *testing.T) {
	srv := newSPATestServer(t)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/prompts/123", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("deep link: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "<title>app</title>") {
		t.Errorf("deep link did not fall back to index.html: %q", rr.Body.String())
	}
}

func TestHandleSPA_DoesNotShadowBackendPrefixes(t *testing.T) {
	srv := newSPATestServer(t)

	// Unknown paths under backend namespaces must never return the SPA index.
	// They resolve to a non-200 backend response — 404 for the prefixes the SPA
	// guard catches (mcp/oauth2/.well-known/internal), or 401 where an auth
	// middleware on the mounted group intercepts first (api/bo).
	for _, p := range []string{
		"/api/v1/does-not-exist",
		"/mcp/v1/whatever",
		"/oauth2/nope",
		"/.well-known/nope",
		"/bo/v1/nope",
		"/internal/jobs/nope",
	} {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code == http.StatusOK {
			t.Errorf("GET %s: got 200, want a non-200 backend response", p)
		}
		if strings.Contains(rr.Body.String(), "<title>app</title>") {
			t.Errorf("GET %s: SPA index leaked into a backend-prefixed path", p)
		}
	}
}

func TestHandleSPA_NonGETIsNotFound(t *testing.T) {
	srv := newSPATestServer(t)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/some/spa/route", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("POST unknown: got %d, want 404", rr.Code)
	}
}

func TestHandleSPA_NoEmbeddedFrontendReturns404(t *testing.T) {
	// Default build (spaFS nil): the SPA is served by Vite in dev, so an SPA path
	// here 404s rather than returning HTML.
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/prompts/123", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("no-embed SPA path: got %d, want 404", rr.Code)
	}
}

func TestHandleConfigJS_RendersWindowEnv(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{
			SiteName:   "Acme",
			GTMEnabled: "true",
			GTMID:      "GTM-XYZ",
		},
	}
	// config.js is served regardless of whether the frontend is embedded.
	srv := New("8080", nil, "test-api-key", cfg, slog.New(slog.DiscardHandler))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/config.js", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /config.js: got %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("config.js Content-Type = %q, want javascript", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("config.js Cache-Control = %q, want no-cache", cc)
	}
	body := rr.Body.String()
	if !strings.HasPrefix(body, "window.__VIBEXP_ENV__ = ") {
		t.Errorf("config.js body did not assign window.__VIBEXP_ENV__: %q", body)
	}
	for _, want := range []string{`"VITE_SITE_NAME":"Acme"`, `"VITE_GTM_ENABLED":"true"`, `"VITE_GTM_ID":"GTM-XYZ"`} {
		if !strings.Contains(body, want) {
			t.Errorf("config.js missing %s; body = %q", want, body)
		}
	}
	// Unset values must be omitted so the build-time default remains the fallback.
	if strings.Contains(body, "VITE_SITE_URL") {
		t.Errorf("config.js included an unset key; body = %q", body)
	}
}

func TestHandleConfigJS_EmptyConfigIsEmptyObject(t *testing.T) {
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/config.js", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /config.js: got %d, want 200", rr.Code)
	}
	if got := strings.TrimSpace(rr.Body.String()); got != "window.__VIBEXP_ENV__ = {};" {
		t.Errorf("empty config.js = %q, want window.__VIBEXP_ENV__ = {};", got)
	}
}
