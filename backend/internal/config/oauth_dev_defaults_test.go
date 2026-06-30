package config

import (
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
			cfg := &Config{Frontend: FrontendConfig{BaseURL: tc.frontendURL}}
			assert.Equal(t, tc.want, cfg.IsLocalDevelopment())
		})
	}
}

func TestApplyDevOAuthASDefaults(t *testing.T) {
	t.Run("derives issuer and resource in dev when both unset", func(t *testing.T) {
		cfg := &Config{
			Frontend: FrontendConfig{BaseURL: "http://localhost:5173"},
			Server:   ServerConfig{Port: "8080"},
		}
		applyDevOAuthASDefaults(cfg)
		assert.Equal(t, "http://localhost:8080", cfg.Auth.OAuthAS.IssuerURL)
		assert.Equal(t, "http://localhost:8080/mcp/v1/common", cfg.MCP.ResourceURI)
	})

	t.Run("honours a custom port", func(t *testing.T) {
		cfg := &Config{
			Frontend: FrontendConfig{BaseURL: "http://localhost:5173"},
			Server:   ServerConfig{Port: "9090"},
		}
		applyDevOAuthASDefaults(cfg)
		assert.Equal(t, "http://localhost:9090", cfg.Auth.OAuthAS.IssuerURL)
		assert.Equal(t, "http://localhost:9090/mcp/v1/common", cfg.MCP.ResourceURI)
	})

	t.Run("derives resource from an explicitly set issuer", func(t *testing.T) {
		cfg := &Config{
			Frontend: FrontendConfig{BaseURL: "http://localhost:5173"},
			Server:   ServerConfig{Port: "8080"},
			Auth:     AuthConfig{OAuthAS: OAuthASConfig{IssuerURL: "http://localhost:3000"}},
		}
		applyDevOAuthASDefaults(cfg)
		assert.Equal(t, "http://localhost:3000", cfg.Auth.OAuthAS.IssuerURL, "explicit issuer must win")
		assert.Equal(t, "http://localhost:3000/mcp/v1/common", cfg.MCP.ResourceURI)
	})

	t.Run("never overwrites explicit values", func(t *testing.T) {
		cfg := &Config{
			Frontend: FrontendConfig{BaseURL: "http://localhost:5173"},
			Server:   ServerConfig{Port: "8080"},
			Auth:     AuthConfig{OAuthAS: OAuthASConfig{IssuerURL: "https://custom.test"}},
			MCP:      MCPConfig{ResourceURI: "https://custom.test/mcp"},
		}
		applyDevOAuthASDefaults(cfg)
		assert.Equal(t, "https://custom.test", cfg.Auth.OAuthAS.IssuerURL)
		assert.Equal(t, "https://custom.test/mcp", cfg.MCP.ResourceURI)
	})

	t.Run("respects an explicit external MCP issuer opt-out", func(t *testing.T) {
		cfg := &Config{
			Frontend: FrontendConfig{BaseURL: "http://localhost:5173"},
			Server:   ServerConfig{Port: "8080"},
			MCP:      MCPConfig{OAuthIssuer: "https://external-idp.example"},
		}
		applyDevOAuthASDefaults(cfg)
		assert.Empty(t, cfg.Auth.OAuthAS.IssuerURL, "must not auto-enable the AS when pointed at an external issuer")
		assert.Empty(t, cfg.MCP.ResourceURI)
	})

	t.Run("production never derives an issuer", func(t *testing.T) {
		cfg := &Config{
			Frontend: FrontendConfig{BaseURL: "https://app.example.com"},
			Server:   ServerConfig{Port: "8080"},
		}
		applyDevOAuthASDefaults(cfg)
		assert.Empty(t, cfg.Auth.OAuthAS.IssuerURL, "the AS must stay opt-in in production")
		assert.Empty(t, cfg.MCP.ResourceURI)
	})
}

// TestLoad_DevAutoEnablesEmbeddedAS proves the zero-config local path end-to-end:
// with a localhost frontend.base_url (the default) and no oauth_as/mcp config,
// Load derives the issuer, points the MCP resource server at it, and passes
// validateOAuthASConfig.
func TestLoad_DevAutoEnablesEmbeddedAS(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+`
frontend:
  base_url: http://localhost:5173
`)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", cfg.Auth.OAuthAS.IssuerURL)
	assert.Equal(t, "http://localhost:8080/mcp/v1/common", cfg.MCP.ResourceURI)
	assert.Equal(t, cfg.Auth.OAuthAS.IssuerURL, cfg.MCP.OAuthIssuer,
		"the MCP resource server must trust the embedded AS by default")
}

// TestLoad_ProductionDoesNotAutoEnableAS guards the production safety criterion:
// a non-localhost frontend.base_url must leave the AS disabled (no guessed issuer).
func TestLoad_ProductionDoesNotAutoEnableAS(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+`
frontend:
  base_url: https://app.example.com
`)
	require.NoError(t, err)
	assert.Empty(t, cfg.Auth.OAuthAS.IssuerURL)
	assert.Empty(t, cfg.MCP.ResourceURI)
	assert.Empty(t, cfg.MCP.OAuthIssuer)
}
