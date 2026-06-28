package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProtectedResourceMetadata_NotAdvertisedWhenUnconfigured verifies the PRM
// guard: when MCP auth is not configured (empty MCP_RESOURCE_URI), the
// protected-resource metadata route is not registered, so discovery 404s instead
// of serving a structurally-invalid document with empty resource/
// authorization_servers — the change that turns "MCP auth off" into a clear signal
// rather than an opaque client-side "Invalid OAuth error response".
func TestProtectedResourceMetadata_NotAdvertisedWhenUnconfigured(t *testing.T) {
	srv := newMCPOAuthTestServerWithResource(t, "", "")

	paths := []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/mcp/v1/common",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code, "expected 404 for %s when MCP auth is off", path)
		assert.NotContains(t, rr.Body.String(), `"authorization_servers":[""]`,
			"must never serve a PRM document with empty fields")
		assert.NotContains(t, rr.Body.String(), `"resource":""`,
			"must never serve a PRM document with an empty resource")
	}
}
