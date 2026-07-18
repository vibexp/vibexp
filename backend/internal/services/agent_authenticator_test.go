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

// apiKeyAuthCase is one API-key authentication table case.
type apiKeyAuthCase struct {
	name                string
	credentialValue     string
	securityScheme      a2a.SecurityScheme
	expectedHeaderName  string
	expectedHeaderValue string
	expectedQueryParam  string
	expectedQueryValue  string
	shouldError         bool
	errorContains       string
}

// assertAPIKeyAuthOutcome verifies the authentication outcome of one API-key
// table case: the expected error, or the expected header/query credential.
func assertAPIKeyAuthOutcome(t *testing.T, tt apiKeyAuthCase, req *http.Request, err error) {
	t.Helper()
	if tt.shouldError {
		assert.Error(t, err)
		if tt.errorContains != "" {
			assert.Contains(t, err.Error(), tt.errorContains)
		}
		return
	}
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

// TestAgentAuthenticator_APIKeyAuthentication_TableDriven tests various API key authentication scenarios
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentAuthenticator_APIKeyAuthentication_TableDriven(t *testing.T) {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)

	tests := []apiKeyAuthCase{
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

			assertAPIKeyAuthOutcome(t, tt, req, err)
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

// agentWithScheme builds an agent whose card declares a single named security
// scheme with a matching stored credential encrypted under encryptionSvc.
func agentWithScheme(
	t *testing.T, encryptionSvc EncryptionServiceInterface,
	schemeName string, scheme a2a.SecurityScheme, plaintext string,
) *models.Agent {
	t.Helper()
	encrypted, err := encryptionSvc.Encrypt(plaintext)
	require.NoError(t, err)
	credentials := models.AgentCredentials{
		schemeName: models.AgentCredential{Type: "apiKey", Value: encrypted},
	}
	return &models.Agent{
		AgentCard: &models.AgentCard{
			Name:            "Test Agent",
			SecuritySchemes: a2a.NamedSecuritySchemes{a2a.SecuritySchemeName(schemeName): scheme},
		},
		Credentials: &credentials,
	}
}

func TestAgentAuthenticator_DeterministicSchemeSelection(t *testing.T) {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)
	authenticator := NewAgentAuthenticator(encryptionSvc)
	encVal := func(s string) string {
		v, e := encryptionSvc.Encrypt(s)
		require.NoError(t, e)
		return v
	}

	// A card with two header schemes, both with stored credentials.
	twoSchemeAgent := func(reqs a2a.SecurityRequirementsOptions) *models.Agent {
		creds := models.AgentCredentials{
			"alpha": {Type: "apiKey", Value: encVal("alpha-secret")},
			"zeta":  {Type: "apiKey", Value: encVal("zeta-secret")},
		}
		return &models.Agent{
			AgentCard: &models.AgentCard{
				Name:                 "Test Agent",
				SecurityRequirements: reqs,
				SecuritySchemes: a2a.NamedSecuritySchemes{
					"alpha": a2a.APIKeySecurityScheme{Name: "X-Alpha", Location: a2a.APIKeySecuritySchemeLocationHeader},
					"zeta":  a2a.APIKeySecurityScheme{Name: "X-Zeta", Location: a2a.APIKeySecuritySchemeLocationHeader},
				},
			},
			Credentials: &creds,
		}
	}

	// With no security requirement list, selection falls back to sorted scheme
	// names — "alpha" wins on every request, never Go-map-order dependent.
	t.Run("sorted fallback selects the same scheme on every request", func(t *testing.T) {
		agent := twoSchemeAgent(nil)
		for i := 0; i < 50; i++ {
			req, err := http.NewRequest("GET", "http://example.com", nil)
			require.NoError(t, err)
			require.NoError(t, authenticator.ApplyAuthentication(req, agent))
			assert.Equal(t, "alpha-secret", req.Header.Get("X-Alpha"))
			assert.Empty(t, req.Header.Get("X-Zeta"))
		}
	})

	// The card's ordered `security` requirement wins over sort: "zeta" is declared
	// first, so it is selected even though "alpha" sorts earlier.
	t.Run("honors card security-requirement order over sort", func(t *testing.T) {
		agent := twoSchemeAgent(a2a.SecurityRequirementsOptions{{"zeta": {}}, {"alpha": {}}})
		for i := 0; i < 50; i++ {
			req, err := http.NewRequest("GET", "http://example.com", nil)
			require.NoError(t, err)
			require.NoError(t, authenticator.ApplyAuthentication(req, agent))
			assert.Equal(t, "zeta-secret", req.Header.Get("X-Zeta"))
			assert.Empty(t, req.Header.Get("X-Alpha"))
		}
	})
}

//nolint:funlen // Table-driven test with comprehensive scheme coverage
func TestAgentAuthenticator_AuthHeaders(t *testing.T) {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)
	authenticator := NewAgentAuthenticator(encryptionSvc)

	t.Run("API key in header", func(t *testing.T) {
		agent := agentWithScheme(t, encryptionSvc, "apiKey",
			a2a.APIKeySecurityScheme{Name: "X-API-Key", Location: a2a.APIKeySecuritySchemeLocationHeader},
			"secret-key-123")

		headers, err := authenticator.AuthHeaders(agent)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"X-API-Key": "secret-key-123"}, headers)
	})

	t.Run("API key in Authorization header gets Bearer prefix", func(t *testing.T) {
		agent := agentWithScheme(t, encryptionSvc, "apiKey",
			a2a.APIKeySecurityScheme{Name: "Authorization", Location: a2a.APIKeySecuritySchemeLocationHeader},
			"raw-token")

		headers, err := authenticator.AuthHeaders(agent)
		require.NoError(t, err)
		assert.Equal(t, "Bearer raw-token", headers["Authorization"])
	})

	t.Run("HTTP bearer scheme", func(t *testing.T) {
		agent := agentWithScheme(t, encryptionSvc, "bearerAuth",
			a2a.HTTPAuthSecurityScheme{Scheme: "bearer"}, "jwt-token")

		headers, err := authenticator.AuthHeaders(agent)
		require.NoError(t, err)
		assert.Equal(t, "Bearer jwt-token", headers["Authorization"])
	})

	t.Run("HTTP basic scheme", func(t *testing.T) {
		agent := agentWithScheme(t, encryptionSvc, "basicAuth",
			a2a.HTTPAuthSecurityScheme{Scheme: "basic"}, "dXNlcjpwYXNz")

		headers, err := authenticator.AuthHeaders(agent)
		require.NoError(t, err)
		assert.Equal(t, "Basic dXNlcjpwYXNz", headers["Authorization"])
	})

	t.Run("API key in query is not applied as a header", func(t *testing.T) {
		agent := agentWithScheme(t, encryptionSvc, "queryAuth",
			a2a.APIKeySecurityScheme{Name: "api_key", Location: a2a.APIKeySecuritySchemeLocationQuery},
			"query-key")

		headers, err := authenticator.AuthHeaders(agent)
		require.NoError(t, err)
		assert.Empty(t, headers)
	})

	t.Run("API key in cookie is not applied as a header", func(t *testing.T) {
		agent := agentWithScheme(t, encryptionSvc, "cookieAuth",
			a2a.APIKeySecurityScheme{Name: "session", Location: a2a.APIKeySecuritySchemeLocationCookie},
			"cookie-val")

		headers, err := authenticator.AuthHeaders(agent)
		require.NoError(t, err)
		assert.Empty(t, headers)
	})

	t.Run("no card yields empty headers", func(t *testing.T) {
		headers, err := authenticator.AuthHeaders(&models.Agent{})
		require.NoError(t, err)
		assert.Empty(t, headers)
	})

	t.Run("nil agent yields empty headers", func(t *testing.T) {
		headers, err := authenticator.AuthHeaders(nil)
		require.NoError(t, err)
		assert.Empty(t, headers)
	})

	t.Run("scheme without a matching credential is skipped", func(t *testing.T) {
		agent := &models.Agent{
			AgentCard: &models.AgentCard{
				SecuritySchemes: a2a.NamedSecuritySchemes{
					"apiKey": a2a.APIKeySecurityScheme{
						Name: "X-API-Key", Location: a2a.APIKeySecuritySchemeLocationHeader,
					},
				},
			},
			Credentials: &models.AgentCredentials{}, // no credential for "apiKey"
		}

		headers, err := authenticator.AuthHeaders(agent)
		require.NoError(t, err)
		assert.Empty(t, headers)
	})

	t.Run("undecryptable credential returns an error", func(t *testing.T) {
		agent := &models.Agent{
			AgentCard: &models.AgentCard{
				SecuritySchemes: a2a.NamedSecuritySchemes{
					"apiKey": a2a.APIKeySecurityScheme{
						Name: "X-API-Key", Location: a2a.APIKeySecuritySchemeLocationHeader,
					},
				},
			},
			Credentials: &models.AgentCredentials{
				"apiKey": models.AgentCredential{Type: "apiKey", Value: "not-encrypted"},
			},
		}

		_, err := authenticator.AuthHeaders(agent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt credential")
	})
}
