package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests guard the #257 fix: path-segment params are decoded with
// url.PathUnescape, not url.QueryUnescape. The two differ only on '+', which
// QueryUnescape turns into a space — silently corrupting a slug or project_id
// that carries a literal '+' (slugs are generated from user-supplied names, so
// a '+' is plausible). chi hands these params back still-encoded because it
// routes on r.URL.RawPath.

func decodeTestServer() *Server {
	return &Server{logger: slog.New(slog.DiscardHandler)}
}

func TestDecodeProjectSlug_PreservesLiteralPlus(t *testing.T) {
	srv := decodeTestServer()

	// A literal '+' is a valid path-segment character; PathUnescape keeps it,
	// QueryUnescape would return "a b".
	rr := httptest.NewRecorder()
	got, ok := srv.decodeProjectSlug(rr, "user-1", "test", "a+b")
	require.True(t, ok)
	assert.Equal(t, "a+b", got)

	// Percent-encoded '+' (%2B) still decodes to '+'.
	rr = httptest.NewRecorder()
	got, ok = srv.decodeProjectSlug(rr, "user-1", "test", "a%2Bb")
	require.True(t, ok)
	assert.Equal(t, "a+b", got)

	// A malformed escape keeps the 400-on-decode-error behavior.
	rr = httptest.NewRecorder()
	_, ok = srv.decodeProjectSlug(rr, "user-1", "test", "bad%ZZ")
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestDecodeArtifactURLParams_PreservesLiteralPlus(t *testing.T) {
	srv := decodeTestServer()
	rr := httptest.NewRecorder()

	projectID, slug, ok := srv.decodeArtifactURLParams(rr, "user-1", "test", "p+1", "a+b")
	require.True(t, ok)
	assert.Equal(t, "p+1", projectID)
	assert.Equal(t, "a+b", slug)
}
