package mcptoken

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	testResourceURI = "https://connect.vibexp.io/mcp/v1/common"
	testSubject     = "user_oidc_abc"
	testInternalID  = "vibexp-user-42"
	testKeyID       = "test-key-1"
)

// stubResolver implements UserResolver for tests.
type stubResolver struct {
	id  string
	err error
}

func (s stubResolver) ResolveUserID(_ context.Context, _, _ string) (string, error) {
	return s.id, s.err
}

// jwksTestServer holds an ephemeral RSA key and an httptest server that serves
// the corresponding JWKS at <baseURL>/oauth2/jwks, mimicking AuthKit.
type jwksTestServer struct {
	key    *rsa.PrivateKey
	server *httptest.Server
}

func newJWKSTestServer(t *testing.T) *jwksTestServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/jwks", func(w http.ResponseWriter, _ *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		jwks := map[string]any{
			"keys": []map[string]any{
				{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": testKeyID, "n": n, "e": e},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(jwks))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &jwksTestServer{key: key, server: srv}
}

func (j *jwksTestServer) issuer() string { return j.server.URL }

// sign produces an RS256 JWT with the given claims, signed by the server's key.
func (j *jwksTestServer) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKeyID
	signed, err := tok.SignedString(j.key)
	require.NoError(t, err)
	return signed
}

func newTestVerifier(t *testing.T, j *jwksTestServer, resolver UserResolver) *Verifier {
	t.Helper()
	v, err := New(context.Background(), j.issuer(), testResourceURI, resolver)
	require.NoError(t, err)
	return v
}

func validClaims(issuer string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":   issuer,
		"sub":   testSubject,
		"aud":   testResourceURI,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "openid mcp",
	}
}

func TestNew_Validation(t *testing.T) {
	resolver := stubResolver{id: testInternalID}
	tests := []struct {
		name        string
		issuer      string
		resourceURI string
		resolver    UserResolver
	}{
		{"missing issuer", "", testResourceURI, resolver},
		{"missing resource", "https://issuer.example", "", resolver},
		{"missing resolver", "https://issuer.example", testResourceURI, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(context.Background(), tt.issuer, tt.resourceURI, tt.resolver)
			require.Error(t, err)
		})
	}
}

func TestVerify_ValidToken(t *testing.T) {
	j := newJWKSTestServer(t)
	v := newTestVerifier(t, j, stubResolver{id: testInternalID})

	token := j.sign(t, validClaims(j.issuer()))
	info, err := v.Verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/", nil))

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, testInternalID, info.UserID, "UserID must be the internal VibeXP user ID, not the token sub")
	assert.Equal(t, []string{"openid", "mcp"}, info.Scopes)
	assert.False(t, info.Expiration.IsZero())
	assert.Equal(t, testSubject, info.Extra["sub"])
}

func TestVerify_AudienceAsArray(t *testing.T) {
	j := newJWKSTestServer(t)
	v := newTestVerifier(t, j, stubResolver{id: testInternalID})

	claims := validClaims(j.issuer())
	claims["aud"] = []string{"https://other.example", testResourceURI}
	token := j.sign(t, claims)

	info, err := v.Verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/", nil))
	require.NoError(t, err)
	assert.Equal(t, testInternalID, info.UserID)
}

type rejectionCase struct {
	name    string
	mutate  func(jwt.MapClaims)
	resolve UserResolver
}

func rejectionCases() []rejectionCase {
	ok := stubResolver{id: testInternalID}
	return []rejectionCase{
		{"wrong issuer", func(c jwt.MapClaims) { c["iss"] = "https://evil.example" }, ok},
		{"wrong audience", func(c jwt.MapClaims) { c["aud"] = "https://connect.vibexp.io/other" }, ok},
		{"audience array without resource", func(c jwt.MapClaims) {
			c["aud"] = []string{"https://a.example", "https://b.example"}
		}, ok},
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }, ok},
		{"missing expiration", func(c jwt.MapClaims) { delete(c, "exp") }, ok},
		{"missing subject", func(c jwt.MapClaims) { delete(c, "sub") }, ok},
		{"subject not provisioned", func(jwt.MapClaims) {}, stubResolver{err: repositories.ErrUserNotFound}},
		{"subject resolves to empty id", func(jwt.MapClaims) {}, stubResolver{id: ""}},
		{"not-yet-valid nbf", func(c jwt.MapClaims) {
			c["nbf"] = time.Now().Add(2 * time.Hour).Unix()
		}, ok},
	}
}

func TestVerify_Rejections(t *testing.T) {
	j := newJWKSTestServer(t)

	for _, tt := range rejectionCases() {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestVerifier(t, j, tt.resolve)
			claims := validClaims(j.issuer())
			tt.mutate(claims)
			token := j.sign(t, claims)

			_, err := v.Verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Error(t, err)
			assert.True(t, errors.Is(err, mcpauth.ErrInvalidToken),
				"error must unwrap to ErrInvalidToken so middleware emits 401, got %v", err)
		})
	}
}

func TestVerify_BadSignature(t *testing.T) {
	j := newJWKSTestServer(t)
	v := newTestVerifier(t, j, stubResolver{id: testInternalID})

	// Sign with a different key than the one served in the JWKS.
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims(j.issuer()))
	tok.Header["kid"] = testKeyID
	signed, err := tok.SignedString(otherKey)
	require.NoError(t, err)

	_, err = v.Verify(context.Background(), signed, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, mcpauth.ErrInvalidToken))
}

func TestVerify_MalformedToken(t *testing.T) {
	j := newJWKSTestServer(t)
	v := newTestVerifier(t, j, stubResolver{id: testInternalID})

	_, err := v.Verify(context.Background(), "not-a-jwt", httptest.NewRequest(http.MethodGet, "/", nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, mcpauth.ErrInvalidToken))
}

