package errors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The GitHub webhook routing token is a credential sitting in a URL path. It
// must not reach anything that echoes the request path — which in this package
// means BOTH the error-response `instance` field and the middleware log.
func TestRedactSensitivePath(t *testing.T) {
	const routingSegment = "test-routing-segment"

	assert.Equal(t, "/api/v1/webhooks/github/[redacted]",
		RedactSensitivePath("/api/v1/webhooks/github/"+routingSegment))
	assert.NotContains(t, RedactSensitivePath("/api/v1/webhooks/github/"+routingSegment), routingSegment)

	// The bare route has no token to strip, and unrelated paths must stay
	// verbatim — redaction must not blunt the logs generally.
	for _, path := range []string{
		"/api/v1/webhooks/github",
		"/api/v1/team-1/prompts",
		"/healthz",
		"",
	} {
		assert.Equal(t, path, RedactSensitivePath(path))
	}
}

// The leak this guards against was real: WriteJSONError copies r.URL.Path into
// the problem-JSON `instance` field, so a 404 for an unknown token replied with
// the token in the body. Redacting only the access log would have missed it.
func TestWriteJSONError_RedactsTokenFromInstance(t *testing.T) {
	const routingSegment = "test-routing-segment"

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/"+routingSegment, nil)
	w := httptest.NewRecorder()

	WriteJSONError(w, req, NewResourceNotFoundError("endpoint", "Not found"))

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), routingSegment,
		"the routing token must never be echoed back in an error body")
	assert.Contains(t, w.Body.String(), "/api/v1/webhooks/github/[redacted]")
}
