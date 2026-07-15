package server

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realisticPaddedToken mirrors what generateInvitationToken produced before #251:
// padded base64.URLEncoding of 32 bytes — 44 chars ending in exactly one '='.
// Tokens of this shape are still live in the database, so the handlers must keep
// resolving them.
//
// Derived from the very encoding that caused the bug rather than pasted as a
// literal: it keeps the fixture honest (change the encoding, change the fixture)
// and keeps a high-entropy token-shaped string out of the source, which the
// gitleaks pre-commit hook rightly rejects.
var realisticPaddedToken = base64.URLEncoding.EncodeToString(
	[]byte("0123456789abcdef0123456789abcdef"), // 32 bytes, as generateInvitationToken uses
)

// TestInvitationTokenParam_DecodesPercentEncodedPaddedToken is the regression test
// for #251.
//
// Clients percent-encode path parameters (encodeURIComponent), so a token's '='
// padding arrives as %3D. chi routes on r.URL.RawPath whenever the path contains
// percent-encoding, so chi.URLParam returns the STILL-ENCODED segment — and the
// repository's exact-match `WHERE token = $1` then misses every time, surfacing as
// a 404 "Invitation not found" on every accept.
//
// Without url.PathUnescape in invitationTokenParam this test fails with
// "…Rio%3D" != "…Rio=".
func TestInvitationTokenParam_DecodesPercentEncodedPaddedToken(t *testing.T) {
	// Sanity-check the fixture is genuinely padded — the whole bug hinges on it.
	require.True(t, strings.HasSuffix(realisticPaddedToken, "="),
		"fixture must be a padded token, otherwise it cannot reproduce #251")

	tests := []struct {
		name    string
		pathSeg string
	}{
		{
			name:    "percent-encoded padding (what clients actually send)",
			pathSeg: url.QueryEscape(realisticPaddedToken), // → …Rio%3D
		},
		{
			name:    "literal padding (unencoded '=' is legal in a path segment)",
			pathSeg: realisticPaddedToken,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			var decodeErr error

			r := chi.NewRouter()
			r.Post("/api/v1/invitations/{token}/accept", func(_ http.ResponseWriter, req *http.Request) {
				got, decodeErr = invitationTokenParam(req)
			})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+tc.pathSeg+"/accept", nil)
			r.ServeHTTP(httptest.NewRecorder(), req)

			require.NoError(t, decodeErr)
			assert.Equal(t, realisticPaddedToken, got,
				"the handler must receive the decoded token, or the exact-match lookup 404s (#251)")
		})
	}
}

// TestInvitationTokenParam_RejectsInvalidEncoding proves a malformed escape is
// reported rather than silently passed through to the token lookup.
//
// The route context is built by hand because net/http rejects a malformed escape
// while parsing the request URI, so such a request never reaches a handler in
// production — this guard is defensive, and this is the only way to exercise it.
func TestInvitationTokenParam_RejectsInvalidEncoding(t *testing.T) {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "bad%ZZtoken") // %ZZ is not a valid percent-escape

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/placeholder/accept", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	_, err := invitationTokenParam(req)
	assert.Error(t, err)
}

// TestInvitationTokenDigest_NeverReturnsTheRawToken guards the logging path: the
// token is a bearer credential, so log lines must carry only a fingerprint.
func TestInvitationTokenDigest_NeverReturnsTheRawToken(t *testing.T) {
	var digest string

	r := chi.NewRouter()
	r.Get("/api/v1/invitations/{token}", func(_ http.ResponseWriter, req *http.Request) {
		digest = invitationTokenDigest(req)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/"+url.QueryEscape(realisticPaddedToken), nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	assert.NotContains(t, digest, realisticPaddedToken)
	assert.Equal(t, redactToken(realisticPaddedToken), digest,
		"digest must fingerprint the DECODED token so it correlates across handlers")
}

// TestInvitationTokenParam_RoundTripsUnpaddedToken pins the post-#251 token shape:
// RawURLEncoding emits no reserved characters, so it survives the path untouched.
func TestInvitationTokenParam_RoundTripsUnpaddedToken(t *testing.T) {
	unpadded := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	require.NotContains(t, unpadded, "=", "RawURLEncoding must not pad")

	var got string
	var decodeErr error

	r := chi.NewRouter()
	r.Post("/api/v1/invitations/{token}/accept", func(_ http.ResponseWriter, req *http.Request) {
		got, decodeErr = invitationTokenParam(req)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+url.QueryEscape(unpadded)+"/accept", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	require.NoError(t, decodeErr)
	assert.Equal(t, unpadded, got)
}
