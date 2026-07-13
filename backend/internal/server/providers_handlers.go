package server

import (
	"net/http"
	"strings"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
)

// AuthProvider describes one enabled login provider for the login UI's provider
// picker: the canonical name to pass back as ?provider= plus a human label.
type AuthProvider struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// ProvidersResponse is the JSON body returned by GET /api/v1/auth/providers.
type ProvidersResponse struct {
	Providers models.JSONArray[AuthProvider] `json:"providers"`
}

// providerDisplayNames maps a canonical provider name to a human label for the
// login UI, so the frontend needs no hardcoded provider list. Unknown providers
// fall back to a title-cased name (see providerDisplayName) so a newly-added or
// generic provider still renders sensibly.
var providerDisplayNames = map[string]string{
	string(idp.ProviderGoogle): "Google",
	string(idp.ProviderGitHub): "GitHub",
	string(idp.ProviderOIDC):   "Single Sign-On",
}

// providerDisplayName returns the UI label for a canonical provider name,
// title-casing unknown names as a sensible default.
func providerDisplayName(name string) string {
	if label, ok := providerDisplayNames[name]; ok {
		return label
	}
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// handleListProviders returns the deployment's enabled login providers with
// display metadata, so the login screen can render a provider picker without
// hardcoding the list. The list mirrors AuthService.EnabledProviders() (stable
// sorted) and may be empty when no provider is configured.
//
// GET /api/v1/auth/providers
func (s *Server) handleListProviders(w http.ResponseWriter, _ *http.Request) {
	enabled := s.container.AuthService().EnabledProviders()
	providers := make([]AuthProvider, len(enabled))
	for i, name := range enabled {
		providers[i] = AuthProvider{Name: name, DisplayName: providerDisplayName(name)}
	}
	writeOK(w, ProvidersResponse{Providers: providers}, s.logger)
}
