package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Regression tests for #464. The /validate endpoints took a caller-supplied
// base_url and made the server fetch it with a bare http.Client, so any
// authenticated team member could probe the host's internal network and read a
// precise success/failure oracle out of the response.

// prodEmbeddingService builds an embedding provider service with a
// PRODUCTION-shaped config, so ssrfGuardForConfig hands it the fail-closed
// policy. Using localDevProviderConfig() here would prove nothing.
func prodEmbeddingService(t *testing.T) *EmbeddingProviderService {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	return NewEmbeddingProviderService(
		mocks.NewMockEmbeddingProviderRepository(t), enc,
		&config.Config{Frontend: config.FrontendConfig{BaseURL: "https://app.example.com"}},
		permissiveProviderAuthz{},
	)
}

func prodModelService(t *testing.T) *ModelProviderService {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	return NewModelProviderService(
		mocks.NewMockModelProviderRepository(t), enc,
		&config.Config{Frontend: config.FrontendConfig{BaseURL: "https://app.example.com"}},
		permissiveProviderAuthz{},
	)
}

// blockedDestinations is the policy ssrfGuard.isBlockedIP already encodes,
// expressed as the URLs an attacker would actually try.
var blockedDestinations = []struct {
	name    string
	baseURL string
}{
	{"loopback v4", "http://127.0.0.1:8080/v1"},
	{"loopback name", "http://localhost:8080/v1"},
	{"private 10/8", "http://10.0.0.1/v1"},
	{"private 192.168/16", "http://192.168.1.1/v1"},
	{"private 172.16/12", "http://172.16.0.1/v1"},
	{"cloud metadata IP", "http://169.254.169.254/latest/meta-data"},
	{"cloud metadata name", "http://metadata.google.internal/v1"},
	{"cloud metadata name, trailing dot + case", "http://Metadata.Google.Internal./v1"},
	{"bare metadata name", "http://metadata/v1"},
	{"loopback v6", "http://[::1]:8080/v1"},
	{"unspecified", "http://0.0.0.0/v1"},
}

func TestValidateEmbeddingProvider_RejectsInternalDestinations(t *testing.T) {
	for _, tt := range blockedDestinations {
		t.Run(tt.name, func(t *testing.T) {
			svc := prodEmbeddingService(t)

			resp, err := svc.ValidateEmbeddingProvider(
				context.Background(), testProviderTeamID, testProviderUserID,
				models.ValidateEmbeddingProviderRequest{
					ProviderType: ProviderTypeOpenAICompatible,
					BaseURL:      tt.baseURL,
					Model:        "some-model",
				},
			)

			require.NoError(t, err, "a blocked destination is a validation result, not a service error")
			assert.False(t, resp.IsValid, "internal destination must never validate")
			assert.Equal(t, providerErrDestinationNotAllowed, resp.Details.ErrorDetails)
		})
	}
}

func TestValidateModelProvider_RejectsInternalDestinations(t *testing.T) {
	for _, tt := range blockedDestinations {
		t.Run(tt.name, func(t *testing.T) {
			svc := prodModelService(t)

			resp, err := svc.ValidateModelProvider(
				context.Background(), testProviderTeamID, testProviderUserID,
				models.ValidateModelProviderRequest{
					ProviderType: ProviderTypeOpenAICompatible,
					BaseURL:      tt.baseURL,
					Model:        "some-model",
				},
			)

			require.NoError(t, err)
			assert.False(t, resp.IsValid)
			assert.Equal(t, providerErrDestinationNotAllowed, resp.Details.ErrorDetails)
		})
	}
}

