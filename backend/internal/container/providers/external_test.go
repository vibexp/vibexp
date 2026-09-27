package providers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
	"github.com/vibexp/vibexp/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// newDiscoverableIssuer returns an httptest server that serves a minimal OIDC
// discovery document, so oidc.New succeeds against it without external network.
func newDiscoverableIssuer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"issuer":                                srv.URL,
			"authorization_endpoint":                srv.URL + "/authorize",
			"token_endpoint":                        srv.URL + "/token",
			"jwks_uri":                              srv.URL + "/jwks",
			"userinfo_endpoint":                     srv.URL + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestProvideRegistry_OIDCDiscoverable_RegistersOIDCClient(t *testing.T) {
	srv := newDiscoverableIssuer(t)
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Provider: "oidc",
			OIDC: config.OIDCAuthConfig{
				IssuerURL:    srv.URL,
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				RedirectURI:  "http://localhost:8080/api/v1/auth/callback",
			},
		},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err)
	assert.Equal(t, []idp.ProviderName{idp.ProviderOIDC}, registry.Enabled())
	provider, ok := registry.Get(idp.ProviderOIDC)
	require.True(t, ok)
	_, isClient := provider.(*oidc.Client)
	assert.True(t, isClient, "AUTH_PROVIDER=oidc with a discoverable issuer should register *oidc.Client")
	assert.Equal(t, idp.ProviderOIDC, provider.Name())
}

func TestProvideRegistry_OIDCDiscoveryFailure_NonFatalSkip(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Provider: "oidc",
			OIDC: config.OIDCAuthConfig{
				IssuerURL:    "https://oidc.invalid.example.com",
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				RedirectURI:  "http://localhost:8080/api/v1/auth/callback",
			},
		},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err, "OIDC discovery failure must be non-fatal")
	assert.Equal(t, 0, registry.Len(), "OIDC discovery failure should skip the provider")
}

func TestProvideRegistry_OIDCMissingConfig_NonFatalSkip(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Provider: "oidc",
			// no OIDC_* fields set -> oidc.Config.Validate fails -> skipped
		},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err, "invalid OIDC config must be non-fatal")
	assert.Equal(t, 0, registry.Len(), "missing OIDC config should skip the provider")
}

func TestProvideRegistry_EmptyProvider_Empty(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Provider: "",
		},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err)
	assert.Equal(t, 0, registry.Len(), "no config means no providers (dev-login path)")
}

func TestProvideRegistry_UnrecognizedProvider_Skipped(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Providers: []string{"okta-magic"},
		},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err, "unrecognized provider must not be fatal")
	assert.Equal(t, 0, registry.Len(), "unrecognized provider name should be skipped")
}

func TestProvideRegistry_CaseInsensitive_OIDC(t *testing.T) {
	srv := newDiscoverableIssuer(t)
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Provider: "  OIDC  ",
			OIDC: config.OIDCAuthConfig{
				IssuerURL:    srv.URL,
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				RedirectURI:  "http://localhost:8080/api/v1/auth/callback",
			},
		},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err)
	_, ok := registry.Get(idp.ProviderOIDC)
	assert.True(t, ok, "AUTH_PROVIDER should be matched case-insensitively and trimmed")
}

// TestProvideRegistry_MultipleProviders covers the AUTH_PROVIDERS multi-provider
// path: GitHub (no network) plus a discoverable OIDC issuer enable two providers
// at once, and Enabled() is stable-sorted.
func TestProvideRegistry_MultipleProviders(t *testing.T) {
	srv := newDiscoverableIssuer(t)
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Providers: []string{"github", "oidc"},
			GitHub: config.GitHubAuthConfig{
				ClientID:     "gh-client-id",
				ClientSecret: "gh-client-secret",
			},
			OIDC: config.OIDCAuthConfig{
				IssuerURL:    srv.URL,
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				RedirectURI:  "http://localhost:8080/api/v1/auth/callback",
			},
		},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err)
	assert.Equal(t, []idp.ProviderName{idp.ProviderGitHub, idp.ProviderOIDC}, registry.Enabled())
}

// TestProvideRegistry_AuthProvidersOverridesAuthProvider confirms AUTH_PROVIDERS
// takes precedence over the legacy single AUTH_PROVIDER shim.
func TestProvideRegistry_AuthProvidersOverridesAuthProvider(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Providers: []string{"github"},
			Provider:  "oidc",
			GitHub: config.GitHubAuthConfig{
				ClientID:     "gh-client-id",
				ClientSecret: "gh-client-secret",
			},
		},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err)
	assert.Equal(t, []idp.ProviderName{idp.ProviderGitHub}, registry.Enabled(),
		"AUTH_PROVIDERS must take precedence; AUTH_PROVIDER shim ignored")
}
