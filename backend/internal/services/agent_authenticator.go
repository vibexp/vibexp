package services

import (
	"fmt"
	"net/http"

	"github.com/vibexp/vibexp/internal/models"
)

// AgentAuthenticator handles applying authentication to HTTP requests for agents
type AgentAuthenticator struct {
	encryptionService EncryptionServiceInterface
}

// NewAgentAuthenticator creates a new agent authenticator
func NewAgentAuthenticator(encryptionService EncryptionServiceInterface) *AgentAuthenticator {
	return &AgentAuthenticator{
		encryptionService: encryptionService,
	}
}

// ApplyAuthentication applies the appropriate authentication to the HTTP request based on agent configuration
func (a *AgentAuthenticator) ApplyAuthentication(req *http.Request, agent *models.Agent) error {
	if agent.AgentCard == nil {
		return fmt.Errorf("agent card is missing")
	}

	// If there are no security schemes defined, no authentication is required
	if len(agent.AgentCard.SecuritySchemes) == 0 {
		return nil
	}

	// If no credentials are stored, return error if security is required
	if agent.Credentials == nil || len(*agent.Credentials) == 0 {
		// Check if authentication is actually required
		if len(agent.AgentCard.Security) > 0 {
			return fmt.Errorf("authentication required but no credentials found")
		}
		return nil
	}

	// Apply authentication based on the first security scheme and available credentials
	for schemeName, scheme := range agent.AgentCard.SecuritySchemes {
		if credential, exists := (*agent.Credentials)[schemeName]; exists {
			return a.applySchemeAuthentication(req, scheme, credential)
		}
	}

	// If we have security schemes but no matching credentials, check if auth is required
	if len(agent.AgentCard.Security) > 0 {
		return fmt.Errorf("authentication required but no matching credentials found")
	}

	return nil
}

// applySchemeAuthentication applies a specific security scheme to the request
func (a *AgentAuthenticator) applySchemeAuthentication(
	req *http.Request, scheme models.AgentSecurityScheme, credential models.AgentCredential,
) error {
	// Decrypt the credential value
	decrypted, err := a.encryptionService.Decrypt(credential.Value)
	if err != nil {
		return fmt.Errorf("failed to decrypt credential: %w", err)
	}

	switch scheme.Type {
	case "apiKey":
		return a.applyAPIKeyAuth(req, scheme, decrypted)
	case "http":
		return a.applyHTTPAuth(req, scheme, decrypted)
	default:
		return fmt.Errorf("unsupported security scheme type: %s", scheme.Type)
	}
}

// applyAPIKeyAuth applies API key authentication
func (a *AgentAuthenticator) applyAPIKeyAuth(req *http.Request, scheme models.AgentSecurityScheme, value string) error {
	switch scheme.In {
	case "header":
		// Special handling for Authorization header - add Bearer prefix if not already present
		if scheme.Name == "Authorization" && !hasAuthPrefix(value) {
			value = "Bearer " + value
		}
		req.Header.Set(scheme.Name, value)
	case "query":
		q := req.URL.Query()
		q.Set(scheme.Name, value)
		req.URL.RawQuery = q.Encode()
	case "cookie":
		req.AddCookie(&http.Cookie{
			Name:  scheme.Name,
			Value: value,
		})
	default:
		return fmt.Errorf("unsupported API key location: %s", scheme.In)
	}
	return nil
}

// hasAuthPrefix checks if the value already has an authentication prefix
func hasAuthPrefix(value string) bool {
	prefixes := []string{"Bearer ", "Basic ", "Token ", "ApiKey "}
	for _, prefix := range prefixes {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// applyHTTPAuth applies HTTP authentication (Basic or Bearer)
func (a *AgentAuthenticator) applyHTTPAuth(req *http.Request, scheme models.AgentSecurityScheme, value string) error {
	// For HTTP auth, the scheme name in the spec typically indicates the auth type
	// For bearer tokens, value should be the token
	// For basic auth, value should be "username:password"
	switch scheme.Name {
	case "bearer", "Bearer":
		// Don't add Bearer prefix if already present
		if !hasAuthPrefix(value) {
			value = "Bearer " + value
		}
		req.Header.Set("Authorization", value)
	case "basic", "Basic":
		// Don't add Basic prefix if already present
		if !hasAuthPrefix(value) {
			value = "Basic " + value
		}
		req.Header.Set("Authorization", value)
	default:
		// Default to bearer if not specified
		if !hasAuthPrefix(value) {
			value = "Bearer " + value
		}
		req.Header.Set("Authorization", value)
	}
	return nil
}
