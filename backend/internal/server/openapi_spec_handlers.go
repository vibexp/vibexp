package server

import (
	"net/http"
	"strings"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/server/openapispec"
)

// handleOpenAPISpecYAML serves the bundled OpenAPI spec as application/yaml.
func (s *Server) handleOpenAPISpecYAML(w http.ResponseWriter, r *http.Request) {
	s.serveOpenAPISpec(w, r, "application/yaml", openapispec.YAML, openapispec.ETagYAML)
}

// handleOpenAPISpecJSON serves the bundled OpenAPI spec as application/json.
func (s *Server) handleOpenAPISpecJSON(w http.ResponseWriter, r *http.Request) {
	s.serveOpenAPISpec(w, r, "application/json", openapispec.JSON, openapispec.ETagJSON)
}

// serveOpenAPISpec writes the embedded, self-contained spec with a strong ETag
// and a permissive Access-Control-Allow-Origin. The spec is public, read-only
// data, so `*` lets external tooling (editor.swagger.io, Postman, client
// generators) fetch it cross-origin. A matching If-None-Match short-circuits to
// 304 so unchanged specs are not re-downloaded.
func (s *Server) serveOpenAPISpec(
	w http.ResponseWriter, r *http.Request, contentType string, body []byte, etag string,
) {
	w.Header().Set("ETag", etag)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		contextkeys.GetLoggerFromContext(r.Context()).Error(
			"Failed to write OpenAPI spec response", "error", err, "content_type", contentType,
		)
	}
}

// etagMatches reports whether an If-None-Match header value satisfies the given
// strong ETag. It accepts "*", a single tag, or a comma-separated list, and
// ignores a weak-validator "W/" prefix (RFC 9110 weak comparison — adequate for
// a static, immutable-per-build document).
func etagMatches(ifNoneMatch, etag string) bool {
	ifNoneMatch = strings.TrimSpace(ifNoneMatch)
	if ifNoneMatch == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == etag {
			return true
		}
	}
	return false
}
