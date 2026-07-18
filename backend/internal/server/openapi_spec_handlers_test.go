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

// getSpec issues a GET for path, setting If-None-Match when non-empty.
func getSpec(srv *Server, path, ifNoneMatch string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	srv.ServeHTTP(rr, req)
	return rr
}

// assertSpecFullResponse asserts a 200 with the whole spec, the right content
// type, a strong ETag, and the permissive public CORS header.
func assertSpecFullResponse(t *testing.T, rr *httptest.ResponseRecorder, path, contentType, etag string, bodyLen int) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", path, rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != contentType {
		t.Errorf("GET %s: Content-Type = %q, want %q", path, got, contentType)
	}
	if got := rr.Header().Get("ETag"); got != etag {
		t.Errorf("GET %s: ETag = %q, want %q", path, got, etag)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("GET %s: Access-Control-Allow-Origin = %q, want *", path, got)
	}
	if rr.Body.Len() != bodyLen {
		t.Errorf("GET %s: body length = %d, want %d", path, rr.Body.Len(), bodyLen)
	}
}

// assertSpecNotModified asserts a 304 with no body and the strong ETag intact.
func assertSpecNotModified(t *testing.T, rr *httptest.ResponseRecorder, path, etag string) {
	t.Helper()
	if rr.Code != http.StatusNotModified {
		t.Fatalf("GET %s (If-None-Match): status = %d, want 304", path, rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("GET %s (304): body should be empty, got %d bytes", path, rr.Body.Len())
	}
	if got := rr.Header().Get("ETag"); got != etag {
		t.Errorf("GET %s (304): ETag = %q, want %q", path, got, etag)
	}
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
			assertSpecFullResponse(t, getSpec(srv, tc.path, ""), tc.path, tc.contentType, tc.etag, len(tc.body))

			// A matching If-None-Match short-circuits to 304 with no body.
			assertSpecNotModified(t, getSpec(srv, tc.path, tc.etag), tc.path, tc.etag)

			// A non-matching If-None-Match still returns the full 200 response.
			if rr := getSpec(srv, tc.path, `"stale-etag"`); rr.Code != http.StatusOK {
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
