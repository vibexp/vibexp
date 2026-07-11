package services

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"

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

	authRequired := len(agent.AgentCard.SecurityRequirements) > 0

	// If no credentials are stored, return error if security is required
	if agent.Credentials == nil || len(*agent.Credentials) == 0 {
		if authRequired {
			return fmt.Errorf("authentication required but no credentials found")
		}
		return nil
	}

	// Apply authentication based on the first security scheme with a matching credential
	for schemeName, scheme := range agent.AgentCard.SecuritySchemes {
		if credential, exists := (*agent.Credentials)[string(schemeName)]; exists {
			return a.applySchemeAuthentication(req, scheme, credential)
		}
	}

	// If we have security schemes but no matching credentials, check if auth is required
	if authRequired {
		return fmt.Errorf("authentication required but no matching credentials found")
	}

	return nil
}

// applySchemeAuthentication applies a specific security scheme to the request
func (a *AgentAuthenticator) applySchemeAuthentication(
	req *http.Request, scheme a2a.SecurityScheme, credential models.AgentCredential,
) error {
	// Decrypt the credential value
	decrypted, err := a.encryptionService.Decrypt(credential.Value)
	if err != nil {
		return fmt.Errorf("failed to decrypt credential: %w", err)
	}

	switch s := scheme.(type) {
	case a2a.APIKeySecurityScheme:
		return a.applyAPIKeyAuth(req, s, decrypted)
	case a2a.HTTPAuthSecurityScheme:
		return a.applyHTTPAuth(req, s, decrypted)
	default:
		return fmt.Errorf("unsupported security scheme type: %T", scheme)
	}
}

// applyAPIKeyAuth applies API key authentication
func (a *AgentAuthenticator) applyAPIKeyAuth(
	req *http.Request, scheme a2a.APIKeySecurityScheme, value string,
) error {
	switch scheme.Location {
	case a2a.APIKeySecuritySchemeLocationHeader:
		// Special handling for Authorization header - add Bearer prefix if not already present
		if scheme.Name == "Authorization" && !hasAuthPrefix(value) {
			value = "Bearer " + value
		}
		req.Header.Set(scheme.Name, value)
	case a2a.APIKeySecuritySchemeLocationQuery:
		q := req.URL.Query()
		q.Set(scheme.Name, value)
		req.URL.RawQuery = q.Encode()
	case a2a.APIKeySecuritySchemeLocationCookie:
		req.AddCookie(&http.Cookie{
			Name:  scheme.Name,
			Value: value,
		})
	default:
		return fmt.Errorf("unsupported API key location: %s", scheme.Location)
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

// applyHTTPAuth applies HTTP authentication (Basic or Bearer) based on the scheme's
// RFC 7235 HTTP Authentication scheme name.
func (a *AgentAuthenticator) applyHTTPAuth(
	req *http.Request, scheme a2a.HTTPAuthSecurityScheme, value string,
) error {
	switch strings.ToLower(scheme.Scheme) {
	case "basic":
		if !hasAuthPrefix(value) {
			value = "Basic " + value
		}
		req.Header.Set("Authorization", value)
	default:
		// Default to bearer for "bearer" and any unspecified scheme
		if !hasAuthPrefix(value) {
			value = "Bearer " + value
		}
		req.Header.Set("Authorization", value)
	}
	return nil
}
