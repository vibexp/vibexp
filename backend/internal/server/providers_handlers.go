package server

import (
	"net/http"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
)

// AuthProvider describes one enabled login provider for the login UI's provider
// picker: the slug to pass back as ?provider=, a human label and the provider
// type.
type AuthProvider struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

// ProvidersResponse is the JSON body returned by GET /api/v1/auth/providers.
type ProvidersResponse struct {
	Providers models.JSONArray[AuthProvider] `json:"providers"`
}

// handleListProviders returns the deployment's enabled login providers with
// display metadata, so the login screen can render a provider picker without
// hardcoding the list. The list is in the admin-defined sort order and may be
// empty when no provider is enabled. The providers are resolved from the
// database on every call (#1234), so a change applies with no restart.
//
// GET /api/v1/auth/providers
func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.container.AuthService().EnabledProviders(r.Context())
	if err != nil {
		s.logAuthError("handleListProviders", logIdentityProvidersResolveFailed, err)
		errors.WriteJSONError(w, r, errors.NewServiceUnavailableError(msgIdentityProvidersUnavailable))
		return
	}
	providers := make([]AuthProvider, len(enabled))
	for i, p := range enabled {
		providers[i] = AuthProvider{Name: p.Slug, DisplayName: p.DisplayName, Type: p.Type}
	}
	writeOK(w, ProvidersResponse{Providers: providers}, s.logger)
}