// TestValidateProvider_NoOracleInErrorDetails is the exfiltration half of the
// finding: even for a permitted destination, the response must not tell the
// caller *why* it failed in a way that separates a closed port from an open one
// or echoes the URL back.
func TestValidateProvider_NoOracleInErrorDetails(t *testing.T) {
	// A public-looking name that does not resolve — permitted by policy, fails
	// at connect. The old code returned the raw dial error here.
	const unreachable = "http://this-host-does-not-exist.example/v1"

	t.Run("embedding", func(t *testing.T) {
		svc := prodEmbeddingService(t)
		resp, err := svc.ValidateEmbeddingProvider(
			context.Background(), testProviderTeamID, testProviderUserID,
			models.ValidateEmbeddingProviderRequest{
				ProviderType: ProviderTypeOpenAICompatible,
				BaseURL:      unreachable,
				Model:        "m",
			},
		)
		require.NoError(t, err)
		assert.False(t, resp.IsValid)
		assertNoOracle(t, resp.Details.ErrorDetails, unreachable)
	})

	t.Run("model", func(t *testing.T) {
		svc := prodModelService(t)
		resp, err := svc.ValidateModelProvider(
			context.Background(), testProviderTeamID, testProviderUserID,
			models.ValidateModelProviderRequest{
				ProviderType: ProviderTypeOpenAICompatible,
				BaseURL:      unreachable,
				Model:        "m",
			},
		)
		require.NoError(t, err)
		assert.False(t, resp.IsValid)
		assertNoOracle(t, resp.Details.ErrorDetails, unreachable)
	})
}

// assertNoOracle pins that error_details is one of the fixed categories and
// leaks neither the target URL nor connection-level wording.
func assertNoOracle(t *testing.T, details, targetURL string) {
	t.Helper()

	assert.NotContains(t, details, targetURL, "error_details must not echo the target URL")
	assert.NotContains(t, strings.ToLower(details), "this-host-does-not-exist")
	for _, leak := range []string{"refused", "no such host", "dial tcp", "timeout", "lookup"} {
		assert.NotContains(t, strings.ToLower(details), leak,
			"error_details must not reveal the connection outcome")
	}
	assert.Contains(t, []string{
		providerErrDestinationNotAllowed,
		providerErrMisconfigured,
		providerErrUnauthorized,
		providerErrBadDimension,
		providerErrConnectionFailed,
	}, details, "error_details must be one of the fixed categories")
}

// TestValidateProvider_LocalDevelopmentStillPermitsLoopback pins the
// self-hosted local workflow the guard deliberately exempts (Ollama on
// localhost), so the fix does not break `make backend-run-dev`.
func TestValidateProvider_LocalDevelopmentStillPermitsLoopback(t *testing.T) {
	server := httptest.NewServer(embeddingHandlerReturningDimension(t, EmbeddingVectorDimensions))
	defer server.Close()

	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	svc := NewEmbeddingProviderService(
		mocks.NewMockEmbeddingProviderRepository(t), enc,
		localDevProviderConfig(), permissiveProviderAuthz{},
	)

	resp, err := svc.ValidateEmbeddingProvider(
		context.Background(), testProviderTeamID, testProviderUserID,
		models.ValidateEmbeddingProviderRequest{
			ProviderType: ProviderTypeOpenAICompatible,
			BaseURL:      server.URL,
			Model:        "nomic-embed-text",
		},
	)

	require.NoError(t, err)
	assert.True(t, resp.IsValid, "local development must still accept a loopback provider")
}

// TestProviderBaseURLScheme rejects non-HTTP schemes before any dial, which the
// IP-level guard alone would not catch.
func TestProviderBaseURLScheme(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"https", "https://api.openai.com/v1", false},
		{"http", "http://example.com/v1", false},
		{"file", "file:///etc/passwd", true},
		{"gopher", "gopher://example.com/", true},
		{"no scheme", "example.com/v1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProviderBaseURLScheme(tt.baseURL)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestProviderHTTPClient_NilGuardFailsClosed pins that a caller forgetting to
// thread a guard gets the production policy, not an unguarded client.
func TestProviderHTTPClient_NilGuardFailsClosed(t *testing.T) {
	client := newProviderHTTPClient(nil, 0)

	// The guard rejects at dial time, so a loopback request must fail even
	// though nothing validated the URL up front.
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, "http://127.0.0.1:9/", http.NoBody)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disallowed address range")
}
