package services

import (
	"net/http"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentAuthenticator_ApplyAuthentication(t *testing.T) {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)
	authenticator := NewAgentAuthenticator(encryptionSvc)

	t.Run("No authentication required", func(t *testing.T) {
		agent := &models.Agent{
			AgentCard: &models.AgentCard{
				Name: "Test Agent",
			},
		}

		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)
		err = authenticator.ApplyAuthentication(req, agent)

		assert.NoError(t, err)
	})

	t.Run("API Key in header", func(t *testing.T) {
		encrypted, err := encryptionSvc.Encrypt("test-api-key-123")
		require.NoError(t, err)
		credentials := models.AgentCredentials{
			"apiKey": models.AgentCredential{
				Type:  "apiKey",
				Value: encrypted,
			},
		}

		agent := &models.Agent{
			AgentCard: &models.AgentCard{
				Name: "Test Agent",
				SecurityRequirements: a2a.SecurityRequirementsOptions{
					{"apiKey": {}},
				},
				SecuritySchemes: a2a.NamedSecuritySchemes{
					"apiKey": a2a.APIKeySecurityScheme{
						Name:     "X-API-Key",
						Location: a2a.APIKeySecuritySchemeLocationHeader,
					},
				},
			},
			Credentials: &credentials,
		}

		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)
		err = authenticator.ApplyAuthentication(req, agent)

		assert.NoError(t, err)
		assert.Equal(t, "test-api-key-123", req.Header.Get("X-API-Key"))
	})

	t.Run("API Key in query", func(t *testing.T) {
		encrypted, err := encryptionSvc.Encrypt("query-key-456")
		require.NoError(t, err)
		credentials := models.AgentCredentials{
			"queryAuth": models.AgentCredential{
				Type:  "apiKey",
				Value: encrypted,
			},
		}

		agent := &models.Agent{
			AgentCard: &models.AgentCard{
				Name: "Test Agent",
				SecurityRequirements: a2a.SecurityRequirementsOptions{
					{"queryAuth": {}},
				},
				SecuritySchemes: a2a.NamedSecuritySchemes{
					"queryAuth": a2a.APIKeySecurityScheme{
						Name:     "api_key",
						Location: a2a.APIKeySecuritySchemeLocationQuery,
					},
				},
			},
			Credentials: &credentials,
		}

		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)
		err = authenticator.ApplyAuthentication(req, agent)

		assert.NoError(t, err)
		assert.Equal(t, "query-key-456", req.URL.Query().Get("api_key"))
	})

	t.Run("Bearer token authentication", func(t *testing.T) {
		encrypted, err := encryptionSvc.Encrypt("bearer-token-789")
		require.NoError(t, err)
		credentials := models.AgentCredentials{
			"bearerAuth": models.AgentCredential{
				Type:  "http",
				Value: encrypted,
			},
		}

		agent := &models.Agent{
			AgentCard: &models.AgentCard{
				Name: "Test Agent",
				SecurityRequirements: a2a.SecurityRequirementsOptions{
					{"bearerAuth": {}},
				},
				SecuritySchemes: a2a.NamedSecuritySchemes{
					"bearerAuth": a2a.HTTPAuthSecurityScheme{
						Scheme: "bearer",
					},
				},
			},
			Credentials: &credentials,
		}

		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)
		err = authenticator.ApplyAuthentication(req, agent)

		assert.NoError(t, err)
		assert.Equal(t, "Bearer bearer-token-789", req.Header.Get("Authorization"))
	})

	t.Run("Missing required credentials", func(t *testing.T) {
		agent := &models.Agent{
			AgentCard: &models.AgentCard{
				Name: "Test Agent",
				SecurityRequirements: a2a.SecurityRequirementsOptions{
					{"apiKey": {}},
				},
				SecuritySchemes: a2a.NamedSecuritySchemes{
					"apiKey": a2a.APIKeySecurityScheme{
						Name:     "X-API-Key",
						Location: a2a.APIKeySecuritySchemeLocationHeader,
					},
				},
			},
		}

		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)
		err = authenticator.ApplyAuthentication(req, agent)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "authentication required but no credentials found")
	})

	t.Run("Missing agent card", func(t *testing.T) {
		agent := &models.Agent{
			Name: "Test Agent",
		}

		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)
		err = authenticator.ApplyAuthentication(req, agent)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent card is missing")
	})
}

