package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/server/openapispec"
)

// newSpecTestServer builds a DB-free server, like newDriftTestServer, so the
// public OpenAPI spec routes can be exercised end to end.
func newSpecTestServer(t *testing.T) *Server {
	t.Helper()
	return New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
}

func TestOpenAPISpecRoutes(t *testing.T) {
	srv := newSpecTestServer(t)

	cases := []struct {
		path        string
		contentType string
		etag        string
		body        []byte
	}{
		{"/openapi.yaml", "application/yaml", openapispec.ETagYAML, openapispec.YAML},
		{"/openapi.json", "application/json", openapispec.ETagJSON, openapispec.JSON},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			// A plain GET returns 200, the whole spec, the right content type,
			// a strong ETag, and the permissive public CORS header.
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rr.Code != http.StatusOK {
				t.Fatalf("GET %s: status = %d, want 200", tc.path, rr.Code)
			}
			if got := rr.Header().Get("Content-Type"); got != tc.contentType {
				t.Errorf("GET %s: Content-Type = %q, want %q", tc.path, got, tc.contentType)
			}
			if got := rr.Header().Get("ETag"); got != tc.etag {
				t.Errorf("GET %s: ETag = %q, want %q", tc.path, got, tc.etag)
			}
			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("GET %s: Access-Control-Allow-Origin = %q, want *", tc.path, got)
			}
			if rr.Body.Len() != len(tc.body) {
				t.Errorf("GET %s: body length = %d, want %d", tc.path, rr.Body.Len(), len(tc.body))
			}

			// A matching If-None-Match short-circuits to 304 with no body.
			rr = httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("If-None-Match", tc.etag)
			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotModified {
				t.Fatalf("GET %s (If-None-Match): status = %d, want 304", tc.path, rr.Code)
			}
			if rr.Body.Len() != 0 {
				t.Errorf("GET %s (304): body should be empty, got %d bytes", tc.path, rr.Body.Len())
			}
			if got := rr.Header().Get("ETag"); got != tc.etag {
				t.Errorf("GET %s (304): ETag = %q, want %q", tc.path, got, tc.etag)
			}

			// A non-matching If-None-Match still returns the full 200 response.
			rr = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("If-None-Match", `"stale-etag"`)
			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("GET %s (stale If-None-Match): status = %d, want 200", tc.path, rr.Code)
			}
		})
	}
}

func TestETagMatches(t *testing.T) {
	const etag = `"abc123"`
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{etag, true},
		{"*", true},
		{`"other"`, false},
		{`W/` + etag, true},
		{`"other", ` + etag, true},
		{`"a", "b"`, false},
	}
	for _, tc := range cases {
		if got := etagMatches(tc.header, etag); got != tc.want {
			t.Errorf("etagMatches(%q, %q) = %v, want %v", tc.header, etag, got, tc.want)
		}
	}
}
