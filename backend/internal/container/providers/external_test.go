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
	"github.com/vibexp/vibexp/internal/external/implementations"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestProvideEmailProvider_SMTPWithConfig(t *testing.T) {
	cfg := &config.Config{
		EmailProvider: "smtp",
		SMTPHost:      "smtp.example.com",
		SMTPPort:      "587",
		SMTPUsername:  "test@example.com",
		SMTPPassword:  "password",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*implementations.SMTPEmailProvider)
	assert.True(t, ok, "Provider should be *SMTPEmailProvider when EMAIL_PROVIDER=smtp")
}

func TestProvideEmailProvider_SMTPWithEmptyConfig_ReturnsStub(t *testing.T) {
	cfg := &config.Config{
		EmailProvider: "smtp",
		SMTPHost:      "",
		SMTPPort:      "",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*stubEmailProvider)
	assert.True(t, ok, "Provider should be *stubEmailProvider when smtp host/port empty")
}

func TestProvideEmailProvider_EmptyProvider_FallsToSMTP(t *testing.T) {
	cfg := &config.Config{
		EmailProvider: "",
		SMTPHost:      "smtp.example.com",
		SMTPPort:      "587",
		SMTPUsername:  "test@example.com",
		SMTPPassword:  "password",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*implementations.SMTPEmailProvider)
	assert.True(t, ok, "Empty EMAIL_PROVIDER should fall through to smtp")
}

func TestProvideEmailProvider_EmptyProvider_NoSMTPConfig_ReturnsStub(t *testing.T) {
	cfg := &config.Config{
		EmailProvider: "",
		SMTPHost:      "",
		SMTPPort:      "",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*stubEmailProvider)
	assert.True(t, ok, "Empty EMAIL_PROVIDER with no SMTP config should return stub")
}

func TestProvideEmailProvider_MailgunWithValidKey(t *testing.T) {
	cfg := &config.Config{
		EmailProvider:     "mailgun",
		MailgunDomain:     "mg.example.com",
		MailgunSendingKey: "key-abc123",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*implementations.MailgunEmailProvider)
	assert.True(t, ok, "Provider should be *MailgunEmailProvider when EMAIL_PROVIDER=mailgun")
}

func TestProvideEmailProvider_MailgunWithEmptyKey_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		EmailProvider:     "mailgun",
		MailgunDomain:     "mg.example.com",
		MailgunSendingKey: "",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MAILGUN_SENDING_KEY")
	assert.Nil(t, provider)
}

func TestProvideEmailProvider_PostmarkWithValidToken(t *testing.T) {
	cfg := &config.Config{
		EmailProvider:       "postmark",
		PostmarkServerToken: "token-abc123",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*implementations.PostmarkEmailProvider)
	assert.True(t, ok, "Provider should be *PostmarkEmailProvider when EMAIL_PROVIDER=postmark")
}

func TestProvideEmailProvider_PostmarkWithEmptyToken_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		EmailProvider:       "postmark",
		PostmarkServerToken: "",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "POSTMARK_SERVER_TOKEN")
	assert.Nil(t, provider)
}

func TestProvideEmailProvider_SendGridWithValidKey(t *testing.T) {
	cfg := &config.Config{
		EmailProvider:  "sendgrid",
		SendGridAPIKey: "test-sendgrid-key",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*implementations.SendGridEmailProvider)
	assert.True(t, ok, "Provider should be *SendGridEmailProvider when EMAIL_PROVIDER=sendgrid")
}

func TestProvideEmailProvider_SendGridWithEmptyKey_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		EmailProvider:  "sendgrid",
		SendGridAPIKey: "",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SENDGRID_API_KEY")
	assert.Nil(t, provider)
}

func TestProvideEmailProvider_UnknownProvider_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		EmailProvider: "ses",
	}

	provider, err := ProvideEmailProvider(cfg, testLogger())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ses")
	assert.Contains(t, err.Error(), "unknown email provider")
	assert.Nil(t, provider)
}