// TestAgentAuthenticator_APIKeyAuthentication_TableDriven tests various API key authentication scenarios
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentAuthenticator_APIKeyAuthentication_TableDriven(t *testing.T) {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)

	tests := []struct {
		name                string
		credentialValue     string
		securityScheme      a2a.SecurityScheme
		expectedHeaderName  string
		expectedHeaderValue string
		expectedQueryParam  string
		expectedQueryValue  string
		shouldError         bool
		errorContains       string
	}{
		{
			name:            "API Key in custom header",
			credentialValue: "custom-api-key-123", // #nosec G101 - test credential
			securityScheme: a2a.APIKeySecurityScheme{
				Name:     "X-Custom-API-Key",
				Location: a2a.APIKeySecuritySchemeLocationHeader,
			},
			expectedHeaderName:  "X-Custom-API-Key",
			expectedHeaderValue: "custom-api-key-123",
			shouldError:         false,
		},
		{
			name:            "API Key in Authorization header (should add Bearer prefix)",
			credentialValue: "secret-token-456",
			securityScheme: a2a.APIKeySecurityScheme{
				Name:     "Authorization",
				Location: a2a.APIKeySecuritySchemeLocationHeader,
			},
			expectedHeaderName:  "Authorization",
			expectedHeaderValue: "Bearer secret-token-456",
			shouldError:         false,
		},
		{
			name:            "API Key in Authorization header with existing Bearer prefix",
			credentialValue: "Bearer existing-prefix-789",
			securityScheme: a2a.APIKeySecurityScheme{
				Name:     "Authorization",
				Location: a2a.APIKeySecuritySchemeLocationHeader,
			},
			expectedHeaderName:  "Authorization",
			expectedHeaderValue: "Bearer existing-prefix-789",
			shouldError:         false,
		},
		{
			name:            "API Key in query parameter",
			credentialValue: "query-key-abc",
			securityScheme: a2a.APIKeySecurityScheme{
				Name:     "apiKey",
				Location: a2a.APIKeySecuritySchemeLocationQuery,
			},
			expectedQueryParam: "apiKey",
			expectedQueryValue: "query-key-abc",
			shouldError:        false,
		},
		{
			name:            "API Key in query with special characters",
			credentialValue: "key-with-special-chars!@#",
			securityScheme: a2a.APIKeySecurityScheme{
				Name:     "api_key",
				Location: a2a.APIKeySecuritySchemeLocationQuery,
			},
			expectedQueryParam: "api_key",
			expectedQueryValue: "key-with-special-chars!@#",
			shouldError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := encryptionSvc.Encrypt(tt.credentialValue)
			require.NoError(t, err)

			credentials := models.AgentCredentials{
				"testScheme": models.AgentCredential{
					Type:  "apiKey",
					Value: encrypted,
				},
			}

			agent := &models.Agent{
				AgentCard: &models.AgentCard{
					SecurityRequirements: a2a.SecurityRequirementsOptions{
						{"testScheme": {}},
					},
					SecuritySchemes: a2a.NamedSecuritySchemes{
						"testScheme": tt.securityScheme,
					},
				},
				Credentials: &credentials,
			}

			authenticator := NewAgentAuthenticator(encryptionSvc)
			req, err := http.NewRequest("GET", "http://example.com/test", nil)
			require.NoError(t, err)
			err = authenticator.ApplyAuthentication(req, agent)

			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify header if expected
				if tt.expectedHeaderName != "" {
					assert.Equal(t, tt.expectedHeaderValue, req.Header.Get(tt.expectedHeaderName),
						"Header %s should have value %s", tt.expectedHeaderName, tt.expectedHeaderValue)
				}

				// Verify query param if expected
				if tt.expectedQueryParam != "" {
					assert.Equal(t, tt.expectedQueryValue, req.URL.Query().Get(tt.expectedQueryParam),
						"Query param %s should have value %s", tt.expectedQueryParam, tt.expectedQueryValue)
				}
			}
		})
	}
}

