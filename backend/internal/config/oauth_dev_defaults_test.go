package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsLocalDevelopment(t *testing.T) {
	cases := []struct {
		name        string
		frontendURL string
		want        bool
	}{
		{"localhost", "http://localhost:5173", true},
		{"loopback ip", "http://127.0.0.1:5173", true},
		{"empty is not dev (fail-closed)", "", false},
		{"production host", "https://app.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{FrontendBaseURL: tc.frontendURL}
			assert.Equal(t, tc.want, cfg.IsLocalDevelopment())
		})
	}
}

func TestApplyDevOAuthASDefaults(t *testing.T) {
	t.Run("derives issuer and resource in dev when both unset", func(t *testing.T) {
		cfg := &Config{FrontendBaseURL: "http://localhost:5173", Port: "8080"}
		applyDevOAuthASDefaults(cfg)
		assert.Equal(t, "http://localhost:8080", cfg.OAuthASIssuerURL)
		assert.Equal(t, "http://localhost:8080/mcp/v1/common", cfg.MCPResourceURI)
	})

	t.Run("honours a custom port", func(t *testing.T) {
		cfg := &Config{FrontendBaseURL: "http://localhost:5173", Port: "9090"}
		applyDevOAuthASDefaults(cfg)
		assert.Equal(t, "http://localhost:9090", cfg.OAuthASIssuerURL)
		assert.Equal(t, "http://localhost:9090/mcp/v1/common", cfg.MCPResourceURI)
	})

	t.Run("derives resource from an explicitly set issuer", func(t *testing.T) {
		cfg := &Config{
			FrontendBaseURL:  "http://localhost:5173",
			Port:             "8080",
			OAuthASIssuerURL: "http://localhost:3000",
		}
		applyDevOAuthASDefaults(cfg)
		assert.Equal(t, "http://localhost:3000", cfg.OAuthASIssuerURL, "explicit issuer must win")
		assert.Equal(t, "http://localhost:3000/mcp/v1/common", cfg.MCPResourceURI)
	})

	t.Run("never overwrites explicit values", func(t *testing.T) {
		cfg := &Config{
			FrontendBaseURL:  "http://localhost:5173",
			Port:             "8080",
			OAuthASIssuerURL: "https://custom.test",
			MCPResourceURI:   "https://custom.test/mcp",
		}
		applyDevOAuthASDefaults(cfg)
		assert.Equal(t, "https://custom.test", cfg.OAuthASIssuerURL)
		assert.Equal(t, "https://custom.test/mcp", cfg.MCPResourceURI)
	})

	t.Run("respects an explicit external MCP issuer opt-out", func(t *testing.T) {
		cfg := &Config{
			FrontendBaseURL: "http://localhost:5173",
			Port:            "8080",
			MCPOAuthIssuer:  "https://external-idp.example",
		}
		applyDevOAuthASDefaults(cfg)
		assert.Empty(t, cfg.OAuthASIssuerURL, "must not auto-enable the AS when pointed at an external issuer")
		assert.Empty(t, cfg.MCPResourceURI)
	})

	t.Run("production never derives an issuer", func(t *testing.T) {
		cfg := &Config{FrontendBaseURL: "https://app.example.com", Port: "8080"}
		applyDevOAuthASDefaults(cfg)
		assert.Empty(t, cfg.OAuthASIssuerURL, "the AS must stay opt-in in production")
		assert.Empty(t, cfg.MCPResourceURI)
	})
}

// TestLoad_DevAutoEnablesEmbeddedAS proves the zero-config local path end-to-end:
// with a localhost FRONTEND_BASE_URL and no OAuth/MCP env set, Load derives the
// issuer, points the MCP resource server at it, and passes validateOAuthASConfig.
func TestLoad_DevAutoEnablesEmbeddedAS(t *testing.T) {
	clearOAuthEnv(t)
	t.Setenv("FRONTEND_BASE_URL", "http://localhost:5173")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", cfg.OAuthASIssuerURL)
	assert.Equal(t, "http://localhost:8080/mcp/v1/common", cfg.MCPResourceURI)
	assert.Equal(t, cfg.OAuthASIssuerURL, cfg.MCPOAuthIssuer,
		"the MCP resource server must trust the embedded AS by default")
}

// TestLoad_ProductionDoesNotAutoEnableAS guards the production safety criterion:
// a non-localhost FRONTEND_BASE_URL must leave the AS disabled (no guessed issuer).
func TestLoad_ProductionDoesNotAutoEnableAS(t *testing.T) {
	clearOAuthEnv(t)
	t.Setenv("FRONTEND_BASE_URL", "https://app.example.com")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.OAuthASIssuerURL)
	assert.Empty(t, cfg.MCPResourceURI)
	assert.Empty(t, cfg.MCPOAuthIssuer)
}

// clearOAuthEnv unsets the OAuth/MCP env vars for the duration of a test so the
// derivation path is exercised from a clean slate, restoring them afterwards.
func clearOAuthEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"OAUTH_AS_ISSUER_URL", "MCP_RESOURCE_URI", "MCP_OAUTH_ISSUER"} {
		if prev, ok := os.LookupEnv(key); ok {
			require.NoError(t, os.Unsetenv(key))
			t.Cleanup(func() { require.NoError(t, os.Setenv(key, prev)) })
		}
	}
}
