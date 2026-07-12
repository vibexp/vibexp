package services

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/vibexp/vibexp/internal/models"
)

// orderedSchemeNames returns the card's security-scheme names in a deterministic
// order so scheme selection is reproducible across requests (a Go map range is
// not). It honors the card's ordered `security` requirement list first — ties
// within one requirement broken by sorted name — then appends any remaining
// declared schemes by sorted name.
func orderedSchemeNames(card *models.AgentCard) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(card.SecuritySchemes))
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}

	for _, requirement := range card.SecurityRequirements {
		names := make([]string, 0, len(requirement))
		for name := range requirement {
			if _, ok := card.SecuritySchemes[name]; ok {
				names = append(names, string(name))
			}
		}
		sort.Strings(names)
		for _, name := range names {
			add(name)
		}
	}

	rest := make([]string, 0, len(card.SecuritySchemes))
	for name := range card.SecuritySchemes {
		rest = append(rest, string(name))
	}
	sort.Strings(rest)
	for _, name := range rest {
		add(name)
	}
	return out
}

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

	// Apply authentication using the first credentialed scheme in deterministic
	// order (card security-requirement order, then sorted names) — never Go map
	// iteration order, so the same request always authenticates the same way.
	for _, schemeName := range orderedSchemeNames(agent.AgentCard) {
		if credential, exists := (*agent.Credentials)[schemeName]; exists {
			scheme := agent.AgentCard.SecuritySchemes[a2a.SecuritySchemeName(schemeName)]
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
		name, headerValue, _ := headerForScheme(scheme, value)
		req.Header.Set(name, headerValue)
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
	name, headerValue, _ := headerForScheme(scheme, value)
	req.Header.Set(name, headerValue)
	return nil
}

// headerForScheme derives the HTTP header name/value that a header-based security
// scheme contributes for the given decrypted credential value. It is the single
// source of truth for header derivation, shared by request-mutating auth
// (ApplyAuthentication) and by auth-on-card-fetch (AuthHeaders).
//
// ok is false for schemes that are not applied as request headers — an API key
// located in the query string or a cookie — so callers can skip them.
func headerForScheme(scheme a2a.SecurityScheme, value string) (name, headerValue string, ok bool) {
	switch s := scheme.(type) {
	case a2a.APIKeySecurityScheme:
		if s.Location != a2a.APIKeySecuritySchemeLocationHeader {
			return "", "", false
		}
		// Special handling for Authorization header - add Bearer prefix if not already present
		if s.Name == "Authorization" && !hasAuthPrefix(value) {
			value = "Bearer " + value
		}
		return s.Name, value, true
	case a2a.HTTPAuthSecurityScheme:
		if strings.ToLower(s.Scheme) == "basic" {
			if !hasAuthPrefix(value) {
				value = "Basic " + value
			}
		} else {
			// Default to bearer for "bearer" and any unspecified scheme
			if !hasAuthPrefix(value) {
				value = "Bearer " + value
			}
		}
		return "Authorization", value, true
	default:
		return "", "", false
	}
}

// AuthHeaders returns the HTTP header name/value pairs that the agent's
// header-based security schemes would apply, for use when discovering a card that
// sits behind header authentication. It decrypts each stored credential whose
// scheme name matches a security scheme on the card and, when that scheme is
// header-based (API key in a header, or HTTP Basic/Bearer), adds the derived
// header. API keys located in the query string or a cookie are intentionally
// skipped — only headers are applied on card fetch.
//
// It returns an empty (non-nil) map — never an error — when the agent has no card,
// no security schemes, or no stored credentials, so a plain public-card fetch is
// unaffected.
func (a *AgentAuthenticator) AuthHeaders(agent *models.Agent) (map[string]string, error) {
	headers := make(map[string]string)
	if agent == nil || agent.AgentCard == nil {
		return headers, nil
	}
	if len(agent.AgentCard.SecuritySchemes) == 0 || agent.Credentials == nil || len(*agent.Credentials) == 0 {
		return headers, nil
	}

	// Iterate in deterministic order so that if two schemes contribute the same
	// header, the winner is reproducible rather than Go-map-order dependent.
	for _, schemeName := range orderedSchemeNames(agent.AgentCard) {
		credential, exists := (*agent.Credentials)[schemeName]
		if !exists {
			continue
		}
		decrypted, err := a.encryptionService.Decrypt(credential.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt credential: %w", err)
		}
		scheme := agent.AgentCard.SecuritySchemes[a2a.SecuritySchemeName(schemeName)]
		name, headerValue, ok := headerForScheme(scheme, decrypted)
		if !ok {
			continue // non-header scheme (query/cookie) — not applied on fetch
		}
		headers[name] = headerValue
	}
	return headers, nil
}