// TestAgentAuthenticator_HTTPAuthentication_TableDriven tests HTTP Bearer token authentication
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentAuthenticator_HTTPAuthentication_TableDriven(t *testing.T) {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)

	tests := []struct {
		name                string
		credentialValue     string
		securityScheme      a2a.HTTPAuthSecurityScheme
		expectedHeaderValue string
		shouldError         bool
	}{
		{
			name:            "Bearer token without prefix",
			credentialValue: "token-abc-123",
			securityScheme: a2a.HTTPAuthSecurityScheme{
				Scheme: "bearer",
			},
			expectedHeaderValue: "Bearer token-abc-123",
			shouldError:         false,
		},
		{
			name:            "Bearer token with existing Bearer prefix",
			credentialValue: "Bearer token-xyz-789",
			securityScheme: a2a.HTTPAuthSecurityScheme{
				Scheme: "bearer",
			},
			expectedHeaderValue: "Bearer token-xyz-789",
			shouldError:         false,
		},
		{
			name:            "Long bearer token",
			credentialValue: "very-long-token-" + string(make([]byte, 100)),
			securityScheme: a2a.HTTPAuthSecurityScheme{
				Scheme: "bearer",
			},
			expectedHeaderValue: "Bearer very-long-token-" + string(make([]byte, 100)),
			shouldError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := encryptionSvc.Encrypt(tt.credentialValue)
			require.NoError(t, err)

			credentials := models.AgentCredentials{
				"bearerAuth": models.AgentCredential{
					Type:  "http",
					Value: encrypted,
				},
			}

			agent := &models.Agent{
				AgentCard: &models.AgentCard{
					SecurityRequirements: a2a.SecurityRequirementsOptions{
						{"bearerAuth": {}},
					},
					SecuritySchemes: a2a.NamedSecuritySchemes{
						"bearerAuth": tt.securityScheme,
					},
				},
				Credentials: &credentials,
			}

			authenticator := NewAgentAuthenticator(encryptionSvc)
			req, err := http.NewRequest("GET", "http://example.com", nil)
			require.NoError(t, err)
			err = authenticator.ApplyAuthentication(req, agent)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedHeaderValue, req.Header.Get("Authorization"))
			}
		})
	}
}

// TestAgentAuthenticator_MissingCredentials_TableDriven tests error scenarios
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentAuthenticator_MissingCredentials_TableDriven(t *testing.T) {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)

	tests := []struct {
		name          string
		agent         *models.Agent
		expectedError string
	}{
		{
			name: "Missing credentials for required scheme",
			agent: &models.Agent{
				AgentCard: &models.AgentCard{
					SecurityRequirements: a2a.SecurityRequirementsOptions{
						{"apiKey": {}},
					},
					SecuritySchemes: a2a.NamedSecuritySchemes{
						"apiKey": a2a.APIKeySecurityScheme{
							Name:     "X-API-Key",
							Location: a2a.APIKeySecuritySchemeLocationHeader,
						},
					},
				},
				Credentials: nil,
			},
			expectedError: "authentication required but no credentials found",
		},
		{
			name: "Credentials present but wrong scheme name",
			agent: &models.Agent{
				AgentCard: &models.AgentCard{
					SecurityRequirements: a2a.SecurityRequirementsOptions{
						{"apiKey": {}},
					},
					SecuritySchemes: a2a.NamedSecuritySchemes{
						"apiKey": a2a.APIKeySecurityScheme{
							Name:     "X-API-Key",
							Location: a2a.APIKeySecuritySchemeLocationHeader,
						},
					},
				},
				Credentials: &models.AgentCredentials{
					"wrongScheme": models.AgentCredential{
						Type:  "apiKey",
						Value: "encrypted-value",
					},
				},
			},
			expectedError: "authentication required",
		},
		{
			name: "Missing agent card",
			agent: &models.Agent{
				Name: "Test Agent",
			},
			expectedError: "agent card is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticator := NewAgentAuthenticator(encryptionSvc)
			req, err := http.NewRequest("GET", "http://example.com", nil)
			require.NoError(t, err)
			err = authenticator.ApplyAuthentication(req, tt.agent)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

// TestAgentAuthenticator_DecryptionErrors tests decryption failure scenarios
func TestAgentAuthenticator_DecryptionErrors(t *testing.T) {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)

	// Create credentials with invalid encrypted value
	credentials := models.AgentCredentials{
		"apiKey": models.AgentCredential{
			Type:  "apiKey",
			Value: "invalid-encrypted-value-that-cannot-be-decrypted",
		},
	}

	agent := &models.Agent{
		AgentCard: &models.AgentCard{
			SecurityRequirements: a2a.SecurityRequirementsOptions{
				{"apiKey": {}},
			},
			SecuritySchemes: a2a.NamedSecuritySchemes{
				"apiKey": a2a.APIKeySecurityScheme{
					Name:     "X-API-Key",
					Location: a2a.APIKeySecuritySchemeLocationHeader,
				},
			},
		},
		Credentials: &credentials,
	}

	authenticator := NewAgentAuthenticator(encryptionSvc)
	req, err := http.NewRequest("GET", "http://example.com", nil)
	require.NoError(t, err)
	err = authenticator.ApplyAuthentication(req, agent)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt credential")
}
