package server

import (
	"net/http"

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

// providerDisplayName returns the UI label for a canonical provider name. The
// labels live in idp so the boot-time import of the legacy providers (#1232)
// stores the same ones.
func providerDisplayName(name string) string {
	return idp.DefaultDisplayName(idp.ProviderName(name))
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