// TestVerify_AlgNoneRejected verifies an unsigned ("alg":"none") token is
// rejected by the application-level algorithm allow-list, not just the key set.
func TestVerify_AlgNoneRejected(t *testing.T) {
	j := newJWKSTestServer(t)
	v := newTestVerifier(t, j, stubResolver{id: testInternalID})

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims(j.issuer()))
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = v.Verify(context.Background(), signed, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, mcpauth.ErrInvalidToken))
}

// TestVerify_HS256Rejected verifies an HMAC-signed token whose secret is the RSA
// public key material is rejected by the algorithm allow-list. This is the
// classic algorithm-substitution attack the alg pin defends against.
func TestVerify_HS256Rejected(t *testing.T) {
	j := newJWKSTestServer(t)
	v := newTestVerifier(t, j, stubResolver{id: testInternalID})

	// Use the RSA public modulus as the HMAC secret, mimicking an attacker who
	// signs with the (public) verification key under a symmetric algorithm.
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims(j.issuer()))
	tok.Header["kid"] = testKeyID
	signed, err := tok.SignedString(j.key.N.Bytes())
	require.NoError(t, err)

	_, err = v.Verify(context.Background(), signed, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, mcpauth.ErrInvalidToken))
}

// TestVerify_InfraErrorNotInvalidToken verifies that a transient infrastructure
// error during subject resolution yields a non-ErrInvalidToken error (so the
// middleware maps it to 500), distinct from the not-found case which stays a 401
// auth failure.
func TestVerify_InfraErrorNotInvalidToken(t *testing.T) {
	j := newJWKSTestServer(t)

	t.Run("infra error does not unwrap to ErrInvalidToken", func(t *testing.T) {
		v := newTestVerifier(t, j, stubResolver{err: errors.New("connection refused")})
		token := j.sign(t, validClaims(j.issuer()))

		_, err := v.Verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Error(t, err)
		assert.False(t, errors.Is(err, mcpauth.ErrInvalidToken),
			"infra errors must not unwrap to ErrInvalidToken (would yield 500, not 401)")
		assert.NotContains(t, err.Error(), "connection refused",
			"raw infra detail must not leak into the client-facing error")
	})

	t.Run("not-found stays ErrInvalidToken", func(t *testing.T) {
		v := newTestVerifier(t, j, stubResolver{err: repositories.ErrUserNotFound})
		token := j.sign(t, validClaims(j.issuer()))

		_, err := v.Verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Error(t, err)
		assert.True(t, errors.Is(err, mcpauth.ErrInvalidToken),
			"unknown subject is an auth failure → 401")
	})
}

// TestVerify_ErrorBodies pins the exact error strings: the MCP SDK's
// RequireBearerToken writes err.Error() verbatim into the 401 response body,
// so these strings are the wire contract. An unknown subject must yield the
// bare "invalid token" — anything more is an account-enumeration oracle
// revealing that a cryptographically valid token's user is unprovisioned.
func TestVerify_ErrorBodies(t *testing.T) {
	j := newJWKSTestServer(t)

	t.Run("unknown subject is opaque", func(t *testing.T) {
		v := newTestVerifier(t, j, stubResolver{err: repositories.ErrUserNotFound})
		token := j.sign(t, validClaims(j.issuer()))

		_, err := v.Verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Error(t, err)
		assert.Equal(t, "invalid token", err.Error())
	})

	t.Run("expired keeps the pre-extraction reason", func(t *testing.T) {
		v := newTestVerifier(t, j, stubResolver{id: testInternalID})
		claims := validClaims(j.issuer())
		claims["exp"] = time.Now().Add(-time.Hour).Unix()

		_, err := v.Verify(context.Background(), j.sign(t, claims),
			httptest.NewRequest(http.MethodGet, "/", nil))
		require.Error(t, err)
		assert.Equal(t, "invalid token: token expired", err.Error())
	})

	t.Run("wrong audience keeps the pre-extraction reason", func(t *testing.T) {
		v := newTestVerifier(t, j, stubResolver{id: testInternalID})
		claims := validClaims(j.issuer())
		claims["aud"] = "https://connect.vibexp.io/other"

		_, err := v.Verify(context.Background(), j.sign(t, claims),
			httptest.NewRequest(http.MethodGet, "/", nil))
		require.Error(t, err)
		assert.Equal(t, "invalid token: token audience does not include the expected resource", err.Error())
	})
}

// TestVerify_ExpiryLeeway verifies a token that expired within the clock-skew
// leeway is still accepted, while one expired well beyond it is rejected.
func TestVerify_ExpiryLeeway(t *testing.T) {
	j := newJWKSTestServer(t)
	v := newTestVerifier(t, j, stubResolver{id: testInternalID})

	t.Run("within leeway accepted", func(t *testing.T) {
		claims := validClaims(j.issuer())
		claims["exp"] = time.Now().Add(-clockSkewLeeway / 2).Unix()
		token := j.sign(t, claims)

		info, err := v.Verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/", nil))
		require.NoError(t, err)
		assert.Equal(t, testInternalID, info.UserID)
	})

	t.Run("beyond leeway rejected", func(t *testing.T) {
		claims := validClaims(j.issuer())
		claims["exp"] = time.Now().Add(-2 * clockSkewLeeway).Unix()
		token := j.sign(t, claims)

		_, err := v.Verify(context.Background(), token, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Error(t, err)
		assert.True(t, errors.Is(err, mcpauth.ErrInvalidToken))
	})
}
