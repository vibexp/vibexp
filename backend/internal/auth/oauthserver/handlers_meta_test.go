package oauthserver

import (
	"encoding/json"
	"net/http"
	"testing"

	gojose "github.com/go-jose/go-jose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetadata_HasAllRequiredRFC8414Fields checks the AS metadata document
// carries every field a compliant client needs to drive the flow: the RFC 8414
// MUSTs plus the OAuth-2.1/MCP-specific advertisements (S256, DCR endpoint,
// resource-indicator support).
func TestMetadata_HasAllRequiredRFC8414Fields(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.get(t, MetadataPath)
	require.Equal(t, http.StatusOK, res.status)
	md := jsonBody(t, res)

	// RFC 8414 + OAuth 2.1 / MCP required advertisements must be present and non-empty.
	for _, field := range []string{
		"issuer", "authorization_endpoint", "token_endpoint", "registration_endpoint",
		"jwks_uri", "response_types_supported", "grant_types_supported",
		"code_challenge_methods_supported", "token_endpoint_auth_methods_supported",
		"scopes_supported",
	} {
		assert.NotEmpty(t, md[field], "metadata field %q must be present and non-empty", field)
	}

	assert.Equal(t, []string{"S256"}, toStrings(md["code_challenge_methods_supported"]),
		"only S256 PKCE is advertised (plain is forbidden)")
	assert.ElementsMatch(t, []string{"authorization_code", "refresh_token"},
		toStrings(md["grant_types_supported"]), "only the OAuth 2.1 core grants are advertised")
	assert.Equal(t, true, md["resource_indicators_supported"], "RFC 8707 must be advertised")
}

// TestJWKS_PublishesOnlyPublicKeyMaterial guards against the worst JWKS bug:
// leaking private key components. Every published key must validate as a public
// RSA key and carry a kid; none may contain the private exponent.
func TestJWKS_PublishesOnlyPublicKeyMaterial(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.get(t, JWKSPath)
	require.Equal(t, http.StatusOK, res.status)

	var set gojose.JSONWebKeySet
	require.NoError(t, json.Unmarshal(res.body, &set))
	require.NotEmpty(t, set.Keys, "JWKS must publish at least the active key")

	// Re-parse raw so we can assert the private exponent "d" is absent — go-jose
	// drops unknown/private fields on its typed key, so check the wire form.
	var raw struct {
		Keys []map[string]any `json:"keys"`
	}
	require.NoError(t, json.Unmarshal(res.body, &raw))
	for i, k := range set.Keys {
		assert.True(t, k.IsPublic(), "published key %d must be public", i)
		assert.NotEmpty(t, k.KeyID, "published key %d must carry a kid", i)
	}
	for i, k := range raw.Keys {
		_, hasPrivate := k["d"]
		assert.False(t, hasPrivate, "published key %d must not contain the private exponent 'd'", i)
	}
}

func toStrings(v any) []string { return audienceStrings(v) }
