package server

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// configJSPath is the runtime-config script the SPA loads before its module
// bundle (issue #57). It is served from env vars, not embedded, so it works
// even in builds without an embedded frontend.
const configJSPath = "/config.js"

// Cache-Control values for SPA responses: mutable entry points (index.html,
// /config.js) must revalidate on every load, while hashed static assets are
// immutable (see handleSPA).
const (
	spaHeaderCacheControl = "Cache-Control"
	spaCacheNoCache       = "no-cache"
)

// backendPathPrefixes are the request-path prefixes owned by the backend (API,
// MCP, OAuth AS, discovery, internal jobs). The SPA catch-all must never serve
// index.html for these: an unmatched path under one of them is a genuine 404
// (e.g. a typo'd API call), not a client-side route, so returning HTML would
// mask real errors. The SPA owns every other path.
var backendPathPrefixes = []string{
	"/api/",
	"/bo/",
	"/mcp/",
	"/oauth2/",
	"/.well-known/",
	"/internal/",
}

// handleSPA is the single-page-app catch-all, registered as the router's
// NotFound handler (server.go:setupRoutes) so it runs only when no API/MCP/OAuth
// route matched. It serves, in order: the runtime /config.js, embedded static
// assets, and index.html as the fallback so deep links (client-side routes)
// resolve. Requests under a backend prefix, and non-GET/HEAD requests, get an
// honest 404 instead of HTML.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	// config.js is rendered from env vars regardless of whether the SPA bundle is
	// embedded, so the same handler answers it in every build.
	if r.URL.Path == configJSPath {
		s.handleConfigJS(w, r)
		return
	}

	// Only GET/HEAD can produce SPA content; anything else that fell through to
	// NotFound is a genuine 404 (POST to an unknown path, etc.).
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	// Never let the SPA shadow a backend namespace.
	for _, prefix := range backendPathPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
	}

	if s.spaFS == nil {
		// Default dev/CI build: the frontend is not embedded. The Vite dev server
		// serves the SPA in local development; here there is nothing to serve.
		http.NotFound(w, r)
		return
	}

	s.serveSPAAsset(w, r)
}

// serveSPAAsset serves the embedded asset at the request path, falling back to
// index.html for any path that is not an embedded file (the SPA's client-side
// routes). Mirrors the retired nginx `try_files $uri $uri/ /index.html`.
func (s *Server) serveSPAAsset(w http.ResponseWriter, r *http.Request) {
	reqPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if reqPath == "" {
		s.serveSPAIndex(w, r)
		return
	}

	info, err := fs.Stat(s.spaFS, reqPath)
	if err != nil || info.IsDir() {
		// Unknown path or directory → a client-side route; serve the app shell.
		s.serveSPAIndex(w, r)
		return
	}

	data, err := fs.ReadFile(s.spaFS, reqPath)
	if err != nil {
		s.serveSPAIndex(w, r)
		return
	}

	setSPACacheHeaders(w, reqPath)
	// http.ServeContent sets Content-Type (by extension, then sniffing) and
	// handles HEAD and range requests. embed.FS exposes no real modtimes, so pass
	// the zero time to omit Last-Modified (Cache-Control above governs caching).
	http.ServeContent(w, r, reqPath, time.Time{}, bytes.NewReader(data))
}

// serveSPAIndex writes the embedded index.html with a 200 so client-side deep
// links resolve. index.html is never cached so a new deployment's asset
// references are picked up immediately.
func (s *Server) serveSPAIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.spaFS, "index.html")
	if err != nil {
		contextkeys.GetLoggerFromContext(r.Context()).
			Error("SPA index.html missing from embedded assets", "error", err)
		http.Error(w, "frontend not available", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set(spaHeaderCacheControl, spaCacheNoCache)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(data); err != nil {
		contextkeys.GetLoggerFromContext(r.Context()).
			Error("failed to write SPA index.html", "error", err)
	}
}

// setSPACacheHeaders caches Vite's content-hashed assets aggressively and
// forbids caching everything else (index.html and other root files), which must
// be revalidated so a redeploy is picked up without stale references.
func setSPACacheHeaders(w http.ResponseWriter, reqPath string) {
	if strings.HasPrefix(reqPath, "assets/") {
		w.Header().Set(spaHeaderCacheControl, "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set(spaHeaderCacheControl, spaCacheNoCache)
}

// handleConfigJS renders /config.js: the deploy-time frontend configuration as
// `window.__VIBEXP_ENV__`, read by the SPA's getEnv() before its module bundle
// runs (issue #57). Served no-cache so an operator's env change takes effect on
// the next load after a restart, with no rebuild.
func (s *Server) handleConfigJS(w http.ResponseWriter, r *http.Request) {
	// json.Marshal HTML-escapes <, > and & (e.g. a value containing
	// "</script>"), so the result is safe to inline in a <script> element.
	payload, err := json.Marshal(s.config.RuntimeFrontendEnv())
	if err != nil {
		contextkeys.GetLoggerFromContext(r.Context()).
			Error("failed to marshal runtime frontend env", "error", err)
		http.Error(w, "config unavailable", http.StatusInternalServerError)
		return
	}

	body := make([]byte, 0, len(payload)+32)
	body = append(body, "window.__VIBEXP_ENV__ = "...)
	body = append(body, payload...)
	body = append(body, ";\n"...)

	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set(spaHeaderCacheControl, spaCacheNoCache)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		contextkeys.GetLoggerFromContext(r.Context()).
			Error("failed to write config.js", "error", err)
	}
}