func TestProvideEmailProvider_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name          string
		emailProvider string
	}{
		{"uppercase MAILGUN", "MAILGUN"},
		{"mixed case Mailgun", "Mailgun"},
		{"padded smtp", "  smtp  "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				EmailProvider:     tc.emailProvider,
				MailgunDomain:     "mg.example.com",
				MailgunSendingKey: "key-abc123",
				SMTPHost:          "smtp.example.com",
				SMTPPort:          "587",
				SMTPUsername:      "test@example.com",
				SMTPPassword:      "password",
			}
			provider, err := ProvideEmailProvider(cfg, testLogger())
			require.NoError(t, err)
			assert.NotNil(t, provider)
		})
	}
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
		AuthProvider:     "oidc",
		OIDCIssuerURL:    srv.URL,
		OIDCClientID:     "client-id",
		OIDCClientSecret: "client-secret",
		OIDCRedirectURI:  "http://localhost:8080/api/v1/auth/callback",
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
		AuthProvider:     "oidc",
		OIDCIssuerURL:    "https://oidc.invalid.example.com",
		OIDCClientID:     "client-id",
		OIDCClientSecret: "client-secret",
		OIDCRedirectURI:  "http://localhost:8080/api/v1/auth/callback",
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err, "OIDC discovery failure must be non-fatal")
	assert.Equal(t, 0, registry.Len(), "OIDC discovery failure should skip the provider")
}

func TestProvideRegistry_OIDCMissingConfig_NonFatalSkip(t *testing.T) {
	cfg := &config.Config{
		AuthProvider: "oidc",
		// no OIDC_* fields set -> oidc.Config.Validate fails -> skipped
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err, "invalid OIDC config must be non-fatal")
	assert.Equal(t, 0, registry.Len(), "missing OIDC config should skip the provider")
}

func TestProvideRegistry_EmptyProvider_Empty(t *testing.T) {
	cfg := &config.Config{
		AuthProvider: "",
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err)
	assert.Equal(t, 0, registry.Len(), "no config means no providers (dev-login path)")
}

func TestProvideRegistry_UnrecognizedProvider_Skipped(t *testing.T) {
	cfg := &config.Config{
		AuthProviders: []string{"okta-magic"},
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err, "unrecognized provider must not be fatal")
	assert.Equal(t, 0, registry.Len(), "unrecognized provider name should be skipped")
}

func TestProvideRegistry_CaseInsensitive_OIDC(t *testing.T) {
	srv := newDiscoverableIssuer(t)
	cfg := &config.Config{
		AuthProvider:     "  OIDC  ",
		OIDCIssuerURL:    srv.URL,
		OIDCClientID:     "client-id",
		OIDCClientSecret: "client-secret",
		OIDCRedirectURI:  "http://localhost:8080/api/v1/auth/callback",
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
		AuthProviders:      []string{"github", "oidc"},
		GitHubClientID:     "gh-client-id",
		GitHubClientSecret: "gh-client-secret",
		OIDCIssuerURL:      srv.URL,
		OIDCClientID:       "client-id",
		OIDCClientSecret:   "client-secret",
		OIDCRedirectURI:    "http://localhost:8080/api/v1/auth/callback",
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err)
	assert.Equal(t, []idp.ProviderName{idp.ProviderGitHub, idp.ProviderOIDC}, registry.Enabled())
}

// TestProvideRegistry_AuthProvidersOverridesAuthProvider confirms AUTH_PROVIDERS
// takes precedence over the legacy single AUTH_PROVIDER shim.
func TestProvideRegistry_AuthProvidersOverridesAuthProvider(t *testing.T) {
	cfg := &config.Config{
		AuthProviders:      []string{"github"},
		AuthProvider:       "oidc",
		GitHubClientID:     "gh-client-id",
		GitHubClientSecret: "gh-client-secret",
	}

	registry, err := ProvideIdentityProviderRegistry(cfg, testLogger())

	require.NoError(t, err)
	assert.Equal(t, []idp.ProviderName{idp.ProviderGitHub}, registry.Enabled(),
		"AUTH_PROVIDERS must take precedence; AUTH_PROVIDER shim ignored")
}
